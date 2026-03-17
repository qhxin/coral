package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// Tool 描述一个暴露给模型使用的工具（对齐 OpenAI tools 语义）。
type Tool struct {
	Name                 string
	Description          string
	ParametersJSONSchema string
}

// ToolResult 表示一次工具调用的结果。
type ToolResult struct {
	CallID  string
	Name    string
	Content string
	Error   string
}

// ToolExecutor 是具体工具执行函数的签名。
type ToolExecutor func(args json.RawMessage) (string, error)

// OpenAIClient 封装与 OpenAI 兼容后端的 JSON 协议交互。
type OpenAIClient struct {
	Client openai.Client
	Model  string
}

// ChatOnce 调用一次 chat.completions，传入 messages 与 tools，并可指定最大输出 token 数。
func (c *OpenAIClient) ChatOnce(ctx context.Context, messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolUnionParam, maxOutputTokens int) (*openai.ChatCompletion, error) {
	if c.Model == "" {
		return nil, errors.New("openai model is empty")
	}
	params := openai.ChatCompletionNewParams{
		Model:    c.Model,
		Messages: messages,
	}
	if maxOutputTokens > 0 {
		params.MaxTokens = openai.Int(int64(maxOutputTokens))
	}
	if len(tools) > 0 {
		params.Tools = tools
	}
	resp, err := c.Client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty choices from completion")
	}
	return resp, nil
}

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

// AgentCore 使用 OpenAI JSON 协议管理系统提示和对话历史。
type AgentCore struct {
	Client            *OpenAIClient
	FS                *WorkspaceFS
	Tools             []Tool
	Executors         map[string]ToolExecutor
	SystemContent     string
	UserProfile       string
	MaxContextTokens  int
	MaxOutputTokens   int
	SummaryWindowDays int
}

// NewAgentCore 创建一个新的 AgentCore，由外部注入 OpenAIClient 与初始上下文。
func NewAgentCore(client *OpenAIClient, systemContent, userProfile string, fs *WorkspaceFS, maxContextTokens, maxOutputTokens int) *AgentCore {
	var tools []Tool
	executors := make(map[string]ToolExecutor)
	if fs != nil {
		tools, executors = defaultFilesystemTools(fs)
	}
	return &AgentCore{
		Client:            client,
		FS:                fs,
		Tools:             tools,
		Executors:         executors,
		SystemContent:     systemContent,
		UserProfile:       userProfile,
		MaxContextTokens:  maxContextTokens,
		MaxOutputTokens:   maxOutputTokens,
		SummaryWindowDays: defaultSummaryWindowDays,
	}
}

// defaultFilesystemTools 构造基于 WorkspaceFS 的默认文件系统工具集合。
func defaultFilesystemTools(fs *WorkspaceFS) ([]Tool, map[string]ToolExecutor) {
	tools := []Tool{
		{
			Name:                 "workspace_read_file",
			Description:          "读取 workspace 目录下的相对路径文件内容。",
			ParametersJSONSchema: `{"type":"object","properties":{"path":{"type":"string","description":"要读取的相对路径，例如 AGENT.md"}},"required":["path"]}`,
		},
		{
			Name:                 "workspace_write_file",
			Description:          "覆盖写入 workspace 目录下的相对路径文件内容。",
			ParametersJSONSchema: `{"type":"object","properties":{"path":{"type":"string","description":"要写入的相对路径，例如 NOTES.md"},"content":{"type":"string","description":"要写入文件的完整文本内容"}},"required":["path","content"]}`,
		},
		{
			Name:                 "memory_write_important",
			Description:          "记录需要长期记住的重要信息到 MEMORY.md。",
			ParametersJSONSchema: `{"type":"object","properties":{"content":{"type":"string","description":"需要长期记住的重要信息"}},"required":["content"]}`,
		},
	}

	executors := map[string]ToolExecutor{
		"workspace_read_file": func(args json.RawMessage) (string, error) {
			var payload struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(args, &payload); err != nil {
				return "", fmt.Errorf("解析参数失败: %w", err)
			}
			if payload.Path == "" {
				return "", fmt.Errorf("path 不能为空")
			}
			return fs.Read(payload.Path)
		},
		"workspace_write_file": func(args json.RawMessage) (string, error) {
			var payload struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal(args, &payload); err != nil {
				return "", fmt.Errorf("解析参数失败: %w", err)
			}
			if payload.Path == "" {
				return "", fmt.Errorf("path 不能为空")
			}
			if err := fs.Write(payload.Path, payload.Content); err != nil {
				return "", err
			}
			return "写入成功", nil
		},
		"memory_write_important": func(args json.RawMessage) (string, error) {
			var payload struct {
				Content string `json:"content"`
			}
			if err := json.Unmarshal(args, &payload); err != nil {
				return "", fmt.Errorf("解析参数失败: %w", err)
			}
			if strings.TrimSpace(payload.Content) == "" {
				return "", fmt.Errorf("content 不能为空")
			}
			now := nowInShanghai()
			entry := fmt.Sprintf("\n\n## Memo at %s\n%s\n", now.Format(time.RFC3339), payload.Content)
			if err := fs.Append("MEMORY.md", entry); err != nil {
				return "", err
			}
			return "写入 MEMORY.md 成功", nil
		},
	}

	return tools, executors
}

