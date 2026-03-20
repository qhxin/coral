package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	openai "github.com/openai/openai-go/v3"
)

func TestNewLLMRequestLimiter_nilAndAcquireRelease(t *testing.T) {
	if newLLMRequestLimiter(0) != nil {
		t.Fatal()
	}
	var nilLim *LLMRequestLimiter
	if nilLim.Acquire(context.Background()) != nil {
		t.Fatal()
	}
	nilLim.Release()
	lim := newLLMRequestLimiter(2)
	ctx := context.Background()
	if err := lim.Acquire(ctx); err != nil {
		t.Fatal(err)
	}
	if err := lim.Acquire(ctx); err != nil {
		t.Fatal(err)
	}
	lim.Release()
	lim.Release()
}

func TestLLMRequestLimiterAcquire_cancel(t *testing.T) {
	lim := newLLMRequestLimiter(1)
	if err := lim.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer lim.Release()
	cctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := lim.Acquire(cctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestLLMRequestLimiterAcquire_singlePermit(t *testing.T) {
	lim := newLLMRequestLimiter(1)
	defer lim.Release()
	if err := lim.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestNewLLMRequestLogMetaFromAgent(t *testing.T) {
	m := newLLMRequestLogMetaFromAgent(nil, "l", 128)
	if m.CallLabel != "l" || m.MaxOutputTokens != 128 {
		t.Fatal()
	}
	a := &AgentCore{MaxContextTokens: 8000, MaxOutputTokens: 500}
	m = newLLMRequestLogMetaFromAgent(a, "x", 256)
	if m.InputBudgetTokens != 7500 || m.MaxContextTokens != 8000 {
		t.Fatal(m.InputBudgetTokens)
	}
	a = &AgentCore{MaxContextTokens: 100, MaxOutputTokens: 0}
	m = newLLMRequestLogMetaFromAgent(a, "y", 64)
	if m.InputBudgetTokens != 100 {
		t.Fatal(m.InputBudgetTokens)
	}
}

func TestOpenAIClientChatOnce_nilAndEmptyModel(t *testing.T) {
	ctx := context.Background()
	var c *OpenAIClient
	if _, err := c.ChatOnce(ctx, nil, nil, 0, "", "", nil); err == nil {
		t.Fatal()
	}
	c = &OpenAIClient{Client: openai.Client{}, Model: "", Limiter: nil}
	if _, err := c.ChatOnce(ctx, nil, nil, 0, "", "", nil); err == nil {
		t.Fatal()
	}
}

func TestOpenAIClientChatOnce_stubUsageAndEmptyChoices(t *testing.T) {
	srv := newTestOpenAIServer(t,
		completionJSONNoUsage("hi"),
		completionJSONEmptyChoices(),
	)
	defer srv.Close()
	c := newStubOpenAIClient(t, srv.URL, "m", 1)
	ctx := context.Background()

	r1, err := c.ChatOnce(ctx, []openai.ChatCompletionMessageParamUnion{openai.UserMessage("u")}, nil, 10, "", "", nil)
	if err != nil || r1.Choices[0].Message.Content != "hi" {
		t.Fatal(err, r1)
	}
	_, err = c.ChatOnce(ctx, []openai.ChatCompletionMessageParamUnion{openai.UserMessage("u")}, nil, 10, "", "", nil)
	if err == nil {
		t.Fatal("expected empty choices error")
	}
}

func TestOpenAIClientChatOnce_apiError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad"}}`))
	}))
	defer srv.Close()
	c := newStubOpenAIClient(t, srv.URL, "m", 1)
	_, err := c.ChatOnce(context.Background(), []openai.ChatCompletionMessageParamUnion{openai.UserMessage("u")}, nil, 10, "", "", nil)
	if err == nil {
		t.Fatal()
	}
}
