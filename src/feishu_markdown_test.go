package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func TestFeishuPostMessageChunks_empty(t *testing.T) {
	chunks, err := feishuPostMessageChunks("")
	if err != nil || len(chunks) != 1 {
		t.Fatal(err, len(chunks))
	}
	var env feishuPostEnvelopeStruct
	if err := json.Unmarshal([]byte(chunks[0]), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.ZhCN.Content) == 0 {
		t.Fatal()
	}
}

func TestFeishuPostMessageChunks_basic(t *testing.T) {
	md := "# Title\n\nHello **bold** and [link](https://example.com).\n\n- a\n- b\n"
	chunks, err := feishuPostMessageChunks(md)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected chunks")
	}
	var env feishuPostEnvelopeStruct
	if err := json.Unmarshal([]byte(chunks[0]), &env); err != nil {
		t.Fatal(err)
	}
	if env.ZhCN.Title != "Title" {
		t.Errorf("title = %q", env.ZhCN.Title)
	}
	if len(env.ZhCN.Content) < 2 {
		t.Errorf("expected at least paragraph + list rows, got %d", len(env.ZhCN.Content))
	}
}

func TestFeishuPostMessageChunks_listOnly(t *testing.T) {
	src := []byte("- a\n- b\n")
	doc := feishuMarkdownParser.Parser().Parse(text.NewReader(src))
	var kinds []string
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			kinds = append(kinds, fmt.Sprintf("%T", n))
		}
		return ast.WalkContinue, nil
	})
	t.Log(kinds)
	chunks, err := feishuPostMessageChunks(string(src))
	if err != nil {
		t.Fatal(err)
	}
	var env feishuPostEnvelopeStruct
	_ = json.Unmarshal([]byte(chunks[0]), &env)
	if len(env.ZhCN.Content) < 2 {
		t.Fatalf("list rows got %d", len(env.ZhCN.Content))
	}
}

func TestFeishuPostMessageChunks_tableBlockquoteCode(t *testing.T) {
	md := "## Sub\n\n| ColA | ColB |\n| --- | --- |\n| 1 | 2 |\n\n> Hello **bold** in quote.\n\n```\nline1\nline2\n```\n\n---\n"
	chunks, err := feishuPostMessageChunks(md)
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.Join(chunks, "")
	if !strings.Contains(raw, "ColA") || !strings.Contains(raw, "Hello") || !strings.Contains(raw, "line1") {
		t.Fatalf("unexpected payload: %s", raw)
	}
}

func TestFeishuPostMessageChunks_splitLong(t *testing.T) {
	md := strings.Repeat("x", feishuPostContentMaxBytes*2)
	chunks, err := feishuPostMessageChunks(md)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected split into multiple posts, got %d", len(chunks))
	}
	for i, c := range chunks {
		if len(c) > feishuPostContentMaxBytes+1024 {
			t.Errorf("chunk %d too large: %d", i, len(c))
		}
	}
}

func TestFeishuPostMessageChunks_inlineRich(t *testing.T) {
	md := "![duck](https://x.test/a.png)\n\n" +
		"**bold** *italic* `code` and [u](https://z)\n\n" +
		"line1  \nline2\n\n" +
		"<b>raw</b>\n\n" +
		"1. one\n2. two\n"
	chunks, err := feishuPostMessageChunks(md)
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.Join(chunks, "")
	if !strings.Contains(raw, "[图片:") || !strings.Contains(raw, "raw") {
		t.Fatal(raw[:min(200, len(raw))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
