package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// parseWorkspacePath 从命令行参数中解析 workspace 目录（可能为空字符串）。
func parseWorkspacePath(args []string) (string, error) {
	var ws string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--workspace=") {
			ws = strings.TrimPrefix(arg, "--workspace=")
			break
		}
		if arg == "--workspace" || arg == "-w" {
			if i+1 < len(args) {
				ws = args[i+1]
				break
			}
		}
	}
	if ws == "" {
		return "", nil
	}
	abs, err := filepath.Abs(ws)
	if err != nil {
		return "", err
	}
	return abs, nil
}

// resolveWorkspaceDirFromArgs 根据 argv 切片解析 workspace；未指定时基于可执行文件目录。
func resolveWorkspaceDirFromArgs(args []string) (string, error) {
	ws, err := parseWorkspacePath(args)
	if err != nil {
		return "", err
	}
	if ws != "" {
		return ws, nil
	}
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	exeDir := filepath.Dir(exePath)
	return filepath.Join(exeDir, "workspace"), nil
}

// resolveWorkspaceDir 解析最终的 workspace 目录（使用 os.Args[1:]）。
func resolveWorkspaceDir() (string, error) {
	return resolveWorkspaceDirFromArgs(os.Args[1:])
}

// parseFeishuMode 判断是否启用飞书长连接模式。
func parseFeishuMode(args []string) bool {
	for _, arg := range args {
		if arg == "--feishu" {
			return true
		}
	}
	return false
}

// initWorkspace 创建 workspace 目录及关键文件，并返回三个文件路径。
func initWorkspace(dir string) (agentPath, userPath, memoryPath string, err error) {
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return "", "", "", err
	}
	agentPath = filepath.Join(dir, "AGENT.md")
	userPath = filepath.Join(dir, "USER.md")
	memoryPath = filepath.Join(dir, "MEMORY.md")

	if _, errStat := os.Stat(agentPath); errors.Is(errStat, os.ErrNotExist) {
		if err = os.WriteFile(agentPath, []byte(defaultAgent), 0o644); err != nil {
			return "", "", "", err
		}
	}
	if _, errStat := os.Stat(userPath); errors.Is(errStat, os.ErrNotExist) {
		if err = os.WriteFile(userPath, []byte(defaultUser), 0o644); err != nil {
			return "", "", "", err
		}
	}
	if _, errStat := os.Stat(memoryPath); errors.Is(errStat, os.ErrNotExist) {
		if err = os.WriteFile(memoryPath, []byte(defaultMemory), 0o644); err != nil {
			return "", "", "", err
		}
	}
	if err = initWorkspaceSkills(dir); err != nil {
		return "", "", "", err
	}
	return agentPath, userPath, memoryPath, nil
}

func initWorkspaceSkills(workspaceDir string) error {
	skillsDir := filepath.Join(workspaceDir, "skills")
	st, err := os.Stat(skillsDir)
	if err == nil {
		if !st.IsDir() {
			return fmt.Errorf("workspace skills path is not a directory: %s", skillsDir)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return fs.WalkDir(defaultSkillsFS, embeddedSkillsRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}

		rel := strings.TrimPrefix(path, embeddedSkillsRoot+"/")
		if rel == path || rel == "" {
			return nil
		}
		dst := filepath.Join(skillsDir, filepath.FromSlash(rel))
		if mkErr := os.MkdirAll(filepath.Dir(dst), 0o755); mkErr != nil {
			return mkErr
		}
		content, readErr := defaultSkillsFS.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if writeErr := os.WriteFile(dst, content, 0o644); writeErr != nil {
			return writeErr
		}
		return nil
	})
}

// readFileAsString 读取文本文件为字符串。
func readFileAsString(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

