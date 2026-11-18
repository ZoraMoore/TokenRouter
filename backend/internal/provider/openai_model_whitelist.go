package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

const (
	// SettingKeyOpenAIOAuthModelWhitelist 保存管理员确认的最终模型 ID 数组。
	SettingKeyOpenAIOAuthModelWhitelist = "openai_oauth_model_whitelist"
	// OpenAIOAuthModelWhitelistMaxBytes 限制管理请求和数据库设置值的 JSON 长度。
	OpenAIOAuthModelWhitelistMaxBytes = 512 << 10
	openAIOAuthMaxModels              = 256

	openAIOAuthModelCacheTTL  = 5 * time.Second
	openAIOAuthModelErrorTTL  = time.Second
	openAIOAuthModelDBTimeout = 2 * time.Second
)

var openAIModelIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/-]{0,254}$`)

// OpenAIOAuthModelWhitelist 将当前模型列表和管理员的保存状态返回给设置页与账号创建流程。
type OpenAIOAuthModelWhitelist struct {
	Models     []string `json:"models"`
	Configured bool     `json:"configured"`
}

// OpenAIOAuthModelPolicy 保存可供多个请求并发查询的最终模型名单。
// 零值表示管理员尚未保存白名单，此时网关继续检查账号自身的模型规则。
type OpenAIOAuthModelPolicy struct {
	configured bool
	models     []string
	allowed    map[string]struct{}
}

type cachedOpenAIOAuthModelPolicy struct {
	policy    OpenAIOAuthModelPolicy
	err       error
	expiresAt time.Time
}

type openAIOAuthModelCache struct {
	current atomic.Pointer[cachedOpenAIOAuthModelPolicy]
	// 刷新和保存按数据库操作的顺序发布结果。
	mu sync.Mutex
}

// NewOpenAIOAuthModelPolicy 为完整的模型 ID 建立精确匹配索引。空切片表示拒绝所有型号。
func NewOpenAIOAuthModelPolicy(models []string) OpenAIOAuthModelPolicy {
	if len(models) == 0 {
		return OpenAIOAuthModelPolicy{configured: true}
	}
	policy := OpenAIOAuthModelPolicy{
		configured: true,
		allowed:    make(map[string]struct{}, len(models)),
		models:     make([]string, 0, len(models)),
	}
	for _, model := range models {
		if _, exists := policy.allowed[model]; !exists {
			policy.allowed[model] = struct{}{}
			policy.models = append(policy.models, model)
		}
	}
	return policy
}

// Configured 表示当前请求需要检查全局名单。
func (p OpenAIOAuthModelPolicy) Configured() bool { return p.configured }

// Allows 按大小写和完整的上游模型 ID 判断是否放行。
func (p OpenAIOAuthModelPolicy) Allows(model string) bool {
	if !p.configured {
		return true
	}
	_, allowed := p.allowed[model]
	return allowed
}

// Models 返回全局名单的独立副本，供调度枚举已配置的型号。
func (p OpenAIOAuthModelPolicy) Models() []string {
	return slices.Clone(p.models)
}

// GetOpenAIOAuthModelWhitelist 返回可编辑的模型列表。首次保存前显示新建账号的内置预设。
func (s *RuntimeSettings) GetOpenAIOAuthModelWhitelist(ctx context.Context) (OpenAIOAuthModelWhitelist, error) {
	policy, err := s.OpenAIOAuthModelWhitelistSnapshot(ctx)
	models := policy.Models()
	if !policy.Configured() {
		models = slices.Clone(openAIDefaults.Models)
	}
	return OpenAIOAuthModelWhitelist{Models: models, Configured: policy.Configured()}, err
}

// OpenAIOAuthModelWhitelistSnapshot 供调度和转发按账号检查模型。读取故障期间拒绝 OAuth 模型。
// @project-doc docs/interfaces/configuration.md#runtime_settings
func (s *RuntimeSettings) OpenAIOAuthModelWhitelistSnapshot(ctx context.Context) (OpenAIOAuthModelPolicy, error) {
	if s == nil || s.settingRepo == nil {
		return OpenAIOAuthModelPolicy{}, nil
	}
	cache := &s.openAIOAuthModels
	if cached := cache.fresh(); cached != nil {
		return cached.policy, cached.err
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cached := cache.fresh(); cached != nil {
		return cached.policy, cached.err
	}
	// 刷新的结果由多个请求共用，触发刷新的请求断开时仍完成本次查询。
	dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), openAIOAuthModelDBTimeout)
	defer cancel()
	expiresAt := time.Now().Add(openAIOAuthModelCacheTTL)
	policy, err := s.loadOpenAIOAuthModelPolicy(dbCtx)
	if err != nil {
		policy = OpenAIOAuthModelPolicy{configured: true}
		expiresAt = time.Now().Add(openAIOAuthModelErrorTTL)
	}
	cache.current.Store(&cachedOpenAIOAuthModelPolicy{policy: policy, err: err, expiresAt: expiresAt})
	return policy, err
}

func (c *openAIOAuthModelCache) fresh() *cachedOpenAIOAuthModelPolicy {
	if value := c.current.Load(); value != nil && time.Now().Before(value.expiresAt) {
		return value
	}
	return nil
}

func (s *RuntimeSettings) loadOpenAIOAuthModelPolicy(ctx context.Context) (OpenAIOAuthModelPolicy, error) {
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyOpenAIOAuthModelWhitelist)
	if err != nil {
		if errors.Is(err, s.notFound) {
			return OpenAIOAuthModelPolicy{}, nil
		}
		return OpenAIOAuthModelPolicy{}, apperror.InternalServer(
			"OPENAI_OAUTH_MODEL_WHITELIST_UNAVAILABLE",
			"Could not load the OpenAI OAuth model whitelist.",
		).WithCause(err)
	}
	if len(raw) > OpenAIOAuthModelWhitelistMaxBytes {
		return OpenAIOAuthModelPolicy{}, errors.New("stored OpenAI OAuth model whitelist exceeds 512 KiB")
	}
	var models []string
	if err := json.Unmarshal([]byte(raw), &models); err != nil {
		return OpenAIOAuthModelPolicy{}, fmt.Errorf("decode OpenAI OAuth model whitelist: %w", err)
	}
	return parseOpenAIOAuthModelPolicy(models)
}

// SetOpenAIOAuthModelWhitelist 在数据库写入成功后发布新名单。
func (s *RuntimeSettings) SetOpenAIOAuthModelWhitelist(ctx context.Context, models []string) (OpenAIOAuthModelWhitelist, error) {
	policy, err := parseOpenAIOAuthModelPolicy(models)
	if err != nil {
		return OpenAIOAuthModelWhitelist{}, apperror.BadRequest("INVALID_OPENAI_OAUTH_MODEL_WHITELIST", err.Error())
	}
	if s == nil || s.settingRepo == nil {
		return OpenAIOAuthModelWhitelist{}, errors.New("OpenAI OAuth model whitelist storage is unavailable")
	}
	raw, err := json.Marshal(policy.models)
	if err != nil {
		return OpenAIOAuthModelWhitelist{}, err
	}
	cache := &s.openAIOAuthModels
	cache.mu.Lock()
	defer cache.mu.Unlock()
	dbCtx, cancel := context.WithTimeout(ctx, openAIOAuthModelDBTimeout)
	defer cancel()
	expiresAt := time.Now().Add(openAIOAuthModelCacheTTL)
	if err := s.settingRepo.Set(dbCtx, SettingKeyOpenAIOAuthModelWhitelist, string(raw)); err != nil {
		// 连接中断时数据库可能已经提交写入，下次读取以持久化结果为准。
		cache.current.Store(nil)
		return OpenAIOAuthModelWhitelist{}, apperror.InternalServer(
			"OPENAI_OAUTH_MODEL_WHITELIST_WRITE_FAILED",
			"Could not save the OpenAI OAuth model whitelist.",
		).WithCause(err)
	}
	cache.current.Store(&cachedOpenAIOAuthModelPolicy{policy: policy, expiresAt: expiresAt})
	return OpenAIOAuthModelWhitelist{Models: policy.Models(), Configured: true}, nil
}

func parseOpenAIOAuthModelPolicy(models []string) (OpenAIOAuthModelPolicy, error) {
	if len(models) == 0 || len(models) > openAIOAuthMaxModels {
		return OpenAIOAuthModelPolicy{}, fmt.Errorf("models must contain 1 to %d model IDs", openAIOAuthMaxModels)
	}
	normalized := make([]string, len(models))
	for i, model := range models {
		normalized[i] = strings.Trim(model, " ")
		if !validOpenAIModelID(normalized[i]) {
			return OpenAIOAuthModelPolicy{}, fmt.Errorf("invalid model ID at position %d", i+1)
		}
	}
	return NewOpenAIOAuthModelPolicy(normalized), nil
}

// validOpenAIModelID 接受带供应商或微调标识的型号，并排除模型通配符和 URL。
func validOpenAIModelID(model string) bool {
	return openAIModelIDPattern.MatchString(model) && !strings.Contains(model, "://")
}
