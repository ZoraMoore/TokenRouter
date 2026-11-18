package forward

import (
	"github.com/TokenFlux/TokenRouter/internal/protocol/bridge"
	protocolopenai "github.com/TokenFlux/TokenRouter/internal/protocol/openai"
)

// Lines 将扫描大小和实际读取留给传输 Adapter。
type Lines interface {
	Scan() bool
	Text() string
	Err() error
}

// Response 只提供本次响应的明确报文事实。
type Response struct {
	StatusCode int
	Close      func()
	Runtime    bridge.Runtime
	RequestID  string
	Headers    map[string][]string
	Lines      Lines
	// MaxSSEFrameBytes 限制转换器累计的单帧字节数，零值使用默认上限。
	MaxSSEFrameBytes int
}

// Output 同步写入协议事件并返回写入错误，HTTP 状态和缓冲由适配器管理。
type Output interface {
	CopyHeaders(map[string][]string)
	BeginJSON()
	BeginStream()
	ReverseTools([]byte) []byte
	JSONBytes([]byte)
	ResponsesJSON(*protocolopenai.ResponsesResponse)
	ChatJSON(*protocolopenai.ChatCompletionsResponse)
	Event(string, []byte) (int, error)
	Flush()
	Error(int, string, string)
	Observe(string, string, error, string, string)
}
