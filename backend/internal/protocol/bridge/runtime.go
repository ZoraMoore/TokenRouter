package bridge

import "time"

// Runtime 提供转换所需的时刻和随机字节，来源、格式和错误处理由调用方决定。
// 转换器不读取环境，不安装全局时钟或随机源。
// @project-doc docs/architecture/gateway_request_lifecycle.md#protocol_conversion_boundary
type Runtime struct {
	Now        func() time.Time
	ReadRandom func([]byte) (int, error)
}
