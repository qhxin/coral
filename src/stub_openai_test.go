package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// openAISeqHandler 按顺序返回预设 JSON 响应体（每条对应一次 chat/completions POST）。
type openAISeqHandler struct {
	bodies []string
	i      atomic.Int32
	status int
}

func (s *openAISeqHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.Contains(r.URL.Path, "chat/completions") {
		http.NotFound(w, r)
		return
	}
	st := s.status
	if st == 0 {
		st = http.StatusOK
	}
	idx := int(s.i.Load())
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if idx >= len(s.bodies) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":{"message":"no more stubs"}}`)
		return
	}
	s.i.Add(1)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(st)
	_, _ = io.WriteString(w, s.bodies[idx])
}

func newTestOpenAIServer(t *testing.T, bodies ...string) *httptest.Server {
	t.Helper()
	h := &openAISeqHandler{bodies: bodies}
	return httptest.NewServer(h)
}

func newStubOpenAIClient(t *testing.T, serverURL, model string, window int) *OpenAIClient {
	t.Helper()
	c := openai.NewClient(
		option.WithBaseURL(strings.TrimRight(serverURL, "/")+"/v1"),
		option.WithAPIKey("test-key"),
	)
	return &OpenAIClient{
		Client:  c,
		Model:   model,
		Limiter: newLLMRequestLimiter(window),
	}
}

func completionJSON(content string, toolCallsJSON string) string {
	q, _ := json.Marshal(content)
	qs := string(q)
	if toolCallsJSON != "" {
		return `{"id":"x","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"message":{"role":"assistant","content":` + qs + `,"tool_calls":` + toolCallsJSON + `},"finish_reason":"tool_calls"}]}`
	}
	return `{"id":"x","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"message":{"role":"assistant","content":` + qs + `},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`
}

func completionJSONNoUsage(content string) string {
	q, _ := json.Marshal(content)
	return `{"id":"x","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"message":{"role":"assistant","content":` + string(q) + `},"finish_reason":"stop"}]}`
}

func completionJSONEmptyChoices() string {
	return `{"id":"x","object":"chat.completion","created":1,"model":"m","choices":[]}`
}
