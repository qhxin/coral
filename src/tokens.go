package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	tiktoken "github.com/pkoukk/tiktoken-go"
	openai "github.com/openai/openai-go/v3"
)

// SimpleMsg 用于在做 token 估算和裁剪时的轻量消息结构。
type SimpleMsg struct {
	Role    string
	Content string
	// ImageCount 仅本轮末尾 user 可能 >0，用于 vision 输入的保守 token 估算。
	ImageCount int
}

var (
	encOnce sync.Once
	enc     *tiktoken.Tiktoken
	encErr  error
)

// getEncoder 返回全局复用的 tokenizer。
// 目前默认使用 cl100k_base 编码，足以覆盖大多数兼容模型。
func getEncoder() (*tiktoken.Tiktoken, error) {
	encOnce.Do(func() {
		enc, encErr = tiktoken.GetEncoding("cl100k_base")
	})
	return enc, encErr
}

// estimateTokensSimple 使用 tiktoken 对一组 SimpleMsg 精确估算 token 数。
func estimateTokensJSONChunks(parts [][]byte) int {
	enc, err := getEncoder()
	if err != nil {
		n := 0
		for _, p := range parts {
			n += len(p) / 4
		}
		if n < 1 && len(parts) > 0 {
			return 1
		}
		return n
	}
	total := 0
	for _, p := range parts {
		ids := enc.Encode(string(p), nil, nil)
		total += len(ids)
	}
	return total
}

// estimateChatRequestInputTokens 用请求 JSON（与 cl100k_base）估算消息与 tools 的 token，供日志使用。
func estimateChatRequestInputTokens(messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolUnionParam) (msgTok int, toolTok int, sum int) {
	msgParts := make([][]byte, 0, len(messages))
	for _, m := range messages {
		b, err := json.Marshal(m)
		if err != nil {
			continue
		}
		msgParts = append(msgParts, b)
	}
	msgTok = estimateTokensJSONChunks(msgParts)
	toolParts := make([][]byte, 0, len(tools))
	for _, t := range tools {
		b, err := json.Marshal(t)
		if err != nil {
			continue
		}
		toolParts = append(toolParts, b)
	}
	toolTok = estimateTokensJSONChunks(toolParts)
	return msgTok, toolTok, msgTok + toolTok
}

func classifyChatMessageRole(m openai.ChatCompletionMessageParamUnion) string {
	switch {
	case m.OfDeveloper != nil:
		return "developer"
	case m.OfSystem != nil:
		return "system"
	case m.OfUser != nil:
		return "user"
	case m.OfAssistant != nil:
		return "assistant"
	case m.OfTool != nil:
		return "tool"
	case m.OfFunction != nil:
		return "function"
	default:
		return "unknown"
	}
}

// chatMessagesRoleSummary 统计各 role 条数，如 system:1,user:3,assistant:2。
func chatMessagesRoleSummary(messages []openai.ChatCompletionMessageParamUnion) string {
	order := []string{"developer", "system", "user", "assistant", "tool", "function", "unknown"}
	counts := make(map[string]int)
	for _, m := range messages {
		counts[classifyChatMessageRole(m)]++
	}
	var parts []string
	for _, role := range order {
		if n := counts[role]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s:%d", role, n))
		}
	}
	return strings.Join(parts, ",")
}

func estimateTokensSimple(msgs []SimpleMsg) int {
	extraVision := 0
	for _, m := range msgs {
		if m.ImageCount > 0 {
			extraVision += m.ImageCount * visionTokensPerImage()
		}
	}
	encoder, err := getEncoder()
	if err != nil {
		// 如果 tokenizer 初始化失败，记录警告日志并退化为保守估计。
		log.Printf("warn: tiktoken encoder init failed, fallback to rough token estimate: %v", err)
		return len(msgs)*50 + extraVision
	}
	total := 0
	for _, m := range msgs {
		text := m.Role + ":" + m.Content
		ids := encoder.Encode(text, nil, nil)
		total += len(ids)
	}
	return total + extraVision
}

// ensureContextWithinLimitSimple 使用滚动摘要 reduce 的方式在给定上限下压缩 SimpleMsg。
// 不直接删除历史，只通过多层摘要折叠到 bounded 窗口内。
func ensureContextWithinLimitSimple(msgs []SimpleMsg, maxTokens int, agent *AgentCore) []SimpleMsg {
	if maxTokens <= 0 || agent == nil {
		return msgs
	}
	compressed, err := reduceHistory(msgs, maxTokens, agent)
	if err != nil {
		// 失败时记录错误并回退为原始消息，避免影响主流程。
		log.Printf("error: reduceHistory failed, fallback to original messages: %v", err)
		return msgs
	}
	return compressed
}

