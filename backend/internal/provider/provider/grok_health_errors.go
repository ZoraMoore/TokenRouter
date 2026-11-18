package provider

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
	"github.com/TokenFlux/TokenRouter/internal/upstream/grok"
)

// GrokHealthInput 保存当前尝试的模型和观测数据。
type GrokHealthInput struct {
	Observation     HealthObservation
	Models          []string
	QuotaModel      string
	TeamModel       string
	RequestScoped   bool
	ServerTransient bool
}

func firstGrokHealthModel(models []string) string {
	if len(models) == 0 {
		return ""
	}
	return strings.TrimSpace(models[0])
}

// ObserveError 依次处理配置策略、模型状态、配额和默认冷却。
func (s *GrokHealth) ObserveError(ctx context.Context, value *providercore.Record, input GrokHealthInput) providercore.UpstreamErrorDecision {
	statusCode, headers, responseBody := input.Observation.Status, input.Observation.Headers, input.Observation.Body
	canonicalModel := input.Models

	if s == nil || value == nil {
		return providercore.UpstreamErrorDecision{Policy: providercore.ErrorPolicyNone}
	}
	if input.RequestScoped {
		return providercore.UpstreamErrorDecision{Policy: providercore.ErrorPolicyNone}
	}
	if model := firstGrokHealthModel(canonicalModel); model != "" {
		canonicalModel = []string{s.NormalizeModel(value, model)}
	}
	stateCtx, cancel := ProviderStateContext(ctx)
	defer cancel()
	observation := input.Observation
	observation.Headers = nil
	observation.ModelProvided = len(canonicalModel) > 0
	if len(canonicalModel) > 0 {
		observation.Model = canonicalModel[0]
	}
	if model := firstGrokHealthModel(canonicalModel); model != "" {
		observation.EffectiveModel = model
	}

	now := time.Now()
	quotaSnapshot := grok.ParseQuotaObservation(headers, statusCode, now)
	quotaModel := firstGrokHealthModel(canonicalModel)
	if quotaModel == "" {
		quotaModel = input.QuotaModel
	}
	providercore.StampGrokQuotaPlan(providercore.CloneRecord(value), quotaSnapshot, quotaModel, grok.ResolveGrokTextResponsesModelID, grok.ApplyGrok45ResponsesPlanSignal)
	// 模型容量 429 按请求压力处理，保存额度观测，提供商限流状态保持不变。
	snapshotFailure := grok.ClassifyGrokUpstreamFailure(statusCode, responseBody, quotaModel)
	s.StoreSnapshot(stateCtx, value, quotaSnapshot, snapshotFailure.Class != grok.GrokFailureModelCapacity, input.TeamModel)

	decision := providercore.ErrorDecisionWithoutPersistence(value, statusCode)
	if s.Health != nil {
		if value.IsPoolMode() || value.IsCustomErrorCodesEnabled() {
			decision.Policy = s.Health.ApplyExplicitErrorPolicy(stateCtx, providercore.CloneRecord(value), observation)
			decision.StopScheduling = decision.Policy == providercore.ErrorPolicyCustomMatched || decision.Policy == providercore.ErrorPolicyTempUnscheduled
		} else {
			decision = providercore.UpstreamErrorDecision{Policy: providercore.ErrorPolicyNone}
		}
	}
	switch decision.Policy {
	case providercore.ErrorPolicyCustomMatched:
		decision.StopScheduling = true
		s.Runtime.BlockProviderScheduling(value, time.Time{}, "upstream_disable")
		return decision
	case providercore.ErrorPolicyTempUnscheduled:
		decision.StopScheduling = true
		return decision
	case providercore.ErrorPolicyCustomSkipped, providercore.ErrorPolicyPoolBypassed:
		return decision
	}

	if s.Health != nil && len(canonicalModel) > 0 &&
		s.Health.Models.Observe(stateCtx, providercore.CloneRecord(value), canonicalModel[0], statusCode, responseBody, observation.Thinking, observation.ImagesEndpoint) {
		decision.StopScheduling = true
		return decision
	}
	// 普通提供商先保留模型不存在等精确处理，再应用管理员临时规则。
	if s.Health != nil && statusCode != http.StatusUnauthorized &&
		!value.IsPoolMode() && !value.IsCustomErrorCodesEnabled() &&
		s.Health.Core.HandleTempUnschedulable(stateCtx, providercore.CloneRecord(value), statusCode, responseBody, observation.EffectiveModel) {
		decision.Policy = providercore.ErrorPolicyTempUnscheduled
		decision.StopScheduling = true
		if firstGrokHealthModel(canonicalModel) == "" {
			s.Runtime.BlockProviderScheduling(value, time.Time{}, "upstream_disable")
		}
		return decision
	}

	// Grok API Key 的 5xx 与 OpenAI API Key 共用提供商+最终模型的瞬态冷却；
	// OAuth 提供商和模型未知的请求按提供商级规则退避。
	model := firstGrokHealthModel(canonicalModel)
	if model == "" {
		model = quotaModel
	}
	if value.Type == capability.ProviderTypeAPIKey && model != "" && statusCode >= 500 &&
		input.ServerTransient {
		s.recordModelTransient(value, model)
		return decision
	}

	// 响应体中的免费额度、账单、空输出和容量错误优先于通用状态码处理。
	failure := grok.ClassifyGrokUpstreamFailure(statusCode, responseBody, model)
	if failure.ShouldCooldown && failure.Class != grok.GrokFailureNone && failure.Class != grok.GrokFailureRateLimit {
		if failure.Class == grok.GrokFailureFreeUsage {
			if resetAt, limited := providercore.GrokRateLimitResetAtForProvider(value, quotaSnapshot, now); limited && resetAt.After(now) {
				if failure.Model != "" && providercore.IsGrokModelSpecificFreeUsage(strings.ToLower(failure.Reason), failure.Model) {
					providercore.MarkGrokModelQuotaBlock(value.ID, failure.Model, resetAt)
					decision.StopScheduling = true
					return decision
				}
				s.RateLimit(stateCtx, value, resetAt, input.TeamModel)
				decision.StopScheduling = true
				return decision
			}
		}
		if s.ApplyFailure(stateCtx, value, failure, input.TeamModel) {
			decision.StopScheduling = true
			return decision
		}
	}

	switch statusCode {
	case http.StatusUnauthorized:
		s.TempUnschedule(stateCtx, value, 10*time.Minute, "grok credentials unauthorized")
		decision.StopScheduling = true
		return decision
	case http.StatusPaymentRequired:
		// 402 表示提供商计费不可用，此处将它短期停调。
		s.TempUnschedule(stateCtx, value, 30*time.Minute, "grok payment required")
		decision.StopScheduling = true
		return decision
	case http.StatusForbidden:
		if s.applyForbiddenPolicy(stateCtx, value, responseBody, observation.EffectiveModel) {
			decision.StopScheduling = true
			return decision
		}
		s.TempUnschedule(stateCtx, value, 30*time.Minute, "grok access or entitlement denied")
		decision.StopScheduling = true
		return decision
	case http.StatusMethodNotAllowed:
		// 提供商不支持所选 Grok 端点时，临时停调该提供商。
		s.TempUnschedule(stateCtx, value, 30*time.Minute, "grok endpoint not supported (405)")
		decision.StopScheduling = true
		return decision
	case http.StatusTooManyRequests:
		// 快照处理已同时写入运行时和持久化限流状态。
		decision.StopScheduling = true
		return decision
	default:
		if statusCode >= 500 {
			s.TempUnschedule(stateCtx, value, 2*time.Minute, "grok upstream temporary error")
			decision.StopScheduling = true
			return decision
		}
	}
	return decision
}

