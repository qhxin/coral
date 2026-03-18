param(
    [string]$ReadmePath = "README.md"
)

# 切到仓库根目录（shells 的上级）
Set-Location -Path "$PSScriptRoot\.."

$excludeDirs = @("\build\", "\.cursor\", "\vendor\")
$excludeFiles = @("env_help_gen.go")

Write-Host "===> Counting Go LOC (excluding: build/, .cursor/, vendor/)"

$countGoCodeLines = {
    param(
        [string[]]$Lines
    )

    $inBlockComment = $false
    $inRawString = $false # Go raw string literal: `...` can span lines
    $codeLines = 0

    foreach ($line in $Lines) {
        $i = 0
        $hasCode = $false
        $inString = $false   # interpreted string: "..."
        $inRune = $false     # rune literal: 'a'
        $escape = $false

        if ($inRawString) {
            # 处于 raw string 内部的行本身属于代码（字符串字面量的一部分）
            $hasCode = $true
        }

        while ($i -lt $line.Length) {
            $ch = $line[$i]
            $next = if ($i + 1 -lt $line.Length) { $line[$i + 1] } else { [char]0 }

            if ($inBlockComment) {
                if ($ch -eq '*' -and $next -eq '/') {
                    $inBlockComment = $false
                    $i += 2
                    continue
                }
                $i++
                continue
            }

            if ($inRawString) {
                if ($ch -eq '`') {
                    $inRawString = $false
                }
                $i++
                continue
            }

            if ($inString) {
                $hasCode = $true
                if ($escape) {
                    $escape = $false
                    $i++
                    continue
                }
                if ($ch -eq '\') {
                    $escape = $true
                    $i++
                    continue
                }
                if ($ch -eq '"') {
                    $inString = $false
                }
                $i++
                continue
            }

            if ($inRune) {
                $hasCode = $true
                if ($escape) {
                    $escape = $false
                    $i++
                    continue
                }
                if ($ch -eq '\') {
                    $escape = $true
                    $i++
                    continue
                }
                if ($ch -eq "'") {
                    $inRune = $false
                }
                $i++
                continue
            }

            # 非字符串/注释状态下，优先识别注释起始
            if ($ch -eq '/' -and $next -eq '/') {
                # 行注释开始：结束扫描
                break
            }
            if ($ch -eq '/' -and $next -eq '*') {
                $inBlockComment = $true
                $i += 2
                continue
            }

            # 识别字符串起始
            if ($ch -eq '`') {
                $inRawString = $true
                $hasCode = $true
                $i++
                continue
            }
            if ($ch -eq '"') {
                $inString = $true
                $hasCode = $true
                $i++
                continue
            }
            if ($ch -eq "'") {
                $inRune = $true
                $hasCode = $true
                $i++
                continue
            }

            if (-not [char]::IsWhiteSpace($ch)) {
                $hasCode = $true
            }
            $i++
        }

        if ($hasCode) {
            $codeLines++
        }
    }

    return $codeLines
}

$goFiles = Get-ChildItem -Path . -Recurse -File -Filter *.go | Where-Object {
    $full = $_.FullName
    $name = $_.Name
    foreach ($d in $excludeDirs) {
        if ($full -like "*$d*") { return $false }
    }
    foreach ($n in $excludeFiles) {
        if ($name -ieq $n) { return $false }
    }
    return $true
}

$fileCount = $goFiles.Count
$totalLines = 0

foreach ($f in $goFiles) {
    # Go 源码通常为 UTF-8，这里显式指定编码避免系统默认编码导致的计数异常
    $lines = Get-Content -LiteralPath $f.FullName -Encoding UTF8
    $lineCount = & $countGoCodeLines $lines
    $totalLines += $lineCount
}

$timestamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss"

# 为避免 Windows PowerShell 在不同代码页下写入中文导致 README 乱码，
# 这里生成的 LOC 区块只使用 ASCII 文本。
$locBlock = @"
Updated at: $timestamp

- Go files: $fileCount
- Go LOC (code lines, excludes blanks & comments): $totalLines
"@

if (-not (Test-Path $ReadmePath)) {
    Write-Host "README not found at: $ReadmePath" -ForegroundColor Red
    exit 1
}

$readme = Get-Content -Path $ReadmePath -Raw -Encoding UTF8
$pattern = '(?s)<!-- LOC:START -->.*?<!-- LOC:END -->'
$replacement = "<!-- LOC:START -->`n$locBlock`n<!-- LOC:END -->"

if ($readme -notmatch $pattern) {
    Write-Host "LOC markers not found in README.md (<!-- LOC:START --> / <!-- LOC:END -->)" -ForegroundColor Red
    exit 1
}

$updated = [regex]::Replace($readme, $pattern, $replacement)
Set-Content -Path $ReadmePath -Value $updated -Encoding UTF8

Write-Host "Updated $ReadmePath" -ForegroundColor Green
Write-Host "Go files: $fileCount"
Write-Host "Go LOC:   $totalLines"

