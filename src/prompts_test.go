package main

import (
	"strings"
	"testing"
)

func TestDefaultPromptConstants(t *testing.T) {
	if len(defaultAgent) < 100 || !strings.Contains(defaultAgent, "workspace_read_file") {
		t.Fatal("defaultAgent")
	}
	if !strings.Contains(defaultUser, "User Profile") {
		t.Fatal("defaultUser")
	}
	if !strings.Contains(defaultMemory, "Long-term Memory") {
		t.Fatal("defaultMemory")
	}
}
