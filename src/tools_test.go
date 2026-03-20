package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	openai "github.com/openai/openai-go/v3"
)

func TestShouldForceMemoryTool(t *testing.T) {
	if shouldForceMemoryTool("") || shouldForceMemoryTool("   ") {
		t.Fatal()
	}
	if !shouldForceMemoryTool("请长期记住我的偏好") {
		t.Fatal()
	}
	if !shouldForceMemoryTool("记住我的原则") {
		t.Fatal()
	}
	if shouldForceMemoryTool("随便聊聊") {
		t.Fatal()
	}
}

func TestDefaultFilesystemTools_readWriteMemory(t *testing.T) {
	root := t.TempDir()
	fs := &WorkspaceFS{Root: root}
	tools, exec := defaultFilesystemTools(fs)
	if len(tools) != 3 || len(exec) != 3 {
		t.Fatal(len(tools), len(exec))
	}
	if _, err := exec["workspace_read_file"](json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected err empty payload")
	}
	if err := os.WriteFile(filepath.Join(root, "AGENT.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec["workspace_read_file"](json.RawMessage(`{"path":"AGENT.md"}`))
	if err != nil || out != "x" {
		t.Fatal(err, out)
	}
	out, err = exec["workspace_write_file"](json.RawMessage(`{"path":"n.txt","content":"hi"}`))
	if err != nil || out != "写入成功" {
		t.Fatal(err, out)
	}
	out, err = exec["memory_write_important"](json.RawMessage(`{"content":"memo"}`))
	if err != nil || out != "写入 MEMORY.md 成功" {
		t.Fatal(err, out)
	}
	b, _ := os.ReadFile(filepath.Join(root, "MEMORY.md"))
	if len(b) == 0 || string(b) == "" {
		t.Fatal("MEMORY.md")
	}
}

func toolCallsFromJSON(t *testing.T, js string) []openai.ChatCompletionMessageToolCallUnion {
	t.Helper()
	var wrap struct {
		Choices []struct {
			Message struct {
				ToolCalls []openai.ChatCompletionMessageToolCallUnion `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(js), &wrap); err != nil {
		t.Fatal(err)
	}
	if len(wrap.Choices) == 0 {
		t.Fatal("no choices")
	}
	return wrap.Choices[0].Message.ToolCalls
}

func TestDispatchToolsOpenAI(t *testing.T) {
	root := t.TempDir()
	fs := &WorkspaceFS{Root: root}
	_ = os.WriteFile(filepath.Join(root, "AGENT.md"), []byte("ok"), 0o644)
	_, exec := defaultFilesystemTools(fs)

	raw := `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"unknown","arguments":"{}"}}]}}]}`
	res := dispatchToolsOpenAI(toolCallsFromJSON(t, raw), exec)
	if len(res) != 1 || res[0].Error == "" || res[0].Content != "" {
		t.Fatalf("%+v", res)
	}

	raw = `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"c2","type":"function","function":{"name":"workspace_read_file","arguments":"{\"path\":\"AGENT.md\"}"}}]}}]}`
	res = dispatchToolsOpenAI(toolCallsFromJSON(t, raw), exec)
	if len(res) != 1 || res[0].Error != "" || res[0].Content != "ok" {
		t.Fatalf("%+v", res)
	}

	raw = `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"c3","type":"function","function":{"name":"workspace_read_file","arguments":"{\"path\":\"\"}"}}]}}]}`
	res = dispatchToolsOpenAI(toolCallsFromJSON(t, raw), exec)
	if len(res) != 1 || res[0].Error == "" {
		t.Fatalf("%+v", res)
	}
}
