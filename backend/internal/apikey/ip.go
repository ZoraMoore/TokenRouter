package apikey

import (
	"net"

	"github.com/TokenFlux/TokenRouter/internal/pkg/ipmatch"
)

// CheckIPRestrictionWithCompiledRules 使用预编译规则检查 IP 是否允许访问。
func CheckIPRestrictionWithCompiledRules(clientIP string, whitelist, blacklist *ipmatch.CompiledIPRules) (bool, string) {
	// 规范化 IP
	clientIP = ipmatch.NormalizeIP(clientIP)
	if clientIP == "" {
		return false, "access denied"
	}
	parsedIP := net.ParseIP(clientIP)
	if parsedIP == nil {
		return false, "access denied"
	}

	// 1. 检查黑名单
	if blacklist != nil && blacklist.PatternCount > 0 && ipmatch.MatchesCompiledRules(parsedIP, blacklist) {
		return false, "access denied"
	}

	// 2. 检查白名单（如果设置了白名单，IP 必须在其中）
	if whitelist != nil && whitelist.PatternCount > 0 && !ipmatch.MatchesCompiledRules(parsedIP, whitelist) {
		return false, "access denied"
	}

	return true, ""
}
