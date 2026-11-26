package upstream

import (
	"strings"

	"github.com/tidwall/gjson"
)

// IsSSEHeartbeatEvent 判断上游发送的空事件和心跳事件。
func IsSSEHeartbeatEvent(event, data string) bool {
	data = strings.TrimSpace(data)
	if data == "" {
		return true
	}
	kind := gjson.Get(data, "type").String()
	if kind == "" {
		kind = strings.TrimSpace(event)
	}
	return kind == "ping" || kind == "heartbeat" || kind == "keepalive"
}
