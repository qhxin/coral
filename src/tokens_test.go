package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openai "github.com/openai/openai-go/v3"
)

func TestClassifyChatMessageRole(t *testing.T) {
	cases := []struct {
		m    openai.ChatCompletionMessageParamUnion
		want string
	}{
		{openai.DeveloperMessage("d"), "developer"},
		{openai.SystemMessage("s"), "system"},
		{openai.UserMessage("u"), "user"},
		{openai.AssistantMessage("a"), "assistant"},
		{openai.ToolMessage("x", "id"), "tool"},
		{openai.ChatCompletionMessageParamOfFunction("c", "f"), "function"},
		{openai.ChatCompletionMessageParamUnion{}, "unknown"},
	}
	for _, tc := range cases {
		if g := classifyChatMessageRole(tc.m); g != tc.want {
			t.Fatalf("%v -> %s want %s", tc.m, g, tc.want)
		}
	}
}

func TestChatMessagesRoleSummary(t *testing.T) {
	msgs := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage("s"),
		openai.UserMessage("u1"),
		openai.UserMessage("u2"),
		openai.AssistantMessage("a1"),
	}
	s := chatMessagesRoleSummary(msgs)
	if s != "system:1,user:2,assistant:1" {
		t.Fatal(s)
	}
}

func TestEstimateChatRequestInputTokens_skipBadMarshal(t *testing.T) {
	// 无法 Marshal 的 channel 等不会出现在 union 中；用非法结构较难构造。
	// 至少覆盖空切片路径。
	m, tkn, sum := estimateChatRequestInputTokens(nil, nil)
	if m != 0 || tkn != 0 || sum != 0 {
		t.Fatal(m, tkn, sum)
	}
}

func TestSplitIntoChunks(t *testing.T) {
	ch := splitIntoChunks(nil, 100)
	if ch != nil {
		t.Fatal()
	}
	msgs := []SimpleMsg{{Role: "user", Content: strings.Repeat("x", 5000)}}
	ch = splitIntoChunks(msgs, 50)
	if len(ch) < 1 {
		t.Fatal()
	}
}

func TestEnsureContextWithinLimitSimple_earlyExit(t *testing.T) {
	msgs := []SimpleMsg{{Role: "user", Content: "a"}}
	if len(ensureContextWithinLimitSimple(msgs, 0, &AgentCore{})) != 1 {
		t.Fatal()
	}
	if len(ensureContextWithinLimitSimple(msgs, 100, nil)) != 1 {
		t.Fatal()
	}
}

func TestReduceHistory_earlyExit(t *testing.T) {
	msgs := []SimpleMsg{{Role: "u", Content: "x"}}
	out, err := reduceHistory(msgs, 0, &AgentCore{})
	if err != nil || len(out) != 1 {
		t.Fatal(err, out)
	}
	out, err = reduceHistory(msgs, 100, nil)
	if err != nil || len(out) != 1 {
		t.Fatal(err)
	}
	a := &AgentCore{}
	out, err = reduceHistory(msgs, 100, a)
	if err != nil || len(out) != 1 {
		t.Fatal(err)
	}
}

func TestSummarizeSimpleChunkWithLLM_edges(t *testing.T) {
	if _, err := summarizeSimpleChunkWithLLM(nil, []SimpleMsg{{Role: "u", Content: "x"}}); err != nil {
		t.Fatal(err)
	}
	srv := newTestOpenAIServer(t, completionJSON("小结", ""))
	defer srv.Close()
	cli := newStubOpenAIClient(t, srv.URL, "m", 1)
	a := &AgentCore{Client: cli}
	msg, err := summarizeSimpleChunkWithLLM(a, []SimpleMsg{{Role: "user", Content: "hello"}})
	if err != nil || msg.Role != "system" || msg.Content != "小结" {
		t.Fatal(err, msg)
	}
}

func TestEstimateTokensJSONChunks_viaPublicPath(t *testing.T) {
	parts := [][]byte{[]byte(`{"a":1}`)}
	n := estimateTokensJSONChunks(parts)
	if n < 1 {
		t.Fatal(n)
	}
}

func TestReduceHistory_withStubLLM(t *testing.T) {
	var bodies []string
	for i := 0; i < 24; i++ {
		bodies = append(bodies, completionJSON("压缩", ""))
	}
	srv := newTestOpenAIServer(t, bodies...)
	defer srv.Close()
	ag := &AgentCore{Client: newStubOpenAIClient(t, srv.URL, "m", 8)}
	msgs := []SimpleMsg{
		{Role: "user", Content: strings.Repeat("x", 4000)},
		{Role: "assistant", Content: strings.Repeat("y", 4000)},
	}
	out, err := reduceHistory(msgs, 200, ag)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("expected compressed output")
	}
}

func TestMarshal_skips_badToolForEstimate(t *testing.T) {
	// estimateChatRequestInputTokens continues on marshal error — 用含不可编码值的 map 不可行。
	// 依赖正常 union 路径已在集成里覆盖。
	var u openai.ChatCompletionMessageParamUnion
	b, err := json.Marshal(u)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) == 0 {
		t.Fatal()
	}
}

func TestEstimateTokensSimple_visionAddon(t *testing.T) {
	t.Setenv("AGENT_VISION_TOKENS_PER_IMAGE", "100")
	base := estimateTokensSimple([]SimpleMsg{{Role: "user", Content: "a"}})
	with := estimateTokensSimple([]SimpleMsg{{Role: "user", Content: "a", ImageCount: 2}})
	if with < base+200 {
		t.Fatalf("want >= base+200, base=%d with=%d", base, with)
	}
}

func TestReduceHistory_pinsLastMessage(t *testing.T) {
	var bodies []string
	for i := 0; i < 24; i++ {
		bodies = append(bodies, completionJSON("摘要", ""))
	}
	srv := newTestOpenAIServer(t, bodies...)
	defer srv.Close()
	ag := &AgentCore{Client: newStubOpenAIClient(t, srv.URL, "m", 2)}
	long := strings.Repeat("x", 500)
	tail := SimpleMsg{Role: "user", Content: "must-keep-question-unique-789"}
	msgs := []SimpleMsg{
		{Role: "user", Content: long},
		{Role: "assistant", Content: long},
		tail,
	}
	out, err := reduceHistory(msgs, 80, ag)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 || out[len(out)-1].Content != tail.Content {
		t.Fatalf("tail not preserved: %#v", out)
	}
}

func TestEnsureContextWithinLimitSimple_onReduceError_returnsOriginal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	ag := &AgentCore{Client: newStubOpenAIClient(t, srv.URL, "m", 2)}
	msgs := []SimpleMsg{
		{Role: "user", Content: strings.Repeat("a", 400)},
		{Role: "assistant", Content: strings.Repeat("b", 400)},
		{Role: "user", Content: "tail"},
	}
	out := ensureContextWithinLimitSimple(msgs, 80, ag)
	if len(out) != len(msgs) {
		t.Fatalf("got %d want %d", len(out), len(msgs))
	}
}

func TestSummarizeSimpleChunkWithLLM_toolRoleLine(t *testing.T) {
	srv := newTestOpenAIServer(t, completionJSON(" ok ", ""))
	t.Cleanup(srv.Close)
	a := &AgentCore{Client: newStubOpenAIClient(t, srv.URL, "m", 1)}
	msg, err := summarizeSimpleChunkWithLLM(a, []SimpleMsg{{Role: "tool", Content: "out"}})
	if err != nil || msg.Role != "system" || msg.Content != "ok" {
		t.Fatal(err, msg)
	}
}
