---
name: workspace-and-config-files-refactor
overview: 为本地 Go Agent 设计并落地 workspace 目录机制，引入 AGENT.md / USER.md / MEMORY.md 三个配置与记忆文件，并将当前硬编码系统提示词迁移到文件驱动。
todos:
  - id: parse-workspace-arg
    content: 实现命令行 workspace 参数解析与默认 workspace 目录推导逻辑
    status: completed
  - id: init-workspace-files
    content: 实现 workspace 目录创建与 AGENT.md / USER.md / MEMORY.md 三个文件的初始化函数
    status: completed
  - id: inject-soul-into-agent
    content: 修改 AgentCore 构造逻辑，从 AGENT.md 读取 system prompt 并移除硬编码提示词
    status: completed
  - id: update-cli-messages
    content: 更新启动提示信息，展示 workspace 路径与三个配置文件作用
    status: completed
  - id: file-io-skill
    content: 实现受限于 workspace 目录的文件读写技能，并在系统提示词中写明使用方法
    status: completed
isProject: false
---

### 目标

- **统一 workspace 机制**：程序运行时基于一个可配置的 workspace 目录工作，可通过命令行参数指定；未指定时以当前可执行文件所在目录下的 `workspace` 作为默认路径；目录不存在时自动创建。
- **外置提示词与身份定义**：用 `AGENT.md` 存放系统提示词和 Agent 身份定义，将当前在 `main.go` 中硬编码的系统提示词迁移到该文件，并改为运行时读取。
- **用户属性与长期记忆文件**：在 workspace 下默认存在 `USER.md`（用户身份属性）与 `MEMORY.md`（长期记忆），为后续扩展提供结构。

### 一、workspace 目录设计

- **目录定位规则**
  - **命令行参数优先**：支持形如 `--workspace` 或 `-w` 的参数，参数值为绝对或相对路径：
    - 解析时使用 `filepath.Abs` 将其转为绝对路径。
  - **默认路径规则**：若未指定参数：
    - 通过 `os.Executable()` 获取当前可执行文件路径，取其目录 `exeDir`。
    - 设定默认 `workspaceDir = filepath.Join(exeDir, "workspace")`。
  - **目录创建策略**：
    - 使用 `os.MkdirAll(workspaceDir, 0o755)` 确保目录存在。
    - 若创建失败，直接在 `main` 中打印错误并退出。
- **关键文件约定**
  - `AGENT.md`：系统提示词与 Agent 身份定义。
  - `USER.md`：用户身份属性（姓名、时区、国家/地区/城市等）。
  - `MEMORY.md`：长期记忆，由应用动态维护。
  - 三个文件路径：
    - `agentPath := filepath.Join(workspaceDir, "AGENT.md")`
    - `userPath := filepath.Join(workspaceDir, "USER.md")`
    - `memoryPath := filepath.Join(workspaceDir, "MEMORY.md")`

### 二、AGENT.md / USER.md / MEMORY.md 文件内容与生命周期（读写并参与提示词组装）

- **AGENT.md 设计**
  - **初始内容结构（首次运行自动生成时）**：
    - 顶部保留系统角色定义和风格要求，例如：
      - 标题：`# System` / `# Agent Soul`。
      - 角色描述：命令行 Agent、仅输出 Markdown、语言习惯等。
      - 可预留小节供未来扩展：如“工具调用说明”、“安全策略”等。
  - **与代码交互方式（读 + 参与提示词组装）**：
    - 在启动时读取整个 `AGENT.md` 文本，作为 `AgentCore.SystemPrompt` 的基础内容，不再在 `NewAgentCore` 中硬编码。
    - 后续如有需要，可以在运行过程中根据配置或管理命令对 `AGENT.md` 做更新（写入），以支持动态调整 Agent 行为。
    - 如文件不存在：
      - 使用内置默认模板字符串写入 `AGENT.md`。
      - 写入成功后再从磁盘读取实际内容给 `AgentCore`（保证运行时读取路径统一）。
- **USER.md 设计**
  - **初始模板字段**（首次运行自动生成，若文件不存在时自动写入）示例：
    - `# User Profile`
    - `- Name: 未命名用户`
    - `- Timezone: Asia/Shanghai`
    - `- Country: China`
    - `- City:`
    - `- Language: zh-CN`
  - **与代码交互方式（读 + 注入上下文）**：
    - 启动时读取 `USER.md`，解析其中的字段（姓名、时区、国家/地区/城市、语言等），组装成一段结构化文本。
    - 将这段用户画像文本并入最终的系统提示词中（例如作为 system prompt 的一个小节 `# User Profile`），让大模型在每次对话时都能感知用户属性。
    - 未来可以在交互或配置指令中更新 `USER.md`（写入），以反映用户资料的变化。
