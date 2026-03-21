package main

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	openai "github.com/openai/openai-go/v3"
)

func TestInputBudgetAfterSlack(t *testing.T) {
	t.Setenv("AGENT_CONTEXT_TOKEN_SLACK", "")
	b := inputBudgetAfterSlack(10000)
	if b <= 0 || b >= 10000 {
		t.Fatal(b)
	}
	t.Setenv("AGENT_CONTEXT_TOKEN_SLACK", "100")
	if g := inputBudgetAfterSlack(10000); g != 9900 {
		t.Fatal(g)
	}
	if inputBudgetAfterSlack(0) != 0 || inputBudgetAfterSlack(-3) != 0 {
		t.Fatal("non-positive limit should yield 0 budget")
	}
	if inputBudgetAfterSlack(1) < 1 {
		t.Fatal(inputBudgetAfterSlack(1))
	}
}

func TestContextTokenSlackFromEnv_branches(t *testing.T) {
	t.Setenv("AGENT_CONTEXT_TOKEN_SLACK", "")
	if contextTokenSlackFromEnv(0) != 0 || contextTokenSlackFromEnv(-1) != 0 {
		t.Fatal()
	}
	t.Setenv("AGENT_CONTEXT_TOKEN_SLACK", "50")
	if contextTokenSlackFromEnv(1000) != 50 {
		t.Fatal(contextTokenSlackFromEnv(1000))
	}
	t.Setenv("AGENT_CONTEXT_TOKEN_SLACK", "99999")
	if g := contextTokenSlackFromEnv(80); g != maxInt(1, 80/4) {
		t.Fatal(g)
	}
}

func assistantParamFromCompletionJSON(t *testing.T, body string) openai.ChatCompletionMessageParamUnion {
	t.Helper()
	var resp openai.ChatCompletion
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Choices) == 0 {
		t.Fatal("no choices")
	}
	return assistantMessageParamFromCompletion(resp.Choices[0].Message)
}

func TestMaxRunesFromJSONLen(t *testing.T) {
	if maxRunesFromJSONLen(-1) != 0 || maxRunesFromJSONLen(0) != 0 {
		t.Fatal()
	}
	if maxRunesFromJSONLen(100) != 25 {
		t.Fatal(maxRunesFromJSONLen(100))
	}
}

func TestTruncateRunesWithSuffix(t *testing.T) {
	if truncateRunesWithSuffix("hello", 0) != contextTruncatedSuffix {
		t.Fatal()
	}
	if truncateRunesWithSuffix("hi", 50) != "hi" {
		t.Fatal()
	}
	long := strings.Repeat("β", 80)
	suf := []rune(contextTruncatedSuffix)
	// maxRunes 必须大于后缀 rune 数，才会走「正文 + …(truncated)」分支
	out := truncateRunesWithSuffix(long, len(suf)+8)
	if !strings.HasSuffix(out, contextTruncatedSuffix) {
		t.Fatal(out)
	}
	out2 := truncateRunesWithSuffix(long, minInt(3, len(suf)))
	if utf8.RuneCountInString(out2) != minInt(3, len(suf)) {
		t.Fatal(utf8.RuneCountInString(out2))
	}
}

func TestSegmentSizeAt(t *testing.T) {
	if segmentSizeAt(nil, 0) != 0 {
		t.Fatal()
	}
	ms := []openai.ChatCompletionMessageParamUnion{openai.UserMessage("x")}
	if segmentSizeAt(ms, -1) != 0 || segmentSizeAt(ms, 1) != 0 {
		t.Fatal()
	}
	if segmentSizeAt(ms, 0) != 1 {
		t.Fatal()
	}
	toolCalls := `[{"id":"c1","type":"function","function":{"name":"f","arguments":"{}"}}]`
	asst := assistantParamFromCompletionJSON(t, completionJSON("", toolCalls))
	tool := openai.ToolMessage(`{"ok":true}`, "c1")
	pair := []openai.ChatCompletionMessageParamUnion{asst, tool}
	if segmentSizeAt(pair, 0) != 2 {
		t.Fatal(segmentSizeAt(pair, 0))
	}
	broken := []openai.ChatCompletionMessageParamUnion{asst, openai.UserMessage("cut"), tool}
	if segmentSizeAt(broken, 0) != 1 {
		t.Fatal(segmentSizeAt(broken, 0))
	}
}

func TestTruncateMessageUnionByRunes_roles(t *testing.T) {
	toolLong := openai.ToolMessage(strings.Repeat("z", 400), "idz")
	out := truncateMessageUnionByRunes(toolLong, 30, true)
	if strings.Contains(string(mustJSON(t, out)), strings.Repeat("z", 200)) {
		t.Fatal("expected tool content shrink")
	}
	asst := openai.AssistantMessage(strings.Repeat("a", 500))
	out = truncateMessageUnionByRunes(asst, 25, true)
	if utf8.RuneCountInString(strings.TrimSpace(extractAssistantContent(t, out))) > 30 {
		t.Fatal()
	}
	toolCalls := `[{"id":"tc","type":"function","function":{"name":"fn","arguments":"{}"}}]`
	withCalls := assistantParamFromCompletionJSON(t, completionJSON("x", toolCalls))
	unchanged := truncateMessageUnionByRunes(withCalls, 5, true)
	if len(unchanged.OfAssistant.ToolCalls) != len(withCalls.OfAssistant.ToolCalls) {
		t.Fatal("assistant with tool_calls should not truncate")
	}
	imgParts := []openai.ChatCompletionContentPartUnionParam{
		openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{URL: "data:image/png;base64,AAAA"}),
	}
	userImg := openai.UserMessage(imgParts)
	out = truncateMessageUnionByRunes(userImg, 80, true)
	js := string(mustJSON(t, out))
	if !strings.Contains(js, "图片因长度限制已省略") {
		t.Fatalf("expected placeholder in %s", js)
	}
	sys := openai.SystemMessage("system text long " + strings.Repeat("s", 200))
	if truncateMessageUnionByRunes(sys, 10, false) != sys {
		t.Fatal("system should pass through default branch")
	}
	var empty openai.ChatCompletionMessageParamUnion
	if truncateMessageUnionByRunes(empty, 5, true) != empty {
		t.Fatal()
	}
}

