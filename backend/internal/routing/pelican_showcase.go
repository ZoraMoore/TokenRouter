package routing

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"time"
)

// PelicanShowcaseSettings 控制用户作品展示、API Key 读取和每组作品的保留范围。
type PelicanShowcaseSettings struct {
	Enabled       bool `json:"enabled"`
	APIEnabled    bool `json:"api_enabled"`
	MaxItems      int  `json:"max_items"`
	AutoCleanup   bool `json:"auto_cleanup"`
	RetentionDays int  `json:"retention_days"`
}

// PelicanVisibility 是用户菜单和页面实际可用的展示入口。
type PelicanVisibility struct {
	Enabled    bool `json:"enabled"`
	APIEnabled bool `json:"api_enabled"`
}

// PelicanKeyShowcase 列出当前 API Key 可以读取的分组作品。
type PelicanKeyShowcase struct {
	PelicanVisibility
	SchemaVersion       int               `json:"schema_version"`
	ResultScope         string            `json:"result_scope"`
	PollIntervalSeconds int               `json:"poll_interval_seconds"`
	MaxItems            int               `json:"max_items"`
	MaxItemsPerGroup    int               `json:"max_items_per_group"`
	RetentionDays       int               `json:"retention_days"`
	LatestGeneratedAt   *time.Time        `json:"latest_generated_at"`
	Groups              []PelicanKeyGroup `json:"groups"`
}

// PelicanKeyGroup 保存一个可见分组中按生成时间排列的作品。
type PelicanKeyGroup struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// 分组可混用多个平台，保留兼容字段并使用空值。
	Platform string           `json:"platform"`
	Items    []PelicanKeyItem `json:"items"`
}

// PelicanKeyItem 只含已发布绘图的标识、模型、作答时间和推理档位。
type PelicanKeyItem struct {
	ID              int64     `json:"id"`
	GroupID         int64     `json:"group_id"`
	ModelID         string    `json:"model_id"`
	ReasoningEffort string    `json:"reasoning_effort"`
	Status          string    `json:"status"`
	LatencyMs       int64     `json:"latency_ms"`
	GeneratedAt     time.Time `json:"generated_at"`
	ContentURL      string    `json:"content_url"`
}

// PelicanKeyArtwork 在列表字段之外提供该作品的 SVG 正文。
type PelicanKeyArtwork struct {
	PelicanKeyItem
	ResponseText string `json:"response_text"`
}

// ShowcaseSettings 返回管理员保存的开关，发现损坏的保留范围时停止展示。
func (s *PelicanService) ShowcaseSettings(ctx context.Context) (PelicanShowcaseSettings, error) {
	settings, err := s.repo.ShowcaseSettings(ctx)
	if err == nil && !settings.valid() {
		return PelicanShowcaseSettings{}, errors.New("鹈鹕作品保留设置超出允许范围")
	}
	return settings, err
}

// SaveShowcaseSettings 在写入和清理现有作品前检查数量和天数。
func (s *PelicanService) SaveShowcaseSettings(ctx context.Context, settings PelicanShowcaseSettings) (PelicanShowcaseSettings, error) {
	if !settings.valid() {
		return PelicanShowcaseSettings{}, ErrPelicanInvalid
	}
	return s.repo.SaveShowcaseSettings(ctx, settings)
}

func (settings PelicanShowcaseSettings) valid() bool {
	return settings.MaxItems >= 1 && settings.MaxItems <= 100 && settings.RetentionDays >= 1 && settings.RetentionDays <= 90
}

func (s *PelicanService) Visibility(ctx context.Context) (PelicanVisibility, error) {
	settings, err := s.ShowcaseSettings(ctx)
	if err != nil {
		return PelicanVisibility{}, err
	}
	return settings.visibility(), nil
}

func (settings PelicanShowcaseSettings) visibility() PelicanVisibility {
	return PelicanVisibility{Enabled: settings.Enabled, APIEnabled: settings.Enabled && settings.APIEnabled}
}

func (s *PelicanService) requireShowcase(ctx context.Context, key bool) (PelicanShowcaseSettings, error) {
	settings, err := s.ShowcaseSettings(ctx)
	if err != nil {
		return PelicanShowcaseSettings{}, err
	}
	if !settings.Enabled || key && !settings.APIEnabled {
		return PelicanShowcaseSettings{}, ErrPelicanNotFound
	}
	return settings, nil
}

