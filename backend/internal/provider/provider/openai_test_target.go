package provider

import (
	"context"
	"net/http"

	"github.com/TokenFlux/TokenRouter/internal/provider"
)

type openaiTestTarget struct {
	executor *OpenAIProviderTest
	record   *provider.Record
}

// Target 将本次请求状态与提供商记录隔离，不向公开测试信息暴露凭据。
func (s *OpenAIProviderTest) Target(value *provider.Record) provider.TestTarget {
	return openaiTestTarget{executor: s, record: value}
}

func (t openaiTestTarget) Information() provider.TestTargetInfo {
	return provider.TestTargetInfo{ProviderSnapshot: t.record.RoutingSnapshot(), APIProtocol: (provider.ProtocolTarget{Record: t.record}).GetAPIProtocol()}
}

// IQMappedModel 预检实际模型映射，阻止将图像等专用模型送入数字指纹题。
func (t openaiTestTarget) IQMappedModel(model string) string {
	return mappedTestModel(t.record, model)
}

func (t openaiTestTarget) Execute(ctx context.Context, request provider.PreparedTestRequest, sink provider.TestEventSink) error {
	headers := make(http.Header)
	headers.Set("User-Agent", request.UserAgent)
	headers.Set("originator", request.Originator)
	run := NewTestRun(ctx, headers, sink)
	run.FullAnswer = request.FullAnswer
	run.SVGAnswer = request.SVGAnswer
	run.IQMode = request.IQMode
	run.ReasoningEffort = request.ReasoningEffort
	defer run.Cancel()
	run.Automatic = request.Automatic
	run.RequestedProtocol = provider.TextProtocol(request.Protocol)
	if t.executor.Prepare != nil {
		if err := t.executor.Prepare(run, t.record); err != nil {
			return run.Result((TestStreamOutput{}).Error(run, err.Error()))
		}
	}
	return run.Result(t.executor.Execute(run, t.record, request.Model, request.Prompt, request.Mode, request.TestType))
}
