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
	return a.HandleWithSessionWithMedia(sessionID, userInput, nil)
}

// HandleWithSessionWithMedia 与 HandleWithSession 相同，但允许附带图像（仅随本轮请求提交；会话文件仅存描述文本）。
func (a *AgentCore) HandleWithSessionWithMedia(sessionID string, userInput string, images []UserImage) (string, error) {
	rawText := strings.TrimSpace(userInput)
	if len(images) > 0 {
		if err := validateUserImages(images); err != nil {
			return "", err
		}
	}
	if rawText == "" && len(images) == 0 {
		return "", fmt.Errorf("empty input")
	}
	modelUserText := rawText
	if modelUserText == "" {
		modelUserText = visionEmptyPrompt()
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
	persistText := persistenceUserTextForVisionTurn(rawText, len(images))
	meta := map[string]interface{}{
		"session_id":  sessionID,
		"image_count": len(images),
	}
	if paths := saveInboundMediaIfEnabled(a.FS, sessionID, images); len(paths) > 0 {
		meta["media_files"] = paths
	}
	userMsg := newUserMessage(persistText, now, meta)
	simpleMsgs = append(simpleMsgs, SimpleMsg{
		Role:       "user",
		Content:    persistText,
		ImageCount: len(images),
	})

	effectiveLimit := a.MaxContextTokens
	if a.MaxOutputTokens > 0 && a.MaxContextTokens > a.MaxOutputTokens {
		effectiveLimit = a.MaxContextTokens - a.MaxOutputTokens
	}
	simpleMsgs = ensureContextWithinLimitSimple(simpleMsgs, effectiveLimit, a)

	// 最后一条 user 如含图则构造多段 content；其余仍为纯文本。
	lastUserImages := images
	if len(images) > 0 {
		last := simpleMsgs[len(simpleMsgs)-1]
		if last.ImageCount == 0 {
			lastUserImages = nil
		}
	}
	messages := simpleMsgsToOpenAI(simpleMsgs, modelUserText, lastUserImages)

	tools := a.asOpenAITools()

	forceKey := rawText
	if forceKey == "" {
		forceKey = modelUserText
	}
	forceMemory := shouldForceMemoryTool(forceKey)

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

		if len(msg.ToolCalls) == 0 {
			finalReply := msg.Content
			if a.FS != nil {
				assistantMsg := newAssistantMessage(finalReply, now, map[string]interface{}{
					"session_id": sessionID,
				})
				_ = appendToSessionFiles(a, sessionID, userMsg, assistantMsg, now)
			}
			return finalReply, nil
		}

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

// simpleMsgsToOpenAI 将 SimpleMsg 转为 API 消息；仅当末条为带图的当前 user 时使用多模态。
// modelUserText 为发给模型的用户正文（无图时与 persist 文本一致；纯图时为默认提示句）。
func simpleMsgsToOpenAI(msgs []SimpleMsg, modelUserText string, lastUserImages []UserImage) []openai.ChatCompletionMessageParamUnion {
	if len(msgs) == 0 {
		return nil
	}
	last := len(msgs) - 1
	out := make([]openai.ChatCompletionMessageParamUnion, 0, len(msgs))
	for i, m := range msgs {
		switch m.Role {
		case "system":
			out = append(out, openai.SystemMessage(m.Content))
		case "assistant":
			out = append(out, openai.AssistantMessage(m.Content))
		default:
			if i == last && len(lastUserImages) > 0 {
				out = append(out, openAIUserMessageForTurn(modelUserText, lastUserImages))
			} else {
				out = append(out, openai.UserMessage(m.Content))
			}
		}
	}
	return out
}

func openAIUserMessageForTurn(text string, images []UserImage) openai.ChatCompletionMessageParamUnion {
	if len(images) == 0 {
		return openai.UserMessage(text)
	}
	parts := make([]openai.ChatCompletionContentPartUnionParam, 0, 1+len(images))
	parts = append(parts, openai.TextContentPart(text))
	for _, im := range images {
		parts = append(parts, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
			URL: imageDataURL(im),
		}))
	}
	return openai.UserMessage(parts)
}
