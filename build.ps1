# build.ps1
param(
    [string]$Output = "corval.exe"
)

Write-Host "===> Building Go agent: $Output"

Set-Location -Path "$PSScriptRoot"

go mod tidy
if ($LASTEXITCODE -ne 0) {
    Write-Host "go mod tidy failed." -ForegroundColor Red
    exit 1
}

go build -o $Output
if ($LASTEXITCODE -ne 0) {
    Write-Host "Build failed." -ForegroundColor Red
    exit 1
}

Write-Host "Build succeeded: $Output" -ForegroundColor Green

