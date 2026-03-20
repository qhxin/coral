package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseWorkspacePath(t *testing.T) {
	tmp := t.TempDir()
	abs, err := filepath.Abs(tmp)
	if err != nil {
		t.Fatal(err)
	}
	got, err := parseWorkspacePath([]string{"--workspace=" + tmp})
	if err != nil || got != abs {
		t.Fatalf("got %q err %v", got, err)
	}
	got, err = parseWorkspacePath([]string{"--workspace", tmp})
	if err != nil || got != abs {
		t.Fatalf("flag form got %q", got)
	}
	got, err = parseWorkspacePath([]string{"-w", tmp})
	if err != nil || got != abs {
		t.Fatalf("-w got %q", got)
	}
	got, err = parseWorkspacePath([]string{"-w"})
	if err != nil || got != "" {
		t.Fatalf("missing value got %q", got)
	}
}

func TestParseFeishuMode(t *testing.T) {
	if !parseFeishuMode([]string{"--feishu"}) {
		t.Fatal()
	}
	if parseFeishuMode([]string{"--feishu-x"}) {
		t.Fatal()
	}
}

func TestResolveWorkspaceDirFromArgs_explicit(t *testing.T) {
	tmp := t.TempDir()
	dir, err := resolveWorkspaceDirFromArgs([]string{"-w", tmp})
	if err != nil {
		t.Fatal(err)
	}
	if abs, err := filepath.Abs(tmp); err != nil || dir != abs {
		t.Fatalf("dir=%q want %q", dir, abs)
	}
}

func TestInitWorkspace_createsDefaults(t *testing.T) {
	dir := t.TempDir()
	agent, user, mem, err := initWorkspace(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{agent, user, mem} {
		b, err := os.ReadFile(p)
		if err != nil || len(b) == 0 {
			t.Fatalf("file %s: %v", p, err)
		}
	}
	// 再次调用不应覆盖已有文件
	if err := os.WriteFile(agent, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := initWorkspace(dir); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(agent)
	if string(b) != "keep" {
		t.Fatal("expected existing AGENT.md unchanged")
	}
}

func TestReadFileAsString(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.txt")
	if err := os.WriteFile(p, []byte("ab"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := readFileAsString(p)
	if err != nil || s != "ab" {
		t.Fatal(err, s)
	}
	_, err = readFileAsString(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal()
	}
}

func TestResolveWorkspaceDirFromArgs_defaultUsesExe(t *testing.T) {
	dir, err := resolveWorkspaceDirFromArgs([]string{})
	if err != nil || !strings.HasSuffix(dir, "workspace") {
		t.Fatalf("dir=%q err=%v", dir, err)
	}
}
