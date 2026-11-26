package gemini

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"go.uber.org/zap"

	"github.com/TokenFlux/TokenRouter/internal/infra/telemetry"
	"github.com/TokenFlux/TokenRouter/internal/infra/telemetry/logging"
	"github.com/TokenFlux/TokenRouter/internal/upstream"
)

// responseUsageAudit 记录计数字段和读取状态，用于请求级用量对账。
type responseUsageAudit struct {
	Frames        int                `json:"frames"`
	UsageFrames   int                `json:"usage_frames"`
	CacheMissing  int                `json:"cache_missing"`
	CacheNull     int                `json:"cache_null"`
	CacheZero     int                `json:"cache_zero"`
	CachePositive int                `json:"cache_positive"`
	CacheResets   int                `json:"cache_resets"`
	Bytes         int64              `json:"bytes"`
	EOF           bool               `json:"eof"`
	ReadError     bool               `json:"read_error"`
	Samples       []usageAuditSample `json:"samples"`
	Last          usageAuditSample   `json:"last"`
	cachePositive bool
}

type usageAuditSample struct {
	Frame  int                        `json:"frame"`
	Path   string                     `json:"path"`
	Fields map[string]usageAuditField `json:"fields"`
}

type usageAuditField struct {
	Type  string `json:"type"`
	Value string `json:"value,omitempty"`
}

type auditedResponseBody struct {
	io.ReadCloser
	audit *responseUsageAudit
}

func (b *auditedResponseBody) Read(data []byte) (int, error) {
	n, err := b.ReadCloser.Read(data)
	b.audit.Bytes += int64(n)
	b.audit.EOF = b.audit.EOF || errors.Is(err, io.EOF)
	b.audit.ReadError = b.audit.ReadError || (err != nil && !errors.Is(err, io.EOF))
	return n, err
}

func (a *responseUsageAudit) observe(raw []byte) {
	a.Frames++
	path := "usageMetadata"
	usage := gjson.GetBytes(raw, path)
	if !usage.IsObject() {
		path = "response.usageMetadata"
		usage = gjson.GetBytes(raw, path)
	}
	if !usage.IsObject() {
		return
	}
	a.UsageFrames++
	sample := usageAuditSample{Frame: a.Frames, Path: path, Fields: make(map[string]usageAuditField)}
	for _, name := range []string{"promptTokenCount", "cachedContentTokenCount", "candidatesTokenCount", "thoughtsTokenCount", "totalTokenCount"} {
		value := usage.Get(name)
		field := usageAuditField{Type: value.Type.String()}
		if !value.Exists() {
			field.Type = "missing"
		}
		number := value.Raw
		if value.Type == gjson.String {
			number = value.Str
		}
		// 数字字符串仅接受数字字符，其他字符串只记录类型。
		if len(number) <= 48 && number != "" && strings.Trim(number, "0123456789.+-eE") == "" {
			field.Value = number
		}
		sample.Fields[name] = field
		if name == "cachedContentTokenCount" {
			switch {
			case !value.Exists():
				a.CacheMissing++
			case value.Type == gjson.Null:
				a.CacheNull++
			default:
				count, err := strconv.ParseFloat(field.Value, 64)
				if err == nil && count >= 0 {
					if count > 0 {
						a.CachePositive++
					} else {
						a.CacheZero++
						if a.cachePositive {
							a.CacheResets++
						}
					}
					a.cachePositive = count > 0
				}
			}
		}
	}
	a.Last = sample
	if len(a.Samples) < 8 {
		a.Samples = append(a.Samples, sample)
	}
}

func (a *responseUsageAudit) record(ctx context.Context, providerID int64, result *upstream.AttemptResult, failure error) {
	logging.FromContext(ctx).Info("Gemini usage observation",
		zap.String("component", "audit.gemini_usage"),
		zap.String("request_id", telemetry.RequestIDValue(ctx)),
		zap.String("upstream_request_id", result.RequestID),
		zap.Int64("provider_id", providerID),
		zap.String("platform", "gemini"),
		zap.String("model", result.UpstreamModel),
		zap.Any("usage_observation", a),
		zap.Int("input_tokens", result.Usage.InputTokens),
		zap.Int("cache_read_tokens", result.Usage.CacheReadInputTokens),
		zap.Int("output_tokens", result.Usage.OutputTokens),
		zap.Bool("client_disconnected", result.ClientDisconnect),
		zap.Bool("context_canceled", ctx.Err() != nil),
		zap.Bool("upstream_failed", failure != nil),
	)
}
