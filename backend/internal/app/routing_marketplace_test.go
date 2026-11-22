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
	"github.com/TokenFlux/TokenRouter/internal/settings"
	"github.com/TokenFlux/TokenRouter/internal/settings/testkit"
	"github.com/TokenFlux/TokenRouter/internal/site"
	sitehttp "github.com/TokenFlux/TokenRouter/internal/site/httpapi"
	"github.com/TokenFlux/TokenRouter/internal/usage"
)

// publicDashboardStatsProbe 记录公开统计是否读取了用量数据。
type publicDashboardStatsProbe struct{ calls int }

// publicHomeStatsSettingsProbe 模拟公开统计读取设置时遇到的数据库错误。
type publicHomeStatsSettingsProbe struct {
	*testkit.Memory
	err error
}

func (p *publicHomeStatsSettingsProbe) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.Memory.GetMultiple(ctx, keys)
}

func (p *publicDashboardStatsProbe) GetDashboardPublicStats(context.Context, time.Time, time.Time, bool) (*usage.DashboardPublicStats, error) {
	p.calls++
	return &usage.DashboardPublicStats{TodayTokens: 12, TotalTokens: 34, TotalUsers: 0}, nil
}

// TestMarketplacePublicHomeStats 验证匿名统计请求使用当前开关，公开配置与 HTML 注入返回同一个值。
func TestMarketplacePublicHomeStats(t *testing.T) {
	ctx := context.Background()
	repo := &publicHomeStatsSettingsProbe{Memory: testkit.NewMemory()}
	probe := &publicDashboardStatsProbe{}
	dashboard := usage.NewDashboardService(nil, nil, nil, nil, usage.DashboardReaders{Public: probe})
	public := newSitePublicSettingsFixture(repo.Memory, &config.Config{})
	router := gin.New()
	routinghttp.RegisterPublicMarketplaceRoutes(router.Group("/api/v1"), provideMarketplaceHTTP(nil, dashboard, settings.New(repo)))
	router.GET("/api/v1/settings/public", sitehttp.NewPublicHandler(public, "test-version").GetPublicSettings)

	for _, item := range []struct {
		name    string
		stored  string
		metrics string
		readErr error
		data    string
		queries int
	}{
		{name: "缺省"},
		{name: "开启时默认展示四项", stored: "true", data: `{"today_tokens":12,"total_tokens":34,"total_users":0,"show_supported_models":true}`, queries: 1},
		{name: "省略未勾选的数字和模型数标记", stored: "true", metrics: `["total_users"]`, data: `{"total_users":0}`, queries: 1},
		{name: "仅展示模型数", stored: "true", metrics: `["supported_models"]`, data: `{"show_supported_models":true}`},
		{name: "全部取消勾选", stored: "true", metrics: `[]`, data: `{}`},
		{name: "子项存储值无效", stored: "true", metrics: `["today_tokens","unknown"]`, data: `{}`},
		{name: "关闭", stored: "false", metrics: `["total_users"]`},
		{name: "无效值", stored: "invalid"},
		{name: "读取失败", stored: "true", readErr: errors.New("settings unavailable")},
	} {
		t.Run(item.name, func(t *testing.T) {
			if item.stored != "" {
				require.NoError(t, repo.Set(ctx, site.SettingKeyShowPublicHomeStats, item.stored))
			}
			if item.metrics != "" {
				require.NoError(t, repo.Set(ctx, site.SettingKeyPublicHomeStatsMetrics, item.metrics))
			}
			repo.err = item.readErr
			probe.calls = 0

			stats := httptest.NewRecorder()
			router.ServeHTTP(stats, httptest.NewRequest(http.MethodGet, "/api/v1/marketplace/stats", nil))
			require.Equal(t, "no-store", stats.Header().Get("Cache-Control"))
			if item.data != "" {
				require.Equal(t, http.StatusOK, stats.Code)
				require.JSONEq(t, `{"code":0,"message":"success","data":`+item.data+`}`, stats.Body.String())
				require.Equal(t, item.queries, probe.calls)
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
			if item.stored == "true" {
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
