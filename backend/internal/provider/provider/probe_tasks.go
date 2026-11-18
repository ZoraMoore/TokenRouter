package provider

import (
	"context"
	"sync"

	"github.com/TokenFlux/TokenRouter/internal/provider"
)

// ProbeTasks 为提供商测试和模型预览共用原未持久提供商互斥；持久提供商仍由统一协调器认领。
type ProbeTasks struct {
	Coordinator *provider.OpenAITaskCoordinator
	Options     provider.OpenAITaskOptions
	fallback    sync.Mutex
}

func (p *ProbeTasks) Ensure(ctx context.Context, value *provider.Record, expected string) error {
	options := p.Options
	options.FallbackMutex = &p.fallback
	return p.Coordinator.Ensure(ctx, options, value, expected)
}
