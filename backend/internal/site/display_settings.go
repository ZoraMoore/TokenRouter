package site

import (
	"context"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/pkg/locale"
)

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

// ShowPublicHomeStats 读取公开首页统计开关。设置缺失或读取失败时按关闭处理。
func (s *DisplaySettings) ShowPublicHomeStats(ctx context.Context) bool {
	value, err := s.store.GetValue(ctx, SettingKeyShowPublicHomeStats)
	return err == nil && value == "true"
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
