package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
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

	// 新增: 技能注册表（默认启用，除非CORAL_USE_SKILL_REGISTRY=false）
	SkillRegistry *SkillRegistry
	// 新增: Prompt管理器（默认启用，除非CORAL_USE_PROMPT_FIRST=false）
	PromptManager *PromptManager
	// 新增: RAG记忆系统（默认启用，除非CORAL_USE_RAG_MEMORY=false）
	RAGMemory *RAGMemory
	// 新增: 意图检测器（默认启用，除非CORAL_USE_INTENT_DETECTOR=false）
	IntentDetector *IntentDetector
}

// NewAgentCore 创建一个新的 AgentCore，由外部注入 OpenAIClient 与初始上下文。
func NewAgentCore(client *OpenAIClient, systemContent, userProfile string, fs *WorkspaceFS, maxContextTokens, maxOutputTokens int) *AgentCore {
	agent := &AgentCore{
		Client:            client,
		FS:                fs,
		SystemContent:     systemContent,
		UserProfile:       userProfile,
		MaxContextTokens:  maxContextTokens,
		MaxOutputTokens:   maxOutputTokens,
		SummaryWindowDays: defaultSummaryWindowDays,
	}

	// 初始化PromptManager（默认启用Prompt-First模式）
	if os.Getenv("CORAL_USE_PROMPT_FIRST") != "false" {
		agent.PromptManager = NewPromptManager(defaultAgentBase)
		log.Printf("info: using prompt-first tool guidance")
	}

	// 初始化RAG记忆系统（默认启用RAG模式）
	if os.Getenv("CORAL_USE_RAG_MEMORY") != "false" && fs != nil {
		agent.RAGMemory = NewRAGMemory(fs)
		if err := agent.RAGMemory.Load(); err != nil {
			log.Printf("warn: load RAG memory failed: %v", err)
		} else {
			log.Printf("info: loaded %d memories into RAG", len(agent.RAGMemory.entries))
		}
	}

	// 初始化意图检测器（默认启用）
	if os.Getenv("CORAL_USE_INTENT_DETECTOR") != "false" {
		agent.IntentDetector = NewIntentDetector(agent.Tools)
		log.Printf("info: using intent detector")
	}

	// 根据环境变量决定使用技能注册表还是硬编码工具（默认启用技能注册表）
	if os.Getenv("CORAL_USE_SKILL_REGISTRY") != "false" && fs != nil {
		agent.SkillRegistry = NewSkillRegistry(fs)
		agent.SkillRegistry.RegisterBuiltinHandlers()

		// 加载skills目录
		skillsDir := filepath.Join(fs.Root, "skills")
		if _, err := os.Stat(skillsDir); err == nil {
			if err := agent.SkillRegistry.LoadFromDir(skillsDir); err != nil {
				log.Printf("warn: load skills failed: %v, fallback to default tools", err)
				agent.loadDefaultTools(fs)
			} else {
				agent.Tools, agent.Executors = agent.SkillRegistry.ToTools()
				log.Printf("info: loaded %d skills from %s", len(agent.Tools), skillsDir)
			}
		} else {
			log.Printf("warn: skills dir not found at %s, using default tools", skillsDir)
			agent.loadDefaultTools(fs)
		}
	} else {
		agent.loadDefaultTools(fs)
	}

	return agent
}

// loadDefaultTools 加载原有硬编码工具（回退方案）
func (a *AgentCore) loadDefaultTools(fs *WorkspaceFS) {
	if fs != nil {
		a.Tools, a.Executors = defaultFilesystemTools(fs)
	}
}

// loadMemoryContent 加载MEMORY.md内容
func (a *AgentCore) loadMemoryContent() string {
	if a.FS == nil {
		return ""
	}
	content, err := a.FS.Read("MEMORY.md")
	if err != nil {
		return ""
	}
	return content
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

	// 确定系统提示词：使用PromptManager动态构建，或回退到固定systemContent
	var systemContent string
	if a.PromptManager != nil && a.SkillRegistry != nil {
		// 使用Prompt-First模式：动态构建包含工具描述的系统提示
		tools, _ := a.SkillRegistry.ToTools()

		// 如果使用RAG，则基于查询检索相关记忆；否则加载全部记忆
		if a.RAGMemory != nil {
			systemContent = a.PromptManager.BuildSystemPromptWithRAGAndWorkspace(
				tools,
				a.RAGMemory,
				rawText,
				a.SystemContent,
				a.UserProfile,
			)
		} else {
			memoryContent := a.loadMemoryContent()
			systemContent = a.PromptManager.BuildSystemPromptWithWorkspaceContext(
				tools,
				memoryContent,
				a.SystemContent,
				a.UserProfile,
			)
		}
	} else {
		// 回退到固定系统提示
		systemContent = a.SystemContent
	}

	if strings.TrimSpace(systemContent) != "" {
		simpleMsgs = append(simpleMsgs, SimpleMsg{Role: "system", Content: systemContent})
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
	headKeep := headKeepCountForAgentMessages(a.SystemContent, a.UserProfile)
	tailKeep := 1
	inputBudget := 0
	if effectiveLimit > 0 {
		inputBudget = inputBudgetAfterSlack(effectiveLimit)
	}

	// 工具选择策略：默认auto模式
	toolChoice := "auto"
	forceFunction := ""

	// 仅在Prompt-First模式禁用时使用硬编码强制逻辑
	if os.Getenv("CORAL_USE_PROMPT_FIRST") == "false" {
		forceKey := rawText
		if forceKey == "" {
			forceKey = modelUserText
		}
		if shouldForceMemoryTool(forceKey) {
			forceFunction = "memory_write_important"
		}
	} else if a.IntentDetector != nil {
		// Prompt-First模式下，可选使用意图检测器辅助决策（高置信度时强制）
		intent, confidence := a.IntentDetector.Detect(rawText)
		if confidence > 0.85 {
			toolChoice = "required"
			forceFunction = intent.Action
			log.Printf("info: intent detector triggered with confidence %.2f for action %s", confidence, intent.Action)
		}
	}

	hadToolError := false
	toolRound := 0
	for {
		if inputBudget > 0 {
			messages = trimOpenAIMessagesToBudget(messages, tools, inputBudget, headKeep, tailKeep)
		}

		if hadToolError {
			toolChoice = "required"
		}

		llmBudgetForLog := effectiveLimit
		if inputBudget > 0 {
			llmBudgetForLog = inputBudget
		}
		llmMeta := &LLMRequestLogMeta{
			SessionID:         sessionID,
			CallLabel:         "agent",
			InputBudgetTokens: llmBudgetForLog,
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
		messages = append(messages, assistantMessageParamFromCompletion(msg))
		tailKeep++
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
			tailKeep++
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
