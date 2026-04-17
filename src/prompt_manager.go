package main

import (
	"fmt"
	"strings"
)

// PromptManager 动态组装system prompt
type PromptManager struct {
	basePrompt string
}

// NewPromptManager 创建Prompt管理器
func NewPromptManager(basePrompt string) *PromptManager {
	return &PromptManager{
		basePrompt: basePrompt,
	}
}

// BuildSystemPrompt 构建完整的system prompt
func (pm *PromptManager) BuildSystemPrompt(tools []Tool, memory string) string {
	return pm.BuildSystemPromptWithWorkspaceContext(tools, memory, "", "")
}

// BuildSystemPromptWithWorkspaceContext 构建完整 system prompt，
// 并将 workspace 中的 AGENT.md / USER.md 以权威上下文注入高优先级指令层。
func (pm *PromptManager) BuildSystemPromptWithWorkspaceContext(
	tools []Tool,
	memory string,
	agentPolicy string,
	userProfile string,
) string {
	var b strings.Builder

	// 基础系统提示
	b.WriteString(pm.basePrompt)
	b.WriteString("\n\n")

	// 可用工具列表（包含详细描述和示例）
	b.WriteString("## Available Tools\n")
	b.WriteString("You have access to the following tools. Use them when appropriate based on their descriptions.\n\n")

	for _, tool := range tools {
		b.WriteString(fmt.Sprintf("### %s\n", tool.Name))
		b.WriteString(tool.Description)
		b.WriteString("\n\n")
	}

	b.WriteString("When you need to use a tool, respond with a tool_calls block.\n")
	b.WriteString("When you receive tool results, incorporate them into your final response.\n\n")

	// 长期记忆（如果有）
	if memory != "" {
		b.WriteString("## Long-term Memory\n")
		b.WriteString("The following information has been remembered from previous conversations:\n")
		b.WriteString(memory)
		b.WriteString("\n\n")
	}

	if strings.TrimSpace(agentPolicy) != "" {
		b.WriteString("## Workspace Agent Policy (AGENT.md, authoritative)\n")
		b.WriteString(agentPolicy)
		b.WriteString("\n\n")
	}

	if strings.TrimSpace(userProfile) != "" {
		b.WriteString("## Workspace User Profile (USER.md, authoritative)\n")
		b.WriteString(userProfile)
		b.WriteString("\n\n")
	}

	b.WriteString("## Guidelines\n")
	b.WriteString("- Use tools proactively when the user's request clearly matches a tool's purpose\n")
	b.WriteString("- Do not ask permission to use tools, just use them when appropriate\n")
	b.WriteString("- Always provide helpful responses based on tool results\n")
	b.WriteString("- USER.md is authoritative for user identity, locale, preferences, and profile facts\n")
	b.WriteString("- For questions like \"who am I\" or \"my profile/preferences\", answer from USER.md first\n")
	b.WriteString("- Do not claim memory loss if USER.md already provides the requested user fact\n")

	return b.String()
}

// ExtractToolDescriptions 从skills提取工具描述（包含Examples）
func (pm *PromptManager) ExtractToolDescriptions(registry *SkillRegistry) []Tool {
	if registry == nil {
		return nil
	}

	tools, _ := registry.ToTools()
	return tools
}

// BuildSystemPromptWithRAG 使用RAG检索相关记忆构建系统提示
func (pm *PromptManager) BuildSystemPromptWithRAG(
	tools []Tool,
	ragMemory *RAGMemory,
	query string,
) string {
	return pm.BuildSystemPromptWithRAGAndWorkspace(tools, ragMemory, query, "", "")
}

// BuildSystemPromptWithRAGAndWorkspace 在 RAG 记忆基础上追加 AGENT.md / USER.md 权威上下文。
func (pm *PromptManager) BuildSystemPromptWithRAGAndWorkspace(
	tools []Tool,
	ragMemory *RAGMemory,
	query string,
	agentPolicy string,
	userProfile string,
) string {
	var memoryContent string
	if ragMemory != nil {
		// 检索与当前查询相关的记忆
		relevantMemories := ragMemory.Retrieve(query, 5)
		if len(relevantMemories) > 0 {
			var parts []string
			for _, m := range relevantMemories {
				parts = append(parts, fmt.Sprintf("- %s", m.Content))
			}
			memoryContent = strings.Join(parts, "\n")
		}
	}

	return pm.BuildSystemPromptWithWorkspaceContext(tools, memoryContent, agentPolicy, userProfile)
}
