# run.ps1
# 运行脚本优先从 Markdown/环境变量读取配置；
# - 若存在 config.md：从中解析 LLAMA_* 变量

Set-Location -Path "$PSScriptRoot"

function Set-EnvFromMarkdown([string]$path) {
    Write-Host "Loading config from $path"
    $lines = Get-Content $path
    foreach ($line in $lines) {
        if ($line -match '^\s*(LLAMA_[A-Z0-9_]+)\s*=\s*(.+)\s*$') {
            $name = $matches[1]
            $value = $matches[2]
            # 动态设置环境变量，需要通过 env: 驱动 + Set-Item
            Set-Item -Path "Env:$name" -Value $value
        }
    }
}

# 优先从 Markdown 本地配置读取
if (Test-Path ".\config.md") {
    Set-EnvFromMarkdown ".\config.md"
}

# 若环境变量未设置，则给出默认值
if (-not $env:LLAMA_SERVER_ENDPOINT) {
    $env:LLAMA_SERVER_ENDPOINT = "http://localhost:8080/v1/chat/completions"
}
if (-not $env:LLAMA_MODEL) {
    $env:LLAMA_MODEL = "Qwen3.5-9B"
}

if (-not (Test-Path ".\corval.exe")) {
    Write-Host "corval.exe not found, building..." -ForegroundColor Yellow
    .\build.ps1
    if ($LASTEXITCODE -ne 0) {
        Write-Host "Build failed, cannot start Corval." -ForegroundColor Red
        exit 1
    }
}

Write-Host "===> Starting Corval agent..."
.\corval.exe