- **MEMORY.md 设计**
  - **初始内容**：
    - `# Long-term Memory`
    - 简要注释说明：该文件由系统维护，用于记录长期重要信息，请谨慎手工修改。
  - **与代码交互方式（读 + 写长期记忆）**：
    - 启动或新会话时，从 `MEMORY.md` 读取已有的长期记忆，将其作为一段“长期背景信息”前置到系统提示词或对话历史中，让大模型在回答时能够利用长期记忆。
    - 在对话过程中，根据模型输出或内部策略，从用户输入/AI 回复中抽取“需要长期记住的重要信息”，以追加的方式写回 `MEMORY.md`。
    - 通过读 + 写的闭环，让 `MEMORY.md` 成为大模型长期记忆的唯一持久化载体。

### 三、命令行参数与启动流程调整

- **参数解析策略**
  - 出于简单性与当前项目规模，直接使用 `os.Args` 手写解析（无需额外依赖）：
    - 支持：`--workspace=/path/to/ws`、`--workspace /path/to/ws`、`-w /path/to/ws`。
    - 忽略未知参数，或在控制台给出简要提示。
  - 为避免与现有逻辑耦合，在 `main` 顶部新增小工具函数：
    - `parseWorkspacePath(args []string) (string, error)`：返回用户指定的路径字符串或空串。
- **主函数启动流程重构**（伪代码）：
  - 1）解析环境变量（LLAMA_SERVER_ENDPOINT / LLAMA_MODEL / LLAMA_AUTH_TOKEN）。
  - 2）解析命令行参数获取 `workspaceDir`，如果为空则计算默认路径。
  - 3）创建 workspace 目录，确保 `AGENT.md` / `USER.md` / `MEMORY.md` 存在（调用一个新的 `initWorkspace` 函数）。
  - 4）从 `AGENT.md` 读取 system prompt 内容。
  - 5）构造 `LlamaClient` 和 `AgentCore`（传入 system prompt 文本）。
  - 6）打印启动信息时增加 workspace 目录显示。
  - 7）进入当前已有的命令行对话循环逻辑。

### 四、AgentCore 结构与初始化方式调整

- **结构变更**
  - 保持 `AgentCore` 字段不变：`SystemPrompt string`, `History string`, `Llama *LlamaClient`。
- **构造方式调整**
  - 将 `NewAgentCore` 从“固定默认 systemPrompt”改为“外部注入 systemPrompt”：
    - 修改签名为：`func NewAgentCore(llama *LlamaClient, systemPrompt string) *AgentCore`。
    - 内部使用传入的 `systemPrompt`，若为空可回退到一个内置最小默认值。
    - `History` 初始化仍为 `"# Conversation\n"`。
  - 在 `main` 中：
    - 读取 `AGENT.md` 内容为字符串 `agentContent`。
    - 使用 `agent := NewAgentCore(llama, agentContent)` 进行初始化。

### 五、workspace 初始化辅助函数设计

- **核心函数**
  - `func resolveWorkspaceDir(args []string) (string, error)`：
    - 内部调用 `parseWorkspacePath`；若返回空，则计算默认 `workspace` 目录。
    - 返回绝对路径，并处理 Windows / Unix 路径差异（交给 `filepath`）。
  - `func initWorkspace(dir string) (agentPath, userPath, memoryPath string, err error)`：
    - 调用 `os.MkdirAll(dir, 0o755)` 确保 workspace 目录存在。
    - 对三大文件 `AGENT.md` / `USER.md` / `MEMORY.md`：
      - **若文件不存在**：自动创建，并写入各自的默认模板内容。
      - **若文件已存在**：保持不动，不覆盖用户修改。
    - 返回三个文件的绝对路径。
  - `func readFileAsString(path string) (string, error)`：简化读文本文件逻辑，统一错误处理。
- **错误处理策略**
  - 若 workspace 解析或创建失败：
    - 在 `main` 中打印明确错误信息（包括目标路径与错误原因），并退出非零码。
  - 若 `AGENT.md` 读取失败：
    - 尝试 fallback 到内置最小 system prompt，并在控制台提示“未能读取 AGENT.md，使用内置默认系统提示”。

### 六、USER.md / MEMORY.md 的解析与记忆机制（本次就要实现）

- **USER.md 解析与上下文注入**
  - 在 `AgentCore` 中新增字段：`UserProfile map[string]string` 或专门结构体。
  - 启动时解析 `USER.md`，将用户称呼、时区、国家/地区、城市、语言等信息组装成一段文本，注入到最终 system prompt 中（例如作为 `# User Profile` 小节），让所有对话轮次都能共享这些信息。
- **MEMORY.md 长期记忆机制**
  - 在本次迭代中先实现**最小可用版本**：在对话结束或每轮对话后，根据简单规则（例如：人工调用一个内部方法）将需长期保留的信息以 Markdown 追加的方式写入 `MEMORY.md`。
  - 启动或进入新会话时，从 `MEMORY.md` 读取历史长期记忆，并将其以“长期背景信息”的形式并入 system prompt 或对话历史的前置部分，使模型持续利用这些记忆。

