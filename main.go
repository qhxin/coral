package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// LlamaClient 封装与本地 llama-server 的 HTTP 通信。
type LlamaClient struct {
	Endpoint string
	Model    string
	Client   *http.Client
	AuthToken string
}

// chatRequest / chatMessage / chatResponse 仅用于与 llama-server 之间的 JSON 通信。
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

// Complete 接收一个 Markdown prompt，向 llama-server 发起请求并返回模型回复的 Markdown 文本。
func (lc *LlamaClient) Complete(markdownPrompt string) (string, error) {
	if lc == nil {
		return "", errors.New("llama client is nil")
	}
	if lc.Endpoint == "" {
		return "", errors.New("llama endpoint is empty")
	}
	if lc.Model == "" {
		return "", errors.New("llama model is empty")
	}

	reqBody := chatRequest{
		Model: lc.Model,
		Messages: []chatMessage{
			{Role: "user", Content: markdownPrompt},
		},
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequest(http.MethodPost, lc.Endpoint, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if lc.AuthToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+lc.AuthToken)
	}

	resp, err := lc.Client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("llama-server status %d: %s", resp.StatusCode, string(body))
	}

	var cr chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return "", err
	}

	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("empty choices from llama-server")
	}

	return cr.Choices[0].Message.Content, nil
}

// AgentCore 使用 Markdown 管理系统提示和对话历史。
type AgentCore struct {
	SystemPrompt string
	History      string
	Llama        *LlamaClient
}

// NewAgentCore 创建一个新的 AgentCore。
func NewAgentCore(llama *LlamaClient) *AgentCore {
	return &AgentCore{
		SystemPrompt: "# System\n你是一个命令行 Agent，只输出 Markdown。\n",
		History:      "# Conversation\n",
		Llama:        llama,
	}
}

// Handle 处理一轮用户输入，返回本轮模型回复的 Markdown 文本。
func (a *AgentCore) Handle(userInput string) (string, error) {
	userInput = strings.TrimSpace(userInput)
	if userInput == "" {
		return "", fmt.Errorf("empty input")
	}

	// 1. 追加本轮用户输入到 Markdown 历史中。
	a.History += fmt.Sprintf("\n## user\n%s\n", userInput)

	// 2. 构造完整的 prompt。
	fullPrompt := a.SystemPrompt + "\n" + a.History +
		"\n\n请以 Markdown 形式回复本轮 assistant 的内容。"

	// 3. 调用 llama-server。
	reply, err := a.Llama.Complete(fullPrompt)
	if err != nil {
		return "", err
	}

	// 4. 将模型回复保存到历史中。
	a.History += fmt.Sprintf("\n## assistant\n%s\n", reply)

	return reply, nil
}

// envOrDefault 从环境变量读取值，若为空则返回默认值。
func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	endpoint := envOrDefault("LLAMA_SERVER_ENDPOINT", "http://localhost:8080/v1/chat/completions")
	model := envOrDefault("LLAMA_MODEL", "Qwen3.5-9B")
	authToken := os.Getenv("LLAMA_AUTH_TOKEN")

	llama := &LlamaClient{
		Endpoint:  endpoint,
		Model:     model,
		AuthToken: authToken,
		Client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}

	agent := NewAgentCore(llama)

	fmt.Println("Minimal Go Agent (Markdown + local llama-server)")
	fmt.Println("环境变量：LLAMA_SERVER_ENDPOINT / LLAMA_MODEL 可覆盖默认配置。")
	fmt.Println("输入内容并回车，与模型对话。输入 /exit 退出。")

	// 未来扩展点示例（HTTP webhook / 飞书 WebSocket）：
	// - HTTP webhook:
	//   使用 net/http 启动一个 HTTP 服务器，在处理函数中读取请求体文本，
	//   调用 agent.Handle(...) 获取 Markdown，然后将其写回响应。
	// - 飞书 WebSocket:
	//   在飞书 SDK 的回调中获取用户发送的文本，同样通过 agent.Handle(...)
	//   获得 Markdown 回复，再通过 SDK 发送到对应会话。

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

