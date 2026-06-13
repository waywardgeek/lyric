#!/bin/bash
# test_lyric.sh — Test the Lyric compiler against all testdata/*.ly files
# Usage: ./test_lyric.sh [--verbose] [pattern]
#
# Builds lyric from lyric.c, then compiles+runs each test file.
# Test files with test_ functions use `lyric test`, others use `lyric compile`.
# If a golden file exists in testdata/golden/, output is compared.

set -euo pipefail

cd "$(dirname "$0")"

RUNTIME_DIR="runtime"
TMPDIR=$(mktemp -d -t lyric_test_XXXXXX)
GOLDEN_DIR="testdata/golden"
trap "rm -rf $TMPDIR" EXIT

VERBOSE=false
PATTERN=""

for arg in "$@"; do
  case "$arg" in
    --verbose) VERBOSE=true ;;
    *) PATTERN="$arg" ;;
  esac
done

# Build lyric from checked-in C
echo "Building lyric..."
make -s lyric
LYRIC="./lyric"
echo ""

SKIP_FILES=""

PASS=0
FAIL=0
SKIP=0
FAILURES=""

for fg in testdata/*.ly; do
  name=$(basename "$fg")
  base="${name%.ly}"

  # Filter by pattern if given
  if [ -n "$PATTERN" ] && [[ "$name" != *"$PATTERN"* ]]; then
    continue
  fi

  # Check skip list
  skip=false
  for s in $SKIP_FILES; do
    if [ "$name" = "$s" ]; then
      skip=true
      break
    fi
  done
  if $skip; then
    SKIP=$((SKIP + 1))
    if $VERBOSE; then echo "SKIP  $name"; fi
    continue
  fi

  # Detect test-only files (have test_ functions but no main)
  CMD="compile"
  if grep -q 'func test_' "$fg" && ! grep -q 'func main()' "$fg" && ! grep -q 'func Main()' "$fg"; then
    CMD="test"
  fi

  # Determine dependencies for unit tests
  DEPS=""
  case "$name" in
    test_lexer.ly) DEPS="src/lexer/lexer.ly src/ast/ast.ly src/parser/parser.ly src/parser/expr_parser.ly" ;;
    test_parser.ly) DEPS="src/parser/parser.ly src/parser/expr_parser.ly src/lexer/lexer.ly src/ast/ast.ly" ;;
    test_desugar.ly) DEPS="src/desugar/desugar.ly src/parser/parser.ly src/parser/expr_parser.ly src/lexer/lexer.ly src/ast/ast.ly" ;;
    test_min.ly) DEPS="src/parser/parser.ly src/parser/expr_parser.ly src/lexer/lexer.ly src/ast/ast.ly" ;;
  esac

  out_c="$TMPDIR/${base}.c"
  out_bin="$TMPDIR/${base}"

  # Step 1: Compile .ly → .c
  if ! $LYRIC $CMD "$fg" $DEPS -o "$out_c" 2>"$TMPDIR/err"; then
    FAIL=$((FAIL + 1))
    err=$(cat "$TMPDIR/err")
    FAILURES="$FAILURES\nFAIL  $name  (compile: $err)"
    echo "FAIL  $name  (compile)"
    if $VERBOSE; then cat "$TMPDIR/err"; fi
    continue
  fi

  # Step 2: GCC compile
  if ! gcc -std=gnu11 -O2 -w -o "$out_bin" "$out_c" -I "$RUNTIME_DIR" -lm -lpthread 2>"$TMPDIR/err"; then
    FAIL=$((FAIL + 1))
    err=$(head -5 "$TMPDIR/err")
    FAILURES="$FAILURES\nFAIL  $name  (gcc: $err)"
    echo "FAIL  $name  (gcc)"
    if $VERBOSE; then head -20 "$TMPDIR/err"; fi
    continue
  fi

  # Step 3: Run and capture output
  if ! "$out_bin" >"$TMPDIR/${base}.out" 2>"$TMPDIR/stderr"; then
    FAIL=$((FAIL + 1))
    err=$(tail -5 "$TMPDIR/stderr")
    FAILURES="$FAILURES\nFAIL  $name  (runtime: $err)"
    echo "FAIL  $name  (runtime)"
    if $VERBOSE; then tail -10 "$TMPDIR/stderr"; fi
    continue
  fi

  # Step 4: Compare against golden output (if exists)
  golden="$GOLDEN_DIR/${base}.expected"
  if [ -f "$golden" ]; then
    if ! diff -q "$golden" "$TMPDIR/${base}.out" >/dev/null 2>&1; then
      FAIL=$((FAIL + 1))
      FAILURES="$FAILURES\nFAIL  $name  (output mismatch)"
      echo "FAIL  $name  (output mismatch)"
      if $VERBOSE; then diff -u "$golden" "$TMPDIR/${base}.out" | head -30; fi
      continue
    fi
  fi

  PASS=$((PASS + 1))
  echo "PASS  $name"
done

echo ""
echo "=== Results ==="
echo "PASS: $PASS  FAIL: $FAIL  SKIP: $SKIP  TOTAL: $((PASS + FAIL + SKIP))"
if [ -n "$FAILURES" ]; then
  echo ""
  echo "=== Failures ==="
  echo -e "$FAILURES"
fi

exit $( [ $FAIL -eq 0 ] && echo 0 || echo 1 )
