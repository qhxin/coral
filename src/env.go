package main

import (
	"os"
	"strconv"
)

// EnvVarHelp 描述一个环境变量及其说明。
type EnvVarHelp struct {
	Name        string // 环境变量名，例如 "OPENAI_BASE_URL"
	Description string // 说明文本（不含注释前缀）
}

// envVarHelps 在构建时由构建脚本 shells/build-all.sh 从 .env.template 解析并生成的 env_help_gen.go 中填充。
// 注意：env_help_gen.go 为生成文件，请勿手工修改其中的 envVarHelps 内容。
// 需要调整环境变量说明时，请编辑 .env.template 并通过 shells/build-all.sh 重新构建。
var envVarHelps []EnvVarHelp

// envOrDefault 从环境变量读取值，若为空则返回默认值。
func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envIntOrDefault 从环境变量读取整数值，若为空或解析失败则返回默认值。
func envIntOrDefault(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// llmConcurrencyWindowFromEnv 读取并校验用于控制并发窗口的环境变量。
// 默认值在模块内固定为 1，避免并发窗口被外部误配导致系统负载不可控。
func llmConcurrencyWindowFromEnv() int {
	const defaultVal = 1
	n := envIntOrDefault("LLM_CONCURRENCY_WINDOW", defaultVal)
	if n <= 0 {
		return defaultVal
	}
	return n
}

