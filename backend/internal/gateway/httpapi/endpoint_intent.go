package httpapi

import (
	"net/http"
	"strings"
)

// IsBatchImageManagementRequest 标识不会创建新生成任务的批任务管理请求。
func IsBatchImageManagementRequest(method, path string) bool {
	path = strings.TrimRight(path, "/")
	const root = "/v1/images/batches"
	if method == http.MethodGet && (path == root || strings.HasPrefix(path, root+"/")) {
		return true
	}
	if method == http.MethodDelete && strings.HasPrefix(path, root+"/") {
		return true
	}
	return method == http.MethodPost && strings.HasPrefix(path, root+"/") && strings.HasSuffix(path, "/cancel")
}

// IsBatchImageBillingBypassRequest 排除模型列表，仅让既有批任务的查询和管理绕过计费。
func IsBatchImageBillingBypassRequest(method, path string) bool {
	path = strings.TrimRight(path, "/")
	return path != "/v1/images/batches/models" && IsBatchImageManagementRequest(method, path)
}

// IsAPIKeyNonConsumingRequest 标识只读取已有状态或释放资源的网关请求。
// 未知路径按消费请求检查团队限额。
func IsAPIKeyNonConsumingRequest(method, path string) bool {
	path = strings.TrimRight(path, "/")
	if IsAPIKeyUsageRequest(method, path) {
		return true
	}
	if method == http.MethodGet {
		if strings.HasSuffix(path, "/models") || IsBatchImageManagementRequest(method, path) || IsGrokVideoTaskRead(method, path) {
			return true
		}
	}
	if IsBatchImageManagementRequest(method, path) {
		return true
	}
	if method == http.MethodPost && strings.HasSuffix(path, "/messages/count_tokens") {
		return true
	}
	return false
}

// IsAPIKeyUsageRequest 识别两套 Claude 风格的 Key 用量查询入口。
// 查询读取 Key 自身状态，订阅或 Key 配额耗尽时仍可查询。
func IsAPIKeyUsageRequest(method, path string) bool {
	if method != http.MethodGet {
		return false
	}
	switch strings.TrimRight(path, "/") {
	case "/v1/usage", "/antigravity/v1/usage":
		return true
	default:
		return false
	}
}
