package provider

import (
	"crypto/sha256"
	"encoding/binary"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// 绘图题面与交付要求取自 ranxi2001/sub2api 的 0591a206 版本。
	pelicanFixedPrompt      = "创建一个 HTML，内容是 SVG 绘制一个鹈鹕骑自行车的 2D 动画，你不需要任何测试"
	pelicanDeliveryContract = "所有账号使用相同交付约定：直接返回独立 HTML，不使用 Markdown 代码块或外部依赖。只输出 HTML，不要解释。"
	pelicanBankVersion      = "svg-motion-v1"
	pelicanCyclingTask      = "鹈鹕骑自行车：画出长喙和喉囊、双轮车架、脚踏与腿部联动，背景路标向后移动。"
	candyExpectedAnswer     = "21"
	candyPrompt             = `在一个黑色的袋子里放有三种口味的糖果，每种糖果有两种不同的形状（圆形和五角星形，不同的形状靠手感可以分辨）。现已知不同口味的糖和不同形状的数量统计如下表。参赛者需要在活动前决定摸出的糖果数目，那么，最少取出多少个糖果才能保证手中同时拥有不同形状的苹果味和桃子味的糖？（同时手中有圆形苹果味匹配五角星桃子味糖果，或者有圆形桃子味匹配五角星苹果味糖果都满足要求）
苹果味 桃子味 西瓜味
圆形 7 9 8
五角星形 7 6 4

只输出最终整数。`
)

var (
	// 同一轮的所有提供商按名义轮次选择题目，任务延迟和重试不会换题。
	pelicanBank = [...]struct {
		task     string
		criteria [4]string
	}{
		{
			task: pelicanCyclingTask,
			criteria: [4]string{
				"长喙与喉囊清晰可辨",
				"两轮、车架及脚踏结构完整",
				"车轮转动与双腿踩踏连续协调",
				"路标向后移动，角色保持在画面内",
			},
		},
		{
			task: "企鹅骑自行车：画出黑白身体和短喙、双轮车架、脚踏与双脚联动，背景冰块向后移动。",
			criteria: [4]string{
				"黑白身体与短喙清晰可辨",
				"两轮、车架及脚踏结构完整",
				"车轮转动与双脚踩踏连续协调",
				"冰块向后移动，角色保持在画面内",
			},
		},
		{
			task: "兔子骑双轮滑板车：画出长耳、踏板和车把，一脚站立、另一脚蹬地后收回，背景草丛向后移动。",
			criteria: [4]string{
				"长耳与兔子轮廓清晰可辨",
				"两轮、踏板及车把结构完整",
				"车轮转动与单腿蹬地收回连续协调",
				"草丛向后移动，角色保持在画面内",
			},
		},
		{
			task: "水獭骑双轮滑板车：画出圆耳和长尾、踏板和车把，一脚站立、另一脚蹬地后收回，背景石块向后移动。",
			criteria: [4]string{
				"圆耳与长尾清晰可辨",
				"两轮、踏板及车把结构完整",
				"车轮转动与单腿蹬地收回连续协调",
				"石块向后移动，角色保持在画面内",
			},
		},
	}

	candyFinalAnswer = regexp.MustCompile(`^(?:(?:最终)?(?:答案|结果)(?:为|是)?[:：]?|(?:最少|至少)(?:需要)?(?:取出|抽取|摸出)?|(?:需要|取出|抽取|摸出))?(-?[0-9]+(?:\.[0-9]+)?|[零〇一二三四五六七八九十百千万]+)(?:个|颗|粒)?(?:糖果|糖)?(?:即可)?[。.!！]?$|^(?:(?:the)?answer(?:is|:)?|atleast)?(-?[0-9]+(?:\.[0-9]+)?)(?:candies)?[.!]?$`)
	candySplitNumber = regexp.MustCompile(`[0-9０-９][\s\p{Zs}*_]+[0-9０-９]`)
)

// PelicanQuestionDefault 是管理端创建计划时可以直接编辑的内置题。
type PelicanQuestionDefault struct {
	QuestionKind   string `json:"question_kind"`
	QuestionName   string `json:"question_name"`
	QuestionPrompt string `json:"question_prompt"`
	ExpectedAnswer string `json:"expected_answer"`
}

// PelicanQuestionDefaults 返回固定的鹈鹕骑车题和糖果原题，每次调用使用独立的切片。
func PelicanQuestionDefaults() []PelicanQuestionDefault {
	return []PelicanQuestionDefault{
		{QuestionKind: "pelican", QuestionName: "鹈鹕 SVG", QuestionPrompt: pelicanFixedPrompt},
		{QuestionKind: "candy", QuestionName: "糖果题", QuestionPrompt: candyPrompt, ExpectedAnswer: candyExpectedAnswer},
	}
}

