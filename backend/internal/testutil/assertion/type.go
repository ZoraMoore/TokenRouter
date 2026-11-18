package assertion

import "fmt"

// MustType 检查测试替身或解码结果的类型；不符合时直接令当前测试失败。
// 类型不匹配时 panic，消息包含预期类型和实际类型。
func MustType[T any](value any) T {
	result, ok := value.(T)
	if !ok {
		var expected T
		panic(fmt.Sprintf("expected %T, got %T", expected, value))
	}
	return result
}