func mustJSON(t *testing.T, m openai.ChatCompletionMessageParamUnion) []byte {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func extractAssistantContent(t *testing.T, m openai.ChatCompletionMessageParamUnion) string {
	t.Helper()
	if m.OfAssistant == nil || !m.OfAssistant.Content.OfString.Valid() {
		t.Fatal("expected assistant string content")
	}
	return m.OfAssistant.Content.OfString.Value
}

func TestTrimOpenAIMessagesToBudget_edgeCases(t *testing.T) {
	t.Setenv("AGENT_CONTEXT_TOKEN_SLACK", "")
	msgs := []openai.ChatCompletionMessageParamUnion{openai.UserMessage("only")}
	if len(trimOpenAIMessagesToBudget(msgs, nil, 0, 0, 0)) != 1 {
		t.Fatal()
	}
	if trimOpenAIMessagesToBudget(nil, nil, 100, 0, 0) != nil {
		t.Fatal()
	}
	big := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage("s"),
		openai.UserMessage(strings.Repeat("U", 6000)),
	}
	_, _, est := estimateChatRequestInputTokens(big, nil)
	out := trimOpenAIMessagesToBudget(big, nil, est/25, 9, 9)
	_, _, after := estimateChatRequestInputTokens(out, nil)
	if after > est/25+400 {
		t.Fatalf("after=%d cap~%d", after, est/25)
	}
}

func TestAggressiveTruncateOpenAIMessages_shrinks(t *testing.T) {
	t.Setenv("AGENT_CONTEXT_TOKEN_SLACK", "")
	msgs := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage("s"),
		openai.UserMessage(strings.Repeat("M", 5000)),
		openai.UserMessage("tail"),
	}
	_, _, est := estimateChatRequestInputTokens(msgs, nil)
	out := aggressiveTruncateOpenAIMessages(msgs, nil, est/15, 1, 1)
	_, _, after := estimateChatRequestInputTokens(out, nil)
	if after > est/15+500 {
		t.Fatalf("after=%d limit~%d", after, est/15)
	}
}

func TestTruncateProtectedTailUser_shrinks(t *testing.T) {
	t.Setenv("AGENT_CONTEXT_TOKEN_SLACK", "")
	msgs := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage("s"),
		openai.UserMessage(strings.Repeat("b", 4500)),
		openai.UserMessage("tail"),
	}
	_, _, est := estimateChatRequestInputTokens(msgs, nil)
	limit := est / 3
	if limit < 1 {
		t.Fatal(est)
	}
	out := truncateProtectedTailUser(msgs, nil, limit, 1)
	_, _, after := estimateChatRequestInputTokens(out, nil)
	if after > limit+800 {
		t.Fatalf("after=%d limit=%d", after, limit)
	}
	if len(truncateProtectedTailUser(msgs, nil, est, 0)) != len(msgs) {
		t.Fatal("tailKeep 0 should return copy unchanged")
	}
}

func TestTrimOpenAIMessagesToBudget_dropsOldTurns(t *testing.T) {
	var msgs []openai.ChatCompletionMessageParamUnion
	msgs = append(msgs, openai.SystemMessage("sys"))
	msgs = append(msgs, openai.UserMessage("profile"))
	for i := 0; i < 20; i++ {
		msgs = append(msgs, openai.UserMessage(strings.Repeat("u", 200)))
		msgs = append(msgs, openai.AssistantMessage(strings.Repeat("a", 200)))
	}
	msgs = append(msgs, openai.UserMessage("current"))
	tools := []openai.ChatCompletionToolUnionParam{
		openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{Name: "x", Description: openai.String("d")}),
	}
	headKeep, tailKeep := 2, 1
	_, _, before := estimateChatRequestInputTokens(msgs, tools)
	out := trimOpenAIMessagesToBudget(msgs, tools, before/3, headKeep, tailKeep)
	_, _, after := estimateChatRequestInputTokens(out, tools)
	if after > before/3+50 {
		t.Fatalf("expected trim, before=%d after=%d limit~%d", before, after, before/3)
	}
	if len(out) < tailKeep+headKeep {
		t.Fatal(len(out))
	}
	last := out[len(out)-1]
	if classifyChatMessageRole(last) != "user" {
		t.Fatal(classifyChatMessageRole(last))
	}
}

func TestHeadKeepCountForAgentMessages(t *testing.T) {
	if n := headKeepCountForAgentMessages("", ""); n != 0 {
		t.Fatal(n)
	}
	if n := headKeepCountForAgentMessages("s", ""); n != 1 {
		t.Fatal(n)
	}
	if n := headKeepCountForAgentMessages("s", "u"); n != 2 {
		t.Fatal(n)
	}
}
