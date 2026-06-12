#!/bin/bash
# test_bootstrap2.sh — Test bootstrap2 (self-compiled) against testdata/*.fg
# Compares bootstrap2 output against Go compiler output.
# Usage: ./test_bootstrap2.sh [--verbose] [--rebuild] [pattern]
#
# This tests the SECOND-generation bootstrap: Go→bootstrap1→bootstrap2
# If bootstrap2 matches Go compiler output, we're close to fixed point.

set -euo pipefail

cd "$(dirname "$0")"

FORGE="./forge"
RUNTIME_DIR="runtime"
TMPDIR=$(mktemp -d -t forge_b2_test_XXXXXX)

VERBOSE=false
REBUILD=false
PATTERN=""

for arg in "$@"; do
  case "$arg" in
    --verbose) VERBOSE=true ;;
    --rebuild) REBUILD=true ;;
    *) PATTERN="$arg" ;;
  esac
done

BOOTSTRAP_FILES=(
  bootstrap/ast/ast.fg
  bootstrap/lexer/lexer.fg
  bootstrap/parser/parser.fg
  bootstrap/parser/expr_parser.fg
  bootstrap/desugar/desugar.fg
  bootstrap/checker/checker.fg
  bootstrap/lir/lir.fg
  bootstrap/lowerer/lowerer.fg
  bootstrap/optimizer/optimizer.fg
  bootstrap/monomorphizer/monomorphizer.fg
  bootstrap/c_backend/c_backend.fg
  bootstrap/main/main.fg
)

B2="/tmp/bootstrap2"

if [ "$REBUILD" = true ] || [ ! -f "$B2" ]; then
  echo "=== Building bootstrap2 (Go → bootstrap1 → bootstrap2) ==="

  echo "  Building Go compiler..."
  go build -o "$TMPDIR/forge_go" ./cmd/forge/

  echo "  Stage 1: Go → bootstrap1..."
  "$TMPDIR/forge_go" compile "${BOOTSTRAP_FILES[@]}" -o "$TMPDIR/bootstrap1.c" 2>/dev/null
  gcc -std=gnu11 -O2 -w -o "$TMPDIR/bootstrap1" "$TMPDIR/bootstrap1.c" -I "$RUNTIME_DIR"

  echo "  Stage 2: bootstrap1 → bootstrap2..."
  "$TMPDIR/bootstrap1" compile "${BOOTSTRAP_FILES[@]}" -o "$TMPDIR/bootstrap2.c" 2>/dev/null
  gcc -std=gnu11 -O2 -w -o "$B2" "$TMPDIR/bootstrap2.c" -I "$RUNTIME_DIR"

  echo "  bootstrap2 built at $B2"
  echo ""
fi

if [ ! -f "$B2" ]; then
  echo "ERROR: bootstrap2 not found at $B2. Run with --rebuild."
  exit 1
fi

# Build Go compiler for reference
echo "Building Go compiler..."
go build -o forge ./cmd/forge
echo ""

SKIP_FILES="channels.fg spawn.fg select.fg lock.fg guarded_by.fg"

PASS=0
FAIL=0
SKIP=0
TOTAL=0
FAILURES=""
CRASH=0
COMPILE_FAIL=0
GCC_FAIL=0
MISMATCH=0

