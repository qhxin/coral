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

// bootstrapAgent 解析 workspace、初始化日志与 OpenAI 客户端，构造 AgentCore。
func bootstrapAgent() (*AgentCore, string, error) {
	return bootstrapAgentWithWorkspaceResolve(resolveWorkspaceDir)
}

// bootstrapAgentWithWorkspaceResolve 与 bootstrapAgent 相同，但允许注入 workspace 解析函数（用于测试）。
func bootstrapAgentWithWorkspaceResolve(resolve func() (string, error)) (*AgentCore, string, error) {
	workspaceDir, err := resolve()
	if err != nil {
		return nil, "", fmt.Errorf("解析 workspace 目录: %w", err)
	}

	logsDir := filepath.Join(workspaceDir, "logs")
	log.SetOutput(newDailyFileLogger(logsDir))
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	agentPath, userPath, memoryPath, err := initWorkspace(workspaceDir)
	if err != nil {
		return nil, "", fmt.Errorf("初始化 workspace: %w", err)
	}

	fs := &WorkspaceFS{Root: workspaceDir}

	agentContent, err := readFileAsString(agentPath)
	if err != nil {
		agentContent = defaultAgent
	}
	userContent, err := readFileAsString(userPath)
	if err != nil {
		userContent = ""
	}
	memoryContent, err := readFileAsString(memoryPath)
	if err != nil {
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
	return agent, workspaceDir, nil
}

func runCLIPrompt(agent *AgentCore, workspaceDir string) {
	fmt.Println("Minimal Go Agent (OpenAI JSON + local llama-server/Qwen)")
	fmt.Printf("Workspace 目录: %s\n", workspaceDir)
	fmt.Println("配置文件: AGENT.md / USER.md / MEMORY.md 均位于该目录下。")
	fmt.Println("Agent 会在需要时通过 OpenAI tools 协议调用文件系统工具（读取/写入 workspace 内的文件）。")
	fmt.Println("环境变量：OPENAI_BASE_URL / OPENAI_MODEL / OPENAI_API_KEY（兼容 LLAMA_SERVER_ENDPOINT / LLAMA_MODEL / LLAMA_AUTH_TOKEN）。")
	fmt.Println("多模态：`/img 路径 你的问题` 或 `/img \"含空格路径\" 问题`（模型须支持视觉）。")
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

		if imgPath, imgPrompt, ok := parseCLIImgLine(line); ok {
			data, err := os.ReadFile(imgPath)
			if err != nil {
				fmt.Println("错误:", err)
				continue
			}
			fmt.Println("已接收图片，处理中...")
			reply, err := agent.HandleWithSessionWithMedia("cli-default", imgPrompt, []UserImage{{Data: data}})
			if err != nil {
				fmt.Println("错误:", err)
				continue
			}
			fmt.Println()
			fmt.Println(reply)
			fmt.Println()
			continue
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
