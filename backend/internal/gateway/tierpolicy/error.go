package tierpolicy

// BlockedError 表示服务档位策略拒绝本次请求，保存可返回客户端的错误消息。
type BlockedError struct{ Message string }

func (e *BlockedError) Error() string { return e.Message }
