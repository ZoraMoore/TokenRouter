package compact

import (
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/pkg/logredact"
)

// RetryObservation 是单次重试的提供商/响应事实与日志配置快照。
type RetryObservation struct {
	ProviderPresent      bool
	Platform             string
	ProviderID           int64
	ProviderName         string
	Status               int
	RequestID            string
	Passthrough, LogBody bool
	LogBodyMaxBytes      int
}

// RetryNotice 保留诊断字段及正文长度限制，不执行日志或状态写入。
type RetryNotice struct {
	RetryObservation
	Message, Detail string
	Kind, Reason    string
}

// Notice 将已确定的恢复事件转换为通知，正文按启用设置提供。
func Notice(in RetryObservation, payload []byte, message string, truncate func(string, int) string) *RetryNotice {
	if !in.ProviderPresent {
		return nil
	}
	detail := ""
	if in.LogBody {
		limit := in.LogBodyMaxBytes
		if limit <= 0 {
			limit = 2048
		}
		detail = truncate(string(payload), limit)
	}
	return &RetryNotice{RetryObservation: in, Message: logredact.SanitizeUpstreamQueries(strings.TrimSpace(message)), Detail: detail, Kind: "retry", Reason: "compact_model_fallback"}
}
