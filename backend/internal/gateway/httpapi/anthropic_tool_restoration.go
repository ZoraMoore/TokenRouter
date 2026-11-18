package httpapi

import (
	claude "github.com/TokenFlux/TokenRouter/internal/upstream/anthropic"
)

// toolNameRewriteKey 是 gin.Context 上存 ToolNameRewrite 映射的 key。
// 请求阶段写入，响应阶段读取，用于 bytes 级逆向还原假名 → 真名。
const toolNameRewriteKey = "claude_tool_name_rewrite"

var staticToolNameRewrites = claude.StaticToolNameRewrites

// toolNameRewriteFromContext 从 gin.Context 读取请求阶段保存的工具名映射。
// c 为 nil、键缺失或类型不符时返回 nil，调用方需要处理空结果。
func toolNameRewriteFromContext(c interface {
	Get(string) (any, bool)
},
) *claude.ToolNameRewrite {
	if c == nil {
		return nil
	}
	raw, ok := c.Get(toolNameRewriteKey)
	if !ok || raw == nil {
		return nil
	}
	rw, _ := raw.(*claude.ToolNameRewrite)
	return rw
}

// ReverseToolNamesIfPresent 是响应侧 5 处注入点的统一封装：从 c 取出 mapping
// 并对 chunk 做 bytes 级假名→真名替换。c 没有 mapping 时仍会做静态前缀还原。
func ReverseToolNamesIfPresent(c interface {
	Get(string) (any, bool)
}, chunk []byte,
) []byte {
	rw := toolNameRewriteFromContext(c)
	if rw == nil && len(staticToolNameRewrites) == 0 {
		return chunk
	}
	return claude.RestoreToolNamesInBytes(chunk, rw)
}
