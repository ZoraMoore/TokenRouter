package requeststate

import "github.com/tidwall/gjson"

// IsRealUserMessage 判断最后一条消息是否为用户输入，与 claude-relay-service 使用相同条件：
// messages 非空，最后一条 role 为 user，且数组 content 中没有 tool_result 或 tool_use_result。
func IsRealUserMessage(parsed *ParsedRequest) bool {
	if parsed == nil {
		return false
	}
	messagesRaw := parsed.MessagesRaw()
	if len(messagesRaw) == 0 {
		return false
	}

	messages := gjson.ParseBytes(messagesRaw)
	if !messages.IsArray() {
		return false
	}
	lastMsg := gjson.Result{}
	messages.ForEach(func(_, msg gjson.Result) bool {
		lastMsg = msg
		return true
	})
	if !lastMsg.Exists() || !lastMsg.IsObject() {
		return false
	}
	if lastMsg.Get("role").String() != "user" {
		return false
	}

	content := lastMsg.Get("content")
	if !content.Exists() {
		return true
	}
	if !content.IsArray() {
		return true
	}

	isReal := true
	content.ForEach(func(_, item gjson.Result) bool {
		itemType := item.Get("type").String()
		if itemType == "tool_result" || itemType == "tool_use_result" {
			isReal = false
			return false
		}
		return true
	})
	return isReal
}
