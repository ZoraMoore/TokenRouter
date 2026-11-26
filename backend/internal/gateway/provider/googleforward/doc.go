// Package googleforward 准备 Gemini 和 Antigravity 的上游请求，处理提供商策略并返回用量。
//
// 阅读入口：
//   - gemini_attempts.go：Gemini Messages 和原生接口的调用流程。
//   - gemini_openai.go：Gemini 与 OpenAI 客户端协议之间的转换。
//   - antigravity_openai.go：Antigravity 的 Chat 和 Responses 调用流程。
//
// 文件分组：
//   - gemini*.go：请求准备、用量返回、流式失败和错误策略。
//   - antigravity*.go：身份、请求转换、重试和原生调用。
//   - contracts.go：依赖接口和调用配置。
package googleforward
