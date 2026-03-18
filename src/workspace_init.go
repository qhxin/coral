package main

import (
	"errors"
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

// resolveWorkspaceDir 解析最终的 workspace 目录，若未指定则基于可执行文件所在目录。
func resolveWorkspaceDir() (string, error) {
	ws, err := parseWorkspacePath(os.Args[1:])
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
	return agentPath, userPath, memoryPath, nil
}

// readFileAsString 读取文本文件为字符串。
func readFileAsString(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

