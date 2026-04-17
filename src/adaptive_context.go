package main

import (
	"fmt"
	"strings"

	openai "github.com/openai/openai-go/v3"
)

// AdaptiveContextManager 基于相关性智能管理上下文
type AdaptiveContextManager struct {
	MaxTokens int
	RAGMemory *RAGMemory // 使用其bm25Features方法
}

// NewAdaptiveContextManager 创建自适应上下文管理器
func NewAdaptiveContextManager(maxTokens int, rag *RAGMemory) *AdaptiveContextManager {
	return &AdaptiveContextManager{
		MaxTokens: maxTokens,
		RAGMemory: rag,
	}
}

// BuildMessages 构建消息列表，智能压缩历史
func (acm *AdaptiveContextManager) BuildMessages(
	systemPrompt string,
	userInput string,
	history []ChatMessage,
) []openai.ChatCompletionMessageParamUnion {

	// 1. 检索相关记忆
	var memoryContext string
	if acm.RAGMemory != nil {
		memories := acm.RAGMemory.Retrieve(userInput, 3)
		if len(memories) > 0 {
			var parts []string
			for _, m := range memories {
				parts = append(parts, m.Content)
			}
			memoryContext = "相关记忆:\n" + strings.Join(parts, "\n")
		}
	}

	// 2. 构建完整的消息列表（还未裁剪）
	var msgs []openai.ChatCompletionMessageParamUnion

	// System prompt（包含相关记忆）
	fullSystem := systemPrompt
	if memoryContext != "" {
		fullSystem += "\n\n" + memoryContext
	}
	msgs = append(msgs, openai.SystemMessage(fullSystem))

	// 3. 处理历史消息
	processedHistory := acm.processHistory(history, userInput)
	for _, h := range processedHistory {
		msgs = append(msgs, chatMessageToParam(h))
	}

	// 4. 当前用户输入
	msgs = append(msgs, openai.UserMessage(userInput))

	// 5. 应用token限制（必要时智能裁剪）
	return acm.applyTokenLimit(msgs)
}

// processHistory 处理历史消息，按相关性分类
func (acm *AdaptiveContextManager) processHistory(
	history []ChatMessage,
	currentInput string,
) []ChatMessage {
	if len(history) == 0 {
		return nil
	}

	// 保留最近的N条完整记录（通常是5-10轮）
	recentCount := 5
	if len(history) <= recentCount {
		return history
	}

	recent := history[len(history)-recentCount:]
	older := history[:len(history)-recentCount]

	// 计算查询向量
	queryVec := acm.RAGMemory.bm25Features(currentInput)

	// 对历史消息按相关性分类
	var relevant, other []ChatMessage
	for _, msg := range older {
		msgVec := acm.RAGMemory.bm25Features(msg.Content)
		similarity := cosineSimilarity(queryVec, msgVec)

		if similarity > 0.5 {
			relevant = append(relevant, msg)
		} else {
			other = append(other, msg)
		}
	}

	// 构建处理后的历史
	var result []ChatMessage

	// 相关历史保留完整（但限制数量）
	if len(relevant) > 10 {
		relevant = relevant[len(relevant)-10:]
	}
	result = append(result, relevant...)

	// 无关历史生成摘要
	if len(other) > 0 {
		summary := acm.summarizeMessages(other)
		result = append(result, ChatMessage{
			Role:    "system",
			Content: fmt.Sprintf("[前文摘要] %s", summary),
		})
	}

	// 添加最近完整记录
	result = append(result, recent...)

	return result
}

// summarizeMessages 摘要一批消息
func (acm *AdaptiveContextManager) summarizeMessages(msgs []ChatMessage) string {
	if len(msgs) == 0 {
		return ""
	}

	// 简单规则：提取每个user消息的前50字作为要点
	var points []string
	for _, m := range msgs {
		if m.Role == "user" {
			content := m.Content
			if len([]rune(content)) > 50 {
				content = string([]rune(content)[:50]) + "..."
			}
			points = append(points, fmt.Sprintf("问: %s", content))
		}
	}

	if len(points) == 0 {
		return fmt.Sprintf("(%d条历史消息)", len(msgs))
	}

	return strings.Join(points, "; ")
}

// applyTokenLimit 应用token限制，必要时截断
func (acm *AdaptiveContextManager) applyTokenLimit(
	msgs []openai.ChatCompletionMessageParamUnion,
) []openai.ChatCompletionMessageParamUnion {
	if acm.MaxTokens <= 0 {
		return msgs
	}

	// 转换为SimpleMsg进行估算
	simpleMsgs := paramsToSimpleMsgs(msgs)

	// 如果未超限，直接返回
	if estimateTokensSimple(simpleMsgs) <= acm.MaxTokens {
		return msgs
	}

	// 超限时，保留system和最近的消息，截断中间部分
	if len(msgs) <= 2 {
		return msgs // 太少无法裁剪
	}

	// 保留system（第1条）和最近2条
	head := msgs[0:1]
	tail := msgs[len(msgs)-2:]

	// 中间部分用一个摘要消息代替
	middleCount := len(msgs) - 3
	summary := openai.SystemMessage(
		fmt.Sprintf("(中间%d条消息因长度限制已省略)", middleCount),
	)

	result := append(head, summary)
	result = append(result, tail...)

	return result
}

// 辅助函数
func chatMessageToParam(msg ChatMessage) openai.ChatCompletionMessageParamUnion {
	switch msg.Role {
	case "system":
		return openai.SystemMessage(msg.Content)
	case "assistant":
		return openai.AssistantMessage(msg.Content)
	case "user":
		return openai.UserMessage(msg.Content)
	default:
		return openai.UserMessage(msg.Content)
	}
}

func paramsToSimpleMsgs(msgs []openai.ChatCompletionMessageParamUnion) []SimpleMsg {
	// 转换为SimpleMsg进行token估算
	var result []SimpleMsg
	for _, m := range msgs {
		result = append(result, SimpleMsg{
			Role:    classifyChatMessageRole(m),
			Content: extractContentFromParam(m),
		})
	}
	return result
}

func extractContentFromParam(m openai.ChatCompletionMessageParamUnion) string {
	// 简化处理：token估算不需要精确内容，返回空字符串即可
	// 实际实现需要更复杂的类型检查
	return ""
}