for fg in testdata/*.fg; do
  name=$(basename "$fg")

  # Filter by pattern
  if [ -n "$PATTERN" ] && [[ "$name" != *"$PATTERN"* ]]; then
    continue
  fi

  # Skip list
  skip=false
  for s in $SKIP_FILES; do
    if [ "$name" = "$s" ]; then skip=true; break; fi
  done
  if $skip; then
    SKIP=$((SKIP + 1))
    TOTAL=$((TOTAL + 1))
    continue
  fi

  # Detect test vs compile mode
  CMD="compile"
  if grep -q 'func test_' "$fg" && ! grep -q 'func main()' "$fg" && ! grep -q 'func Main()' "$fg"; then
    CMD="test"
  fi

  # Skip unit tests that need deps (bootstrap2 can't handle multi-file)
  case "$name" in
    test_lexer.fg|test_parser.fg|test_desugar.fg|test_min.fg)
      SKIP=$((SKIP + 1))
      TOTAL=$((TOTAL + 1))
      continue
      ;;
  esac

  TOTAL=$((TOTAL + 1))

  # Step 1: Go compiler as reference
  go_c="$TMPDIR/go_${name%.fg}.c"
  go_out="$TMPDIR/go_${name%.fg}"
  if ! $FORGE $CMD "$fg" -o "$go_c" 2>/dev/null; then
    SKIP=$((SKIP + 1))
    if $VERBOSE; then echo "SKIP  $name (Go compiler fails)"; fi
    continue
  fi
  if ! gcc -std=gnu11 -O2 -w -o "$go_out" "$go_c" -I "$RUNTIME_DIR" 2>/dev/null; then
    SKIP=$((SKIP + 1))
    if $VERBOSE; then echo "SKIP  $name (Go GCC fails)"; fi
    continue
  fi

  # Step 2: bootstrap2 compile
  bs_c="$TMPDIR/bs2_${name%.fg}.c"
  bs_out="$TMPDIR/bs2_${name%.fg}"

  if ! "$B2" $CMD "$fg" -o "$bs_c" 2>"$TMPDIR/err"; then
    FAIL=$((FAIL + 1))
    COMPILE_FAIL=$((COMPILE_FAIL + 1))
    err=$(head -3 "$TMPDIR/err" 2>/dev/null || echo "crashed/timeout")
    FAILURES="$FAILURES\nFAIL  $name  (b2 compile: $err)"
    echo "FAIL  $name  (b2 compile)"
    if $VERBOSE; then head -5 "$TMPDIR/err" 2>/dev/null; fi
    continue
  fi

  # Step 3: GCC compile bootstrap2 output
  if ! gcc -std=gnu11 -O2 -w -o "$bs_out" "$bs_c" -I "$RUNTIME_DIR" 2>"$TMPDIR/err"; then
    FAIL=$((FAIL + 1))
    GCC_FAIL=$((GCC_FAIL + 1))
    err=$(grep 'error:' "$TMPDIR/err" | head -3)
    FAILURES="$FAILURES\nFAIL  $name  (b2 gcc: $err)"
    echo "FAIL  $name  (b2 gcc)"
    if $VERBOSE; then grep 'error:' "$TMPDIR/err" | head -5; fi
    continue
  fi

  # Step 4: Run both, compare output
  "$go_out" >"$TMPDIR/go_stdout" 2>/dev/null || true
  if ! "$bs_out" >"$TMPDIR/bs_stdout" 2>/dev/null; then
    FAIL=$((FAIL + 1))
    CRASH=$((CRASH + 1))
    FAILURES="$FAILURES\nFAIL  $name  (b2 runtime crash)"
    echo "FAIL  $name  (b2 runtime crash)"
    continue
  fi

  if ! diff -q "$TMPDIR/go_stdout" "$TMPDIR/bs_stdout" >/dev/null 2>&1; then
    FAIL=$((FAIL + 1))
    MISMATCH=$((MISMATCH + 1))
    FAILURES="$FAILURES\nFAIL  $name  (output mismatch)"
    echo "FAIL  $name  (output mismatch)"
    if $VERBOSE; then
      echo "  expected: $(head -3 "$TMPDIR/go_stdout")"
      echo "  got:      $(head -3 "$TMPDIR/bs_stdout")"
    fi
    continue
  fi

  PASS=$((PASS + 1))
  echo "PASS  $name"
done

echo ""
echo "=== Bootstrap2 Results ==="
echo "PASS: $PASS  FAIL: $FAIL  SKIP: $SKIP  TOTAL: $TOTAL"
if [ $FAIL -gt 0 ]; then
  echo ""
  echo "Breakdown: compile=$COMPILE_FAIL  gcc=$GCC_FAIL  crash=$CRASH  mismatch=$MISMATCH"
  echo ""
  echo "=== Failures ==="
  echo -e "$FAILURES"
fi

# Cleanup
rm -rf "$TMPDIR"

exit $( [ $FAIL -eq 0 ] && echo 0 || echo 1 )
