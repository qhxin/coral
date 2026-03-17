---
name: multi-channel-session-and-memory-architecture
overview: Refactor the agent to support multi-channel, per-session chat history with weekly archives, active session summaries, and a dedicated long-term MEMORY tool, all with token-aware context assembly.
todos:
  - id: define-session-storage-structure
    content: 设计 sessions 目录结构、JSON 消息结构、周文件命名与时区规则，并在代码中确定相关常量与辅助函数接口
    status: completed
  - id: extend-agentcore-for-sessions
    content: 为 AgentCore 增加 HandleWithSession 等接口，并实现基于 session_id 读取 active.json 与构造上下文的逻辑
    status: completed
  - id: implement-session-write-logic
    content: 实现每轮对话同时写入 sessions/{session_id}/active.json 与周归档文件的逻辑及辅助函数
    status: completed
  - id: implement-window-and-summary
    content: 实现 active.json 的7天窗口维护与历史消息滚动摘要生成和存储策略
    status: completed
  - id: add-token-limit-and-pruning
    content: 引入上下文 token/长度限制配置并实现按优先级裁剪历史消息的逻辑
    status: completed
  - id: create-memory-tool
    content: 新增 MEMORY 工具封装对 MEMORY.md 的写入，并在 AGENT.md 中对其使用场景进行说明
    status: completed
  - id: add-tests-and-validate
    content: 针对 CLI 流程编写或执行手工测试，验证 session 存储、摘要、token 限制和 MEMORY 工具行为
    status: completed
isProject: false
---

# 多渠道会话与记忆体系落地计划

## 一、整体架构调整

- **目标**：从“单一 MEMORY.md 线性日志”演进为“多会话 session 存储 + 长期 MEMORY 工具 + token 感知上下文装配”。
- **主要改动点**：
  - 在 `workspace` 下新增 `sessions/{session_id}` 目录结构。
  - 扩展 `AgentCore` 接口以感知 `session_id`，并从对应 `active.json` 读取/写入会话历史。
  - 引入 token/长度限制配置与裁剪逻辑。
  - 新增 MEMORY 工具，专门向 `MEMORY.md` 写入长期记忆。

## 二、目录与数据结构设计

- **2.1 目录结构扩展**
  - 在现有 `workspace` 目录下增加：
    - `sessions/`
      - `{session_id}/`
        - `active.json`
        - `{Year-Month-Week}.json`（如 `2026-03-W03.json`）
  - 约定 `session_id` 需进行安全化（过滤 `/` 等字符，必要时做 slug 或简单替换）。
- **2.2 Session 文件 JSON 结构**
  - 统一使用 OpenAI 标准消息格式数组：
    - `role`: `system` | `user` | `assistant`
    - `content`: 字符串
    - `metadata`（可选）：对象，例如 `{ "timestamp": "2026-03-17T10:12:00+08:00", "channel": "feishu", "message_id": "..." }`。
  - `sessions/{session_id}/{Year-Month-Week}.json`：该周所有对话消息的线性 append 历史。
  - `sessions/{session_id}/active.json`：
    - 若文件不存在，初始化为空数组 `[]`。
    - 数组前部：若干 `system` 角色的历史摘要消息。
    - 数组后部：最近 7 天内的 `user` / `assistant` 原始消息。
- **2.3 周文件命名与时区规则**
  - 时区固定使用 UTC+8（Asia/Shanghai）。
  - 周编号采用 ISO Week（周一为一周的开始），文件名格式：`YYYY-MM-Www.json`，如 `2026-03-W03.json`。
  - 需要一个工具函数，根据 `time.Time`（UTC+8）计算 Year、Month、Week，并返回对应文件名字符串。

## 三、AgentCore 接口与调用流程改造

- **3.1 会话感知的 Handle 接口**
  - 在 `AgentCore` 上新增方法：
    - `HandleWithSession(sessionID string, userInput string) (string, error)`。
  - 内部流程：
    - （1）读取 `sessions/{session_id}/active.json`，反序列化为消息数组（若不存在则用 `[]`）。
    - （2）在构造实际请求 messages 时，将：
      - `AGENT.md` 内容作为 system 消息；
      - `MEMORY.md` 内容作为追加的 system 消息；
      - `USER.md` 内容作为 user profile 消息（已有逻辑可复用）；
      - `active.json` 中的历史消息按顺序附加；
      - 最后追加本轮的 `user` 消息（包含 timestamp 元数据）。
    - （3）在调用底层 `ChatOnce` 前，进行 token/长度检查与裁剪（见第六部分）。
