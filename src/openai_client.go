package main

import (
	"context"
	"errors"
	"fmt"
	"log"

	openai "github.com/openai/openai-go/v3"
)

// LLMRequestLogMeta 单次 Chat Completions 请求日志所需的上下文（可为 nil，仅记录估算 token 与 roles）。
type LLMRequestLogMeta struct {
	SessionID         string
	CallLabel         string // 如 agent / history_reduce_chunk / session_weekly_summary
	InputBudgetTokens int    // 输入侧预算（与 agent 裁剪逻辑一致时便于算占用率）
	MaxContextTokens int // 配置的总上下文上限
	MaxOutputTokens  int // 本请求 max_output
	ToolRound         int    // 工具循环轮次，从 0 起
}

// requestMaxOut 为本轮 API 的 max_output；InputBudgetTokens 与主会话一致，用 agent 配置的上下文预算便于对比占比。
func newLLMRequestLogMetaFromAgent(agent *AgentCore, label string, requestMaxOut int) *LLMRequestLogMeta {
	meta := &LLMRequestLogMeta{
		CallLabel:       label,
		MaxOutputTokens: requestMaxOut,
	}
	if agent == nil {
		return meta
	}
	meta.MaxContextTokens = agent.MaxContextTokens
	if agent.MaxContextTokens > 0 && agent.MaxOutputTokens > 0 && agent.MaxContextTokens > agent.MaxOutputTokens {
		meta.InputBudgetTokens = agent.MaxContextTokens - agent.MaxOutputTokens
	} else if agent.MaxContextTokens > 0 {
		meta.InputBudgetTokens = agent.MaxContextTokens
	}
	return meta
}

func logLLMRequestPre(
	c *OpenAIClient,
	messages []openai.ChatCompletionMessageParamUnion,
	tools []openai.ChatCompletionToolUnionParam,
	meta *LLMRequestLogMeta,
	msgTok, toolTok, estTotal int,
) {
	model := ""
	if c != nil {
		model = c.Model
	}
	call, session, round := "", "", 0
	inputBudget, maxCtx, maxOut := 0, 0, 0
	if meta != nil {
		call = meta.CallLabel
		session = meta.SessionID
		round = meta.ToolRound
		inputBudget = meta.InputBudgetTokens
		maxCtx = meta.MaxContextTokens
		maxOut = meta.MaxOutputTokens
	}
	utilBudgetStr, utilWindowStr := "n/a", "n/a"
	if inputBudget > 0 {
		utilBudgetStr = fmt.Sprintf("%.2f", 100*float64(estTotal)/float64(inputBudget))
	}
	if maxCtx > 0 {
		utilWindowStr = fmt.Sprintf("%.2f", 100*float64(estTotal+maxOut)/float64(maxCtx))
	}
	roles := chatMessagesRoleSummary(messages)
	log.Printf(
		"llm_request phase=pre model=%s call=%s session=%s round=%d msg_count=%d tool_defs=%d est_msg_tokens=%d est_tools_tokens=%d est_input_total=%d input_budget=%d max_ctx=%d max_out=%d util_vs_input_budget_pct=%s util_vs_full_window_pct=%s roles=%s",
		model, call, session, round, len(messages), len(tools), msgTok, toolTok, estTotal, inputBudget, maxCtx, maxOut, utilBudgetStr, utilWindowStr, roles,
	)
}

// LLMRequestLimiter 用于限制同时进行的 LLM 请求数。
type LLMRequestLimiter struct {
	sem chan struct{}
}

func newLLMRequestLimiter(window int) *LLMRequestLimiter {
	if window <= 0 {
		return nil
	}
	return &LLMRequestLimiter{sem: make(chan struct{}, window)}
}

func (l *LLMRequestLimiter) Acquire(ctx context.Context) error {
	if l == nil || l.sem == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case l.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l *LLMRequestLimiter) Release() {
	if l == nil || l.sem == nil {
		return
	}
	<-l.sem
}

// OpenAIClient 封装与 OpenAI 兼容后端的 JSON 协议交互。
type OpenAIClient struct {
	Client  openai.Client
	Model   string
	Limiter *LLMRequestLimiter
}

// ChatOnce 调用一次 chat.completions，传入 messages 与 tools，并可指定最大输出 token 数与 tool_choice。
// toolChoiceMode 取值建议："auto" / "required"；forceFunctionName 非空时强制调用指定 function tool。
// logMeta 非 nil 时会在发送前记录估算 token、上下文摘要与相对预算占比；其中 MaxOutputTokens 建议与本参数 maxOutputTokens 一致以便计算窗口占比。
func (c *OpenAIClient) ChatOnce(
	ctx context.Context,
	messages []openai.ChatCompletionMessageParamUnion,
	tools []openai.ChatCompletionToolUnionParam,
	maxOutputTokens int,
	toolChoiceMode string,
	forceFunctionName string,
	logMeta *LLMRequestLogMeta,
) (*openai.ChatCompletion, error) {
	if c == nil {
		return nil, errors.New("openai client is nil")
	}

	if err := c.Limiter.Acquire(ctx); err != nil {
		return nil, err
	}
	defer c.Limiter.Release()

	if c.Model == "" {
		err := errors.New("openai model is empty")
		log.Printf("error: ChatOnce called with empty model: %v", err)
		return nil, err
	}

	params := openai.ChatCompletionNewParams{
		Model:    c.Model,
		Messages: messages,
	}
	if maxOutputTokens > 0 {
		params.MaxTokens = openai.Int(int64(maxOutputTokens))
	}
	if len(tools) > 0 {
		params.Tools = tools
		if forceFunctionName != "" {
			params.ToolChoice = openai.ToolChoiceOptionFunctionToolChoice(openai.ChatCompletionNamedToolChoiceFunctionParam{
				Name: forceFunctionName,
			})
		} else if toolChoiceMode != "" {
			params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{
				OfAuto: openai.String(toolChoiceMode),
			}
		}
	}

	msgTok, toolTok, estTotal := estimateChatRequestInputTokens(messages, tools)
	logLLMRequestPre(c, messages, tools, logMeta, msgTok, toolTok, estTotal)

	resp, err := c.Client.Chat.Completions.New(ctx, params)
	if err != nil {
		log.Printf("error: ChatOnce completion request failed: %v", err)
		return nil, err
	}
	call, session := "", ""
	if logMeta != nil {
		call = logMeta.CallLabel
		session = logMeta.SessionID
	}
	if resp.JSON.Usage.Valid() {
		log.Printf(
			"llm_response phase=post model=%s call=%s session=%s prompt_tokens=%d completion_tokens=%d total_tokens=%d",
			c.Model, call, session, resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens,
		)
	} else {
		log.Printf("llm_response phase=post model=%s call=%s session=%s usage=(not_reported)", c.Model, call, session)
	}

	if len(resp.Choices) == 0 {
		err := fmt.Errorf("empty choices from completion")
		log.Printf("warn: ChatOnce got empty choices from model")
		return nil, err
	}

	return resp, nil
}

