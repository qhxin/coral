# Coral Agent

Coral Agent 是一个用 Go 编写的轻量命令行 Agent：通过 **OpenAI 兼容的 JSON 协议** 与模型交互，并提供受限的本地 workspace 文件读写工具（OpenAI tools 语义）。

## 功能概览

- **OpenAI 兼容**：支持 `OPENAI_BASE_URL / OPENAI_MODEL / OPENAI_API_KEY`（并兼容 `LLAMA_*` 环境变量）。
- **本地 workspace**：自动初始化 `AGENT.md / USER.md / MEMORY.md`，并把会话历史写入 `workspace/sessions/`。
- **工具调用**：暴露 `workspace_read_file / workspace_write_file / memory_write_important` 等工具给模型调用。
- **帮助信息**：`--help/-h` 自动汇总 CLI 参数与环境变量说明（环境变量说明由 `.env.template` 在构建期生成）。

## 快速开始（跨平台：macOS / Linux / Windows）

Windows 下建议使用 **Git Bash** 或 **MSYS2** 来运行 `sh` 脚本（不要求 WSL）。

构建：

```sh
sh ./shells/build-all.sh
```

运行（会自动选择 `build/` 下与当前平台匹配的产物执行；不存在则提示先构建）：

```sh
sh ./shells/run.sh
```

查看帮助：

```sh
./build/coral -h
```

指定 workspace：

```sh
./build/coral --workspace /path/to/workspace
```

## 代码行数（Go）

<!-- LOC:START -->
Updated at: 2026-03-20 15:40:58

- Go files: 13
- Go LOC (code lines, excludes blanks & comments): 1317
<!-- LOC:END -->

统计并更新本文件：

```sh
sh ./shells/count-go-loc.sh
```

## 开发脚本

- `shells/build-all.sh`：使用 `CGO_ENABLED=0` 交叉编译生成多平台产物（含 `windows/x86`、`linux/x86`），输出到 `build/`。
- `shells/run.sh`：从 `.env` 加载配置（如有），并自动选择 `build/` 下与当前平台匹配的产物运行（若没有匹配产物则提示先执行 `build-all.sh`）。
- `shells/check-time.sh`：静态检查，禁止直接使用 `time.Now()`（要求用 `Now()`）。
- `shells/count-go-loc.sh`：统计 Go 代码行数并更新 `README.md`。







