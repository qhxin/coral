package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

func main() {
	// 优先处理 --help/-h 请求，打印帮助后直接退出。
	if isHelpRequested(os.Args[1:]) {
		printHelp()
		return
	}

	workspaceDir, err := resolveWorkspaceDir()
	if err != nil {
		fmt.Println("解析 workspace 目录失败:", err)
		return
	}

	// 初始化按天归档的文件日志到 workspace/logs 目录。
	logsDir := filepath.Join(workspaceDir, "logs")
	log.SetOutput(newDailyFileLogger(logsDir))
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	agentPath, userPath, memoryPath, err := initWorkspace(workspaceDir)
	if err != nil {
		fmt.Println("初始化 workspace 失败:", err)
		return
	}

	fs := &WorkspaceFS{Root: workspaceDir}

	agentContent, err := readFileAsString(agentPath)
	if err != nil {
		fmt.Println("读取 AGENT.md 失败，将使用内置默认系统提示。错误:", err)
		agentContent = defaultAgent
	}

	userContent, err := readFileAsString(userPath)
	if err != nil {
		fmt.Println("读取 USER.md 失败:", err)
		userContent = ""
	}

	memoryContent, err := readFileAsString(memoryPath)
	if err != nil {
		fmt.Println("读取 MEMORY.md 失败:", err)
		memoryContent = ""
	}

	systemContent := agentContent + "\n\n" + memoryContent
	userProfile := userContent

	baseURL := envOrDefault("OPENAI_BASE_URL", envOrDefault("LLAMA_SERVER_ENDPOINT", "http://localhost:8080/v1"))
	model := envOrDefault("OPENAI_MODEL", envOrDefault("LLAMA_MODEL", "Qwen3.5-9B"))
	apiKey := envOrDefault("OPENAI_API_KEY", os.Getenv("LLAMA_AUTH_TOKEN"))
	maxContextTokens := envIntOrDefault("AGENT_MAX_CONTEXT_TOKENS", 0)
	maxOutputTokens := envIntOrDefault("AGENT_MAX_OUTPUT_TOKENS", 0)

	concurrencyWindow := llmConcurrencyWindowFromEnv()

	opts := []option.RequestOption{}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	client := openai.NewClient(opts...)

	openaiClient := &OpenAIClient{
		Client:  client,
		Model:   model,
		Limiter: newLLMRequestLimiter(concurrencyWindow),
	}

	agent := NewAgentCore(openaiClient, systemContent, userProfile, fs, maxContextTokens, maxOutputTokens)

	fmt.Println("Minimal Go Agent (OpenAI JSON + local llama-server/Qwen)")
	fmt.Printf("Workspace 目录: %s\n", workspaceDir)
	fmt.Println("配置文件: AGENT.md / USER.md / MEMORY.md 均位于该目录下。")
	fmt.Println("Agent 会在需要时通过 OpenAI tools 协议调用文件系统工具（读取/写入 workspace 内的文件）。")
	fmt.Println("环境变量：OPENAI_BASE_URL / OPENAI_MODEL / OPENAI_API_KEY（兼容 LLAMA_SERVER_ENDPOINT / LLAMA_MODEL / LLAMA_AUTH_TOKEN）。")
	fmt.Println("输入内容并回车，与模型对话。输入 /exit 退出。")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line == "/exit" {
			fmt.Println("再见。")
			return
		}

		reply, err := agent.Handle(line)
		if err != nil {
			fmt.Println("错误:", err)
			continue
		}

		fmt.Println()
		fmt.Println(reply)
		fmt.Println()
	}

	if err := scanner.Err(); err != nil {
		fmt.Println("读取输入错误:", err)
	}
}