// IsStandardPelicanCandy 识别允许自动停调的题面和答案。
func IsStandardPelicanCandy(prompt, answer string) bool {
	return (prompt == "" || prompt == candyPrompt) && usesStandardCandyAnswer("candy", answer)
}

func pelicanQuestion(kind, round string, config PelicanQuestionConfig) PelicanEvidence {
	result := PelicanEvidence{Status: "inconclusive", DetailCode: "upstream_error"}
	// 糖果题的题面固定，定时任务可以省略轮次。
	if len(round) > 512 || (round == "" && kind != "candy") {
		return result
	}
	prompt := strings.TrimSpace(config.Prompt)
	expected := strings.TrimSpace(config.ExpectedAnswer)
	if (prompt != "" && !validPelicanQuestionText(prompt, 8192)) || (expected != "" && !validPelicanQuestionText(expected, 1024)) {
		return result
	}
	switch kind {
	case "pelican", "custom_svg":
		if expected != "" || (kind == "custom_svg" && prompt == "") {
			return result
		}
		if prompt == "" {
			// 旧计划没有保存题面，同一轮仍按原版本的题库选题。
			hash := sha256.Sum256([]byte(pelicanBankVersion + ":" + round))
			task := pelicanBank[binary.BigEndian.Uint64(hash[:8])%uint64(len(pelicanBank))]
			prompt = pelicanSVGPrompt(task.task)
		}
		result.Prompt = prompt + "\n\n" + pelicanDeliveryContract
		result.Criteria = pelicanVisualCriteria(prompt)
	case "candy", "custom_text":
		if kind == "candy" && prompt == "" {
			prompt = candyPrompt
		}
		if prompt == "" || (kind == "custom_text" && expected == "") {
			return result
		}
		result.Prompt = prompt
		if usesStandardCandyAnswer(kind, expected) {
			result.Criteria = []string{"按最终答案 21 判分，接受等价的中文数字、常用答案前缀和糖果单位。"}
		} else {
			result.Criteria = []string{"去掉回答首尾空白后，逐字比对参考答案。", "参考答案：" + expected}
		}
	}
	return result
}

// pelicanSVGPrompt 拼接按轮次抽取的动画题。
func pelicanSVGPrompt(task string) string {
	return "创建一个独立 HTML，使用内联 SVG 绘制循环的 2D 动画。" + task + "不得使用外部资源。"
}

// pelicanVisualCriteria 仅在题面与题库原文相同时附上该场景的评判要求。
func pelicanVisualCriteria(prompt string) []string {
	if prompt == pelicanFixedPrompt {
		return append([]string(nil), pelicanBank[0].criteria[:]...)
	}
	for _, question := range pelicanBank {
		if prompt == pelicanSVGPrompt(question.task) {
			return append([]string(nil), question.criteria[:]...)
		}
	}
	return []string{"对照题面，复核图中要求的对象、细节和动作。"}
}

// usesStandardCandyAnswer 保留原题对中文数字、答案前缀和单位的兼容。
func usesStandardCandyAnswer(kind, expected string) bool {
	return kind == "candy" && (expected == "" || expected == candyExpectedAnswer)
}

// validPelicanQuestionText 允许题面和答案使用换行和制表符。
func validPelicanQuestionText(text string, limit int) bool {
	if text == "" || len(text) > limit || !utf8.ValidString(text) {
		return false
	}
	return strings.IndexFunc(text, func(r rune) bool {
		return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t'
	}) < 0
}

// candyVerdict 读取最终数值。含网址、上游说明或多个候选数值的文字无法确认答案。
func candyVerdict(text string) (string, string) {
	if len(text) > 512 {
		return "inconclusive", "output_limit"
	}
	if candySplitNumber.MatchString(text) {
		return "inconclusive", "incomplete"
	}
	normalized := strings.Map(func(r rune) rune {
		switch {
		case unicode.IsSpace(r) || strings.ContainsRune("*_\x60\"'“”", r):
			return -1
		case r >= '０' && r <= '９':
			return r - '０' + '0'
		default:
			return unicode.ToLower(r)
		}
	}, text)
	matches := candyFinalAnswer.FindStringSubmatch(normalized)
	if matches == nil {
		return "inconclusive", "incomplete"
	}
	if matches[1] == candyExpectedAnswer || matches[1] == "二十一" || matches[2] == candyExpectedAnswer {
		return "passed", ""
	}
	return "suspected", "answer_mismatch"
}
