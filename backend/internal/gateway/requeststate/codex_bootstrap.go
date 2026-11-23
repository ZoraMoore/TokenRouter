package requeststate

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"strconv"
	"strings"
	"time"
)

func HasUniqueJSONMembers(body []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(body))
	if !ConsumeUniqueJSONValue(decoder) {
		return false
	}
	_, err := decoder.Token()
	return err == io.EOF
}

func ConsumeUniqueJSONValue(decoder *json.Decoder) bool {
	token, err := decoder.Token()
	if err != nil {
		return false
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return true
	}

	switch delim {
	case '{':
		members := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return false
			}
			key, ok := keyToken.(string)
			if !ok {
				return false
			}
			if _, duplicate := members[key]; duplicate {
				return false
			}
			members[key] = struct{}{}
			if !ConsumeUniqueJSONValue(decoder) {
				return false
			}
		}
		end, err := decoder.Token()
		return err == nil && end == json.Delim('}')
	case '[':
		for decoder.More() {
			if !ConsumeUniqueJSONValue(decoder) {
				return false
			}
		}
		end, err := decoder.Token()
		return err == nil && end == json.Delim(']')
	default:
		return false
	}
}

func IsCodexDelegationTool(namespace, name string) bool {
	return (namespace == "codex_app" || namespace == "codex_tui") &&
		(name == "create_thread" || name == "send_message_to_thread")
}

// IsCodexAutomationTool 匹配桌面端投递自动化提示使用的工具名。
func IsCodexAutomationTool(namespace, name string) bool {
	return namespace == "codex_app" && name == "automation_update"
}

func ValidCodexAutomationBootstrap(value string) bool {
	normalized := strings.ReplaceAll(value, "\r\n", "\n")
	if strings.ContainsRune(normalized, '\r') {
		return false
	}
	lines := strings.Split(normalized, "\n")
	if len(lines) < 6 {
		return false
	}
	if _, ok := CodexAutomationHeaderValue(lines[0], "Automation: "); !ok {
		return false
	}
	automationID, ok := CodexAutomationHeaderValue(lines[1], "Automation ID: ")
	if !ok || !ValidCodexAutomationID(automationID) {
		return false
	}
	expectedMemory := "Automation memory: $CODEX_HOME/automations/" + automationID + "/memory.md"
	if lines[2] != expectedMemory {
		return false
	}
	lastRun, ok := CodexAutomationHeaderValue(lines[3], "Last run: ")
	if !ok || !ValidCodexAutomationLastRun(lastRun) || lines[4] != "" {
		return false
	}
	return strings.TrimSpace(strings.Join(lines[5:], "\n")) != ""
}

func CodexAutomationHeaderValue(line, prefix string) (string, bool) {
	if !strings.HasPrefix(line, prefix) {
		return "", false
	}
	value := strings.TrimPrefix(line, prefix)
	return value, value != "" && strings.TrimSpace(value) == value
}

func ValidCodexAutomationID(value string) bool {
	if value == "" || len(value) > 128 || value == "." || value == ".." {
		return false
	}
	for i := range len(value) {
		c := value[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' {
			continue
		}
		return false
	}
	return true
}

func ValidCodexAutomationLastRun(value string) bool {
	if value == "never" {
		return true
	}
	separator := strings.LastIndex(value, " (")
	if separator <= 0 || !strings.HasSuffix(value, ")") {
		return false
	}
	runAt, err := time.Parse(time.RFC3339Nano, value[:separator])
	if err != nil {
		return false
	}
	epochMillis, err := strconv.ParseInt(value[separator+2:len(value)-1], 10, 64)
	return err == nil && runAt.UnixMilli() == epochMillis
}

func ValidCodexAutomationHeartbeat(value string) bool {
	decoder := xml.NewDecoder(strings.NewReader(value))
	var rootSeen, automationIDSeen bool
	var automationID bytes.Buffer
	depth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			id := automationID.String()
			return rootSeen && automationIDSeen && depth == 0 &&
				strings.TrimSpace(id) == id && ValidCodexAutomationID(id)
		}
		if err != nil {
			return false
		}
		switch current := token.(type) {
		case xml.StartElement:
			depth++
			if current.Name.Space != "" || len(current.Attr) != 0 || depth > 2 {
				return false
			}
			if depth == 1 {
				if rootSeen || current.Name.Local != "heartbeat" {
					return false
				}
				rootSeen = true
			} else if automationIDSeen || current.Name.Local != "automation_id" {
				return false
			}
			automationIDSeen = depth == 2
		case xml.EndElement:
			if current.Name.Space != "" {
				return false
			}
			depth--
			if depth < 0 {
				return false
			}
		case xml.CharData:
			if depth == 2 {
				_, _ = automationID.Write(current)
			} else if len(bytes.TrimSpace(current)) != 0 {
				return false
			}
		case xml.Comment, xml.ProcInst, xml.Directive:
			return false
		}
	}
}

func ValidCodexDelegationEnvelope(value string) bool {
	decoder := xml.NewDecoder(strings.NewReader(value))
	var rootSeen, sourceSeen, inputSeen bool
	var childName string
	var childText bytes.Buffer
	depth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return rootSeen && depth == 0 && sourceSeen && inputSeen
		}
		if err != nil {
			return false
		}
		switch current := token.(type) {
		case xml.StartElement:
			depth++
			if current.Name.Space != "" || len(current.Attr) != 0 || (depth == 1 && current.Name.Local != "codex_delegation") || depth > 2 {
				return false
			}
			if depth == 1 {
				if rootSeen {
					return false
				}
				rootSeen = true
				continue
			}
			if current.Name.Local != "source_thread_id" && current.Name.Local != "input" {
				return false
			}
			childName = current.Name.Local
			childText.Reset()
		case xml.EndElement:
			if current.Name.Space != "" {
				return false
			}
			if depth == 2 {
				if current.Name.Local != childName || strings.TrimSpace(childText.String()) == "" {
					return false
				}
				if childName == "source_thread_id" {
					if sourceSeen {
						return false
					}
					sourceSeen = true
				} else {
					if inputSeen {
						return false
					}
					inputSeen = true
				}
				childName = ""
			}
			depth--
			if depth < 0 {
				return false
			}
		case xml.CharData:
			if depth == 2 {
				_, _ = childText.Write(current)
			} else if len(bytes.TrimSpace(current)) != 0 {
				return false
			}
		case xml.Comment:
			return false
		case xml.ProcInst, xml.Directive:
			return false
		}
	}
}
