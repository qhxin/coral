package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var envKeyPattern = regexp.MustCompile(`^[A-Z0-9_]+$`)

// osExecutableFn 允许测试注入可执行文件路径。
var osExecutableFn = os.Executable

// loadDotenvFromExecutableDir 从可执行文件同目录加载 .env（若存在）。
// 仅补充缺失环境变量，不覆盖进程中已存在值。
func loadDotenvFromExecutableDir() error {
	exePath, err := osExecutableFn()
	if err != nil {
		return err
	}
	dotenvPath := filepath.Join(filepath.Dir(exePath), ".env")
	return loadDotenvFileIfPresent(dotenvPath)
}

// loadDotenvFileIfPresent 安全读取 .env：
// - 忽略空行和注释行
// - 仅接受 KEY=VALUE 且 KEY 匹配 [A-Z0-9_]+
// - 不覆盖已存在环境变量
func loadDotenvFileIfPresent(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.ReplaceAll(scanner.Text(), "\r", ""))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if !envKeyPattern.MatchString(key) {
			continue
		}
		value = strings.TrimSpace(value)
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set env %s: %w", key, err)
		}
	}
	return scanner.Err()
}