// reduceHistory 对按时间排序的 simpleMsgs 做多层滚动摘要，确保整体 token 数不超过 windowLimit。
// 始终保留最后一条消息（通常为当前用户输入，含多模态配额），避免摘要吃掉当前提问或图片占位。
func reduceHistory(simpleMsgs []SimpleMsg, windowLimit int, agent *AgentCore) ([]SimpleMsg, error) {
	if windowLimit <= 0 || len(simpleMsgs) == 0 || agent == nil || agent.Client == nil {
		return simpleMsgs, nil
	}

	tail := simpleMsgs[len(simpleMsgs)-1]
	prefix := simpleMsgs[:len(simpleMsgs)-1]

	for level := 0; level < 4; level++ {
		cur := append(append([]SimpleMsg{}, prefix...), tail)
		if estimateTokensSimple(cur) <= windowLimit {
			return cur, nil
		}
		if len(prefix) == 0 {
			return cur, nil
		}
		chunks := splitIntoChunks(prefix, windowLimit)
		summaries := make([]SimpleMsg, 0, len(chunks))
		for _, chunk := range chunks {
			s, err := summarizeSimpleChunkWithLLM(agent, chunk)
			if err != nil {
				log.Printf("error: summarizeSimpleChunkWithLLM failed at level %d: %v", level, err)
				return cur, err
			}
			if strings.TrimSpace(s.Content) == "" {
				summaries = append(summaries, chunk...)
			} else {
				summaries = append(summaries, s)
			}
		}
		if len(summaries) == 0 {
			break
		}
		prefix = summaries
	}
	return append(append([]SimpleMsg{}, prefix...), tail), nil
}

// splitIntoChunks 将消息按 windowLimit 切分为若干分块，保证每块估算 token 不超过 windowLimit（单条超长除外）。
func splitIntoChunks(msgs []SimpleMsg, windowLimit int) [][]SimpleMsg {
	if len(msgs) == 0 {
		return nil
	}
	var chunks [][]SimpleMsg
	var current []SimpleMsg
	for _, m := range msgs {
		withNew := append(append([]SimpleMsg{}, current...), m)
		if len(current) > 0 && estimateTokensSimple(withNew) > windowLimit {
			// 当前块已接近上限，先收束为一个 chunk。
			chunks = append(chunks, current)
			current = []SimpleMsg{m}
			// 如果单条也超过上限，仍然作为一个独立块交给摘要处理。
			if estimateTokensSimple(current) > windowLimit {
				chunks = append(chunks, current)
				current = nil
			}
		} else {
			current = withNew
		}
	}
	if len(current) > 0 {
		chunks = append(chunks, current)
	}
	return chunks
}

// summarizeSimpleChunkWithLLM 使用当前模型对一段 SimpleMsg 历史生成简短摘要。
func summarizeSimpleChunkWithLLM(agent *AgentCore, chunk []SimpleMsg) (SimpleMsg, error) {
	if len(chunk) == 0 || agent == nil || agent.Client == nil {
		return SimpleMsg{}, nil
	}
	var historyText strings.Builder
	for _, m := range chunk {
		switch m.Role {
		case "user":
			fmt.Fprintf(&historyText, "用户: %s\n", m.Content)
		case "assistant":
			fmt.Fprintf(&historyText, "助手: %s\n", m.Content)
		case "system":
			fmt.Fprintf(&historyText, "系统: %s\n", m.Content)
		default:
			fmt.Fprintf(&historyText, "%s: %s\n", m.Role, m.Content)
		}
	}

	sys := openai.SystemMessage("你是一个会话总结助手，请用简短中文总结以下对话的要点，突出长期偏好、重要结论和需要跨轮次记住的信息。尽量精炼，控制在200字以内。")
	user := openai.UserMessage(historyText.String())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 摘要本身也使用较小的输出上限（例如 256），不必占用过多 token。
	llmMeta := newLLMRequestLogMetaFromAgent(agent, "history_reduce_chunk", 256)
	resp, err := agent.Client.ChatOnce(ctx, []openai.ChatCompletionMessageParamUnion{sys, user}, nil, 256, "", "", llmMeta)
	if err != nil {
		return SimpleMsg{}, err
	}
	if len(resp.Choices) == 0 {
		return SimpleMsg{}, nil
	}
	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	return SimpleMsg{
		Role:    "system",
		Content: content,
	}, nil
}

