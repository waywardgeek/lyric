#!/bin/bash
# build_bootstrap.sh — Build the Lyric compiler from source
# Uses the checked-in lyric.c binary to compile src source.
# Usage: ./build_bootstrap.sh [output_binary]
# Default output: ./lyric
set -e

cd "$(dirname "$0")"

OUTPUT="${1:-./lyric}"

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
  src/c_backend/c_backend.ly
  src/main/main.ly
)

TMPDIR_BUILD=$(mktemp -d -t lyric_build_XXXXXX)
trap "rm -rf $TMPDIR_BUILD" EXIT

# Build from checked-in C if no lyric binary exists
if [ ! -f ./lyric ]; then
  echo "=== Building lyric from lyric.c ==="
  gcc -std=gnu11 -O2 -w -I runtime -o ./lyric lyric.c -lm
fi

echo "=== Compiling src → C ==="
./lyric compile "${BOOTSTRAP_FILES[@]}" -o "$TMPDIR_BUILD/lyric_new.c" 2>&1

echo "=== GCC compile ==="
gcc -std=gnu11 -O2 -w -I runtime -o "$OUTPUT" "$TMPDIR_BUILD/lyric_new.c" -lm
echo "=== Built: $OUTPUT ==="
