package provider

import (
	"context"

	"github.com/TokenFlux/TokenRouter/internal/pkg/requestcontext"
)

// DetachStreamUpstreamContext 将流式上游请求与客户端取消信号分离。
func DetachStreamUpstreamContext(ctx context.Context, stream bool) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.Background(), func() {}
	}
	if !stream {
		return ctx, func() {}
	}
	return requestcontext.Detach(ctx), func() {}
}

func DetachUpstreamContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.Background(), func() {}
	}
	return requestcontext.Detach(ctx), func() {}
}