### 七、workspace 受限的文件读写技能设计

- **设计目标**
  - 为大模型提供一个“文件读写技能”，用于读写 `workspace` 目录下的配置与记忆文件（尤其是 `AGENT.md` / `USER.md` / `MEMORY.md` 以及未来扩展文件）。
  - **严格限制**：该技能只能在当前解析出的 `workspaceDir` 下操作，禁止访问其外部路径，避免越权读写。
  - 在系统提示词中**清晰描述技能的能力、限制与调用方式**，让大模型知道何时、如何安全使用。
- **技术实现要点**
  - 在 Go 代码中封装一个小层，例如：`type WorkspaceFS struct { Root string }`，暴露受限方法：
    - `Read(path string) (string, error)`：只能读 `Root` 下的相对路径（内部用 `filepath.Join` + `filepath.Rel` 校验）。
    - `Write(path string, content string) error`：同样只允许写 `Root` 下的相对路径。
  - 对外暴露给 Agent 的接口可以是：
    - 直接在 `AgentCore` 中提供方法：`ReadFileFromWorkspace(relPath string)` / `WriteFileToWorkspace(relPath string, content string)`。
    - 或规划为“技能调用”风格：例如在提示词中约定调用格式：
      - 读取：`[FS_READ path="USER.md"]`
      - 写入：`[FS_WRITE path="MEMORY.md"] ...内容... [/FS_WRITE]`
  - 在 `Handle` 逻辑中：
    - 执行前，对模型生成的文本做简单解析，检测是否包含上述 FS 调用标记。
    - 若检测到：
      - 按标记解析出路径与内容，调用受限的 `WorkspaceFS` 执行实际读写。
      - 再把真实读写结果（例如文件内容、成功/失败信息）整理为自然语言/Markdown，作为最终对用户可见的回复的一部分。
- **系统提示词中的内置说明**
  - 在 `SOUL.md` 中新增一个章节，例如 `## Filesystem Skill`，描述：
    - 你拥有一个只能在 workspace 目录下读写文件的技能。
    - 只能读写相对路径（如 `SOUL.md`、`USER.md`、`MEMORY.md`、`notes/todo.md` 等）。
    - 必须使用约定的调用格式（例如 `FS_READ` / `FS_WRITE` 标记），由宿主程序解析并执行。
    - 不允许尝试访问绝对路径或跳出 workspace 的相对路径（禁止 `..` 等）。

### 八、具体落地步骤（实施顺序）

- **步骤 1：命令行解析与 workspace 路径解析**
  - 在 `main.go` 中新增 `parseWorkspacePath` 与 `resolveWorkspaceDir`。
  - 在 `main` 函数一开始解析 `workspaceDir`，并将其打印出来以便调试。
- **步骤 2：workspace 目录与三大文件初始化**
  - 新增 `initWorkspace` 与默认模板字符串常量（`defaultAgent`, `defaultUser`, `defaultMemory`）。
  - 在 `main` 中调用 `initWorkspace`，确保目录与文件就绪。
- **步骤 3：实现 workspace 受限文件读写封装**
  - 实现 `WorkspaceFS`（或等价封装），确保所有读写都被限制在 `workspaceDir` 之内。
  - 提供统一的 `ReadFileFromWorkspace` / `WriteFileToWorkspace` 等辅助函数。
- **步骤 4：调整 AgentCore 构造方式并接入 AGENT.md / USER.md / MEMORY.md**
  - 修改 `NewAgentCore` 签名与实现，使其从参数注入最终组装后的 system prompt。
  - 启动时使用受限文件读写封装读取 `AGENT.md` / `USER.md` / `MEMORY.md`，完成：
    - system prompt 基础部分（来自 `AGENT.md`）
    - 用户画像部分（来自 `USER.md`）
    - 长期记忆部分（来自 `MEMORY.md`）
  - 删除原本硬编码 system prompt 的字符串，避免两处定义不一致。
- **步骤 5：USER.md 解析与 MEMORY.md 长期记忆（本次必须完成）**
  - 实现 `USER.md` 解析逻辑与对应结构体，将用户信息自然融入 system prompt。
  - 实现一版简单但可用的长期记忆规则，驱动 `MEMORY.md` 的读写与自动更新。
- **步骤 6：在 AGENT.md 中内置文件读写技能使用说明**
  - 更新默认 `AGENT.md` 模板，增加 `Filesystem Skill` 章节。
  - 明确说明允许的操作范围（仅 workspace）、调用格式以及使用时机。
- **步骤 7：控制台输出与文档微调**
  - 在启动文本中增加 workspace 路径提示与 `AGENT.md / USER.md / MEMORY.md` 的简要说明。
  - 简要说明已经内置了受限文件读写能力，主要用于配置与长期记忆维护。

