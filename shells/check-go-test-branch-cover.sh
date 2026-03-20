#!/bin/sh
# 在运行全量构建前检查 Go 单测是否达到「分支」覆盖门槛。
#
# 说明：Go 工具链暂不输出独立的分支覆盖率摘要；本脚本解析 go test 生成的
# cover profile（基本块粒度），以「至少命中一次的基本块数 / 基本块总数」作为
# 分支/路径覆盖的工程近似。门槛默认 80%，可通过环境变量 MIN_BRANCH_PCT 覆盖。
#
# Usage: sh ./shells/check-go-test-branch-cover.sh
set -eu

MIN_BRANCH_PCT_RAW=${MIN_BRANCH_PCT:-80}

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
cd "$ROOT_DIR"

COVER_PROFILE="${TMPDIR:-/tmp}/coral-go-branch-cover-$$.out"
trap 'rm -f "$COVER_PROFILE"' EXIT INT HUP TERM

# 仅接受非负整数或带小数的百分数阈值；统一交给 awk 校验。
case $MIN_BRANCH_PCT_RAW in
  ''|*[!0-9.]*) echo "MIN_BRANCH_PCT must be a non-negative number, got: $MIN_BRANCH_PCT_RAW" >&2; exit 2 ;;
esac

echo "===> Go test: basic-block (branch-like) coverage, minimum ${MIN_BRANCH_PCT_RAW}%"

go test -count=1 -covermode=atomic \
  -coverprofile="$COVER_PROFILE" \
  -coverpkg=./src \
  ./src/...

awk -v min="$MIN_BRANCH_PCT_RAW" '
function isnum(s,   t) {
  t = s + 0
  return (t == t && length(s) > 0)
}
BEGIN {
  if (!isnum(min) || min + 0 < 0 || min + 0 > 100) {
    print "ERROR: MIN_BRANCH_PCT must be in [0, 100], got: " min > "/dev/stderr"
    exit 2
  }
}
NR == 1 {
  if ($0 !~ /^mode:/) {
    print "ERROR: unexpected coverage profile (missing mode line)" > "/dev/stderr"
    exit 1
  }
  next
}
NF < 3 { next }
{
  cnt = $NF + 0
  nblocks++
  if (cnt > 0) nhit++
}
END {
  if (nblocks + 0 < 1) {
    print "ERROR: no coverage blocks in profile" > "/dev/stderr"
    exit 1
  }
  pct = 100.0 * nhit / nblocks
  printf "basic_block_coverage: %.2f%% (%d/%d blocks; approx. branch/path coverage)\n", pct, nhit, nblocks
  if (pct + 1e-9 < min + 0) {
    printf "ERROR: coverage %.2f%% is below required %.2f%%\n", pct, min + 0 > "/dev/stderr"
    exit 1
  }
}
' "$COVER_PROFILE"
