package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/identity/httpapi/authctx"
	"github.com/TokenFlux/TokenRouter/internal/routing"
)

type pelicanDetailFixture struct {
	routing.PelicanRepository
}

func (pelicanDetailFixture) ShowcaseSettings(context.Context) (routing.PelicanShowcaseSettings, error) {
	return routing.PelicanShowcaseSettings{Enabled: true, MaxItems: 20, RetentionDays: 7}, nil
}

func (pelicanDetailFixture) Result(_ context.Context, id int64, groups []int64) (*routing.PelicanResult, error) {
	if len(groups) != 1 || groups[0] != 3 || (id != 5 && id != 6) {
		return nil, routing.ErrPelicanNotFound
	}
	account := int64(45)
	sample := &routing.PelicanResult{PelicanSummary: routing.PelicanSummary{ID: id, Status: "passed"}, ProviderID: &account, ProviderName: "private-provider", Answer: "21", RawAnswer: "<html><style>.wheel{stroke:red}</style><script>window.frame=1</script></html>"}
	if id == 6 {
		sample.QuestionKind = routing.PelicanStateProbe
		sample.Answer = "首次请求 HTTP 200，状态长度 20 字节。"
	}
	return sample, nil
}

// TestPelicanShowcasePermission 每次打开作品时检查登录身份和可见分组，用户响应不含账号信息。
func TestPelicanShowcasePermission(t *testing.T) {
	h := NewPelicanHandler(routing.NewPelicanService(pelicanDetailFixture{}, routing.PelicanOptions{
		UserGroups: func(_ context.Context, userID int64) ([]routing.Group, error) {
			return []routing.Group{{ID: userID, Status: routing.StatusActive}}, nil
		},
	}))
	for _, tc := range []struct {
		user int64
		path string
		code int
	}{{0, "/pelican-tests/results/5", 401}, {2, "/pelican-tests/results/5", 404}, {3, "/pelican-tests/results/0", 400}, {3, "/pelican-tests/results/5", 200}, {2, "/pelican-tests/results/6", 404}, {3, "/pelican-tests/results/6", 200}} {
		router := gin.New()
		group := router.Group("")
		if tc.user != 0 {
			group.Use(func(c *gin.Context) { c.Set(authctx.ContextKeyUser, authctx.AuthSubject{UserID: tc.user}) })
		}
		RegisterPelicanUserRoutes(group, h)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		require.Equal(t, tc.code, rec.Code)
		if rec.Code == http.StatusOK {
			var envelope struct {
				Data map[string]any `json:"data"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
			if tc.path == "/pelican-tests/results/6" {
				require.Equal(t, routing.PelicanStateProbe, envelope.Data["question_kind"])
				require.Contains(t, envelope.Data["answer"], "HTTP 200")
				require.NotContains(t, rec.Body.String(), "private-provider")
			} else {
				require.Equal(t, "21", envelope.Data["answer"])
				require.Contains(t, envelope.Data["raw_answer"], "<script>window.frame=1</script>")
			}
			require.NotContains(t, envelope.Data, "provider_id")
			require.NotContains(t, envelope.Data, "provider_name")
		}
	}
}

// TestPelicanPlanRejectsServerFields 管理页面提交的任务状态和执行记录由服务端填写。
func TestPelicanPlanRejectsServerFields(t *testing.T) {
	router := gin.New()
	RegisterPelicanAdminRoutes(router.Group("/admin"), NewPelicanHandler(nil))
	for _, body := range []string{
		`{"group_id":3,"enabled":true,"run_state":"running"}`,
		`{"group_id":3,"model_id":"text","cron_expression":"* * * * *","question_kind":"pelican"}`,
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/pelican-tests/plans", strings.NewReader(body)))
		require.Equal(t, http.StatusBadRequest, rec.Code)
	}
}

// TestPelicanQuestionDefaults 检查计划编辑器可以读取两道内置题的初始字段。
func TestPelicanQuestionDefaults(t *testing.T) {
	router := gin.New()
	RegisterPelicanAdminRoutes(router.Group("/admin"), NewPelicanHandler(nil))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/pelican-tests/question-defaults", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var response struct {
		Data []map[string]string `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.Len(t, response.Data, 2)
	kinds := make(map[string]string)
	for _, question := range response.Data {
		require.NotEmpty(t, question["question_prompt"])
		require.NotEmpty(t, question["question_name"])
		require.Contains(t, question, "expected_answer")
		kinds[question["question_kind"]] = question["expected_answer"]
	}
	require.Equal(t, map[string]string{"pelican": "", "candy": "21"}, kinds)
}
