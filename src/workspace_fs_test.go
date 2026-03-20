package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceFS_ReadWriteAppend(t *testing.T) {
	root := t.TempDir()
	fs := &WorkspaceFS{Root: root}

	if err := fs.Write("a/b.txt", "hello"); err != nil {
		t.Fatal(err)
	}
	s, err := fs.Read("a/b.txt")
	if err != nil || s != "hello" {
		t.Fatalf("read=%q err=%v", s, err)
	}
	if err := fs.Append("a/b.txt", " world"); err != nil {
		t.Fatal(err)
	}
	s, err = fs.Read("a/b.txt")
	if err != nil || s != "hello world" {
		t.Fatalf("append read=%q", s)
	}
}

func TestWorkspaceFS_resolveRel_escape(t *testing.T) {
	fs := &WorkspaceFS{Root: t.TempDir()}
	_, err := fs.resolveRel("")
	if err == nil {
		t.Fatal("expected error empty path")
	}
	_, err = fs.resolveRel(".." + string(filepath.Separator) + "etc" + string(filepath.Separator) + "passwd")
	if err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("expected escape error, got %v", err)
	}
}

func TestWorkspaceFS_Write_rootIsFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := &WorkspaceFS{Root: f}
	if err := fs.Write("child.txt", "a"); err == nil {
		t.Fatal("expected mkdir error")
	}
}

func TestWorkspaceFS_Read_missing(t *testing.T) {
	fs := &WorkspaceFS{Root: t.TempDir()}
	_, err := fs.Read("nope.txt")
	if err == nil || !os.IsNotExist(err) {
		t.Fatalf("got %v", err)
	}
}
