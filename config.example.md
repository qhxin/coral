# Corval Agent 配置模板

> 使用说明：
> 1. 复制本文件为 `config.md`。
> 2. 修改下面代码块中的配置项为你自己的值（特别是 `LLAMA_AUTH_TOKEN`）。
> 3. `run.ps1` 会自动读取 `config.md` 并设置对应环境变量。

```env
LLAMA_SERVER_ENDPOINT=http://localhost:8080/v1/chat/completions
LLAMA_MODEL=Qwen3.5-9B
LLAMA_AUTH_TOKEN=REPLACE_WITH_REAL_TOKEN
```

