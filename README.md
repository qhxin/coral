# Coral Agent

Coral Agent 是一个用 Go 编写的轻量命令行 Agent：通过 **OpenAI 兼容的 JSON 协议** 与模型交互，并提供受限的本地 workspace 文件读写工具（OpenAI tools 语义）。

## 功能概览

- **OpenAI 兼容**：支持 `OPENAI_BASE_URL / OPENAI_MODEL / OPENAI_API_KEY`（并兼容 `LLAMA_*` 环境变量）。
- **本地 workspace**：自动初始化 `AGENT.md / USER.md / MEMORY.md`，并把会话历史写入 `workspace/sessions/`。
- **工具调用**：暴露 `workspace_read_file / workspace_write_file / memory_write_important` 等工具给模型调用。
- **帮助信息**：`--help/-h` 自动汇总 CLI 参数与环境变量说明（环境变量说明由 `.env.template` 在构建期生成）。

## 快速开始（Windows / PowerShell）

构建：

```powershell
.\shells\build.ps1
```

运行（默认 workspace 在可执行文件同级 `workspace/`）：

```powershell
.\build\coral.exe
```

查看帮助：

```powershell
.\build\coral.exe -h
```

指定 workspace：

```powershell
.\build\coral.exe --workspace D:\some\workspace
```

## 代码行数（Go）

<!-- LOC:START -->
Updated at: 2026-03-18 16:16:59

- Go files: 4
- Go LOC (code lines, excludes blanks & comments): 1130
<!-- LOC:END -->

统计并更新本文件：

```powershell
.\shells\count-go-loc.ps1
```

## 开发脚本

- `shells/build.ps1`：构建 `build/coral.exe`；构建期从 `.env.template` 生成 `env_help_gen.go`（用于 `--help` 环境变量说明）。
- `shells/run.ps1`：运行本地构建产物（如你已配置该脚本）。
- `shells/check-time.ps1`：静态检查，禁止直接使用 `time.Now()`（要求用 `Now()`）。
- `shells/count-go-loc.ps1`：统计 Go 代码行数并更新 `README.md`。







