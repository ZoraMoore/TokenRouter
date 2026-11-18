package scheduler

import "context"

// selectOnlyKey 标记仅选择提供商的请求，preservedStickyKey 标记需要保留的粘性绑定。
type (
	selectOnlyKey      struct{}
	preservedStickyKey struct{}
)

func WithSelectOnly(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, selectOnlyKey{}, true)
}

func IsSelectOnly(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(selectOnlyKey{}).(bool)
	return v
}

func WithPreservedSticky(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, preservedStickyKey{}, true)
}

func PreserveStickyFromContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(preservedStickyKey{}).(bool)
	return v
}
