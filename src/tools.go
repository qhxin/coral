package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	openai "github.com/openai/openai-go/v3"
)

// Tool 描述一个暴露给模型使用的工具（对齐 OpenAI tools 语义）。
type Tool struct {
	Name                 string
	Description          string
	ParametersJSONSchema string
}

// ToolResult 表示一次工具调用的结果。
type ToolResult struct {
	CallID  string
	Name    string
	Content string
	Error   string
}

// ToolExecutor 是具体工具执行函数的签名。
type ToolExecutor func(args json.RawMessage) (string, error)

// defaultFilesystemTools 构造基于 WorkspaceFS 的默认文件系统工具集合。
// 注意: 默认启用技能注册表，此函数仅作为 fallback 保留（当CORAL_USE_SKILL_REGISTRY=false时）。
// 技能注册表（SkillRegistry）提供了更灵活的 Markdown 配置驱动方式来定义工具。
func defaultFilesystemTools(fs *WorkspaceFS) ([]Tool, map[string]ToolExecutor) {
	tools := []Tool{
		{
			Name:                 "workspace_read_file",
			Description:          "读取 workspace 目录下的相对路径文件内容。",
			ParametersJSONSchema: `{"type":"object","properties":{"path":{"type":"string","description":"要读取的相对路径，例如 AGENT.md"}},"required":["path"]}`,
		},
		{
			Name:                 "workspace_write_file",
			Description:          "覆盖写入 workspace 目录下的相对路径文件内容。",
			ParametersJSONSchema: `{"type":"object","properties":{"path":{"type":"string","description":"要写入的相对路径，例如 NOTES.md"},"content":{"type":"string","description":"要写入文件的完整文本内容"}},"required":["path","content"]}`,
		},
		{
			Name:                 "memory_write_important",
			Description:          "记录需要长期记住的重要信息到 MEMORY.md。",
			ParametersJSONSchema: `{"type":"object","properties":{"content":{"type":"string","description":"需要长期记住的重要信息"}},"required":["content"]}`,
		},
	}

	executors := map[string]ToolExecutor{
		"workspace_read_file": func(args json.RawMessage) (string, error) {
			var payload struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(args, &payload); err != nil {
				return "", fmt.Errorf("解析参数失败: %w。请重新调用 workspace_read_file，并传入 arguments：{\"path\":\"AGENT.md\"}", err)
			}
			if payload.Path == "" {
				return "", fmt.Errorf("path 不能为空。请重新调用 workspace_read_file，并传入 arguments：{\"path\":\"AGENT.md\"}")
			}
			return fs.Read(payload.Path)
		},
		"workspace_write_file": func(args json.RawMessage) (string, error) {
			var payload struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal(args, &payload); err != nil {
				return "", fmt.Errorf("解析参数失败: %w。请重新调用 workspace_write_file，并传入 arguments：{\"path\":\"example.txt\",\"content\":\"example\"}", err)
			}
			if payload.Path == "" {
				return "", fmt.Errorf("path 不能为空（你可能传入了空对象 {}）。请重新调用 workspace_write_file，并传入 arguments：{\"path\":\"example.txt\",\"content\":\"example\"}")
			}
			if err := fs.Write(payload.Path, payload.Content); err != nil {
				return "", fmt.Errorf("写入失败: %w。请确认 path 是 workspace 内相对路径，例如 arguments：{\"path\":\"example.txt\",\"content\":\"example\"}", err)
			}
			return "写入成功", nil
		},
		"memory_write_important": func(args json.RawMessage) (string, error) {
			var payload struct {
				Content string `json:"content"`
			}
			if err := json.Unmarshal(args, &payload); err != nil {
				return "", fmt.Errorf("解析参数失败: %w。请重新调用 memory_write_important，并传入 arguments：{\"content\":\"example\"}", err)
			}
			if strings.TrimSpace(payload.Content) == "" {
				return "", fmt.Errorf("content 不能为空（你可能传入了空对象 {}）。请重新调用 memory_write_important，并传入 arguments：{\"content\":\"example\"}")
			}
			now := Now()
			entry := fmt.Sprintf("\n\n## Memo at %s\n%s\n", now.Format(time.RFC3339), payload.Content)
			if err := fs.Append("MEMORY.md", entry); err != nil {
				return "", fmt.Errorf("写入 MEMORY.md 失败: %w。请确认内容为纯文本，并重试调用 memory_write_important", err)
			}
			return "写入 MEMORY.md 成功", nil
		},
	}

	return tools, executors
}

// dispatchToolsOpenAI 按名称分发并执行 OpenAI tool_calls。
func dispatchToolsOpenAI(calls []openai.ChatCompletionMessageToolCallUnion, executors map[string]ToolExecutor) []ToolResult {
	results := make([]ToolResult, 0, len(calls))
	for _, c := range calls {
		fn := c.AsFunction()
		name := fn.Function.Name
		exec, ok := executors[name]
		if !ok {
			results = append(results, ToolResult{
				CallID:  fn.ID,
				Name:    name,
				Content: "",
				Error:   fmt.Sprintf("未知工具: %s", name),
			})
			continue
		}

		out, err := exec(json.RawMessage(fn.Function.Arguments))
		res := ToolResult{
			CallID:  fn.ID,
			Name:    name,
			Content: out,
		}
		if err != nil {
			res.Error = err.Error()
		}
		results = append(results, res)
	}
	return results
}

// shouldForceMemoryTool 判断是否需要强制调用 memory_write_important。
// 已废弃: 当CORAL_USE_PROMPT_FIRST默认启用时，此函数不会被调用
// 工具调用决策由模型通过system prompt自主完成
// Deprecated: Use prompt-based tool guidance instead
func shouldForceMemoryTool(userInput string) bool {
	s := strings.TrimSpace(userInput)
	if s == "" {
		return false
	}
	// 关键词规则：用户明确要求“长期/永远/永久记住”等，应强制调用 memory_write_important。
	keywords := []string{
		"长期记住",
		"永久记住",
		"永远记住",
		"写入长期记忆",
		"写入记忆",
		"记录长期记忆",
		"记到长期记忆",
		"记入长期记忆",
	}
	for _, k := range keywords {
		if strings.Contains(s, k) {
			return true
		}
	}
	// 更宽松的组合判断：同时包含“记住/记录”和“原则/偏好/信息/事项”也算。
	if (strings.Contains(s, "记住") || strings.Contains(s, "记录")) &&
		(strings.Contains(s, "原则") || strings.Contains(s, "偏好") || strings.Contains(s, "信息") || strings.Contains(s, "事项")) {
		return true
	}
	return false
}
