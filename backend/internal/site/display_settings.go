package site

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
	"github.com/TokenFlux/TokenRouter/internal/pkg/locale"
)

// 首页统计项的标识同时用于持久化和管理设置接口。
const (
	SettingKeyPublicHomeStatsMetrics = "public_home_stats_metrics"
	HomeStatsTodayTokens             = "today_tokens"
	HomeStatsTotalTokens             = "total_tokens"
	HomeStatsTotalUsers              = "total_users"
	HomeStatsSupportedModels         = "supported_models"
)

var defaultHomeStatsMetrics = []string{HomeStatsTodayTokens, HomeStatsTotalTokens, HomeStatsTotalUsers, HomeStatsSupportedModels}

// DisplaySettings 按需读取站点展示设置。
type DisplaySettings struct {
	store interface {
		GetValue(context.Context, string) (string, error)
	}
	frontend func() string
}

func NewDisplaySettings(store interface {
	GetValue(context.Context, string) (string, error)
}, frontend func() string,
) *DisplaySettings {
	return &DisplaySettings{store: store, frontend: frontend}
}

func (s *DisplaySettings) GetSiteName(ctx context.Context) string {
	return locale.ReadGroupedText(ctx, s.store, SettingKeySiteTexts, SettingKeySiteName, "TokenRouter")
}

func (s *DisplaySettings) GetFrontendURL(ctx context.Context) string {
	return ReadFrontendURL(ctx, s.store, s.frontend)
}

// GetCustomMenuItemsRaw 返回保存的菜单内容，读取失败时返回空列表。
func (s *DisplaySettings) GetCustomMenuItemsRaw(ctx context.Context) string {
	value, err := s.store.GetValue(ctx, SettingKeyCustomMenuItems)
	if err != nil {
		return "[]"
	}
	return value
}

// ReadPublicHomeStatsMetrics 一次读取总开关和子项。读取失败时按总开关关闭处理。
func ReadPublicHomeStatsMetrics(ctx context.Context, store PublicInputStore) ([]string, bool) {
	values, err := store.GetMultiple(ctx, []string{SettingKeyShowPublicHomeStats, SettingKeyPublicHomeStatsMetrics})
	if err != nil || values[SettingKeyShowPublicHomeStats] != "true" {
		return nil, false
	}
	return readHomeStatsMetrics(values), true
}

// readHomeStatsMetrics 在缺少设置时返回四项。存储值无效时隐藏全部统计项。
func readHomeStatsMetrics(values map[string]string) []string {
	raw, exists := values[SettingKeyPublicHomeStatsMetrics]
	if !exists {
		return slices.Clone(defaultHomeStatsMetrics)
	}
	var metrics []string
	if err := json.Unmarshal([]byte(raw), &metrics); err != nil || metrics == nil {
		return []string{}
	}
	selected, err := normalizeHomeStatsMetrics(metrics)
	if err != nil {
		return []string{}
	}
	return selected
}

// normalizeHomeStatsMetrics 按首页的展示顺序保存已选项，并拒绝未知或重复的标识。
func normalizeHomeStatsMetrics(metrics []string) ([]string, error) {
	if metrics == nil {
		return slices.Clone(defaultHomeStatsMetrics), nil
	}
	if len(metrics) > len(defaultHomeStatsMetrics) {
		return nil, apperror.BadRequest("INVALID_PUBLIC_HOME_STATS_METRICS", "Select up to four public home stats metrics")
	}
	selected := make([]string, 0, len(defaultHomeStatsMetrics))
	for _, metric := range defaultHomeStatsMetrics {
		if slices.Contains(metrics, metric) {
			selected = append(selected, metric)
		}
	}
	if len(selected) != len(metrics) {
		return nil, apperror.BadRequest("INVALID_PUBLIC_HOME_STATS_METRICS", "Public home stats metrics must be distinct supported values")
	}
	return selected, nil
}

// ReadFrontendURL 优先读取数据库中的前端地址，缺失或读取失败时调用 fallback。
func ReadFrontendURL(ctx context.Context, store interface {
	GetValue(context.Context, string) (string, error)
}, fallback func() string,
) string {
	val, err := store.GetValue(ctx, SettingKeyFrontendURL)
	if err == nil && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return fallback()
}