// ApplyFailure 将分类结果映射到提供商健康状态；返回 true 表示已完整处理，
// 调用方不能再次应用状态码默认逻辑。
func (s *GrokHealth) ApplyFailure(
	ctx context.Context,
	value *providercore.Record,
	decision grok.GrokUpstreamFailureDecision, teamModel string,
) bool {
	if s == nil || value == nil || !decision.ShouldCooldown || decision.Cooldown <= 0 {
		return false
	}
	// 原因保持简短稳定，供运维界面和 temp_unschedulable_reason 使用。
	var reason string
	switch decision.Class {
	case grok.GrokFailureFreeUsage:
		reason = "grok free usage exhausted"
		// 模型级免费额度耗尽只软封禁该模型，使同提供商其它模型仍可调度。
		low := strings.ToLower(decision.Reason)
		if decision.Model != "" && providercore.IsGrokModelSpecificFreeUsage(low, decision.Model) {
			until := time.Now().Add(decision.Cooldown)
			providercore.MarkGrokModelQuotaBlock(value.ID, decision.Model, until)
			// 上游已明确限定到单模型，提供商级冷却会错误移除健康的其它模型。
			return true
		}
	case grok.GrokFailureBilling:
		low := strings.ToLower(decision.Reason)
		if strings.Contains(low, "spending") || strings.Contains(low, "credits") {
			// 消费上限或 credits 耗尽属于账单窗口条件，保留提供商可恢复状态并由常规限流恢复解除。
			s.RateLimit(ctx, value, providercore.GrokSpendingLimitResetAt(providercore.CloneRecord(value), time.Now()), teamModel)
			return true
		}
		// 保留历史 402/payment 原因，兼容运维界面和回归测试。
		reason = "grok payment required"
	case grok.GrokFailureEmptyUpstream:
		reason = "grok empty model output"
	case grok.GrokFailureModelCapacity:
		// 容量压力只作用于请求模型，提供商切换仍由外层有界重试决定。
		if model := strings.TrimSpace(decision.Model); model != "" {
			cooldown := decision.Cooldown
			if cooldown <= 0 {
				cooldown = 3 * time.Minute
			}
			providercore.MarkGrokModelTransientBlock(value.ID, model, time.Now().Add(cooldown))
		}
		return true
	case grok.GrokFailureRateLimit:
		// 429 响应未表明免费额度耗尽时，按 Retry-After 和额度响应头处理。
		return false
	case grok.GrokFailureServer:
		reason = "grok upstream temporary error"
	case grok.GrokFailureCompatibility:
		// 请求格式不兼容时交给外层切换提供商，当前健康状态保持不变。
		return true
	default:
		return false
	}
	s.TempUnschedule(ctx, value, decision.Cooldown, reason)
	return true
}

