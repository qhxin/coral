package main

import (
	"strings"
	"testing"
)

func TestBuildSystemPromptWithWorkspaceContext(t *testing.T) {
	pm := NewPromptManager("base")
	tools := []Tool{{Name: "workspace_read_file", Description: "read file"}}
	got := pm.BuildSystemPromptWithWorkspaceContext(
		tools,
		"- remembered fact",
		"# AGENT\nfollow policy",
		"# User Profile\n- Name: 雨中漫步",
	)

	expectContains := []string{
		"## Workspace Agent Policy (AGENT.md, authoritative)",
		"follow policy",
		"## Workspace User Profile (USER.md, authoritative)",
		"Name: 雨中漫步",
		"USER.md is authoritative for user identity",
		"Do not claim memory loss if USER.md already provides",
	}
	for _, sub := range expectContains {
		if !strings.Contains(got, sub) {
			t.Fatalf("prompt missing %q", sub)
		}
	}
}

func TestBuildSystemPromptWithRAGAndWorkspace(t *testing.T) {
	pm := NewPromptManager("base")
	rag := NewRAGMemory(nil)
	rag.entries = []MemoryEntry{
		{ID: "1", Content: "雨中漫步 使用中文"},
	}
	got := pm.BuildSystemPromptWithRAGAndWorkspace(
		nil,
		rag,
		"雨中漫步 语言",
		"agent-md-content",
		"user-md-content",
	)
	if !strings.Contains(got, "agent-md-content") || !strings.Contains(got, "user-md-content") {
		t.Fatal("workspace context should be present in RAG prompt")
	}
}

