package session

import (
	"strings"
	"unicode/utf8"
)

// MaxPersistedSessionIDLength 保留既有数据库列宽；超长整值拒绝，不截断。
const MaxPersistedSessionIDLength = 255

// SanitizeClientSessionID 规范化客户端提供的原始会话标识：去除首尾空白，包含控制字符
// （CR、LF、制表符、NUL 等）或超过数据库列上限时整值拒绝，防止日志或请求头
// 注入内容进入关联数据。缺失或无效输入返回空字符串。
func SanitizeClientSessionID(raw string) string {
	if !utf8.ValidString(raw) {
		return ""
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	count := 0
	for _, r := range trimmed {
		if r < 0x20 || r == 0x7f {
			// 关联标识含控制字符时丢弃整个值。
			return ""
		}
		count++
		if count > MaxPersistedSessionIDLength {
			return ""
		}
	}
	return trimmed
}

// ExtractClientSessionID 按调用方指定的 Header 顺序读取会话 ID，Grok 扩展由对应入口启用。
func ExtractClientSessionID(header func(string) string, names []string, grok bool) string {
	for _, name := range names {
		if value := SanitizeClientSessionID(header(name)); value != "" {
			return value
		}
	}
	if grok {
		return SanitizeClientSessionID(header("X-Grok-Conv-Id"))
	}
	return ""
}