// applyForbiddenPolicy 保留非内容拒绝 403 的管理员规则；未匹配时使用原权益冷却。
func (s *GrokHealth) applyForbiddenPolicy(ctx context.Context, value *providercore.Record, responseBody []byte, effectiveModel string) bool {
	if value == nil || !value.IsTempUnschedulableEnabled() {
		return false
	}

	matches := providercore.MatchTempUnschedulableRules(providercore.CloneRecord(value), http.StatusForbidden, responseBody)
	if len(matches) == 0 {
		return false
	}

	match := matches[0]
	// 存储可用时调用共享策略处理原因字段和缓存写入。
	if s != nil && s.Health != nil &&
		s.Health.Limits.Plans !=
			nil {
		stateCtx, cancel := ProviderStateContext(ctx)
		handled := s.Health.Core.TryTempUnschedulable(stateCtx, providercore.CloneRecord(value), http.StatusForbidden, responseBody, value.Platform != capability.PlatformAntigravity, effectiveModel)

		cancel()
		if handled {
			return true
		}
	}

	// 服务未完整构造时（例如单元测试网关）仍遵循配置时长，不能静默回退到 30 分钟。
	cooldown := time.Duration(match.Rule.DurationMinutes) * time.Minute
	if cooldown > 0 {
		s.TempUnschedule(ctx, value, cooldown, "grok configured forbidden rule")
	}
	return true
}

// recordModelTransient 更新与选择器共用的提供商和模型状态，并记录日志。
func (s *GrokHealth) recordModelTransient(value *providercore.Record, model string) {
	if s.ModelTransient == nil || value == nil {
		return
	}
	model = providercore.NormalizeTransientModel(model)
	decision := s.ModelTransient.RecordFailure(value.ID, model, time.Now())
	if decision.FailureStreak == 0 {
		return
	}
	slog.Warn("openai_model_transient_state", "provider_id", value.ID, "platform", value.Platform, "model", model, "failure_streak", decision.FailureStreak, "cooldown_ms", decision.Cooldown.Milliseconds(), "block_scope", "provider_model")
}