// KeyShowcase 合并同一组不同计划的已发布作品，按生成时间和 ID 排序。
func (s *PelicanService) KeyShowcase(ctx context.Context, userID int64, bound []int64) (*PelicanKeyShowcase, error) {
	settings, groups, err := s.keyShowcaseGroups(ctx, userID, bound)
	if err != nil {
		return nil, err
	}
	rows, err := pelicanShowcaseGroups(ctx, groups, s.repo.LatestDrawings)
	if err != nil {
		return nil, err
	}
	view := &PelicanKeyShowcase{
		PelicanVisibility: settings.visibility(), SchemaVersion: 1, ResultScope: "published_successes", PollIntervalSeconds: 60,
		MaxItems: settings.MaxItems, MaxItemsPerGroup: settings.MaxItems, RetentionDays: settings.RetentionDays,
		Groups: make([]PelicanKeyGroup, 0, len(rows)),
	}
	for _, row := range rows {
		group := PelicanKeyGroup{ID: row.GroupID, Name: row.GroupName}
		for _, model := range row.Models {
			for _, sample := range model.Artworks {
				if sample.showcaseDrawing() {
					group.Items = append(group.Items, sample.keyItem(row.GroupID))
				}
			}
		}
		if len(group.Items) == 0 {
			continue
		}
		slices.SortFunc(group.Items, func(a, b PelicanKeyItem) int {
			if order := b.GeneratedAt.Compare(a.GeneratedAt); order != 0 {
				return order
			}
			return cmp.Compare(b.ID, a.ID)
		})
		group.Items = group.Items[:min(len(group.Items), settings.MaxItems)]
		latest := group.Items[0].GeneratedAt
		if view.LatestGeneratedAt == nil || latest.After(*view.LatestGeneratedAt) {
			view.LatestGeneratedAt = &latest
		}
		view.Groups = append(view.Groups, group)
	}
	return view, nil
}

// KeyArtwork 每次重验两个展示开关、用户组权限及 Key 绑定，正文只取已发布的 SVG。
func (s *PelicanService) KeyArtwork(ctx context.Context, userID int64, bound []int64, id int64) (*PelicanKeyArtwork, error) {
	_, groups, err := s.keyShowcaseGroups(ctx, userID, bound)
	if err != nil {
		return nil, err
	}
	ids := pelicanGroupIDs(groups)
	if len(ids) == 0 {
		return nil, ErrPelicanNotFound
	}
	sample, err := s.repo.Result(ctx, id, ids)
	if err != nil {
		return nil, err
	}
	if sample == nil || sample.ID != id || !slices.Contains(ids, sample.GroupID) || !sample.showcaseDrawing() || !strings.HasPrefix(sample.Answer, "<svg") {
		return nil, ErrPelicanNotFound
	}
	return &PelicanKeyArtwork{PelicanKeyItem: sample.keyItem(sample.GroupID), ResponseText: sample.Answer}, nil
}

func (s *PelicanService) keyShowcaseGroups(ctx context.Context, userID int64, bound []int64) (PelicanShowcaseSettings, []Group, error) {
	settings, err := s.requireShowcase(ctx, true)
	if err != nil || len(bound) == 0 || userID <= 0 {
		return settings, nil, err
	}
	groups, err := s.options.UserGroups(ctx, userID)
	if err != nil {
		return settings, nil, err
	}
	allowed := make([]Group, 0, len(bound))
	for _, group := range groups {
		if group.IsActive() && slices.Contains(bound, group.ID) {
			allowed = append(allowed, group)
		}
	}
	return settings, allowed, nil
}

func (sample PelicanSummary) showcaseDrawing() bool {
	switch sample.Status {
	case "passed", "suspected", "pending_review":
		return sample.QuestionKind == "pelican" || sample.QuestionKind == "custom_svg"
	default:
		return false
	}
}

func (sample PelicanSummary) keyItem(groupID int64) PelicanKeyItem {
	// 公开状态表示成功生成作品，管理员的复核结论只在管理端和登录展示页出现。
	return PelicanKeyItem{
		ID: sample.ID, GroupID: groupID, ModelID: sample.ModelID, ReasoningEffort: sample.ReasoningEffort,
		Status: "success", LatencyMs: sample.LatencyMs, GeneratedAt: sample.TestedAt,
	}
}
