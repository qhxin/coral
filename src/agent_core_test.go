package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewAgentCore_nilFS(t *testing.T) {
	srv := newTestOpenAIServer(t, completionJSON("ok", ""))
	defer srv.Close()
	c := newStubOpenAIClient(t, srv.URL, "m", 1)
	a := NewAgentCore(c, "s", "u", nil, 100, 50)
	if len(a.Tools) != 0 || len(a.Executors) != 0 || a.FS != nil {
		t.Fatal(len(a.Tools), len(a.Executors), a.FS)
	}
}

func TestAsOpenAITools_badSchema(t *testing.T) {
	srv := newTestOpenAIServer(t, completionJSON("", ""))
	defer srv.Close()
	a := NewAgentCore(newStubOpenAIClient(t, srv.URL, "m", 1), "", "", nil, 0, 0)
	a.Tools = []Tool{{
		Name:                 "bad",
		Description:          "d",
		ParametersJSONSchema: `{not json`,
	}}
	tools := a.asOpenAITools()
	if len(tools) != 1 {
		t.Fatal(len(tools))
	}
}

func TestHandleWithSession_errorsAndHappyPath(t *testing.T) {
	srv := newTestOpenAIServer(t, completionJSON("回答", ""))
	defer srv.Close()
	root := t.TempDir()
	fs := &WorkspaceFS{Root: root}
	_ = os.WriteFile(filepath.Join(root, "AGENT.md"), []byte("#"), 0o644)
	cli := newStubOpenAIClient(t, srv.URL, "model", 2)
	a := NewAgentCore(cli, "system prompt", "user profile", fs, 8000, 500)

	if _, err := a.HandleWithSession("sid", "   "); err == nil {
		t.Fatal("expected empty input")
	}

	reply, err := a.HandleWithSession("sid", "  hello  ")
	if err != nil || reply != "回答" {
		t.Fatal(err, reply)
	}
}

func TestHandle_delegatesToDefaultSession(t *testing.T) {
	srv := newTestOpenAIServer(t, completionJSON("cli-ok", ""))
	defer srv.Close()
	a := NewAgentCore(newStubOpenAIClient(t, srv.URL, "m", 1), "s", "", nil, 8000, 500)
	reply, err := a.Handle("ping")
	if err != nil || reply != "cli-ok" {
		t.Fatal(err, reply)
	}
}

func TestHandleWithSession_toolRound(t *testing.T) {
	tc := `[{"id":"t1","type":"function","function":{"name":"workspace_read_file","arguments":"{\"path\":\"AGENT.md\"}"}}]`
	srv := newTestOpenAIServer(t, completionJSON("", tc), completionJSON("读完", ""))
	defer srv.Close()
	root := t.TempDir()
	fs := &WorkspaceFS{Root: root}
	_ = os.WriteFile(filepath.Join(root, "AGENT.md"), []byte("file-body"), 0o644)
	cli := newStubOpenAIClient(t, srv.URL, "model", 2)
	a := NewAgentCore(cli, "sys", "", fs, 8000, 500)
	reply, err := a.HandleWithSession("tid", "read")
	if err != nil || !strings.Contains(reply, "读完") {
		t.Fatal(err, reply)
	}
}
