#!/bin/sh
# 统计 Go 代码行（用内嵌 awk 去掉空行与注释）。
# 排除：目录 build/、.cursor/、vendor/；所有单元测试文件 *_test.go；生成文件 env_help_gen.go。
# 默认仅统计 src 子树： ./shells/count-go-loc.sh
# 如需覆盖统计范围： COUNT_GO_ROOT=path ./shells/count-go-loc.sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
cd "$ROOT_DIR"

README_PATH=${1:-README.md}
# 搜索起点，默认 src；可通过 COUNT_GO_ROOT 覆盖
SEARCH_ROOT=${COUNT_GO_ROOT:-src}

echo "===> Counting Go LOC under ${SEARCH_ROOT} (excluding: build/, .cursor/, vendor/, *_test.go, env_help_gen.go)"

if [ ! -f "$README_PATH" ]; then
  echo "README not found at: $README_PATH" >&2
  exit 1
fi

tmpfile="${TMPDIR:-/tmp}/coral-go-loc.$$"
trap 'rm -f "$tmpfile"' EXIT INT HUP TERM

# 收集 .go 文件列表（显式排除 Go 单元测试命名约定 *_test.go）
find "$SEARCH_ROOT" \
  -type d \( -name build -o -name .cursor -o -name vendor \) -prune -o \
  -type f -name '*.go' ! -name '*_test.go' ! -name 'env_help_gen.go' -print >"$tmpfile"

file_count=$(awk 'END{print NR+0}' "$tmpfile")

# Count LOC per-file with a Go-aware lexer in awk.
total_lines=$(
  awk '
  function is_space(c) { return (c ~ /[ \t\r\n]/) }
  function count_file(path,   inBlock,inRaw,codeLines,line,n,i,ch,nextch,hasCode,inStr,inRune,esc) {
    inBlock = 0
    inRaw = 0
    codeLines = 0

    while ((getline line < path) > 0) {
      sub(/\r$/, "", line)

      hasCode = inRaw
      inStr = 0
      inRune = 0
      esc = 0

      n = length(line)
      i = 1
      while (i <= n) {
        ch = substr(line, i, 1)
        nextch = (i < n) ? substr(line, i+1, 1) : ""

        if (inBlock) {
          if (ch == "*" && nextch == "/") { inBlock = 0; i += 2; continue }
          i++
          continue
        }

        if (inRaw) {
          if (ch == "`") { inRaw = 0 }
          i++
          continue
        }

        if (inStr) {
          hasCode = 1
          if (esc) { esc = 0; i++; continue }
          if (ch == "\\\\") { esc = 1; i++; continue }
          if (ch == "\"") { inStr = 0 }
          i++
          continue
        }

        if (inRune) {
          hasCode = 1
          if (esc) { esc = 0; i++; continue }
          if (ch == "\\\\") { esc = 1; i++; continue }
          if (ch == "'\''") { inRune = 0 }
          i++
          continue
        }

        # detect comments first
        if (ch == "/" && nextch == "/") { break }
        if (ch == "/" && nextch == "*") { inBlock = 1; i += 2; continue }

        # detect string starts
        if (ch == "`") { inRaw = 1; hasCode = 1; i++; continue }
        if (ch == "\"") { inStr = 1; hasCode = 1; i++; continue }
        if (ch == "'\''") { inRune = 1; hasCode = 1; i++; continue }

        if (!is_space(ch)) { hasCode = 1 }
        i++
      }

      if (hasCode) { codeLines++ }
    }
    close(path)
    return codeLines
  }
  BEGIN { total = 0 }
  {
    total += count_file($0)
  }
  END { print total+0 }
  ' "$tmpfile"
)

timestamp=$(date '+%Y-%m-%d %H:%M:%S')

loc_block=$(cat <<EOF
Updated at: $timestamp

- Go files: $file_count
- Go LOC (code lines, excludes blanks & comments): $total_lines
EOF
)

awk -v block="$loc_block" '
BEGIN { inloc=0; found=0 }
{
  if ($0 ~ /<!-- LOC:START -->/) {
    found=1
    print "<!-- LOC:START -->"
    print block
    inloc=1
    next
  }
  if (inloc) {
    if ($0 ~ /<!-- LOC:END -->/) {
      print "<!-- LOC:END -->"
      inloc=0
    }
    next
  }
  print
}
END {
  if (!found) {
    exit 3
  }
}
' "$README_PATH" > "${README_PATH}.tmp"

if [ $? -ne 0 ]; then
  rm -f "${README_PATH}.tmp"
  echo "LOC markers not found in README.md (<!-- LOC:START --> / <!-- LOC:END -->)" >&2
  exit 1
fi

mv "${README_PATH}.tmp" "$README_PATH"

echo "Updated $README_PATH"
echo "Go files: $file_count"
echo "Go LOC:   $total_lines"

