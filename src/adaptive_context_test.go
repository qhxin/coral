package main

import (
	"strings"
	"testing"

	openai "github.com/openai/openai-go/v3"
)

func TestAdaptiveContextManagerBuildAndApplyLimit(t *testing.T) {
	rag := NewRAGMemory(nil)
	rag.entries = []MemoryEntry{
		{ID: "1", Content: "project coding style prefers small functions"},
	}

	acm := NewAdaptiveContextManager(1, rag)
	history := []ChatMessage{
		{Role: "user", Content: "older question about style"},
		{Role: "assistant", Content: "older answer"},
		{Role: "user", Content: "new question"},
	}

	msgs := acm.BuildMessages("system", "coding style question", history)
	if len(msgs) == 0 {
		t.Fatal("expected messages")
	}
	// With very small limit and >2 messages, applyTokenLimit should keep head+summary+tail.
	if len(msgs) < 4 {
		t.Fatalf("expected summarized message list, got %d", len(msgs))
	}
	if classifyChatMessageRole(msgs[0]) != "system" {
		t.Fatalf("expected first message to be system, got %s", classifyChatMessageRole(msgs[0]))
	}
}

func TestAdaptiveContextProcessHistoryAndSummarize(t *testing.T) {
	rag := NewRAGMemory(nil)
	acm := NewAdaptiveContextManager(0, rag)

	if got := acm.processHistory(nil, "q"); got != nil {
		t.Fatalf("expected nil for empty history, got %+v", got)
	}

	short := []ChatMessage{
		{Role: "user", Content: "a"},
		{Role: "assistant", Content: "b"},
	}
	gotShort := acm.processHistory(short, "a")
	if len(gotShort) != len(short) {
		t.Fatalf("expected short history unchanged")
	}

	var longHistory []ChatMessage
	for i := 0; i < 12; i++ {
		content := "irrelevant topic"
		if i%2 == 0 {
			content = "alpha beta topic"
		}
		longHistory = append(longHistory, ChatMessage{Role: "user", Content: content})
	}
	processed := acm.processHistory(longHistory, "alpha beta")
	if len(processed) == 0 {
		t.Fatal("expected processed history")
	}
	joined := make([]string, 0, len(processed))
	for _, m := range processed {
		joined = append(joined, m.Content)
	}
	if !strings.Contains(strings.Join(joined, "\n"), "[前文摘要]") {
		t.Fatal("expected summary message for unrelated older history")
	}

	if got := acm.summarizeMessages(nil); got != "" {
		t.Fatalf("expected empty summary, got %q", got)
	}
	noUser := acm.summarizeMessages([]ChatMessage{{Role: "assistant", Content: "reply"}})
	if !strings.Contains(noUser, "1条历史消息") {
		t.Fatalf("unexpected non-user summary: %q", noUser)
	}
	longUser := acm.summarizeMessages([]ChatMessage{{Role: "user", Content: strings.Repeat("x", 80)}})
	if !strings.Contains(longUser, "...") {
		t.Fatalf("expected truncated summary point, got %q", longUser)
	}
}

func TestAdaptiveContextHelpers(t *testing.T) {
	if acm := NewAdaptiveContextManager(128, nil); acm.MaxTokens != 128 || acm.RAGMemory != nil {
		t.Fatalf("unexpected manager: %+v", acm)
	}

	msg := chatMessageToParam(ChatMessage{Role: "assistant", Content: "ok"})
	if classifyChatMessageRole(msg) != "assistant" {
		t.Fatalf("unexpected role conversion: %s", classifyChatMessageRole(msg))
	}
	fallback := chatMessageToParam(ChatMessage{Role: "unknown", Content: "x"})
	if classifyChatMessageRole(fallback) != "user" {
		t.Fatalf("unexpected fallback role: %s", classifyChatMessageRole(fallback))
	}

	params := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage("s"),
		openai.UserMessage("u"),
	}
	simple := paramsToSimpleMsgs(params)
	if len(simple) != 2 {
		t.Fatalf("unexpected converted length: %d", len(simple))
	}
	if simple[0].Role != "system" || simple[0].Content != "" {
		t.Fatalf("unexpected first simple message: %+v", simple[0])
	}

	// applyTokenLimit branches
	noLimit := NewAdaptiveContextManager(0, nil)
	if got := noLimit.applyTokenLimit(params); len(got) != len(params) {
		t.Fatalf("expected unchanged when no limit")
	}
	tooShort := NewAdaptiveContextManager(1, nil)
	short := []openai.ChatCompletionMessageParamUnion{openai.SystemMessage("s"), openai.UserMessage("u")}
	if got := tooShort.applyTokenLimit(short); len(got) != len(short) {
		t.Fatalf("expected unchanged when too few messages")
	}
}

