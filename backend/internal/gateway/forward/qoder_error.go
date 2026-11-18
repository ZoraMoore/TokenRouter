package forward

// QoderErrorView 保存单次供应商错误的展示字段。
type QoderErrorView struct {
	Recognized           bool
	Status, SourceStatus int
	Kind, Message, Body  string
}
