package main

import (
	"io"
	"net/http"
	"net/http/httptest"
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

func TestHandleWithSessionWithMedia_imagePayload(t *testing.T) {
	var posted string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "chat/completions") {
			http.NotFound(w, r)
			return
		}
		b, _ := io.ReadAll(r.Body)
		posted = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, completionJSON("识别完成", ""))
	}))
	defer srv.Close()
	a := NewAgentCore(newStubOpenAIClient(t, srv.URL, "vision-model", 1), "sys", "", nil, 8000, 500)
	img := []byte{0xff, 0xd8, 0xff, 0xe0} // JPEG SOI
	_, err := a.HandleWithSessionWithMedia("v1", "这是什么", []UserImage{{Data: img}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(posted, "image_url") || !strings.Contains(posted, "data:image/jpeg") {
		snippetLen := 200
		if len(posted) < snippetLen {
			snippetLen = len(posted)
		}
		t.Fatalf("expected data jpeg in request: %s", posted[:snippetLen])
	}
}

func TestHandleWithSessionWithMedia_validation(t *testing.T) {
	srv := newTestOpenAIServer(t, completionJSON("x", ""))
	defer srv.Close()
	a := NewAgentCore(newStubOpenAIClient(t, srv.URL, "m", 1), "", "", nil, 100, 50)
	if _, err := a.HandleWithSessionWithMedia("s", "", nil); err == nil {
		t.Fatal("expected empty")
	}
	if _, err := a.HandleWithSessionWithMedia("s", "hi", []UserImage{{Data: nil}}); err == nil {
		t.Fatal("expected empty image")
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

func TestSimpleMsgsToOpenAI_empty(t *testing.T) {
	if simpleMsgsToOpenAI(nil, "", nil) != nil {
		t.Fatal()
	}
}

func TestHandleWithSessionWithMedia_toolThenAnswer(t *testing.T) {
	tc := `[{"id":"t1","type":"function","function":{"name":"workspace_read_file","arguments":"{\"path\":\"\"}"}}]`
	srv := newTestOpenAIServer(t, completionJSON("", tc), completionJSON("done", ""))
	defer srv.Close()
	root := t.TempDir()
	fs := &WorkspaceFS{Root: root}
	_ = os.WriteFile(filepath.Join(root, "AGENT.md"), []byte("x"), 0o644)
	cli := newStubOpenAIClient(t, srv.URL, "m", 2)
	a := NewAgentCore(cli, "sys", "", fs, 8000, 500)
	reply, err := a.HandleWithSessionWithMedia("sid", "hi", nil)
	if err != nil || reply != "done" {
		t.Fatal(err, reply)
	}
}

func TestHandleWithSessionWithMedia_imageOnlyUsesVisionPrompt(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, completionJSON("纯图", ""))
	}))
	t.Cleanup(srv.Close)
	a := NewAgentCore(newStubOpenAIClient(t, srv.URL, "m", 1), "", "", nil, 8000, 500)
	img := []byte{0xff, 0xd8, 0xff, 0xe0}
	reply, err := a.HandleWithSessionWithMedia("v", "", []UserImage{{Data: img}})
	if err != nil || reply != "纯图" {
		t.Fatal(err, reply)
	}
	if !strings.Contains(body, "image_url") {
		t.Fatal("expected vision payload")
	}
}
