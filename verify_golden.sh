#!/bin/bash
# verify_golden.sh — Verify bootstrap compiler output matches golden files
set -euo pipefail
cd "$(dirname "$0")"

LYRIC_BS="./lyric"
RUNTIME_DIR="runtime"
TMPDIR=$(mktemp -d -t lyric_verify_XXXXXX)
GOLDEN_DIR="testdata/golden"
trap "rm -rf $TMPDIR" EXIT

SKIP_FILES="guarded_by.ly"
PASS=0
FAIL=0
SKIP=0
FAILURES=""

for fg in testdata/*.ly; do
  name=$(basename "$fg")
  base="${name%.ly}"
  
  skip=false
  for s in $SKIP_FILES; do
    if [ "$name" = "$s" ]; then skip=true; break; fi
  done
  if $skip; then SKIP=$((SKIP + 1)); continue; fi

  golden="$GOLDEN_DIR/${base}.expected"
  if [ ! -f "$golden" ]; then
    echo "MISS  $name  (no golden file)"
    SKIP=$((SKIP + 1))
    continue
  fi

  CMD="compile"
  if grep -q 'func test_' "$fg" && ! grep -q 'func main()' "$fg"; then
    CMD="test"
  fi

  DEPS=""
  case "$name" in
    test_lexer.ly) DEPS="src/lexer/lexer.ly src/ast/ast.ly src/parser/parser.ly src/parser/expr_parser.ly" ;;
    test_parser.ly) DEPS="src/parser/parser.ly src/parser/expr_parser.ly src/lexer/lexer.ly src/ast/ast.ly" ;;
    test_desugar.ly) DEPS="src/desugar/desugar.ly src/parser/parser.ly src/parser/expr_parser.ly src/lexer/lexer.ly src/ast/ast.ly" ;;
    test_min.ly) DEPS="src/parser/parser.ly src/parser/expr_parser.ly src/lexer/lexer.ly src/ast/ast.ly" ;;
  esac

  out_c="$TMPDIR/${base}.c"
  out_bin="$TMPDIR/${base}"

  if [ "$CMD" = "test" ]; then
    if ! $LYRIC_BS test "$fg" $DEPS -o "$out_c" 2>"$TMPDIR/${base}.err" | grep -v '^wrote \|^phase: ' >"$TMPDIR/${base}.out"; then
      FAIL=$((FAIL + 1))
      FAILURES="$FAILURES\nFAIL  $name  (compile error)"
      continue
    fi
  else
    if ! $LYRIC_BS compile "$fg" -o "$out_c" 2>"$TMPDIR/${base}.err"; then
      FAIL=$((FAIL + 1))
      FAILURES="$FAILURES\nFAIL  $name  (compile error)"
      continue
    fi
    if ! gcc -std=gnu11 -O2 -w -o "$out_bin" "$out_c" -I "$RUNTIME_DIR" -lm -lpthread 2>>"$TMPDIR/${base}.err"; then
      FAIL=$((FAIL + 1))
      FAILURES="$FAILURES\nFAIL  $name  (gcc error)"
      continue
    fi
    if ! "$out_bin" >"$TMPDIR/${base}.out" 2>>"$TMPDIR/${base}.err"; then
      FAIL=$((FAIL + 1))
      FAILURES="$FAILURES\nFAIL  $name  (runtime error)"
      continue
    fi
  fi

  if diff -q "$golden" "$TMPDIR/${base}.out" >/dev/null 2>&1; then
    PASS=$((PASS + 1))
    echo "PASS  $name"
  else
    FAIL=$((FAIL + 1))
    FAILURES="$FAILURES\nFAIL  $name  (output mismatch)"
    echo "FAIL  $name  (output mismatch)"
    diff -u "$golden" "$TMPDIR/${base}.out" | head -20
    echo "---"
  fi
done

echo ""
echo "=== Verification Results ==="
echo "PASS: $PASS  FAIL: $FAIL  SKIP: $SKIP  TOTAL: $((PASS + FAIL + SKIP))"
if [ -n "$FAILURES" ]; then
  echo ""
  echo "=== Failures ==="
  echo -e "$FAILURES"
fi

exit $( [ $FAIL -eq 0 ] && echo 0 || echo 1 )
