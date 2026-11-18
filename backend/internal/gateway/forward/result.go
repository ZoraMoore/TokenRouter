package forward

import (
	"time"

	"github.com/TokenFlux/TokenRouter/internal/protocol"
	"github.com/TokenFlux/TokenRouter/internal/upstream"
)

// Result 保存本次转发的观测结果。
type Result struct {
	// UpstreamResponseModel 是协议转换前的上游模型声明；空值表示未声明。
	UpstreamResponseModel                      string
	RequestID                                  string
	UpstreamHeaders                            map[string][]string
	Usage                                      upstream.TokenUsage
	Model, UpstreamModel                       string
	Stream                                     bool
	Duration                                   time.Duration
	FirstTokenMs                               *int
	ClientDisconnect                           bool
	ReasoningEffort, RequestedReasoningEffort  *string
	UpstreamResponseServiceTier                string
	ServiceTier                                *string
	ImageCount                                 int
	ImageSize, ImageInputSize, ImageOutputSize string
	ImageOutputSizes                           []string
	ImageSizeSource                            string
	ImageSizeBreakdown                         map[string]int
	SearchCount                                int
	AudioUsage                                 *protocol.AudioUsage
}
