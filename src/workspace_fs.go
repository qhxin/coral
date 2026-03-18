package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WorkspaceFS 限制所有文件操作都在 workspace 根目录之下。
type WorkspaceFS struct {
	Root string
}

// resolveRel 将传入的相对路径解析为 workspace 下的绝对路径，并防止越权访问。
func (fs *WorkspaceFS) resolveRel(rel string) (string, error) {
	if rel == "" {
		return "", fmt.Errorf("empty relative path")
	}
	joined := filepath.Join(fs.Root, rel)
	cleanRoot, err := filepath.Abs(fs.Root)
	if err != nil {
		return "", err
	}
	cleanJoined, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	relPath, err := filepath.Rel(cleanRoot, cleanJoined)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(relPath, "..") {
		return "", fmt.Errorf("path %q escapes workspace root", rel)
	}
	return cleanJoined, nil
}

// Read 读取 workspace 内相对路径文件的全部内容。
func (fs *WorkspaceFS) Read(rel string) (string, error) {
	full, err := fs.resolveRel(rel)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Write 覆盖写入 workspace 内相对路径文件的内容，必要时创建父目录。
func (fs *WorkspaceFS) Write(rel, content string) error {
	full, err := fs.resolveRel(rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, []byte(content), 0o644)
}

// Append 在 workspace 内相对路径文件末尾追加内容（若文件不存在则创建）。
func (fs *WorkspaceFS) Append(rel, content string) error {
	full, err := fs.resolveRel(rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(full, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

