package scheduler

// Diagnostics 接收 app 提供的日志回调。
type Diagnostics struct {
	Event func(level, event string, fields ...any)
	Logf  func(scope, format string, args ...any)
}

func (d Diagnostics) printf(scope, format string, args ...any) {
	if d.Logf != nil {
		d.Logf(scope, format, args...)
	}
}

func (d Diagnostics) event(level, event string, fields ...any) {
	if d.Event != nil {
		d.Event(level, event, fields...)
	}
}
