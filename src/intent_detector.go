package main

import (
	"strings"
)

// Intent 检测到的意图
type Intent struct {
	Type       string  // 技能名
	Action     string  // 同上
	Confidence float64 // 置信度 0-1
}

// IntentDetector 轻量级意图检测器
type IntentDetector struct {
	tools []Tool
}

// NewIntentDetector 创建意图检测器
func NewIntentDetector(tools []Tool) *IntentDetector {
	return &IntentDetector{tools: tools}
}

// Detect 检测用户输入意图
func (d *IntentDetector) Detect(input string) (Intent, float64) {
	inputLower := strings.ToLower(input)
	inputWords := tokenizeForRAG(inputLower)

	if len(inputWords) == 0 {
		return Intent{}, 0
	}

	var bestMatch struct {
		toolName string
		score    float64
	}

	for _, tool := range d.tools {
		score := d.calculateScore(inputWords, tool)
		if score > bestMatch.score {
			bestMatch.score = score
			bestMatch.toolName = tool.Name
		}
	}

	return Intent{
		Type:   bestMatch.toolName,
		Action: bestMatch.toolName,
	}, bestMatch.score
}

// calculateScore 计算输入与工具的匹配分数
func (d *IntentDetector) calculateScore(inputWords []string, tool Tool) float64 {
	// 提取工具描述中的关键词
	descLower := strings.ToLower(tool.Description)
	descWords := tokenizeForRAG(descLower)

	// 计算重叠
	overlap := 0
	for _, iw := range inputWords {
		for _, dw := range descWords {
			if iw == dw || strings.Contains(dw, iw) || strings.Contains(iw, dw) {
				overlap++
				break
			}
		}
	}

	// 基础分数：重叠词比例
	score := float64(overlap) / float64(len(inputWords))

	// 加分项：工具名匹配
	toolNameLower := strings.ToLower(tool.Name)
	for _, word := range inputWords {
		if strings.Contains(toolNameLower, word) {
			score += 0.2
			break
		}
	}

	// 加分项：关键词匹配（从Description提取的强指示词）
	keywords := d.extractKeywords(tool.Description)
	for _, kw := range keywords {
		for _, word := range inputWords {
			if word == kw || strings.Contains(word, kw) {
				score += 0.3
				break
			}
		}
	}

	if score > 1.0 {
		score = 1.0
	}

	return score
}

// extractKeywords 从描述提取强指示关键词
func (d *IntentDetector) extractKeywords(description string) []string {
	// 简单实现：返回描述中可能的关键词
	// 可扩展为使用NLP或预定义关键词表
	var keywords []string

	// 基于技能类型的常见关键词
	if strings.Contains(description, "memory") || strings.Contains(description, "remember") {
		keywords = append(keywords, "记住", "记录", "保存", "memory", "remember", "note")
	}
	if strings.Contains(description, "read") || strings.Contains(description, "file") {
		keywords = append(keywords, "读取", "查看", "打开", "read", "view", "open", "文件")
	}
	if strings.Contains(description, "write") {
		keywords = append(keywords, "写入", "修改", "保存", "write", "edit", "save")
	}

	return keywords
}
