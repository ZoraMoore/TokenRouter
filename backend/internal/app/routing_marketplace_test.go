package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/config"
	routinghttp "github.com/TokenFlux/TokenRouter/internal/routing/httpapi"
	"github.com/TokenFlux/TokenRouter/internal/settings/testkit"
	"github.com/TokenFlux/TokenRouter/internal/site"
	sitehttp "github.com/TokenFlux/TokenRouter/internal/site/httpapi"
	"github.com/TokenFlux/TokenRouter/internal/usage"
)

// publicDashboardStatsProbe 记录公开统计是否读取了用量数据。
type publicDashboardStatsProbe struct{ calls int }

func (p *publicDashboardStatsProbe) GetDashboardPublicStats(context.Context, time.Time, time.Time, bool) (*usage.DashboardPublicStats, error) {
	p.calls++
	return &usage.DashboardPublicStats{TodayTokens: 12, TotalTokens: 34, TotalUsers: 5}, nil
}

// TestMarketplacePublicHomeStats 验证匿名统计请求使用当前开关，公开配置与 HTML 注入返回同一个值。
func TestMarketplacePublicHomeStats(t *testing.T) {
	ctx := context.Background()
	repo := testkit.NewMemory()
	probe := &publicDashboardStatsProbe{}
	dashboard := usage.NewDashboardService(nil, nil, nil, nil, usage.DashboardReaders{Public: probe})
	public := newSitePublicSettingsFixture(repo, &config.Config{})
	router := gin.New()
	routinghttp.RegisterPublicMarketplaceRoutes(router.Group("/api/v1"), provideMarketplaceHTTP(nil, dashboard, site.NewDisplaySettings(repo, nil)))
	router.GET("/api/v1/settings/public", sitehttp.NewPublicHandler(public, "test-version").GetPublicSettings)

	for _, item := range []struct {
		name    string
		stored  string
		readErr error
		shown   bool
	}{
		{name: "缺省"},
		{name: "开启", stored: "true", shown: true},
		{name: "关闭", stored: "false"},
		{name: "无效值", stored: "invalid"},
		{name: "读取失败", stored: "true", readErr: errors.New("settings unavailable")},
	} {
		t.Run(item.name, func(t *testing.T) {
			if item.stored != "" {
				require.NoError(t, repo.Set(ctx, site.SettingKeyShowPublicHomeStats, item.stored))
			}
			repo.GetValueErr = item.readErr
			probe.calls = 0

			stats := httptest.NewRecorder()
			router.ServeHTTP(stats, httptest.NewRequest(http.MethodGet, "/api/v1/marketplace/stats", nil))
			require.Equal(t, "no-store", stats.Header().Get("Cache-Control"))
			if item.shown {
				require.Equal(t, http.StatusOK, stats.Code)
				require.JSONEq(t, `{"code":0,"message":"success","data":{"today_tokens":12,"total_tokens":34,"total_users":5}}`, stats.Body.String())
				require.Equal(t, 1, probe.calls)
			} else {
				require.Equal(t, http.StatusForbidden, stats.Code)
				require.Contains(t, stats.Body.String(), `"reason":"PUBLIC_HOME_STATS_DISABLED"`)
				require.NotContains(t, stats.Body.String(), `"today_tokens"`)
				require.Zero(t, probe.calls)
			}

			if item.readErr != nil {
				return
			}
			settings := httptest.NewRecorder()
			router.ServeHTTP(settings, httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil))
			require.Equal(t, http.StatusOK, settings.Code)
			marker := `"show_public_home_stats":false`
			if item.shown {
				marker = `"show_public_home_stats":true`
			}
			require.Contains(t, settings.Body.String(), marker)
			injection, err := public.GetPublicSettingsForInjection(ctx)
			require.NoError(t, err)
			payload, err := json.Marshal(injection)
			require.NoError(t, err)
			require.Contains(t, string(payload), marker)
		})
	}
}
