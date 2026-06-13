#!/bin/bash
# generate_golden.sh — Generate golden outputs from Go compiler (preferred) or bootstrap
set -euo pipefail
cd "$(dirname "$0")"

LYRIC_GO="/tmp/lyric-go"
LYRIC_BS="./lyric"
RUNTIME_DIR="runtime"
TMPDIR=$(mktemp -d -t lyric_golden_XXXXXX)
GOLDEN_DIR="testdata/golden"
mkdir -p "$GOLDEN_DIR"

trap "rm -rf $TMPDIR" EXIT

SKIP_FILES="guarded_by.ly"

go_pass=0
go_fail=0
bs_pass=0
bs_fail=0

for fg in testdata/*.ly; do
  name=$(basename "$fg")
  base="${name%.ly}"
  
  # Skip
  skip=false
  for s in $SKIP_FILES; do
    if [ "$name" = "$s" ]; then skip=true; break; fi
  done
  if $skip; then echo "SKIP  $name"; continue; fi

  # Detect test-only files
  CMD="compile"
  if grep -q 'func test_' "$fg" && ! grep -q 'func main()' "$fg"; then
    CMD="test"
  fi

  # Try Go compiler first
  go_ok=false
  if [ "$CMD" = "test" ]; then
    # lyric test files — Go compiler
    if $LYRIC_GO test "$fg" -o "$TMPDIR/${base}_go.c" 2>"$TMPDIR/${base}_go.err" | grep -v '^wrote \|^phase: ' >"$TMPDIR/${base}_go.out"; then
      go_ok=true
    fi
  else
    if $LYRIC_GO compile "$fg" -o "$TMPDIR/${base}_go.c" 2>"$TMPDIR/${base}_go.err"; then
      if gcc -std=gnu11 -O2 -w -o "$TMPDIR/${base}_go" "$TMPDIR/${base}_go.c" -I "$RUNTIME_DIR" -lm -lpthread 2>>"$TMPDIR/${base}_go.err"; then
        if "$TMPDIR/${base}_go" >"$TMPDIR/${base}_go.out" 2>>"$TMPDIR/${base}_go.err"; then
          go_ok=true
        fi
      fi
    fi
  fi

  if $go_ok; then
    cp "$TMPDIR/${base}_go.out" "$GOLDEN_DIR/${base}.expected"
    echo "GO    $name"
    go_pass=$((go_pass + 1))
    continue
  fi
  go_fail=$((go_fail + 1))

  # Fall back to bootstrap compiler
  bs_ok=false
  if [ "$CMD" = "test" ]; then
    DEPS=""
    case "$name" in
      test_lexer.ly) DEPS="src/lexer/lexer.ly src/ast/ast.ly src/parser/parser.ly src/parser/expr_parser.ly" ;;
      test_parser.ly) DEPS="src/parser/parser.ly src/parser/expr_parser.ly src/lexer/lexer.ly src/ast/ast.ly" ;;
      test_desugar.ly) DEPS="src/desugar/desugar.ly src/parser/parser.ly src/parser/expr_parser.ly src/lexer/lexer.ly src/ast/ast.ly" ;;
      test_min.ly) DEPS="src/parser/parser.ly src/parser/expr_parser.ly src/lexer/lexer.ly src/ast/ast.ly" ;;
    esac
    if $LYRIC_BS test "$fg" $DEPS -o "$TMPDIR/${base}_bs.c" 2>"$TMPDIR/${base}_bs.err" | grep -v '^wrote \|^phase: ' >"$TMPDIR/${base}_bs.out"; then
      bs_ok=true
    fi
  else
    if $LYRIC_BS compile "$fg" -o "$TMPDIR/${base}_bs.c" 2>"$TMPDIR/${base}_bs.err"; then
      if gcc -std=gnu11 -O2 -w -o "$TMPDIR/${base}_bs" "$TMPDIR/${base}_bs.c" -I "$RUNTIME_DIR" -lm -lpthread 2>>"$TMPDIR/${base}_bs.err"; then
        if "$TMPDIR/${base}_bs" >"$TMPDIR/${base}_bs.out" 2>>"$TMPDIR/${base}_bs.err"; then
          bs_ok=true
        fi
      fi
    fi
  fi

  if $bs_ok; then
    cp "$TMPDIR/${base}_bs.out" "$GOLDEN_DIR/${base}.expected"
    echo "BS    $name  (Go compiler failed)"
    bs_pass=$((bs_pass + 1))
  else
    echo "FAIL  $name  (both compilers failed)"
    bs_fail=$((bs_fail + 1))
    if [ -f "$TMPDIR/${base}_go.err" ]; then echo "  Go err: $(head -3 "$TMPDIR/${base}_go.err")"; fi
    if [ -f "$TMPDIR/${base}_bs.err" ]; then echo "  BS err: $(head -3 "$TMPDIR/${base}_bs.err")"; fi
  fi
done

echo ""
echo "=== Golden Output Generation ==="
echo "Go compiler:    $go_pass generated, $go_fail failed"
echo "Bootstrap:      $bs_pass generated, $bs_fail failed"
echo "Total golden:   $((go_pass + bs_pass)) files in $GOLDEN_DIR/"