// asOpenAITools 将内部 Tool 列表转换为 OpenAI tools 定义。
func (a *AgentCore) asOpenAITools() []openai.ChatCompletionToolUnionParam {
	if len(a.Tools) == 0 {
		return nil
	}
	out := make([]openai.ChatCompletionToolUnionParam, 0, len(a.Tools))
	for _, t := range a.Tools {
		out = append(out, openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
			Name:        t.Name,
			Description: openai.String(t.Description),
		}))
	}
	return out
}

// Handle 处理一轮用户输入，返回本轮模型回复的 Markdown 文本（使用默认会话）。
func (a *AgentCore) Handle(userInput string) (string, error) {
	return a.HandleWithSession("cli-default", userInput)
}

// HandleWithSession 按 session 维度处理一轮用户输入。
func (a *AgentCore) HandleWithSession(sessionID, userInput string) (string, error) {
	userInput = strings.TrimSpace(userInput)
	if userInput == "" {
		return "", fmt.Errorf("empty input")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 构造轻量消息：系统 + 用户档案 + 会话历史 + 本轮 user，用于 token 估算与裁剪。
	var simpleMsgs []SimpleMsg
	if strings.TrimSpace(a.SystemContent) != "" {
		simpleMsgs = append(simpleMsgs, SimpleMsg{Role: "system", Content: a.SystemContent})
	}
	if strings.TrimSpace(a.UserProfile) != "" {
		simpleMsgs = append(simpleMsgs, SimpleMsg{Role: "user", Content: a.UserProfile})
	}

	// 读取该 session 的 active.json 作为历史。
	if a.FS != nil {
		sDir := sessionDir(sessionID)
		activePath := filepath.Join(sDir, "active.json")
		history, err := readSessionMessages(a.FS, activePath)
		if err == nil {
			for _, m := range history {
				role := strings.TrimSpace(m.Role)
				if role == "" {
					continue
				}
				simpleMsgs = append(simpleMsgs, SimpleMsg{Role: role, Content: m.Content})
			}
		}
	}

	now := nowInShanghai()
	userMsg := newUserMessage(userInput, now, map[string]interface{}{
		"session_id": sessionID,
	})
	simpleMsgs = append(simpleMsgs, SimpleMsg{Role: "user", Content: userMsg.Content})

	// 在调用模型前做上下文长度控制（精确 token 计算）。
	effectiveLimit := a.MaxContextTokens
	if a.MaxOutputTokens > 0 && a.MaxContextTokens > a.MaxOutputTokens {
		effectiveLimit = a.MaxContextTokens - a.MaxOutputTokens
	}
	simpleMsgs = ensureContextWithinLimitSimple(simpleMsgs, effectiveLimit, a)

	// 将 SimpleMsg 转换为 OpenAI SDK 所需的 messages。
	var messages []openai.ChatCompletionMessageParamUnion
	for _, m := range simpleMsgs {
		switch m.Role {
		case "system":
			messages = append(messages, openai.SystemMessage(m.Content))
		case "assistant":
			messages = append(messages, openai.AssistantMessage(m.Content))
		default:
			// 其他一律按 user 处理，包括最初的 userProfile。
			messages = append(messages, openai.UserMessage(m.Content))
		}
	}

	tools := a.asOpenAITools()

	for {
		resp, err := a.Client.ChatOnce(ctx, messages, tools, a.MaxOutputTokens)
		if err != nil {
			return "", err
		}
		choice := resp.Choices[0]
		msg := choice.Message

		// 没有工具调用，直接返回最终回复
		if len(msg.ToolCalls) == 0 {
			finalReply := msg.Content
			if a.FS != nil {
				// 将本轮对话写入 session 存储。
				assistantMsg := newAssistantMessage(finalReply, now, map[string]interface{}{
					"session_id": sessionID,
				})
				_ = appendToSessionFiles(a, sessionID, userMsg, assistantMsg, now)
			}
			return finalReply, nil
		}

		// 执行工具并将结果追加为 tool 消息
		results := dispatchToolsOpenAI(msg.ToolCalls, a.Executors)
		for _, r := range results {
			content := r.Content
			if r.Error != "" {
				if content != "" {
					content += "\n"
				}
				content += "执行出错: " + r.Error
			}
			messages = append(messages, openai.ToolMessage(content, r.CallID))
		}
	}
}

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

// dispatchToolsOpenAI 按名称分发并执行 OpenAI tool_calls。
func dispatchToolsOpenAI(calls []openai.ChatCompletionMessageToolCallUnion, executors map[string]ToolExecutor) []ToolResult {
	results := make([]ToolResult, 0, len(calls))
	for _, c := range calls {
		fn := c.AsFunction()
		name := fn.Function.Name
		exec, ok := executors[name]
		if !ok {
			results = append(results, ToolResult{
				CallID:  fn.ID,
				Name:    name,
				Content: "",
				Error:   fmt.Sprintf("未知工具: %s", name),
			})
			continue
		}

		out, err := exec(json.RawMessage(fn.Function.Arguments))
		res := ToolResult{
			CallID:  fn.ID,
			Name:    name,
			Content: out,
		}
		if err != nil {
			res.Error = err.Error()
		}
		results = append(results, res)
	}
	return results
}

// 默认模板内容。
const defaultAgent = "# System\n" +
	"你是一个命令行 Agent，输入输出都是标准Open AI JSON协议数据\n" +
	"Your name is Coral.\n\n" +
	"## Filesystem Tools（OpenAI tools 协议）\n\n" +
	"- 你可以通过一组工具访问 workspace 目录下的文件。\n" +
	"- 所有路径必须是 workspace 根目录下的**相对路径**（例如：AGENT.md、USER.md、MEMORY.md），禁止访问绝对路径或使用 `..` 逃逸。\n\n" +
	"当前可用工具：\n\n" +
	"1. `workspace_read_file`\n" +
	"   - 功能：读取 workspace 相对路径文件的内容。\n\n" +
	"2. `workspace_write_file`\n" +
	"   - 功能：覆盖写入 workspace 相对路径文件的内容。\n\n" +
	"2. `memory_write_important`\n" +
	"   - 功能：记录需要长期记住的重要信息到 MEMORY.md，例如用户的长期偏好、账号/业务静态信息等。\n" +
	"   - 请只在你确信这些信息在未来多轮对话中仍然重要时才调用，避免把普通对话全部写入 MEMORY。\n\n" +
	"### 工具调用方式\n\n" +
	"- 主程序会通过 OpenAI JSON 协议把这些工具暴露给你；\n" +
	"- 当你需要访问文件时，请通过标准 `tool_calls` 机制选择合适的工具并给出参数；\n" +
	"- 宿主程序会执行工具并将结果以 `tool` 消息形式注入到后续对话中，你在看到工具结果后继续完成本轮回答。\n"

const defaultUser = `# User Profile
- Name: 未命名用户
- Timezone: Asia/Shanghai
- Country: China
- City:
- Language: zh-CN`

const defaultMemory = `# Long-term Memory
该文件由系统维护，用于记录长期需要记住的重要信息，请谨慎手工修改。`

func main() {
	workspaceDir, err := resolveWorkspaceDir()
	if err != nil {
		fmt.Println("解析 workspace 目录失败:", err)
		return
	}

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

	opts := []option.RequestOption{}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	client := openai.NewClient(opts...)
	openaiClient := &OpenAIClient{
		Client: client,
		Model:  model,
	}

	agent := NewAgentCore(openaiClient, systemContent, userProfile, fs, maxContextTokens, maxOutputTokens)

	fmt.Println("Minimal Go Agent (OpenAI JSON + local llama-server/Qwen)")
	fmt.Printf("Workspace 目录: %s\n", workspaceDir)
	fmt.Println("配置文件: AGENT.md / USER.md / MEMORY.md 均位于该目录下。")
	fmt.Println("Agent 会在需要时通过 OpenAI tools 协议调用文件系统工具（读取/写入 workspace 内的文件）。")
	fmt.Println("环境变量：OPENAI_BASE_URL / OPENAI_MODEL / OPENAI_API_KEY（兼容 LLAMA_SERVER_ENDPOINT / LLAMA_MODEL / LLAMA_AUTH_TOKEN）。")
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

