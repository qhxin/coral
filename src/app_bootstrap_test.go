package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func resetTestLogger() {
	if dl, ok := log.Writer().(*dailyFileLogger); ok {
		_ = dl.Close()
	}
	log.SetOutput(os.Stderr)
}

func TestBootstrapAgentWithWorkspaceResolve(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(resetTestLogger)
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("LLAMA_SERVER_ENDPOINT", "")
	t.Setenv("OPENAI_MODEL", "test-model")
	t.Cleanup(func() {
		_ = os.Unsetenv("OPENAI_BASE_URL")
		_ = os.Unsetenv("LLAMA_SERVER_ENDPOINT")
		_ = os.Unsetenv("OPENAI_MODEL")
	})

	agent, ws, err := bootstrapAgentWithWorkspaceResolve(func() (string, error) {
		return dir, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if ws != dir || agent == nil || agent.FS == nil || agent.FS.Root != dir {
		t.Fatal(ws, agent)
	}
	if agent.Client == nil || agent.Client.Model != "test-model" {
		t.Fatal(agent.Client)
	}
	agentMd := filepath.Join(dir, "AGENT.md")
	if b, err := os.ReadFile(agentMd); err != nil || len(b) == 0 {
		t.Fatal(err)
	}
}

func TestBootstrapAgent_viaOsArgs(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(resetTestLogger)
	old := os.Args
	t.Cleanup(func() { os.Args = old })
	os.Args = []string{"coral", "-w", dir}
	t.Setenv("OPENAI_MODEL", "m-args")
	agent, ws, err := bootstrapAgent()
	if err != nil {
		t.Fatal(err)
	}
	if ws != dir || agent.Client.Model != "m-args" {
		t.Fatal(ws, agent.Client.Model)
	}
}

func TestResolveWorkspaceDir(t *testing.T) {
	tmp := t.TempDir()
	old := os.Args
	t.Cleanup(func() { os.Args = old })
	os.Args = []string{"bin", "--workspace", tmp}
	dir, err := resolveWorkspaceDir()
	if err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(tmp)
	if dir != abs {
		t.Fatalf("got %q", dir)
	}
}

func TestRunCLIPrompt_exitLine(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldIn := os.Stdin
	t.Cleanup(func() {
		os.Stdin = oldIn
		_ = r.Close()
	})
	os.Stdin = r
	go func() {
		_, _ = w.WriteString("/exit\n")
		_ = w.Close()
	}()
	runCLIPrompt(&AgentCore{}, t.TempDir())
}

func TestBootstrapAgent_readFilesFallback(t *testing.T) {
	dir := t.TempDir()
	agentPath := filepath.Join(dir, "AGENT.md")
	_ = os.WriteFile(agentPath, []byte("custom agent"), 0o644)
	userPath := filepath.Join(dir, "USER.md")
	_ = os.Remove(userPath)
	memPath := filepath.Join(dir, "MEMORY.md")
	_ = os.Remove(memPath)
	content, err := readFileAsString(agentPath)
	if err != nil || !strings.Contains(content, "custom") {
		t.Fatal(err, content)
	}
}
