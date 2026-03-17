param(
    [string]$Output = "corval.exe"
)

# 切到仓库根目录（shells 的上级）
Set-Location -Path "$PSScriptRoot\.."

$buildDir = "build"
if (-not (Test-Path $buildDir)) {
    New-Item -ItemType Directory -Path $buildDir | Out-Null
}

$outputPath = Join-Path $buildDir $Output

Write-Host "===> Building Go agent: $outputPath"

go mod tidy
if ($LASTEXITCODE -ne 0) {
    Write-Host "go mod tidy failed." -ForegroundColor Red
    exit 1
}

go build -o $outputPath
if ($LASTEXITCODE -ne 0) {
    Write-Host "Build failed." -ForegroundColor Red
    exit 1
}

Write-Host "Build succeeded: $outputPath" -ForegroundColor Green

