#!/bin/bash
# build_bootstrap.sh — Build bootstrap compiler from source
# Usage: ./build_bootstrap.sh [output_binary]
# Default output: ./bootstrap1
set -e

cd "$(dirname "$0")"

OUTPUT="${1:-./bootstrap1}"
# Clean up old build artifacts
rm -rf /tmp/forge_build_*

TMPDIR_BUILD=$(mktemp -d -t forge_build_XXXXXX)


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

echo "=== Building Go compiler ==="
go build -o "$TMPDIR_BUILD/forge_go" ./cmd/forge/

echo "=== Compiling bootstrap → C ==="
"$TMPDIR_BUILD/forge_go" compile "${BOOTSTRAP_FILES[@]}" -o "$TMPDIR_BUILD/bootstrap.c" 2>&1

echo "=== GCC compile ==="
gcc -std=gnu11 -I runtime -o "$OUTPUT" "$TMPDIR_BUILD/bootstrap.c" -lm
echo "=== Built: $OUTPUT ==="