- **3.2 CLI 与未来渠道统一入口**
  - 现有 CLI 直接调用 `agent.Handle(line)`，调整为：
    - 先为 CLI 决定一个固定或可配置的 `session_id`（如 `cli-default`）。
    - 改用 `agent.HandleWithSession("cli-default", line)`。
  - 未来 HTTP Webhook / 飞书 WebSocket：
    - 外层 HTTP/WS Handler 负责从请求或事件中解析出 `session_id`（如飞书 `chat_id`）。
    - 再调用 `HandleWithSession(sessionID, userInput)`，避免在核心逻辑里引入具体渠道依赖。

## 四、会话写入策略与辅助函数

- **4.1 周文件写入逻辑**
  - 封装函数 `appendToWeeklyFile(sessionID string, messages []Message)`：
    - 使用 UTC+8 当前时间计算本轮所处的周文件名。
    - 确保 `sessions/{session_id}` 目录存在。
    - 打开/创建 `{Year-Month-Week}.json`：
      - 读旧内容（如存在），反序列化为数组；
      - 将本轮的 `user` 和 `assistant` 消息 append；
      - 写回 JSON（注意简单、健壮的错误处理）。
- **4.2 active.json 写入与维护**
  - 封装函数 `updateActiveSession(sessionID string, newMessages []Message, now time.Time)`：
    - 读 `active.json`（不存在则使用空数组）。
    - 将新一轮的两条消息 append 进数组。
    - 调用 `compactActiveMessages` 执行“7 天窗口 + 摘要滚动”。
    - 将结果数组序列化写回 `active.json`。
- **4.3 时间与元数据填充**
  - 统一封装创建消息的工厂函数：
    - `newUserMessage(content string, now time.Time, extraMeta map[string]any)`。
    - `newAssistantMessage(content string, now time.Time, extraMeta map[string]any)`。
    - 自动填充 `metadata.timestamp = now.In(Asia/Shanghai)` 的 RFC3339 字符串。

## 五、7 天窗口与摘要滚动策略实现

- **5.1 活跃窗口与过期判断**
  - 定义 `summaryWindowDays = 7`（配置化）。
  - 在 `compactActiveMessages(active []Message, now time.Time)` 中：
    - 遍历所有非摘要消息（`role` 为 `user` 或 `assistant`），读取其 `metadata.timestamp`；
    - 距离 `now` 超过 7 天的归类为“过期消息”。
    - 不含时间戳的历史消息可选择视作“不过期”或用创建时间补写（首次升级时兼容）。
- **5.2 摘要生成流程**
  - 若存在新的“过期消息”：
    - 从 `active` 中提取这些过期消息（按时间排序）。
    - 使用一个内部辅助函数 `summarizeMessagesWithLLM(messages []Message) (string, error)` 调用同一模型生成摘要文本：
      - system 提示可包含“请用简短中文总结以下对话的要点，突出用户偏好、长期事实和重要决策”。
    - 构造一条 `system` 摘要消息：
      - `role: "system"`
      - `content: 摘要文本`
      - `metadata.type = "history_summary"`，并记录 `from` / `to` 时间范围。
    - 处理旧摘要：
      - 初期实现可以保留旧摘要并追加新的摘要，或者更简单：将已有 `history_summary` 的 `system` 合并为一次新摘要（避免摘要层级膨胀）。
    - 从 `active` 中移除所有被摘要覆盖的 `user` / `assistant` 过期消息，仅保留新的摘要和 7 天内的原始消息。
- **5.3 初次启用时的兼容策略**
  - 若 `active.json` 初始为空或无 timestamp：
    - 增量写入逻辑自动开始生效。
    - 旧的 `MEMORY.md` 历史不迁移，只保留为长期记忆文件，由人工或模型逐步提取要点写入 MEMORY 工具。

