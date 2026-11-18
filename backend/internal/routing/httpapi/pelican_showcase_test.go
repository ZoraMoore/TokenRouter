package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/identity/httpapi/authctx"
	"github.com/TokenFlux/TokenRouter/internal/routing"
)

type pelicanSettingsHTTPFixture struct {
	routing.PelicanRepository
	settings routing.PelicanShowcaseSettings
}

func (f *pelicanSettingsHTTPFixture) ShowcaseSettings(context.Context) (routing.PelicanShowcaseSettings, error) {
	return f.settings, nil
}

func (f *pelicanSettingsHTTPFixture) SaveShowcaseSettings(_ context.Context, value routing.PelicanShowcaseSettings) (routing.PelicanShowcaseSettings, error) {
	f.settings = value
	return value, nil
}

func (*pelicanSettingsHTTPFixture) LatestDrawings(context.Context, []int64) ([]routing.PelicanLatest, error) {
	return []routing.PelicanLatest{{GroupID: 7, Artworks: []routing.PelicanSummary{{
		ID: 5, ModelID: "model", QuestionKind: "pelican", Status: "pending_review", LatencyMs: 14, TestedAt: time.Unix(1000, 0).UTC(),
	}}}}, nil
}

func (*pelicanSettingsHTTPFixture) Result(context.Context, int64, []int64) (*routing.PelicanResult, error) {
	return &routing.PelicanResult{
		PelicanSummary: routing.PelicanSummary{ID: 5, QuestionKind: "custom_svg", Status: "passed", ReasoningEffort: "high", LatencyMs: 15},
		GroupID:        7, ProviderName: "private-account", Prompt: "private-test", Answer: "<svg><g/></svg>",
	}, nil
}

func TestPelicanAdminShowcaseSettingsAndVisibility(t *testing.T) {
	repo := &pelicanSettingsHTTPFixture{settings: routing.PelicanShowcaseSettings{APIEnabled: true, MaxItems: 20, AutoCleanup: true, RetentionDays: 7}}
	h := NewPelicanHandler(routing.NewPelicanService(repo, routing.PelicanOptions{}))
	mux := gin.New()
	RegisterPelicanAdminRoutes(mux.Group("/admin"), h)
	users := mux.Group("/user")
	users.Use(func(c *gin.Context) {
		if c.GetHeader("X-Test-User") != "" {
			c.Set(authctx.ContextKeyUser, authctx.AuthSubject{UserID: 3})
		}
	})
	RegisterPelicanUserRoutes(users, h)
	request := func(method, path, body string, user bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if user {
			req.Header.Set("X-Test-User", "1")
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	const settings = "/admin/pelican-showcase/settings"
	const visibility = "/user/pelican-tests/visibility"
	require.Equal(t, http.StatusUnauthorized, request(http.MethodGet, visibility, "", false).Code)
	read := request(http.MethodGet, visibility, "", true)
	require.JSONEq(t, `{"code":0,"message":"success","data":{"enabled":false,"api_enabled":false}}`, read.Body.String())
	read = request(http.MethodGet, settings, "", false)
	require.JSONEq(t, `{"code":0,"message":"success","data":{"enabled":false,"api_enabled":true,"max_items":20,"auto_cleanup":true,"retention_days":7}}`, read.Body.String())
	for _, body := range []string{
		`{"max_items":1}`,
		`{"enabled":true,"api_enabled":true,"max_items":0,"auto_cleanup":false,"retention_days":7}`,
		`{"enabled":true,"api_enabled":true,"max_items":1,"auto_cleanup":false,"retention_days":91}`,
		`{"enabled":true,"api_enabled":true,"max_items":1,"auto_cleanup":false,"retention_days":7,"extra":true}`,
	} {
		require.Equal(t, http.StatusBadRequest, request(http.MethodPut, settings, body, false).Code)
	}
	valid := `{"enabled":true,"api_enabled":true,"max_items":10,"auto_cleanup":false,"retention_days":14}`
	read = request(http.MethodPut, settings, valid, false)
	require.Equal(t, http.StatusOK, read.Code)
	read = request(http.MethodGet, visibility, "", true)
	require.JSONEq(t, `{"code":0,"message":"success","data":{"enabled":true,"api_enabled":true}}`, read.Body.String())
}

func TestPelicanKeyRoutesOnlyExposeDrawingMetadata(t *testing.T) {
	repo := &pelicanSettingsHTTPFixture{settings: routing.PelicanShowcaseSettings{Enabled: true, APIEnabled: true, MaxItems: 20, RetentionDays: 7}}
	h := NewPelicanHandler(routing.NewPelicanService(repo, routing.PelicanOptions{UserGroups: func(context.Context, int64) ([]routing.Group, error) {
		return []routing.Group{{ID: 7, Name: "test-group", Status: routing.StatusActive}}, nil
	}}))
	mux := gin.New()
	RegisterPelicanKeyRoutes(mux.Group("/v1/pelican-showcase"), h, func(c *gin.Context) (int64, []int64, bool) {
		return 3, []int64{7}, c.GetHeader("X-Test-Key") != ""
	})
	for _, tc := range []struct {
		method, suffix string
		authenticated  bool
		status         int
	}{
		{http.MethodGet, "", false, 401},
		{http.MethodHead, "", false, 401},
		{http.MethodGet, "", true, 200},
		{http.MethodHead, "", true, 200},
		{http.MethodGet, "/items/5", true, 200},
		{http.MethodHead, "/items/5", true, 200},
		{http.MethodHead, "/items/0", true, 400},
	} {
		req := httptest.NewRequest(tc.method, "/v1/pelican-showcase"+tc.suffix, nil)
		if tc.authenticated {
			req.Header.Set("X-Test-Key", "1")
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		require.Equal(t, tc.status, rec.Code)
		if tc.method == http.MethodHead {
			require.Empty(t, rec.Body.Bytes())
			continue
		}
		if tc.status != http.StatusOK {
			continue
		}
		var response struct {
			Data map[string]any `json:"data"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
		for _, hidden := range []string{"private-account", "private-test", "provider_id", "plan_id", "question_kind", "pending_review", "criteria"} {
			require.NotContains(t, rec.Body.String(), hidden)
		}
		require.Equal(t, "private, no-store", rec.Header().Get("Cache-Control"))
		require.Contains(t, rec.Body.String(), `"status":"success"`)
		require.Contains(t, rec.Body.String(), `"content_url":"/v1/pelican-showcase/items/5"`)
		if tc.suffix == "" {
			require.Len(t, response.Data, 10)
			require.Equal(t, float64(1), response.Data["schema_version"])
			require.Equal(t, "published_successes", response.Data["result_scope"])
			require.Equal(t, float64(60), response.Data["poll_interval_seconds"])
			require.Equal(t, float64(20), response.Data["max_items_per_group"])
			require.Equal(t, "1970-01-01T00:16:40Z", response.Data["latest_generated_at"])
			require.Contains(t, rec.Body.String(), `"platform":""`)
			require.NotContains(t, rec.Body.String(), "response_text")
		} else {
			require.Len(t, response.Data, 9)
			require.Equal(t, "<svg><g/></svg>", response.Data["response_text"])
		}
	}
}
