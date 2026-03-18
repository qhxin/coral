---
name: openai-json-protocol-migration
overview: 将当前基于 Markdown 串的大模型交互与工具调用升级为 OpenAI JSON 协议与官方 Go SDK，并将配置从 markdown 转为 .env 模板，同时保留现有 Markdown 记忆文件。
todos:
  - id: add-openai-sdk
    content: 在 go.mod 中引入官方 openai-go SDK，并实现 OpenAIClient 封装基础配置与 ChatOnce 调用。
    status: completed
  - id: refactor-agentcore-messages
    content: 重构 AgentCore，改用 OpenAI JSON 消息数组管理对话，将 AGENT/USER/MEMORY Markdown 转为 system/user 消息初始化上下文。
    status: completed
  - id: migrate-tools-to-openai
    content: 将现有 Tool/ToolExecutor 体系映射为 OpenAI tools 定义，改造为标准 tool_calls 循环。
    status: completed
  - id: env-template-migration
    content: 将 config.example.md 迁移为 .env.template，并统一使用 OPENAI_BASE_URL/OPENAI_MODEL/OPENAI_API_KEY 等新环境变量。
    status: completed
  - id: cleanup-legacy-markdown-flow
    content: 删除或废弃旧的 Markdown prompt + ```json 代码块解析逻辑，仅保留 OpenAI JSON 协议路径。
    status: completed
isProject: false
---

## 目标与约束

- **协议统一**：从“Markdown prompt + 纯文本回复解析工具 JSON 代码块”的模式，升级为 **完整 OpenAI Chat Completions JSON 协议**（含 `messages`、`tool_choice`、`tools`、`tool_calls` 等）。
- **SDK 选型**：使用官方 Go SDK `github.com/openai/openai-go`，调用兼容 OpenAI 协议的 **本地 llama-server（内部挂 Qwen3.5）**。
- **工具调用对齐**：Host 端维护 `tools` 列表与函数执行逻辑，模型侧用标准 `tool_calls` 结构发起调用，Host 用 `tool` role 消息回传结果，直至获得最终 `assistant` 消息。
- **记忆保持**：`AGENT.md` / `USER.md` / `MEMORY.md` 仍是 Markdown 文件，但不再直接串接为一个大 Markdown，而是 **拆分填充** 到 JSON `messages` 的 system/user 片段中。
- **配置迁移**：将 `config.example.md` 改造为 `.env.template`，统一基于环境变量读取；你后续会手动把 `config.md` 改成 `.env`。
- **JSON 紧凑性**：所有 Host 端构造的 JSON（包括持久化或日志中的提示样例）采用 **无多余空格与缩进** 的紧凑结构（如 `json.Marshal` 而不是 `MarshalIndent`）。

## 新的高层交互架构

```mermaid
flowchart TD
  userCLI[User_CLI] --> cliTransport[CLITransport]
  cliTransport --> agentCore[AgentCore]
  agentCore --> openaiClient[OpenAIClient]
  openaiClient --> llamaServer[Local_Llama_Server(OpenAI_Protocol)]
  llamaServer --> openaiClient
  agentCore --> toolsExec[ToolExecutors]
  toolsExec --> agentCore
  agentCore --> cliTransport
  cliTransport --> userCLI
