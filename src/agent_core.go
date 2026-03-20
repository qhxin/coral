package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	openai "github.com/openai/openai-go/v3"
)

// AgentCore 使用 OpenAI JSON 协议管理系统提示和对话历史。
type AgentCore struct {
	Client            *OpenAIClient
	FS                *WorkspaceFS
	Tools             []Tool
	Executors         map[string]ToolExecutor
	SystemContent     string
	UserProfile       string
	MaxContextTokens  int
	MaxOutputTokens   int
	SummaryWindowDays int
}

// NewAgentCore 创建一个新的 AgentCore，由外部注入 OpenAIClient 与初始上下文。
func NewAgentCore(client *OpenAIClient, systemContent, userProfile string, fs *WorkspaceFS, maxContextTokens, maxOutputTokens int) *AgentCore {
	var tools []Tool
	executors := make(map[string]ToolExecutor)
	if fs != nil {
		tools, executors = defaultFilesystemTools(fs)
	}
	return &AgentCore{
		Client:            client,
		FS:                fs,
		Tools:             tools,
		Executors:         executors,
		SystemContent:     systemContent,
		UserProfile:       userProfile,
		MaxContextTokens:  maxContextTokens,
		MaxOutputTokens:   maxOutputTokens,
		SummaryWindowDays: defaultSummaryWindowDays,
	}
}

// asOpenAITools 将内部 Tool 列表转换为 OpenAI tools 定义。
func (a *AgentCore) asOpenAITools() []openai.ChatCompletionToolUnionParam {
	if len(a.Tools) == 0 {
		return nil
	}
	out := make([]openai.ChatCompletionToolUnionParam, 0, len(a.Tools))
	for _, t := range a.Tools {
		var paramsSchema map[string]any
		if strings.TrimSpace(t.ParametersJSONSchema) != "" {
			if err := json.Unmarshal([]byte(t.ParametersJSONSchema), &paramsSchema); err != nil {
				log.Printf("warn: failed to parse tool parameters schema for %s: %v", t.Name, err)
				paramsSchema = nil
			}
		}
		out = append(out, openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
			Name:        t.Name,
			Description: openai.String(t.Description),
			Parameters:  paramsSchema,
			Strict:      openai.Bool(true),
		}))
	}
	return out
}

// Handle 处理一轮用户输入，返回本轮模型回复的纯文本内容（使用默认会话）。
func (a *AgentCore) Handle(userInput string) (string, error) {
	return a.HandleWithSession("cli-default", userInput)
}

// HandleWithSession 按 session 维度处理一轮用户输入。
func (a *AgentCore) HandleWithSession(sessionID, userInput string) (string, error) {
	userInput = strings.TrimSpace(userInput)
	if userInput == "" {
		return "", fmt.Errorf("empty input")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 构造轻量消息：系统 + 用户档案 + 会话历史 + 本轮 user，用于 token 估算与裁剪。
	var simpleMsgs []SimpleMsg
	if strings.TrimSpace(a.SystemContent) != "" {
		simpleMsgs = append(simpleMsgs, SimpleMsg{Role: "system", Content: a.SystemContent})
	}
	if strings.TrimSpace(a.UserProfile) != "" {
		simpleMsgs = append(simpleMsgs, SimpleMsg{Role: "user", Content: a.UserProfile})
	}

	// 读取该 session 的 active.json 作为历史。
	if a.FS != nil {
		sDir := sessionDir(sessionID)
		activePath := filepath.Join(sDir, "active.json")
		history, err := readSessionMessages(a.FS, activePath)
		if err == nil {
			for _, m := range history {
				role := strings.TrimSpace(m.Role)
				if role == "" {
					continue
				}
				simpleMsgs = append(simpleMsgs, SimpleMsg{Role: role, Content: m.Content})
			}
		}
	}

	now := Now()
	userMsg := newUserMessage(userInput, now, map[string]interface{}{
		"session_id": sessionID,
	})
	simpleMsgs = append(simpleMsgs, SimpleMsg{Role: "user", Content: userMsg.Content})

	// 在调用模型前做上下文长度控制（精确 token 计算）。
	effectiveLimit := a.MaxContextTokens
	if a.MaxOutputTokens > 0 && a.MaxContextTokens > a.MaxOutputTokens {
		effectiveLimit = a.MaxContextTokens - a.MaxOutputTokens
	}
	simpleMsgs = ensureContextWithinLimitSimple(simpleMsgs, effectiveLimit, a)

	// 将 SimpleMsg 转换为 OpenAI SDK 所需的 messages。
	var messages []openai.ChatCompletionMessageParamUnion
	for _, m := range simpleMsgs {
		switch m.Role {
		case "system":
			messages = append(messages, openai.SystemMessage(m.Content))
		case "assistant":
			messages = append(messages, openai.AssistantMessage(m.Content))
		default:
			// 其他一律按 user 处理，包括最初的 userProfile。
			messages = append(messages, openai.UserMessage(m.Content))
		}
	}

	tools := a.asOpenAITools()

	forceMemory := shouldForceMemoryTool(userInput)

	hadToolError := false
	toolRound := 0
	for {
		toolChoice := "auto"
		forceFunction := ""
		if forceMemory {
			forceFunction = "memory_write_important"
		}
		if hadToolError {
			toolChoice = "required"
		}

		llmMeta := &LLMRequestLogMeta{
			SessionID:         sessionID,
			CallLabel:         "agent",
			InputBudgetTokens: effectiveLimit,
			MaxContextTokens:  a.MaxContextTokens,
			MaxOutputTokens:   a.MaxOutputTokens,
			ToolRound:         toolRound,
		}
		resp, err := a.Client.ChatOnce(ctx, messages, tools, a.MaxOutputTokens, toolChoice, forceFunction, llmMeta)
		toolRound++
		if err != nil {
			return "", err
		}

		choice := resp.Choices[0]
		msg := choice.Message

		// 没有工具调用，直接返回最终回复
		if len(msg.ToolCalls) == 0 {
			finalReply := msg.Content
			if a.FS != nil {
				// 将本轮对话写入 session 存储。
				assistantMsg := newAssistantMessage(finalReply, now, map[string]interface{}{
					"session_id": sessionID,
				})
				_ = appendToSessionFiles(a, sessionID, userMsg, assistantMsg, now)
			}
			return finalReply, nil
		}

		// 执行工具并将结果追加为 tool 消息
		results := dispatchToolsOpenAI(msg.ToolCalls, a.Executors)
		hadToolError = false
		for _, r := range results {
			content := r.Content
			if r.Error != "" {
				hadToolError = true
				if content != "" {
					content += "\n"
				}
				content += "执行出错: " + r.Error
			}
			messages = append(messages, openai.ToolMessage(content, r.CallID))
		}
	}
}

