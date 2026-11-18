package transport

// Options 包含 app 传入的传输参数。
// 参数解析区分 nil 和零值。
type Options struct {
	ValidateResolvedIP          bool
	ConnectionPoolIsolation     string
	MaxUpstreamClients          int
	ClientIdleTTLSeconds        int
	MaxIdleConns                int
	MaxIdleConnsPerHost         int
	MaxConnsPerHost             int
	IdleConnTimeoutSeconds      int
	ResponseHeaderTimeout       int
	OpenAIResponseHeaderTimeout int
	GrokResponseHeaderTimeout   int
	OpenAIHTTP2                 HTTP2Options
}

// HTTP2Options 保留开关、阈值及秒级时间配置，回退状态由 egress 持有。
type HTTP2Options struct {
	Enabled                   bool
	AllowProxyFallbackToHTTP1 bool
	FallbackErrorThreshold    int
	FallbackWindowSeconds     int
	FallbackTTLSeconds        int
}