```



- **OpenAIClient**：封装官方 `openai-go` SDK 调用，暴露一个高层方法（如 `ChatWithTools(...)`）。
- **AgentCore**：
  - 从 Markdown 记忆文件读取并转为 `messages`（system/user/tool/assistant），控制 JSON 协议细节。
  - 维护对话轮次（可选地保存在内存中，不必再写入 Markdown History 字符串）。
  - 负责多轮“模型→工具→模型”的循环，直到得到最终 `assistant` 文本。
- **工具执行层**：
  - 继续维护 `WorkspaceFS` + 文件类工具，但导出为 OpenAI `Tool` 定义（`type:function`+`parameters` JSON Schema）。
  - 将现有 `Tool` / `ToolExecutor` 结构映射到 SDK 所需的 `Tool` 定义与执行过程。

## 协议与数据结构设计

### 1. OpenAI SDK Client 封装

- **新结构体**：`type OpenAIClient struct { Client *openai.Client; Model string }`。
- **初始化**：
  - 从环境变量读取：
    - `OPENAI_BASE_URL`（原来 `LLAMA_SERVER_ENDPOINT`）
    - `OPENAI_API_KEY`（原来 `LLAMA_AUTH_TOKEN`）
    - `OPENAI_MODEL`（原来 `LLAMA_MODEL`）
  - 使用官方 SDK：
    - `client := openai.NewClient(openai.WithBaseURL(baseURL), openai.WithAPIKey(apiKey))`（具体根据 SDK 实际 API 调整）。
- **核心方法**：
  - `ChatOnce(ctx, messages, tools, toolChoice) (*openai.ChatCompletion, error)`：薄封装 `client.Chat.Completions.Create`。

### 2. AgentCore 改造

- **状态改动**：
  - 删除/弱化原来的 `SystemPrompt string` + 大块 `History string` 纯 Markdown 累积方式。
  - 新增：
    - `Messages []openai.ChatCompletionMessageParam`：运行中对话消息数组。
    - `Tools []openai.Tool`：暴露给模型的工具列表（从已有 `Tool` 列表生成）。
    - `Client *OpenAIClient`。
    - `FS *WorkspaceFS` / `Executors map[string]ToolExecutor` 保持不变。
- **初始化 System 与记忆**：
  - 启动时读取 `AGENT.md` / `USER.md` / `MEMORY.md`：
    - `systemContent := agentMD + "\n\n" + memoryImportantPart`（或直接拼接三个文件）。
    - `userProfile := userMD`。
  - 构造初始 `Messages`：
    - `Messages = []MessageParam{ System("systemContent"), User("userProfile") }`，并作为“长期上下文注入 JSON”。
- **Handle 流程改造**：
  - 1）将本轮用户输入追加：`Messages = append(Messages, User(userInput))`。
  - 2）进入循环：
    - 调用 `OpenAIClient.ChatOnce`，传入当前 `Messages` 与 `Tools`/`tool_choice:"auto"`。
    - 解析返回：
      - 若 `assistant` 消息无 `tool_calls`：
        - 将其追加到 `Messages`，返回 `assistant.Content` 给 CLI（本轮结束）。
      - 若存在 `tool_calls`：
        - 将该 `assistant` 消息同样追加到 `Messages`。
        - 遍历 `tool_calls`：
          - 根据 `function.name` 查找对应 `ToolExecutor`，执行 `arguments`。
          - 将工具执行结果转换为一条/多条 `tool` 角色消息：
            - `Messages = append(Messages, ToolMessage(toolName, callID, resultJSONOrText))`。
        - 再次循环调用 `ChatOnce`，直到得到无 `tool_calls` 的最终 `assistant`。

### 3. 工具定义对齐 OpenAI 协议

- **现有 `Tool` 结构**（Name/Description/ParametersJSONSchema）保留，但作为 OpenAI `Tool` 的来源。
- **Host 端构造 Tools 列表**：
  - 将 `ParametersJSONSchema`（目前是缩进多行 JSON）变为紧凑字符串，例如：
    - `{ "type":"object","properties":{...},"required":[...] }`。
  - 映射为 SDK 需要的 `Tool`/`Function` 结构：
    - `Name` => `function.name`
    - `Description` => `function.description`
    - `ParametersJSONSchema` => `function.parameters`（原样 JSON Raw）。
- **工具结果内容**：
  - 为简单起见，工具执行后返回的内容统一使用 **纯文本或 JSON 字符串**，作为 `tool` 消息的 `content`。
  - 建议约定：
    - 若逻辑上天然是结构化的，Host 在内部 `json.Marshal` 为紧凑 JSON 字符串，作为 `tool` 消息文本，让模型去解析。

### 4. 记忆文件在 JSON 中的组织

- **System 端信息**：
  - `AGENT.md`：作为主要 system prompt。
  - `MEMORY.md`：视体积与内容，将关键摘要或全部内容附加到 system 的末尾。
- **User 侧信息**：
  - `USER.md`：作为 `user` 角色的一条初始化消息，例如“用户画像 / 偏好”。
- **示例（逻辑结构）**：
  - `Messages = [`
    - `system: "<AGENT.md + MEMORY 摘要>"`,
    - `user: "<USER.md>"`,
    - `user: "<本轮真实输入>"`,
    - `assistant/tool/...`
  - ]

## 配置与 .env.template 调整

### 1. 新环境变量设计

- **替换原变量名**：
  - 原 `LLAMA_SERVER_ENDPOINT` => 新 `OPENAI_BASE_URL`（指向本地 OpenAI 协议兼容的 llama-server 地址，如 `http://localhost:8080/v1` 或 `/v1` 根）。
  - 原 `LLAMA_MODEL` => 新 `OPENAI_MODEL`（如 `qwen2.5-coder`、`Qwen3.5-72B` 等）。
  - 原 `LLAMA_AUTH_TOKEN` => 新 `OPENAI_API_KEY`。
- **保留兼容层（可选）**：
  - 读取时可以按顺序：
    - 若设置了新变量则优先使用；
    - 否则 fallback 到旧变量名，保证你已有运行脚本一段时间内不用同时改动。

### 2. `.env.template` 内容

- 将 `config.example.md` 改造成文本型 `.env.template` 文件：
  - 不再使用 Markdown 标题/说明块，而是简单注释 + env：
  - 示例：
    - `# Coral Agent 环境变量模板`
    - `OPENAI_BASE_URL=http://localhost:8080/v1`
    - `OPENAI_MODEL=Qwen3.5-9B`
    - `OPENAI_API_KEY=REPLACE_WITH_REAL_TOKEN`
- 运行脚本层（如 `run.ps1`）从 `.env` 读取并注入当前进程环境，你再手动把现有 `config.md` 的值迁移进去。

## 渐进式迁移步骤

### 步骤 1：引入官方 OpenAI SDK 与配置变量

- 在 `go.mod` 中添加 `github.com/openai/openai-go` 依赖，确定使用的版本。
- 在 `main.go` 中：
  - 新增读取 `OPENAI_BASE_URL` / `OPENAI_MODEL` / `OPENAI_API_KEY` 的逻辑。
  - 暂时保留原 `LlamaClient`，并创建一个新的 `OpenAIClient` 结构（先只在 main 中初始化，不立即替换业务调用）。

### 步骤 2：实现 `OpenAIClient.ChatOnce`

- 基于 SDK 写一个简单的 `ChatOnce`：
  - 入参：`ctx`、`messages`、`tools`（可先为空）、`toolChoice`（先默认 `auto`）。
  - 调用 SDK 的 Chat Completions 接口，返回完整结果。
  - 确认与本地 llama-server 的兼容性：
    - baseURL、路径 `/v1/chat/completions` 是否一致；
    - 是否需要额外 HTTP 头（如 `Authorization`）。

### 步骤 3：重构 `AgentCore` 使用 JSON 消息

- 引入 `[]MessageParam` 字段并在构造时用记忆文件填充：
  - 从 `initWorkspace` 读出的 `AGENT.md` / `USER.md` / `MEMORY.md` 构建 system/user 初始化消息。
- 修改 `Handle`：
  - 不再组装 Markdown 大串，而是：
    - `Messages = append(Messages, User(userInput))`；
    - 调用“带工具支持”的新循环逻辑（初期工具列表可为空，为后面扩展预留）。
  - 暂时仍将最终 `assistant` 消息追加到 `MEMORY.md`，保持原有长期记忆机制可用（或调整为只在必要时写入）。

### 步骤 4：将工具体系迁移到 OpenAI `tools`

- 使用现有 `Tool` 和 `ToolExecutor`：
  - 编写一个转换函数：`func (a *AgentCore) asOpenAITools() []openai.Tool`，从 Name/Description/ParametersJSONSchema 构造 SDK 结构。
  - 确保 `ParametersJSONSchema` 变成紧凑 JSON：
    - 如果现在是手写多行字符串，可以离线/一次性调整；
    - 未来新增工具建议先写 Go 结构体再 `json.Marshal` 得到紧凑字符串，减少手写错误。
- 在 `Handle` 循环中：
  - 每次调用 `ChatOnce` 都传递 `Tools`，让模型具备调用能力。

### 步骤 5：实现标准 OpenAI 风格的工具调用循环

- 新增一个私有方法（例如 `runChatWithTools(ctx, userInput) (finalText string, err error)`）：
  - 1）`Messages` 追加用户消息。
  - 2）`for` 循环：
    - 调用 `ChatOnce`。
    - 解析 `assistant` 消息：
      - 若 `len(tool_calls)==0`：
        - 记录该消息到 `Messages` 并返回 `assistant.Content`。
      - 否则：
        - 将该 `assistant` 消息追加到 `Messages`。
        - 遍历每个 `tool_call`：
          - 从 `Executors` 中找到对应函数；
          - 用 `json.RawMessage` 解 `arguments` 并执行；
          - 将执行结果封装为 `tool` 消息并追加到 `Messages`。
    - 下一轮循环继续调用 `ChatOnce`，直到无 tool_calls 为止。
- 这一步完成后，即可彻底删除/停用旧的“Markdown 中嵌 `

```json` 代码块 + 手动解析 tool_calls”逻辑。

### 步骤 6：清理与兼容层

- 删除或废弃：
  - `LlamaClient.Complete(markdownPrompt string)`、`extractToolCalls`、`parseToolCallsJSON`、`ToolCallEnvelope` 等仅服务于旧 Markdown 策略的代码。
- 保留/升级：
  - `WorkspaceFS` 及文件读写工具。
  - `defaultAgent` / `defaultUser` / `defaultMemory` 内容不必大改，只在文案里将“工具调用输出格式”改为描述 **OpenAI tools JSON 协议** 的示例，而不是“输出一个 JSON 代码块”。

### 步骤 7：配置与文档更新

- 将 `config.example.md` 替换为 `.env.template`：
  - 更新说明：从“复制为 config.md”改为“复制为 .env”。
- 在 README / 项目文档中：
  - 更新大模型交互说明为 OpenAI JSON 协议 + 官方 SDK。
  - 指明当前默认适配本地 llama-server（挂 Qwen3.5），但因遵循 OpenAI 协议，也可以直接切换到官方 OpenAI/阿里云 Qwen 等服务。

## 验证与回归检查

- **功能验证**：
  - 用 CLI 进行基本对话，确认回复正常，且本地 llama-server 实际被命中（可通过日志验证）。
- **工具链验证**：
  - 准备一个 prompt 触发 `workspace_read_file` 与 `workspace_write_file`：
    - 确认模型会自动生成 `tool_calls`，Host 正确执行，并且下一轮模型能读取工具结果继续推理。
- **回归检查**：
  - 确保 `.env` 未被提交到 git（`.gitignore` 里已经忽略或新增忽略规则）。
  - 检查 MEMORY 逻辑仍按预期工作，或根据新设计适度精简。

