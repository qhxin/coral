package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	openai "github.com/openai/openai-go/v3"
)

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
func (c *OpenAIClient) ChatOnce(
	ctx context.Context,
	messages []openai.ChatCompletionMessageParamUnion,
	tools []openai.ChatCompletionToolUnionParam,
	maxOutputTokens int,
	toolChoiceMode string,
	forceFunctionName string,
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

	// 记录即将发送的请求内容（不包含敏感信息）。
	if b, err := json.Marshal(params); err == nil {
		log.Printf("openai request: %s", string(b))
	} else {
		log.Printf("warn: failed to marshal openai request params: %v", err)
	}

	resp, err := c.Client.Chat.Completions.New(ctx, params)
	if err != nil {
		log.Printf("error: ChatOnce completion request failed: %v", err)
		return nil, err
	}
	if len(resp.Choices) == 0 {
		err := fmt.Errorf("empty choices from completion")
		log.Printf("warn: ChatOnce got empty choices from model")
		return nil, err
	}

	// 记录收到的响应内容。
	if b, err := json.Marshal(resp); err == nil {
		log.Printf("openai response: %s", string(b))
	} else {
		log.Printf("warn: failed to marshal openai response: %v", err)
	}

	return resp, nil
}

