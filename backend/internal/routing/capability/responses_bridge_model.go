package capability

import (
	"strconv"
	"strings"
)

// ResponsesBridgeSupportsMaxEffort 判断文本桥接是否支持指定型号的 max effort。
func ResponsesBridgeSupportsMaxEffort(model string) bool {
	return isResponsesBridgeModelAtLeastVersion(model, 5, 6)
}

func isResponsesBridgeModelAtLeastVersion(model string, minMajor, minMinor int) bool {
	major, minor, ok := parseResponsesBridgeModelVersion(model)
	if !ok {
		return false
	}
	if major != minMajor {
		return major > minMajor
	}
	return minor >= minMinor
}

func parseResponsesBridgeModelVersion(model string) (major int, minor int, ok bool) {
	normalized := normalizeResponsesBridgeModel(model)
	if normalized == "" || !strings.HasPrefix(normalized, "gpt-") {
		return 0, 0, false
	}

	rest := strings.TrimPrefix(normalized, "gpt-")
	majorEnd := 0
	for majorEnd < len(rest) && rest[majorEnd] >= '0' && rest[majorEnd] <= '9' {
		majorEnd++
	}
	if majorEnd == 0 {
		return 0, 0, false
	}

	major, err := strconv.Atoi(rest[:majorEnd])
	if err != nil {
		return 0, 0, false
	}

	minor = 0
	if majorEnd < len(rest) && rest[majorEnd] == '.' {
		minorStart := majorEnd + 1
		minorEnd := minorStart
		for minorEnd < len(rest) && rest[minorEnd] >= '0' && rest[minorEnd] <= '9' {
			minorEnd++
		}
		if minorEnd == minorStart {
			return 0, 0, false
		}
		minor, err = strconv.Atoi(rest[minorStart:minorEnd])
		if err != nil {
			return 0, 0, false
		}
	}

	return major, minor, true
}

func normalizeResponsesBridgeModel(model string) string {
	// 模型名归一化结果供桥接能力判断使用，请求模型保持输入值。
	return strings.ToLower(LastOpenAIModelSegment(model))
}

// ResponsesBridgeDropsSampling 判断模型是否为 Responses API 下不支持 temperature/top_p 的推理模型。
// 当前所有 gpt-5.x 模型都按推理模型处理。
func ResponsesBridgeDropsSampling(model string) bool {
	return strings.HasPrefix(model, "gpt-5")
}