## 六、token/长度限制与裁剪策略

- **6.1 配置项与估算方式**
  - 在 `envOrDefault` 基础上新增环境变量：
    - `AGENT_MAX_CONTEXT_TOKENS`：最大输入上下文 token；
    - `AGENT_MAX_OUTPUT_TOKENS`：最大输出 token（如需也可透传给模型参数）。
  - 封装一个粗略 token 估算函数：
    - 简单版本：按字符数或字节数 / 固定系数估算（如每 3~4 字符算 1 token），留足安全余量。
    - 将来可替换为更精确的 tiktoken 风格实现。
- **6.2 裁剪顺序与规则**
  - 在构造好完整 `messages`（含 system / user / assistant / 摘要）后，调用 `ensureContextWithinLimit(messages []Message) []Message`：
    - （1）优先保留：
      - `AGENT.md` 系统提示；
      - `MEMORY.md` 长期记忆系统提示；
      - 最新的历史摘要 `system` 消息；
      - 最近 N 轮完整对话（user + assistant）。
    - （2）若超过 `AGENT_MAX_CONTEXT_TOKENS`：
      - 先删除最早的原始历史对话（7 天内最久远的一轮），按“user+assistant”为单位成对删除；
      - 若仍超限，再考虑压缩摘要：
        - 把多条摘要合并成一条更短摘要（再次走一次内部 LLM 总结）。
    - （3）极端情况下：
      - 只保留全局 system + MEMORY system + 最近 1~2 轮完整对话。

## 七、MEMORY 工具设计与接入

- **7.1 新增工具定义**
  - 在 `defaultFilesystemTools` 中新增第三个工具，例如：
    - 名称：`memory_write_important`
    - 描述："记录需要长期记住的重要信息到 MEMORY.md。"
    - JSON Schema：
      - `content`（string，必填）：要记住的文本内容。
  - executor 实现：
    - 内部组装一段带时间戳的 Markdown 条目，例如：
      - `"\n\n## Memo at 2026-03-17 10:20 (UTC+8)\n" + content + "\n"`。
    - 调用 `fs.Append("MEMORY.md", entry)`。
- **7.2 AGENT.md 文档更新**
  - 在默认 `AGENT.md` 文案中增加对 MEMORY 工具的说明：
    - 何时应调用：
      - 用户长期偏好、账号/业务静态信息、必须跨 session 保存的事实。
    - 提示模型不要把所有普通对话都刷入 MEMORY。

## 八、测试与回归验证

- **8.1 基础功能测试**
  - CLI：
    - 连续对话多轮，检查：
      - `sessions/cli-default/active.json` 会不断 append 并包含最近 7 天全部对话；
      - 对应周文件 `{Year-Month-Week}.json` 记录全部轮次。
  - 手工或用小工具读取 `active.json` 验证 JSON 结构和 timestamp 正确。
- **8.2 摘要与裁剪测试**
  - 构造超过 7 天的消息（可通过手动修改 metadata.timestamp 或注入旧时间），触发摘要逻辑：
    - 确认过期对话被移除，生成的 `system` 摘要写入 `active.json`。
  - 构造非常长的上下文（可复制粘贴大段文本），确认 token 限制逻辑按预期裁剪消息集合。
- **8.3 MEMORY 工具测试**
  - 通过模型调用或模拟调用 MEMORY 工具，检查 `MEMORY.md` 是否正确追加内容。
  - 确认程序启动时将 `MEMORY.md` 拼接到 system 提示中，且对话过程中长期记忆生效。

## 九、逐步上线与风险控制

- **9.1 渐进启用**
  - 第一阶段只在 CLI 渠道启用 session 存储和 active/weekly 文件，不立刻切换到外部 Webhook/飞书。
  - 观察 `sessions/` 文件大小与性能情况，必要时优化 JSON 读写方式（例如按行追加 + 流式处理）。
- **9.2 向后兼容**
  - 仍然保留 `MEMORY.md` 的读取逻辑；
  - 不再把完整对话写入 `MEMORY.md`，降低文件膨胀风险；
  - 为旧版本升级路径保留配置开关（如有需要，可加入 `AGENT_SESSIONS_ENABLED=true` 控制是否启用新体系）。

