package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/provider"
)

type recheckStore struct {
	provider.MaintenancePlanStore
}

func (recheckStore) Recheck(_ context.Context, id int64) (int64, error) {
	if id == 211 {
		return 35, nil
	}
	return 0, provider.ErrMaintenanceNotFound
}

// TestMaintenanceRecheckRun 检查重放样本取得同一轮次，历史样本没有关联运行时返回 404。
func TestMaintenanceRecheckRun(t *testing.T) {
	router := gin.New()
	service := provider.NewMaintenanceService(recheckStore{}, provider.MaintenancePlanOptions{})
	RegisterMaintenanceRoutes(router.Group("/admin"), NewMaintenanceHandler(service))
	for _, test := range []struct {
		body   string
		status int
	}{
		{`{"pelican_result_id":211}`, http.StatusAccepted},
		{`{"pelican_result_id":211}`, http.StatusAccepted},
		{`{"pelican_result_id":212}`, http.StatusNotFound},
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/scheduled-tasks/iq-rechecks", strings.NewReader(test.body)))
		require.Equal(t, test.status, rec.Code)
		if test.status == http.StatusAccepted {
			require.Contains(t, rec.Body.String(), `"run_id":35`)
		}
	}
}

// TestMaintenanceRecheckInput 复检目标由分组样本确定，客户端只能提交样本编号。
func TestMaintenanceRecheckInput(t *testing.T) {
	router := gin.New()
	RegisterMaintenanceRoutes(router.Group("/admin"), new(MaintenanceHandler))
	for _, body := range []string{
		"{}", `{"pelican_result_id":0}`, `{"pelican_result_id":-1}`,
		`{"pelican_result_id":"1"}`, `{"pelican_result_id":2,"provider_id":9}`,
		`{"pelican_result_id":1} {}`, strings.Repeat(" ", 4096) + "{}",
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/admin/scheduled-tasks/iq-rechecks", strings.NewReader(body))
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.Equal(t, "private, no-store", rec.Header().Get("Cache-Control"))
	}
}
