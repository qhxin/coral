package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMarkdownSkillParserParseAndExtractors(t *testing.T) {
	md := "# workspace_read_file\n\n" +
		"## Description\n读取文件内容。\n\n" +
		"## Parameters\n" +
		"### path (required)\n" +
		"- **类型**: string\n" +
		"- **描述**: 要读取的文件路径\n\n" +
		"### mode (optional)\n" +
		"- **类型**: string\n" +
		"- **描述**: 读取模式\n\n" +
		"## Examples\n" +
		"### 示例1\n" +
		"场景: 读取配置文件\n" +
		"```json\n" +
		"{\"tool_calls\":[{\"type\":\"function\",\"function\":{\"name\":\"workspace_read_file\",\"arguments\":\"{\\\"path\\\":\\\"AGENT.md\\\"}\"}}]}\n" +
		"```\n"

	if got := extractH1(md); got != "workspace_read_file" {
		t.Fatalf("unexpected h1: %q", got)
	}
	if got := extractH1("no h1"); got != "" {
		t.Fatalf("expected empty h1, got %q", got)
	}

	params := extractParameters(md)
	props, ok := params["properties"].(map[string]interface{})
	if !ok || len(props) != 2 {
		t.Fatalf("unexpected properties: %#v", params["properties"])
	}
	required, ok := params["required"].([]string)
	if !ok || len(required) != 1 || required[0] != "path" {
		t.Fatalf("unexpected required fields: %#v", params["required"])
	}

	examples := extractExamples(md)
	if len(examples) != 1 {
		t.Fatalf("expected 1 example, got %d", len(examples))
	}
	if examples[0].Scenario != "读取配置文件" || examples[0].ToolCall == "" {
		t.Fatalf("unexpected example payload: %+v", examples[0])
	}

	desc := extractDescription(md)
	if desc == "" || !containsAll(desc, "读取文件内容", "Usage Examples", "workspace_read_file") {
		t.Fatalf("description missing expected segments: %q", desc)
	}

	tmp := t.TempDir()
	path := filepath.Join(tmp, "workspace_read_file.md")
	if err := os.WriteFile(path, []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &MarkdownSkillParser{}
	skill, err := p.Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if skill.Name != "workspace_read_file" {
		t.Fatalf("unexpected skill name: %q", skill.Name)
	}
}

func TestExtractHandlerKeyFromPath(t *testing.T) {
	p := filepath.Join("skills", "filesystem", "read_file.md")
	if got := extractHandlerKeyFromPath(p); got != "native:filesystem_read_file" {
		t.Fatalf("unexpected handler key: %q", got)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !contains(s, p) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && (indexOf(s, sub) >= 0))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

