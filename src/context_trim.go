package main

import (
	"encoding/json"
	"log"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
)

const contextTruncatedSuffix = "\n…(truncated)"

// contextTokenSlackFromEnv 返回从输入预算中扣减的余量，用于吸收 Qwen/llama.cpp 与 cl100k JSON 估算的偏差。
// 可通过 AGENT_CONTEXT_TOKEN_SLACK 覆盖；未设置时默认 min(512, max(256, effectiveLimit/10))，且保证小于 effectiveLimit。
func contextTokenSlackFromEnv(effectiveLimit int) int {
	if effectiveLimit <= 0 {
		return 0
	}
	if v := envIntOrDefault("AGENT_CONTEXT_TOKEN_SLACK", 0); v > 0 {
		if v >= effectiveLimit {
			return maxInt(1, effectiveLimit/4)
		}
		return v
	}
	s := effectiveLimit / 10
	s = minInt(512, s)
	s = maxInt(256, s)
	if s >= effectiveLimit {
		return maxInt(1, effectiveLimit/4)
	}
	return s
}

// inputBudgetAfterSlack 为发送 Chat Completions 前使用的实际上限（估算须 ≤ 该值）。
func inputBudgetAfterSlack(effectiveLimit int) int {
	if effectiveLimit <= 0 {
		return 0
	}
	s := contextTokenSlackFromEnv(effectiveLimit)
	if s >= effectiveLimit {
		return maxInt(1, effectiveLimit/4)
	}
	return effectiveLimit - s
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// trimOpenAIMessagesToBudget 在保留头尾各 headKeep/tailKeep 条的前提下，删除或截断中间内容，使 estimateChatRequestInputTokens ≤ limit。
func trimOpenAIMessagesToBudget(
	messages []openai.ChatCompletionMessageParamUnion,
	tools []openai.ChatCompletionToolUnionParam,
	limit int,
	headKeep, tailKeep int,
) []openai.ChatCompletionMessageParamUnion {
	if limit <= 0 || len(messages) == 0 {
		return messages
	}
	out := append([]openai.ChatCompletionMessageParamUnion(nil), messages...)
	headKeep = maxInt(0, headKeep)
	tailKeep = maxInt(0, tailKeep)
	for pass := 0; pass < 5000; pass++ {
		_, _, est := estimateChatRequestInputTokens(out, tools)
		if est <= limit {
			return out
		}
		if headKeep+tailKeep > len(out) {
			out = aggressiveTruncateOpenAIMessages(out, tools, limit, headKeep, tailKeep)
			return out
		}
		remStart := headKeep
		remEnd := len(out) - tailKeep
		if remStart >= remEnd {
			out = aggressiveTruncateOpenAIMessages(out, tools, limit, headKeep, tailKeep)
			return out
		}
		seg := segmentSizeAt(out, remStart)
		if seg <= 0 {
			seg = 1
		}
		if remStart+seg > remEnd {
			out = aggressiveTruncateOpenAIMessages(out, tools, limit, headKeep, tailKeep)
			return out
		}
		out = append(out[:remStart], out[remStart+seg:]...)
	}
	log.Printf("warn: trimOpenAIMessagesToBudget stopped after max passes, est may exceed limit")
	return out
}

// segmentSizeAt 返回从索引 i 起的一条可整体删除的语义段长度（assistant+tool_calls 与其后连续的 tool 为一组）。
func segmentSizeAt(messages []openai.ChatCompletionMessageParamUnion, i int) int {
	if i < 0 || i >= len(messages) {
		return 0
	}
	m := messages[i]
	if classifyChatMessageRole(m) == "assistant" {
		tc := m.GetToolCalls()
		if len(tc) > 0 {
			j := i + 1
			got := 0
			for j < len(messages) && got < len(tc) {
				if classifyChatMessageRole(messages[j]) != "tool" {
					break
				}
				got++
				j++
			}
			return 1 + got
		}
	}
	return 1
}

func aggressiveTruncateOpenAIMessages(
	messages []openai.ChatCompletionMessageParamUnion,
	tools []openai.ChatCompletionToolUnionParam,
	limit int,
	headKeep, tailKeep int,
) []openai.ChatCompletionMessageParamUnion {
	out := append([]openai.ChatCompletionMessageParamUnion(nil), messages...)
	for iter := 0; iter < 256; iter++ {
		_, _, est := estimateChatRequestInputTokens(out, tools)
		if est <= limit {
			return out
		}
		remStart := headKeep
		remEnd := len(out) - tailKeep
		if remStart >= remEnd {
			out = truncateProtectedTailUser(out, tools, limit, tailKeep)
			return out
		}
		maxIdx := -1
		maxLen := 0
		for i := remStart; i < remEnd; i++ {
			b, err := json.Marshal(out[i])
			if err != nil {
				continue
			}
			if len(b) > maxLen {
				maxLen = len(b)
				maxIdx = i
			}
		}
		if maxIdx < 0 || maxLen < 64 {
			out = truncateProtectedTailUser(out, tools, limit, tailKeep)
			return out
		}
		newRunes := maxInt(256, maxRunesFromJSONLen(maxLen)*3/4)
		out[maxIdx] = truncateMessageUnionByRunes(out[maxIdx], newRunes, true)
	}
	return out
}

func maxRunesFromJSONLen(n int) int {
	if n <= 0 {
		return 0
	}
	return n / 4
}

// truncateProtectedTailUser 在可删区间为空时，截断末尾受保护区域内的 user（含去掉多模态图片）。
func truncateProtectedTailUser(
	messages []openai.ChatCompletionMessageParamUnion,
	tools []openai.ChatCompletionToolUnionParam,
	limit int,
	tailKeep int,
) []openai.ChatCompletionMessageParamUnion {
	out := append([]openai.ChatCompletionMessageParamUnion(nil), messages...)
	if tailKeep <= 0 || len(out) == 0 {
		return out
	}
	for iter := 0; iter < 128; iter++ {
		_, _, est := estimateChatRequestInputTokens(out, tools)
		if est <= limit {
			return out
		}
		tailStart := len(out) - tailKeep
		if tailStart < 0 {
			tailStart = 0
		}
		maxIdx := -1
		maxLen := 0
		for i := tailStart; i < len(out); i++ {
			b, err := json.Marshal(out[i])
			if err != nil {
				continue
			}
			if len(b) > maxLen {
				maxLen = len(b)
				maxIdx = i
			}
		}
		if maxIdx < 0 {
			break
		}
		newRunes := maxInt(128, maxRunesFromJSONLen(maxLen)*2/3)
		out[maxIdx] = truncateMessageUnionByRunes(out[maxIdx], newRunes, true)
	}
	return out
}

// truncateMessageUnionByRunes 按 rune 截断可截断的文本；stripImagesInUser 为 true 时去掉 user 多模态中的图片 part。
func truncateMessageUnionByRunes(m openai.ChatCompletionMessageParamUnion, maxRunes int, stripImagesInUser bool) openai.ChatCompletionMessageParamUnion {
	if maxRunes <= 0 {
		maxRunes = 1
	}
	switch classifyChatMessageRole(m) {
	case "tool":
		if m.OfTool == nil {
			return m
		}
		t := *m.OfTool
		if t.Content.OfString.Valid() {
			t.Content.OfString = param.NewOpt(truncateRunesWithSuffix(t.Content.OfString.Value, maxRunes))
			return openai.ChatCompletionMessageParamUnion{OfTool: &t}
		}
		return m
	case "assistant":
		if m.OfAssistant == nil {
			return m
		}
		a := *m.OfAssistant
		if len(a.ToolCalls) > 0 {
			return m
		}
		if a.Content.OfString.Valid() {
			a.Content.OfString = param.NewOpt(truncateRunesWithSuffix(a.Content.OfString.Value, maxRunes))
			return openai.ChatCompletionMessageParamUnion{OfAssistant: &a}
		}
		return m
	case "user":
		if m.OfUser == nil {
			return m
		}
		u := *m.OfUser
		if u.Content.OfString.Valid() {
			u.Content.OfString = param.NewOpt(truncateRunesWithSuffix(u.Content.OfString.Value, maxRunes))
			return openai.ChatCompletionMessageParamUnion{OfUser: &u}
		}
		if stripImagesInUser && len(u.Content.OfArrayOfContentParts) > 0 {
			var textOnly []openai.ChatCompletionContentPartUnionParam
			for _, p := range u.Content.OfArrayOfContentParts {
				if p.OfText != nil && stringsTrimNotEmpty(p.OfText.Text) {
					textOnly = append(textOnly, openai.TextContentPart(p.OfText.Text))
				}
			}
			if len(textOnly) == 0 {
				textOnly = []openai.ChatCompletionContentPartUnionParam{
					openai.TextContentPart("（图片因长度限制已省略）"),
				}
			}
			u.Content.OfArrayOfContentParts = textOnly
			return openai.ChatCompletionMessageParamUnion{OfUser: &u}
		}
		return m
	default:
		return m
	}
}

func truncateRunesWithSuffix(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return contextTruncatedSuffix
	}
	rs := []rune(s)
	suf := []rune(contextTruncatedSuffix)
	if len(rs) <= maxRunes {
		return s
	}
	if maxRunes <= len(suf) {
		return string(rs[:maxRunes])
	}
	return string(rs[:maxRunes-len(suf)]) + contextTruncatedSuffix
}

// assistantMessageParamFromCompletion 将 API 返回的 assistant 消息转为下一轮请求的 param union（含 tool_calls）。
func assistantMessageParamFromCompletion(msg openai.ChatCompletionMessage) openai.ChatCompletionMessageParamUnion {
	p := msg.ToAssistantMessageParam()
	return openai.ChatCompletionMessageParamUnion{OfAssistant: &p}
}

// headKeepCountForAgentMessages 与 HandleWithSessionWithMedia 中 simpleMsgsToOpenAI 的构造顺序一致：system + user profile。
func headKeepCountForAgentMessages(systemContent, userProfile string) int {
	n := 0
	if stringsTrimNotEmpty(systemContent) {
		n++
	}
	if stringsTrimNotEmpty(userProfile) {
		n++
	}
	return n
}

func stringsTrimNotEmpty(s string) bool {
	for _, r := range s {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
			return true
		}
	}
	return false
}
