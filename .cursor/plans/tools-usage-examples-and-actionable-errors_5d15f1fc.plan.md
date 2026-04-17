---
name: tools-usage-examples-and-actionable-errors
overview: Update the system prompt to include concrete OpenAI tool_calls JSON examples, and make tool error messages actionable by embedding exact argument templates so the model can self-correct.
todos:
  - id: add-toolcall-json-examples
    content: 在系统提示（defaultAgent/AGENT.md）中加入 tool_calls JSON 示例与必填字段说明，明确禁止 {} 空 arguments
    status: completed
  - id: make-tool-errors-actionable
    content: 为 workspace_write_file / memory_write_important 等工具的参数校验错误增加可复制的 arguments JSON 模板与明确修复指引
    status: completed
  - id: validate-with-log
    content: 用 CLI 触发写文件/写记忆请求，检查日志中 tool_calls.arguments 不再为 {} 且工具实际执行成功
    status: in_progress
isProject: false
---

# Tool Calling 示例与可执行错误信息落地计划

## 目标

- **系统提示增强**：在系统提示中加入清晰、可复制的 function calling（`tool_calls`）JSON 示例，避免模型只“口头说要调用工具”或传空 `{}`。
- **错误信息可执行**：当工具调用参数缺失/错误时，返回的 `tool` 消息要包含“下一步怎么修正”的明确指令与示例 JSON（尤其是 `path`/`content` 必填）。

## 背景定位（基于日志）

- 从 `[d:\cursor-project\coral\build\workspace\logs\2026-03-18.log](d:\cursor-project\coral\build\workspace\logs\2026-03-18.log)` 可见：模型确实产生了 `tool_calls`，但 `arguments` 一直是 `{}`，宿主侧返回 `执行出错: path 不能为空`，模型无法从错误中推断正确参数结构，导致反复失败。

## 改动点 1：系统提示加入 tool_calls JSON 示例

- **文件范围**：
  - 默认模板：`[d:\cursor-project\coral\main.go](d:\cursor-project\coral\main.go)` 中 `defaultAgent`。
  - 现有 workspace：若你的运行 workspace 使用 `build/workspace`，同步更新 `[d:\cursor-project\coral\build\workspace\AGENT.md](d:\cursor-project\coral\build\workspace\AGENT.md)`（或你真实运行时的 workspace `AGENT.md`）。
- **内容要求**：
  - 明确说明：
    - 工具必须通过 `tool_calls` 发起；
    - `function.arguments` 必须是 **合法 JSON 字符串**，且包含必填字段；
    - 禁止传空对象 `{}`。
  - 给出**完整可复制**示例（至少覆盖两个核心工具）：

示例 1：写文件 `workspace_write_file`

```json
{
  "tool_calls": [
    {
      "type": "function",
      "function": {
        "name": "workspace_write_file",
        "arguments": "{\"path\":\"a.txt\",\"content\":\"abc\"}"
      }
    }
  ]
}
```

示例 2：写长期记忆 `memory_write_important`

```json
{
  "tool_calls": [
    {
      "type": "function",
      "function": {
        "name": "memory_write_important",
        "arguments": "{\"content\":\"事事有回应，件件有着落，凡事有交待\"}"
      }
    }
  ]
}
```

> 说明：示例使用的是 OpenAI tool_calls 协议中“arguments 为 JSON 字符串”的标准形态，模型可以直接照抄并替换字段值。

## 改动点 2：错误信息改为“可执行指导”

- **文件范围**：`[d:\cursor-project\coral\main.go](d:\cursor-project\coral\main.go)` 中 `defaultFilesystemTools` 的各个 executor（至少 `workspace_write_file`、`memory_write_important`）。
- **实现策略**：
  - 当缺必填字段时，不只返回“不能为空”，而是返回包含：
    - **错误原因**（缺少哪个字段）
    - **下一步动作**（请重新调用同名工具）
    - **可复制 JSON 模板**（arguments 内容）
  - 让模型可以在下一轮直接复制模板并填值。

具体例子（返回到 `tool` 的字符串内容示意）：

- `workspace_write_file` 缺 `path`：
  - `执行出错: path 不能为空。请重新调用 workspace_write_file，并传入 arguments：{"path":"a.txt","content":"abc"}`
- `memory_write_important` 缺 `content`：
  - `执行出错: content 不能为空。请重新调用 memory_write_important，并传入 arguments：{"content":"..."}`

> 注意：这段内容会进入下一轮模型上下文，因此要尽量短、确定、可复制。

## 可选增强（如需要）

- **工具 schema 强化**：在工具 `description` 中再重复一遍必填字段和示例（但以系统提示示例为主）。
- **对 `{}` 的专门提示**：如果检测到 arguments 解出来是空对象（或所有必填字段为空），在错误里明确指出“你传入了空对象 {}，必须填入 path/content”。

## 验证方式

- 在 CLI 里输入：
  - `在工作区创建一个文件：a.txt ，内容写abc`
  - `我要你长期永远记住这个做事原则：...`
- 观察 `[d:\cursor-project\coral\build\workspace\logs\YYYY-MM-DD.log](d:\cursor-project\coral\build\workspace\logs\YYYY-MM-DD.log)`：
  - 期望模型第一次或第二次就能产生带完整字段的 `tool_calls.function.arguments`，不再是 `{}`。
- 检查 workspace 文件：
  - `workspace/a.txt` 创建成功
  - `workspace/MEMORY.md` 追加成功

## 预计修改文件清单

- `[d:\cursor-project\coral\main.go](d:\cursor-project\coral\main.go)`：更新 `defaultAgent` 文案；更新工具 executor 的错误返回内容。
- `[d:\cursor-project\coral\build\workspace\AGENT.md](d:\cursor-project\coral\build\workspace\AGENT.md)`：若该目录即运行时 workspace，同步更新以立刻生效（否则更新真实运行 workspace 的 `AGENT.md`）。

