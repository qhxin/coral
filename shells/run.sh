#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
cd "$ROOT_DIR"

load_dotenv() {
  dotenv_path=$1
  echo "Loading env from $dotenv_path"
  # Read line-by-line, ignore comments/empty lines, only accept KEY=VALUE with KEY in [A-Z0-9_]+.
  # This avoids executing arbitrary code (do NOT source .env).
  while IFS= read -r line || [ -n "$line" ]; do
    line=$(printf '%s' "$line" | tr -d '\r')

    trimmed=$(printf '%s' "$line" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
    [ -z "$trimmed" ] && continue
    case "$trimmed" in
      \#*) continue ;;
    esac

    case "$trimmed" in
      *=*)
        key=${trimmed%%=*}
        value=${trimmed#*=}
        key=$(printf '%s' "$key" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
        case "$key" in
          *[!A-Z0-9_]*|'') continue ;;
        esac
        export "$key=$value"
        ;;
    esac
  done < "$dotenv_path"
}

if [ -f "./.env" ]; then
  load_dotenv "./.env"
fi

if [ -z "${OPENAI_BASE_URL:-}" ] && [ -z "${LLAMA_SERVER_ENDPOINT:-}" ]; then
  export OPENAI_BASE_URL="http://localhost:8080/v1"
fi

if [ -z "${OPENAI_MODEL:-}" ] && [ -z "${LLAMA_MODEL:-}" ]; then
  export OPENAI_MODEL="Qwen3.5-9B"
fi

detect_goos() {
  os=$(uname -s 2>/dev/null || echo unknown)
  case "$os" in
    Linux) echo linux ;;
    Darwin) echo darwin ;;
    MINGW*|MSYS*|CYGWIN*) echo windows ;;
    *) echo unknown ;;
  esac
}

detect_goarch() {
  m=$(uname -m 2>/dev/null || echo unknown)
  case "$m" in
    x86_64|amd64) echo amd64 ;;
    aarch64|arm64) echo arm64 ;;
    *) echo unknown ;;
  esac
}

pick_dist_exe() {
  # build-all.sh outputs directly into ./build/
  out_dir="./build"
  [ -d "$out_dir" ] || return 1

  goos=$(detect_goos)
  goarch=$(detect_goarch)
  [ "$goos" = "unknown" ] && return 1
  [ "$goarch" = "unknown" ] && return 1

  # Try canonical name first (amd64/arm64), then accept x86 alias for 386.
  try_archs="$goarch"
  if [ "$goarch" = "386" ]; then
    try_archs="386 x86"
  fi

  for a in $try_archs; do
    cand="$out_dir/coral-$goos-$a"
    if [ "$goos" = "windows" ]; then
      cand="$cand.exe"
    fi
    if [ -f "$cand" ]; then
      echo "$cand"
      return 0
    fi
  done
  return 1
}

EXE=""

# Prefer build-all artifact when available for current platform.
EXE_FROM_DIST=$(pick_dist_exe || true)
if [ -n "$EXE_FROM_DIST" ]; then
  EXE="$EXE_FROM_DIST"
fi

if [ -z "$EXE" ]; then
  echo "No matching artifact found under build/. Please run: sh ./shells/build-all.sh" >&2
  exit 1
fi

if [ -z "$EXE" ]; then
  echo "Failed to locate built executable under build/ (coral-\$goos-\$arch)." >&2
  exit 1
fi

echo "===> Starting Coral agent..."
exec "$EXE"

