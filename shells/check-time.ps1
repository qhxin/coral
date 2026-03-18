Param()

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# Simple static check: forbid direct time.Now() usage outside whitelist.

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$whitelistFiles = @("src\\timeutil.go", "timeutil.go")

# Prefer ripgrep if available, fall back to Select-String otherwise.
$rgResult = $null
if (Get-Command rg -ErrorAction SilentlyContinue) {
    $rgResult = & rg 'time\.Now\(' . --glob '*.go' --glob '!*_test.go' 2>$null
} else {
    $goFiles = Get-ChildItem -Recurse -Filter *.go | Where-Object { $_.Name -notmatch '.*_test\.go$' }
    $lines = @()
    foreach ($file in $goFiles) {
        $matches = Select-String -Path $file.FullName -Pattern 'time\.Now\(' -SimpleMatch -ErrorAction SilentlyContinue
        foreach ($m in $matches) {
            $lines += "$($m.Path):$($m.LineNumber):$($m.Line)"
        }
    }
    if ($lines.Count -gt 0) {
        $rgResult = $lines
    }
}

if (-not $rgResult) {
    Write-Host "OK: no direct time.Now() usage found." -ForegroundColor Green
    exit 0
}

$pattern = ($whitelistFiles -join "|")
$filtered = $rgResult | Where-Object { $_ -notmatch $pattern }

if (-not $filtered) {
    Write-Host "OK: time.Now() only used in whitelist files." -ForegroundColor Green
    exit 0
}

Write-Host "ERROR: found forbidden direct time.Now() usage. Use Now() or asiaShanghaiLocation() instead." -ForegroundColor Red
Write-Host ""
$filtered | ForEach-Object { Write-Host $_ }
exit 1

