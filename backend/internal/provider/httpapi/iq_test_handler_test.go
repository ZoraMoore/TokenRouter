package httpapi

import (
	"bufio"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/provider"
)

type iqHTTPFixture struct {
	providerTests
	trace func(context.Context, int64, provider.IQTestRequest, func(string, any) error) error
	probe func(context.Context, int64, string) (provider.StateProbeResult, error)
}

func (f iqHTTPFixture) RunStateProbe(ctx context.Context, id int64, model string) (provider.StateProbeResult, error) {
	return f.probe(ctx, id, model)
}

// TestStateProbeHTTP 检查客户端只能指定模型，所有观测请求使用路由选定的账号。
func TestStateProbeHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, id, body, model string
		err                   error
		status                int
	}{
		{"指定模型", "7", `{"model_id":"alias"}`, "alias", nil, http.StatusOK},
		{"默认模型", "7", "{}", "", nil, http.StatusOK},
		{"账号忙", "7", "{}", "", provider.ErrIQBusy, http.StatusConflict},
		{"账号不存在", "7", "{}", "", provider.ErrIQNotFound, http.StatusNotFound},
		{"不支持该账号", "7", "{}", "", provider.ErrIQUnsupported, http.StatusBadRequest},
		{"拒绝自定义请求头", "7", `{"headers":{"secret":"private"}}`, "", nil, http.StatusBadRequest},
		{"拒绝空对象以外的值", "7", "null", "", nil, http.StatusBadRequest},
		{"拒绝空值模型", "7", `{"model_id":null}`, "", nil, http.StatusBadRequest},
		{"拒绝非字符串模型", "7", `{"model_id":7}`, "", nil, http.StatusBadRequest},
		{"拒绝超长请求体", "7", `{"model_id":"` + strings.Repeat("a", 9000) + `"}`, "", nil, http.StatusBadRequest},
		{"拒绝非正账号 ID", "0", "{}", "", nil, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			h := &TestHandler{tests: iqHTTPFixture{probe: func(_ context.Context, id int64, model string) (provider.StateProbeResult, error) {
				called = true
				require.EqualValues(t, 7, id)
				require.Equal(t, tc.model, model)
				return provider.StateProbeResult{Verdict: "healthy"}, tc.err
			}}}
			router := gin.New()
			router.POST("/providers/:id/state-probe", h.StateProbe)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/providers/"+tc.id+"/state-probe", strings.NewReader(tc.body)))
			require.Equal(t, tc.status, rec.Code)
			require.Equal(t, tc.status == http.StatusOK || tc.err != nil, called)
			require.Equal(t, "private, no-store", rec.Header().Get("Cache-Control"))
			require.NotContains(t, rec.Body.String(), "private")
			if tc.status == http.StatusOK {
				require.Contains(t, rec.Body.String(), `"verdict":"healthy"`)
			}
		})
	}
}

func (f iqHTTPFixture) RunModelTrace(ctx context.Context, id int64, req provider.IQTestRequest, emit func(string, any) error) error {
	return f.trace(ctx, id, req, emit)
}

// TestIQHandlerRejectsInvalidRequests 检查账号 ID、请求长度和可接受的字段。
func TestIQHandlerRejectsInvalidRequests(t *testing.T) {
	h := new(TestHandler)
	router := gin.New()
	router.POST("/providers/:id/test", h.IQQuestion)
	router.POST("/providers/:id/quality", h.IQQuality)
	router.POST("/providers/:id/trace", h.ModelTrace)
	for _, tt := range []struct{ path, body string }{
		{"/providers/0/test", "{}"},
		{"/providers/-1/test", "{}"},
		{"/providers/nope/test", "{}"},
		{"/providers/1/test", `{"api_key":"private"}`},
		{"/providers/1/quality", `{"model_id":"m","expected_answer":"21"}`},
		{"/providers/1/trace", `{"model_id":"m"} {}`},
		{"/providers/1/test", `{"prompt":"` + strings.Repeat("a", 70000) + `"}`},
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body)))
		require.Equal(t, http.StatusBadRequest, rec.Code, tt.path)
		require.NotContains(t, rec.Body.String(), "private")
	}
}

// TestModelTraceClientCancel 检查管理员关掉已开始的指纹流后，上游执行器能收到取消。
func TestModelTraceClientCancel(t *testing.T) {
	stopped := make(chan struct{})
	h := &TestHandler{tests: iqHTTPFixture{trace: func(ctx context.Context, _ int64, _ provider.IQTestRequest, emit func(string, any) error) error {
		defer close(stopped)
		if err := emit("modeltrace_start", struct{}{}); err != nil {
			return err
		}
		<-ctx.Done()
		return ctx.Err()
	}}}
	router := gin.New()
	router.POST("/providers/:id/trace", h.ModelTrace)
	server := httptest.NewServer(router)
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/providers/7/trace", strings.NewReader(`{"model_id":"alias"}`))
	require.NoError(t, err)
	res, err := server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	line, err := bufio.NewReader(res.Body).ReadString('\n')
	require.NoError(t, err)
	require.Contains(t, line, "modeltrace_start")
	cancel()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("客户端断开后执行器仍在等待")
	}
}

// TestModelTraceHTTP 区分开始前的冲突、完成事件和开始后的上游失败。
func TestModelTraceHTTP(t *testing.T) {
	for _, tc := range []struct {
		name     string
		start    bool
		err      error
		wantCode int
		want     string
	}{
		{"账号忙", false, provider.ErrIQBusy, http.StatusConflict, "IQ_TEST_BUSY"},
		{"读取账号失败", false, provider.ErrIQUnavailable, http.StatusServiceUnavailable, "IQ_TEST_UNAVAILABLE"},
		{"完成", true, nil, http.StatusOK, `"type":"modeltrace_complete"`},
		{"流中断", true, errors.New("sensitive upstream response"), http.StatusOK, `"type":"error"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &TestHandler{tests: iqHTTPFixture{trace: func(_ context.Context, id int64, req provider.IQTestRequest, emit func(string, any) error) error {
				if id != 7 || req.ModelID != "alias" {
					return errors.New("wrong request")
				}
				if tc.start {
					if err := emit("modeltrace_start", gin.H{"challenges": []string{"q"}}); err != nil {
						return err
					}
				}
				if tc.err != nil {
					return tc.err
				}
				return emit("modeltrace_complete", gin.H{"status": "partial"})
			}}}
			router := gin.New()
			router.POST("/providers/:id/trace", h.ModelTrace)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/providers/7/trace", strings.NewReader(`{"model_id":"alias"}`)))
			require.Equal(t, tc.wantCode, rec.Code)
			require.Contains(t, rec.Body.String(), tc.want)
			require.NotContains(t, rec.Body.String(), "sensitive")
			require.Equal(t, "private, no-store", rec.Header().Get("Cache-Control"))
			if tc.start {
				require.Contains(t, rec.Header().Get("Content-Type"), "text/event-stream")
				require.Contains(t, rec.Body.String(), `"type":"modeltrace_start"`)
			}
		})
	}
}
