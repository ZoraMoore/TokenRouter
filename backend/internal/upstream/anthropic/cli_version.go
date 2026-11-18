package anthropic

import (
	"log/slog"
	"os"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/gateway/clientmeta"
)

// CLIVersionEnv 是 CLICurrentVersion 的可选运维覆盖。
//
// 存在的理由：Anthropic 会对新模型设客户端版本下限（例如 claude-fable-5-1 要求
// claude-cli >= 2.1.251），命中时上游直接返回
// `Claude Code X.Y.Z does not support this model; version A.B.C or newer is required`。
// 在没有本开关之前，这类模型必须等 tokenrouter 发一个新版本才能使用，
// 而改动本身只是一个常量。xai 包的 XAI_GROK_CLI_VERSION 已经是同样的做法。
const CLIVersionEnv = "TOKENROUTER_CLAUDE_CLI_VERSION"

// resolvedCLIVersion 在包初始化时解析一次。
//
// 伪装版本在进程内固定，环境变量在包初始化时读取。
// User-Agent 头与请求体 billing attribution 块里的 cc_version 由不同代码路径写入，
// 若两次读到不同的值（例如进程运行中有人改了环境变量），同一个请求就会自相矛盾，
// 被上游判为非正版客户端。
var resolvedCLIVersion = resolveCLIVersion(cliVersionOverride())

// CLIVersion 返回对外伪装的 Claude Code CLI 版本号（三段 semver）。
//
// 调用方需要通过本函数读取版本号，CLICurrentVersion 是未配置覆盖时的默认值。
func CLIVersion() string {
	return resolvedCLIVersion
}

// IsSupportedCLIVersion 使用内置版本检查版本号是否受支持。
func IsSupportedCLIVersion(version string) bool {
	return clientmeta.IsSupportedClaudeCLIVersion(version, CLICurrentVersion)
}

// resolveCLIVersion 把环境变量的原始值解析成可用的版本号。
// 空值使用内置版本，非空非法值使用内置版本并记录告警。
func resolveCLIVersion(raw string) string {
	version := strings.TrimSpace(raw)
	if version == "" {
		return CLICurrentVersion
	}
	if !IsSupportedCLIVersion(version) {
		slog.Warn("ignoring invalid Claude CLI version override; falling back to the built-in pin",
			"env", CLIVersionEnv,
			"value", version,
			"builtin", CLICurrentVersion,
			"requirement", "strict three-part semver, not older than the built-in pin")
		return CLICurrentVersion
	}
	return version
}

// cliVersionOverride 兼容旧部署变量，新变量非空时始终优先。
func cliVersionOverride() string {
	if value := os.Getenv(CLIVersionEnv); value != "" {
		return value
	}
	return os.Getenv("SUB2API_CLAUDE_CLI_VERSION")
}
