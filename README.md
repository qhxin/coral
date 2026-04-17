# Coral Agent

<p align="center">
  <img src="logo.png" alt="Coral logo：小写 coral 字样，粉蓝拼色与珊瑚枝状装饰，透明背景。" width="400" />
</p>

Coral Agent 是一个用 Go 编写的轻量命令行 Agent：通过 **OpenAI 兼容的 JSON 协议** 与模型交互，并提供受限的本地 workspace 文件读写工具（OpenAI tools 语义）。

## 功能概览

- **OpenAI 兼容**：支持 `OPENAI_BASE_URL / OPENAI_MODEL / OPENAI_API_KEY`（并兼容 `LLAMA_*` 环境变量）。
- **本地 workspace**：自动初始化 `AGENT.md / USER.md / MEMORY.md`，并把会话历史写入 `workspace/sessions/`。
- **工具调用**：暴露 `workspace_read_file / workspace_write_file / memory_write_important` 等工具给模型调用。
- **帮助信息**：`--help/-h` 自动汇总 CLI 参数与环境变量说明（环境变量说明由 `.env.template` 在构建期生成）。
- **飞书机器人（长连接）**：`--feishu` 使用官方 WebSocket 通道接收 `im.message.receive_v1`，按聊天 `chat_id` 映射会话；模型回复优先以 **post 富文本** 发送以呈现 Markdown 结构（标题/列表/粗斜体/链接/代码样式等），失败时降级为纯文本。需企业自建应用并开通机器人能力与相关 IM 权限。
- **多模态（图文）**：飞书 **图片消息**（`message_type=image`）会通过消息资源接口拉取后，以 OpenAI 兼容的 `image_url`（data URL）随本轮 user 消息提交；CLI 可使用 `/img 路径 你的问题` 或 `/img "含空格路径" 问题`。**须使用支持视觉的模型与后端**；会话 `active.json` 仅存文字描述与可选的 `metadata.image_count`，大图为避免 JSON 膨胀默认不落盘，需要审计时可设 `CORAL_SAVE_INBOUND_MEDIA=1`。

## 快速开始（跨平台：macOS / Linux / Windows）

Windows 下建议使用 **Git Bash** 或 **MSYS2** 来运行 `sh` 脚本（不要求 WSL）。

构建：

```sh
sh ./shells/build-all.sh
```

运行（直接执行构建产物；程序会自动读取**可执行文件同目录**的 `.env`）：

```sh
./build/coral-windows-amd64.exe
# 或：
# ./build/coral-linux-amd64
# ./build/coral-linux-arm64
# ./build/coral-darwin-amd64
# ./build/coral-darwin-arm64
```

查看帮助：

```sh
./build/coral-windows-amd64.exe -h
```

指定 workspace：

```sh
./build/coral-windows-amd64.exe --workspace /path/to/workspace
```

飞书长连接模式（需设置 `FEISHU_APP_ID`、`FEISHU_APP_SECRET`，并在开放平台完成「使用长连接接收事件」与事件订阅）：

```sh
export FEISHU_APP_ID=cli_xxx
export FEISHU_APP_SECRET=xxx
./build/coral-windows-amd64.exe --feishu --workspace /path/to/workspace
```

说明：

- 会话目录为 `sessions/feishu-chat-<chat_id>/`，与 CLI 默认会话隔离。
- `FEISHU_QUICK_ACK_TEXT`：**留空**或 **`false` / `0`** 则不立即响应；**`true` / `1`** 时对用户本条消息添加点赞类表情（`THUMBSUP`）；**其它字符串**则先发该文案再异步生成完整内容。需在开放平台开通消息表情回复相关权限（若使用点赞模式）。
- `FEISHU_GROUP_AT_ONLY=1` 时，仅当飞书事件里 `mentions` 非空才回复（常用于群内需 @ 机器人的场景）；纯图片消息若无 @ 则同样不会回复。
- **Markdown 渲染**为向飞书 post 结构的映射，复杂表格/Mermaid 等可能等价度有限；极长内容会自动拆成多条消息。
- 多模态相关环境变量见 `.env.template`（`AGENT_VISION_TOKENS_PER_IMAGE`、`AGENT_VISION_MAX_IMAGE_BYTES`、`CORAL_VISION_EMPTY_TEXT`、`CORAL_SAVE_INBOUND_MEDIA`）。

## 代码行数（Go）

<!-- LOC:START -->
Updated at: 2026-04-17 17:58:54

- Go files: 28
- Go LOC (code lines, excludes blanks & comments): 4080
<!-- LOC:END -->

统计并更新本文件：

```sh
sh ./shells/count-go-loc.sh
```

## 开发脚本

- `shells/build-all.sh`：使用 `CGO_ENABLED=0` 交叉编译生成多平台产物（`windows/amd64`、`linux/amd64`、`linux/arm64`、`darwin/amd64`、`darwin/arm64`；不含 32 位：依赖的飞书 SDK 在 `GOARCH=386` 下无法通过编译），输出到 `build/`。
- `shells/check-time.sh`：静态检查，禁止直接使用 `time.Now()`（要求用 `Now()`）。
- `shells/count-go-loc.sh`：统计 Go 代码行数并更新 `README.md`。







