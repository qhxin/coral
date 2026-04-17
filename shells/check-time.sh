#!/bin/sh
set -eu

# Simple static check: forbid direct time.Now() usage outside whitelist.

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
cd "$ROOT_DIR"

WHITELIST_PATTERN='src/timeutil\.go|timeutil\.go'

if command -v rg >/dev/null 2>&1; then
  # Exclude *_test.go to match the ps1 behavior.
  RESULT=$(rg 'time\.Now\(' ./src --glob '*.go' --glob '!*_test.go' 2>/dev/null || true)
else
  # Fallback when rg is not available.
  RESULT=$(grep -RIn --include='*.go' --exclude='*_test.go' 'time.Now(' ./src 2>/dev/null || true)
fi

if [ -z "${RESULT}" ]; then
  printf '%s\n' "OK: no direct time.Now() usage found."
  exit 0
fi

FILTERED=$(printf '%s\n' "$RESULT" | grep -Ev "$WHITELIST_PATTERN" || true)
if [ -z "${FILTERED}" ]; then
  printf '%s\n' "OK: time.Now() only used in whitelist files."
  exit 0
fi

printf '%s\n' "ERROR: found forbidden direct time.Now() usage. Use Now() instead."
printf '\n%s\n' "$FILTERED"
exit 1

