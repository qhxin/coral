package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSkillRegistryLoadAndExecute(t *testing.T) {
	workspace := t.TempDir()
	fs := &WorkspaceFS{Root: workspace}
	if err := fs.Write("hello.txt", "world"); err != nil {
		t.Fatal(err)
	}

	skillsRoot := t.TempDir()
	mustWriteSkill := func(rel, content string) {
		p := filepath.Join(skillsRoot, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mustWriteSkill(filepath.Join("filesystem", "workspace_read_file.md"), skillMarkdown("workspace_read_file", "读取文件", "path", true))
	mustWriteSkill(filepath.Join("filesystem", "workspace_write_file.md"), skillMarkdown("workspace_write_file", "写入文件", "path", true))
	mustWriteSkill(filepath.Join("memory", "memory_write_important.md"), skillMarkdown("memory_write_important", "memory remember important notes", "content", true))
	// No builtin handler for unknown namespace; should be skipped.
	mustWriteSkill(filepath.Join("unknown", "noop.md"), skillMarkdown("noop", "nothing", "x", false))

	r := NewSkillRegistry(fs)
	r.RegisterBuiltinHandlers()
	if err := r.LoadFromDir(skillsRoot); err != nil {
		t.Fatal(err)
	}

	if _, ok := r.Get("workspace_read_file"); !ok {
		t.Fatal("workspace_read_file should be loaded")
	}
	if _, ok := r.Get("noop"); ok {
		t.Fatal("noop should not be loaded without handler")
	}
	if len(r.List()) < 3 {
		t.Fatalf("expected at least 3 loaded skills, got %d", len(r.List()))
	}

	tools, execs := r.ToTools()
	if len(tools) < 3 {
		t.Fatalf("expected tools converted from skills, got %d", len(tools))
	}

	readOut, err := execs["workspace_read_file"](json.RawMessage(`{"path":"hello.txt"}`))
	if err != nil || readOut != "world" {
		t.Fatalf("read executor failed: out=%q err=%v", readOut, err)
	}

	writeOut, err := execs["workspace_write_file"](json.RawMessage(`{"path":"new.txt","content":"abc"}`))
	if err != nil || writeOut != "写入成功" {
		t.Fatalf("write executor failed: out=%q err=%v", writeOut, err)
	}
	if got, err := fs.Read("new.txt"); err != nil || got != "abc" {
		t.Fatalf("write result mismatch: got=%q err=%v", got, err)
	}

	memOut, err := execs["memory_write_important"](json.RawMessage(`{"content":"remember this"}`))
	if err != nil || memOut == "" {
		t.Fatalf("memory executor failed: out=%q err=%v", memOut, err)
	}
	memFile, err := fs.Read("MEMORY.md")
	if err != nil || memFile == "" {
		t.Fatalf("expected MEMORY.md content, err=%v", err)
	}
}

func TestSkillRegistryBuiltinHandlerValidation(t *testing.T) {
	r := NewSkillRegistry(&WorkspaceFS{Root: t.TempDir()})

	read := r.createReadFileHandler()
	if _, err := read(json.RawMessage(`not-json`)); err == nil {
		t.Fatal("expected read handler parse error")
	}
	if _, err := read(json.RawMessage(`{"path":""}`)); err == nil {
		t.Fatal("expected read handler required-path error")
	}

	write := r.createWriteFileHandler()
	if _, err := write(json.RawMessage(`not-json`)); err == nil {
		t.Fatal("expected write handler parse error")
	}
	if _, err := write(json.RawMessage(`{"path":""}`)); err == nil {
		t.Fatal("expected write handler required-path error")
	}

	mem := r.createMemoryWriteHandler()
	if _, err := mem(json.RawMessage(`not-json`)); err == nil {
		t.Fatal("expected memory handler parse error")
	}
	if _, err := mem(json.RawMessage(`{"content":"   "}`)); err == nil {
		t.Fatal("expected memory handler required-content error")
	}
}

func skillMarkdown(name, desc, field string, required bool) string {
	req := "optional"
	if required {
		req = "required"
	}
	return "# " + name + "\n\n" +
		"## Description\n" + desc + "\n\n" +
		"## Parameters\n" +
		"### " + field + " (" + req + ")\n" +
		"- **类型**: string\n" +
		"- **描述**: 参数描述\n\n" +
		"## Examples\n" +
		"### 示例1\n" +
		"场景: 示例场景\n" +
		"```json\n" +
		"{\"tool_calls\":[]}\n" +
		"```\n"
}

