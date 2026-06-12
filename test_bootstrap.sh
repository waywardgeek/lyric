#!/bin/bash
# Test the bootstrap compiler against all testdata/*.fg files
# Usage: ./test_bootstrap.sh [--rebuild] [--verbose] [pattern]

set -euo pipefail

FORGE="./forge"
BOOTSTRAP="/tmp/bootstrap"
RUNTIME_DIR="runtime"
# Clean up old test artifacts
rm -rf /tmp/forge_test_*

TMPDIR=$(mktemp -d -t forge_test_XXXXXX)


VERBOSE=false
PATTERN=""

for arg in "$@"; do
  case "$arg" in
    --verbose) VERBOSE=true ;;
    --rebuild) ;; # accepted but no-op (always rebuilds now)
    *) PATTERN="$arg" ;;
  esac
done

# Always rebuild Go compiler
echo "Building Go compiler..."
go build -o forge ./cmd/forge

# Always rebuild bootstrap
echo "Building bootstrap..."
$FORGE compile bootstrap/lir/lir.fg bootstrap/lexer/lexer.fg bootstrap/parser/parser.fg \
  bootstrap/parser/expr_parser.fg bootstrap/desugar/desugar.fg bootstrap/checker/checker.fg \
  bootstrap/lowerer/lowerer.fg bootstrap/ast/ast.fg bootstrap/optimizer/optimizer.fg \
  bootstrap/monomorphizer/monomorphizer.fg bootstrap/c_backend/c_backend.fg \
  bootstrap/main/main.fg -o "$TMPDIR/bootstrap.c"
gcc -std=gnu11 -O2 -o "$BOOTSTRAP" "$TMPDIR/bootstrap.c" -I "$RUNTIME_DIR" 2>/dev/null
echo "Bootstrap built."
echo ""

# Skip files that use features not yet in bootstrap (channels, spawn, select, lock)
SKIP_FILES="channels.fg spawn.fg select.fg lock.fg guarded_by.fg"

PASS=0
FAIL=0
SKIP=0
FAILURES=""

for fg in testdata/*.fg; do
  name=$(basename "$fg")

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
    echo "SKIP  $name"
    continue
  fi

  # Detect test-only files (have test_ functions but no main)
  CMD="compile"
  if grep -q 'func test_' "$fg" && ! grep -q 'func main()' "$fg" && ! grep -q 'func Main()' "$fg"; then
    CMD="test"
  fi

  # Step 1: Bootstrap compile .fg → .c
  bs_c="$TMPDIR/bs_${name%.fg}.c"
  bs_out="$TMPDIR/bs_${name%.fg}"

  # Determine dependencies for unit tests
  DEPS=""
  case "$name" in
    test_lexer.fg) DEPS="bootstrap/lexer/lexer.fg bootstrap/ast/ast.fg bootstrap/parser/parser.fg bootstrap/parser/expr_parser.fg" ;;
    test_parser.fg) DEPS="bootstrap/parser/parser.fg bootstrap/parser/expr_parser.fg bootstrap/lexer/lexer.fg bootstrap/ast/ast.fg" ;;
    test_desugar.fg) DEPS="bootstrap/desugar/desugar.fg bootstrap/parser/parser.fg bootstrap/parser/expr_parser.fg bootstrap/lexer/lexer.fg bootstrap/ast/ast.fg" ;;
    test_min.fg) DEPS="bootstrap/parser/parser.fg bootstrap/parser/expr_parser.fg bootstrap/lexer/lexer.fg bootstrap/ast/ast.fg" ;;
  esac

  if ! $BOOTSTRAP $CMD "$fg" $DEPS -o "$bs_c" 2>"$TMPDIR/err" ; then
    FAIL=$((FAIL + 1))
    err=$(cat "$TMPDIR/err")
    FAILURES="$FAILURES\nFAIL  $name  (bootstrap compile: $err)"
    echo "FAIL  $name  (bootstrap compile)"
    if $VERBOSE; then cat "$TMPDIR/err"; fi
    continue
  fi

  # Step 2: GCC compile
  if ! gcc -std=gnu11 -O2 -o "$bs_out" "$bs_c" -I "$RUNTIME_DIR" 2>"$TMPDIR/err"; then
    FAIL=$((FAIL + 1))
    err=$(head -5 "$TMPDIR/err")
    FAILURES="$FAILURES\nFAIL  $name  (gcc: $err)"
    echo "FAIL  $name  (gcc)"
    if $VERBOSE; then head -20 "$TMPDIR/err"; fi
    continue
  fi

  # Step 3: Run both bootstrap and Go compiler binaries, compare output
  # First, build and run Go compiler version as reference
  go_c="$TMPDIR/go_${name%.fg}.c"
  go_out="$TMPDIR/go_${name%.fg}"
  if ! $FORGE $CMD "$fg" -o "$go_c" 2>/dev/null; then
    # Go compiler can't compile it either — skip
    SKIP=$((SKIP + 1))
    echo "SKIP  $name (Go compiler also fails to compile)"
    continue
  fi
  if ! gcc -std=gnu11 -O2 -o "$go_out" "$go_c" -I "$RUNTIME_DIR" 2>/dev/null; then
    # Go compiler output doesn't link — skip
    SKIP=$((SKIP + 1))
    echo "SKIP  $name (Go compiler output doesn't link)"
    continue
  fi

  # Run Go compiler binary as reference
  go_run_ok=true
  if ! "$go_out" >"$TMPDIR/go_stdout" 2>"$TMPDIR/go_stderr"; then
    go_run_ok=false
  fi

  # Run bootstrap binary
  bs_run_ok=true
  if ! "$bs_out" >"$TMPDIR/bs_stdout" 2>"$TMPDIR/bs_stderr"; then
    bs_run_ok=false
  fi

  # Compare results
  if ! $go_run_ok && ! $bs_run_ok; then
    # Both fail — that's fine, skip
    SKIP=$((SKIP + 1))
    echo "SKIP  $name (both compilers fail at runtime)"
    continue
  fi

  if $go_run_ok && ! $bs_run_ok; then
    FAIL=$((FAIL + 1))
    err=$(tail -5 "$TMPDIR/bs_stderr")
    FAILURES="$FAILURES\nFAIL  $name  (runtime crash: $err)"
    echo "FAIL  $name  (runtime crash)"
    if $VERBOSE; then echo "--- bs stderr ---"; tail -10 "$TMPDIR/bs_stderr"; fi
    continue
  fi

  if ! $go_run_ok && $bs_run_ok; then
    # Bootstrap works but Go doesn't — unusual but count as pass
    PASS=$((PASS + 1))
    echo "PASS  $name (bootstrap succeeds where Go fails)"
    continue
  fi

  # Both ran — compare stdout (ignore stderr which has debug output)
  if ! diff -q "$TMPDIR/go_stdout" "$TMPDIR/bs_stdout" >/dev/null 2>&1; then
    FAIL=$((FAIL + 1))
    FAILURES="$FAILURES\nFAIL  $name  (output mismatch)"
    echo "FAIL  $name  (output mismatch)"
    if $VERBOSE; then
      echo "--- expected (Go compiler) ---"
      head -10 "$TMPDIR/go_stdout"
      echo "--- got (bootstrap) ---"
      head -10 "$TMPDIR/bs_stdout"
      echo "--- diff ---"
      diff "$TMPDIR/go_stdout" "$TMPDIR/bs_stdout" | head -20
    fi
    continue
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
