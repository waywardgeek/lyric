# Lyric Compiler — Makefile
#
# The Lyric compiler is self-hosting. lyric.c is the checked-in canonical
# compiler output (88K+ lines of C). Building requires only GCC and libc.
#
# Usage:
#   make            — build the lyric binary
#   make test       — run test suite
#   make self-test  — verify fixed-point (lyric compiles itself to identical C)
#   make update     — regenerate lyric.c from src source
#   make clean      — remove build artifacts

CC      ?= gcc
CFLAGS  ?= -std=gnu11 -O2 -w
RUNTIME  = runtime

BOOTSTRAP_FILES = \
  src/ast/ast.ly src/ast/modules.ly \
  src/lexer/lexer.ly \
  src/parser/parser.ly src/parser/expr_parser.ly \
  src/desugar/desugar.ly \
  src/checker/checker.ly \
  src/lir/lir.ly \
  src/lowerer/lowerer.ly \
  src/optimizer/optimizer.ly \
  src/monomorphizer/monomorphizer.ly \
  src/memory/memory.ly \
  src/c_backend/c_backend.ly \
  src/main/main.ly

.PHONY: all test self-test update clean

all: lyric

lyric: lyric.c runtime/lyric_runtime.h
	$(CC) $(CFLAGS) -I $(RUNTIME) -o $@ lyric.c -lm

test: lyric
	@bash test_lyric.sh
	@bash test_cli.sh

self-test: lyric
	@bash test_self_compile.sh

# Regenerate lyric.c from src Lyric source using the current lyric binary
update: lyric
	./lyric compile $(BOOTSTRAP_FILES) -o lyric.c
	@echo "lyric.c updated ($$(wc -l < lyric.c) lines)"

clean:
	rm -f lyric
