#!/bin/bash
# test_self_compile.sh — Two-stage src self-compilation test
# Stage 1: Build lyric from checked-in lyric.c, compile src → lyric_stage2.c
# Stage 2: Build from lyric_stage2.c, compile src → lyric_stage3.c
# Fixed point: lyric_stage2.c == lyric_stage3.c
set -e

cd "$(dirname "$0")"

BOOTSTRAP_FILES=(
  src/ast/ast.ly src/ast/modules.ly
  src/lexer/lexer.ly
  src/parser/parser.ly
  src/parser/expr_parser.ly
  src/desugar/desugar.ly
  src/checker/checker.ly
  src/lir/lir.ly
  src/lowerer/lowerer.ly
  src/optimizer/optimizer.ly
  src/monomorphizer/monomorphizer.ly
  src/memory/memory.ly
  src/c_backend/c_backend.ly
  src/main/main.ly
)

TMPDIR=$(mktemp -d)
trap "rm -rf $TMPDIR" EXIT

echo "=== Stage 0: Build lyric from checked-in lyric.c ==="
make -s lyric
echo "OK"
echo ""

echo "=== Stage 1: lyric compiles itself → lyric_stage2.c ==="
time ./lyric compile "${BOOTSTRAP_FILES[@]}" -o "$TMPDIR/lyric_stage2.c" 2>&1
echo ""
echo "Compiling lyric_stage2.c with GCC..."
time gcc -std=gnu11 -O2 -w -o "$TMPDIR/lyric_stage2" "$TMPDIR/lyric_stage2.c" -I runtime/
echo "lyric_stage2: $(wc -l < "$TMPDIR/lyric_stage2.c") lines C"
echo ""

echo "=== Stage 2: lyric_stage2 compiles itself → lyric_stage3.c ==="
time "$TMPDIR/lyric_stage2" compile "${BOOTSTRAP_FILES[@]}" -o "$TMPDIR/lyric_stage3.c" 2>&1
echo ""
echo "lyric_stage3: $(wc -l < "$TMPDIR/lyric_stage3.c") lines C"
echo ""

echo "=== Comparing Stage 1 and Stage 2 outputs ==="
if diff -q "$TMPDIR/lyric_stage2.c" "$TMPDIR/lyric_stage3.c" > /dev/null 2>&1; then
  echo "✅ FIXED POINT REACHED — lyric_stage2.c == lyric_stage3.c"
else
  echo "❌ lyric_stage2.c != lyric_stage3.c"
  diff "$TMPDIR/lyric_stage2.c" "$TMPDIR/lyric_stage3.c" | head -40
  exit 1
fi

# Also verify lyric.c matches (it should be identical to what lyric produces)
echo ""
echo "=== Verifying checked-in lyric.c is current ==="
if diff -q lyric.c "$TMPDIR/lyric_stage2.c" > /dev/null 2>&1; then
  echo "✅ lyric.c matches compiler output"
else
  echo "⚠️  lyric.c differs from compiler output — run 'make update' to refresh"
  diff lyric.c "$TMPDIR/lyric_stage2.c" | head -20
fi
