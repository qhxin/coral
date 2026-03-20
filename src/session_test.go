package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSessionDir(t *testing.T) {
	if !strings.HasPrefix(sessionDir("ab/c"), filepath.Join("sessions", "c")) {
		t.Fatal(sessionDir("ab/c"))
	}
	if sessionDir("") != filepath.Join("sessions", "default") {
		t.Fatal(sessionDir(""))
	}
}

func TestWeeklyFilename(t *testing.T) {
	// 2026-03-20 is ISO week 12 of 2026
	ts := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	if weeklyFilename(ts) != "2026-03-W12.json" {
		t.Fatal(weeklyFilename(ts))
	}
}

func TestSummaryWindowDaysFromEnv(t *testing.T) {
	k := envSummaryWindowDays
	t.Setenv(k, "")
	if summaryWindowDaysFromEnv() != 0 {
		t.Fatal()
	}
	t.Setenv(k, "nope")
	if summaryWindowDaysFromEnv() != 0 {
		t.Fatal()
	}
	t.Setenv(k, "14")
	if summaryWindowDaysFromEnv() != 14 {
		t.Fatal()
	}
}

func TestReadSessionMessages(t *testing.T) {
	root := t.TempDir()
	fs := &WorkspaceFS{Root: root}
	msgs, err := readSessionMessages(fs, filepath.Join("sessions", "x", "none.json"))
	if err != nil || len(msgs) != 0 {
		t.Fatal(err, len(msgs))
	}
	p := "sessions/x/f.json"
	_ = fs.Write(p, "")
	msgs, err = readSessionMessages(fs, p)
	if err != nil || len(msgs) != 0 {
		t.Fatal(err)
	}
	_ = fs.Write(p, `not json`)
	if _, err := readSessionMessages(fs, p); err == nil {
		t.Fatal("expected json error")
	}
}

func TestNewUserAndAssistantMessage(t *testing.T) {
	ts := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	u := newUserMessage("hi", ts, nil)
	if u.Role != "user" || u.Metadata["timestamp"] != ts.Format(time.RFC3339) {
		t.Fatal(u)
	}
	meta := map[string]interface{}{"timestamp": "keep"}
	u2 := newUserMessage("h", ts, meta)
	if u2.Metadata["timestamp"] != "keep" {
		t.Fatal()
	}
	a := newAssistantMessage("a", ts, nil)
	if a.Role != "assistant" {
		t.Fatal(a)
	}
}

func TestCompactActiveMessages_noExpire_noClient(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -30)
	msgs := []ChatMessage{
		{Role: "system", Content: "s"},
		{Role: "user", Content: "recent", Metadata: map[string]interface{}{"timestamp": now.Format(time.RFC3339)}},
		{Role: "user", Content: "ancient", Metadata: map[string]interface{}{"timestamp": old.Format(time.RFC3339)}},
	}
	out, err := compactActiveMessages(nil, msgs, now)
	if err != nil || len(out) != 2 { // system dropped ancient without client? Actually loop: system always recent; ancient goes expired
		t.Fatalf("err=%v len=%d", err, len(out))
	}
	// without agent client, expired should be dropped and not summarized
	var foundAncient bool
	for _, m := range out {
		if m.Content == "ancient" {
			foundAncient = true
		}
	}
	if foundAncient {
		t.Fatal("expected ancient dropped when no client")
	}
}

func TestCompactActiveMessages_summarize(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -30)
	srv := newTestOpenAIServer(t, completionJSON("历史摘要", ""))
	defer srv.Close()
	cli := newStubOpenAIClient(t, srv.URL, "m", 2)
	agent := &AgentCore{
		Client:            cli,
		SummaryWindowDays: 7,
	}
	msgs := []ChatMessage{
		{Role: "user", Content: "过去的", Metadata: map[string]interface{}{"timestamp": old.Format(time.RFC3339)}},
	}
	out, err := compactActiveMessages(agent, msgs, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Role != "system" || !strings.Contains(out[0].Content, "历史摘要") {
		t.Fatalf("%+v", out)
	}
}

func TestCompactActiveMessages_whitespaceSummarySkipped(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -30)
	srv := newTestOpenAIServer(t, completionJSON("   ", ""))
	defer srv.Close()
	cli := newStubOpenAIClient(t, srv.URL, "m", 2)
	agent := &AgentCore{Client: cli, SummaryWindowDays: 7}
	msgs := []ChatMessage{
		{Role: "user", Content: "old", Metadata: map[string]interface{}{"timestamp": old.Format(time.RFC3339)}},
		{Role: "user", Content: "new", Metadata: map[string]interface{}{"timestamp": now.Format(time.RFC3339)}},
	}
	out, err := compactActiveMessages(agent, msgs, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Content != "new" {
		t.Fatalf("%+v", out)
	}
}

func TestSummarizeMessagesWithLLM_edge(t *testing.T) {
	now := time.Now()
	s, from, to, err := summarizeMessagesWithLLM(nil, nil, now)
	if err != nil || s != "" {
		t.Fatal(err, s)
	}
	if from != now || to != now {
		t.Fatal(from, to)
	}
}

func TestWriteSessionMessages_roundtrip(t *testing.T) {
	root := t.TempDir()
	fs := &WorkspaceFS{Root: root}
	p := "sessions/w/m.json"
	msgs := []ChatMessage{{Role: "user", Content: "c"}}
	if err := writeSessionMessages(fs, p, msgs); err != nil {
		t.Fatal(err)
	}
	got, err := readSessionMessages(fs, p)
	if err != nil || len(got) != 1 || got[0].Content != "c" {
		t.Fatal(err, got)
	}
}

func TestAppendToSessionFiles(t *testing.T) {
	root := t.TempDir()
	fs := &WorkspaceFS{Root: root}
	agent := &AgentCore{FS: fs}
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	u := newUserMessage("u", now, nil)
	a := newAssistantMessage("a", now, nil)
	if err := appendToSessionFiles(agent, "t1", u, a, now); err != nil {
		t.Fatal(err)
	}
	wpath := filepath.Join(sessionDir("t1"), weeklyFilename(now))
	b, err := os.ReadFile(filepath.Join(root, wpath))
	if err != nil || !strings.Contains(string(b), `"role": "user"`) {
		t.Fatal(err, string(b))
	}
	if err := appendToSessionFiles(nil, "t1", u, a, now); err != nil {
		t.Fatal(err)
	}
}
