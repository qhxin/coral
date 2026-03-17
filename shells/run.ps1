# run.ps1
# 从 .env 加载 OPENAI_* 配置并运行构建产物（build\corval.exe）

Set-Location -Path "$PSScriptRoot\.."

function Set-EnvFromDotEnv([string]$path) {
    Write-Host "Loading env from $path"
    $lines = Get-Content $path
    foreach ($line in $lines) {
        $trimmed = $line.Trim()
        if (-not $trimmed) { continue }
        if ($trimmed.StartsWith("#")) { continue }
        if ($trimmed -match '^\s*([A-Z0-9_]+)\s*=\s*(.+)\s*$') {
            $name = $matches[1]
            $value = $matches[2]
            Set-Item -Path "Env:$name" -Value $value
        }
    }
}

# 优先从 .env 加载配置（兼容 OPENAI_* / LLAMA_*）
if (Test-Path ".\.env") {
    Set-EnvFromDotEnv ".\.env"
}

# 若环境变量未设置，则给出默认值（同时兼容旧变量名）
if (-not $env:OPENAI_BASE_URL -and -not $env:LLAMA_SERVER_ENDPOINT) {
    $env:OPENAI_BASE_URL = "http://localhost:8080/v1"
}
if (-not $env:OPENAI_MODEL -and -not $env:LLAMA_MODEL) {
    $env:OPENAI_MODEL = "Qwen3.5-9B"
}

$exePath = ".\build\corval.exe"
if (-not (Test-Path $exePath)) {
    Write-Host "build\corval.exe not found, building..." -ForegroundColor Yellow
    & ".\shells\build.ps1"
    if ($LASTEXITCODE -ne 0) {
        Write-Host "Build failed, cannot start Corval." -ForegroundColor Red
        exit 1
    }
}

Write-Host "===> Starting Corval agent..."
& $exePath

