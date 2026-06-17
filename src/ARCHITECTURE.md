# Lyric Compiler Architecture

Self-hosting bootstrap of the Lyric compiler. Ports the Go compiler (parser → checker → lowerer → LIR) into Lyric itself, targeting the C backend via monomorphization.

## Architecture

Seven packages form a complete frontend pipeline:

Source → lexer/ → parser/ → ast/ → desugar/ → checker/ → lowerer/ → lir/

Compile all packages together:
lyric compile --c bootstrap/ast/ast.ly bootstrap/lexer/lexer.ly \
bootstrap/parser/parser.ly bootstrap/parser/expr_parser.ly \
bootstrap/desugar/desugar.ly bootstrap/checker/checker.ly \
bootstrap/lir/lir.ly bootstrap/lowerer/lowerer.ly

Output: a single C file (~30K lines) linked against lyric_runtime.h.

## Dependency Graph

ast      — leaf (no deps)
lexer    — uses ast (Token, Span, Pos)
parser   — uses ast + lexer
desugar  — uses ast
checker  — uses ast
lir      — uses ast
lowerer  — uses ast + lir + checker

## Invariants — Cross-Cutting

These invariants apply across all bootstrap packages:

1. NULLABLE VARIANT FIELDS: AST Expr/Stmt/TypeExpr/Pattern and LIR
LExpr/LStmt use nullable fields per variant (not Go's any+Kind).
Only the field matching `kind` is non-null. Access pattern:
`if node.variant_field != null { let v = node.variant_field! ... }`

2. DICT<V> WITH SYM KEYS: All registries (checker, lowerer) use
Dict<V> with Sym keys. API: dict_get(d, sym(name)) -> V?
Dict uses open-addressing hash with 75% load rehash.

3. NO IF-EXPRESSIONS: Lyric doesn't support if-as-expression.
Use: let mut x = default; if cond { x = value }

4. NO LINE CONTINUATION: Multi-condition expressions must be on one
line or extracted into helpers.

5. TWO-PHASE PATTERN: Both checker and lowerer use two phases:
Phase 1 registers all declarations, Phase 2 processes bodies.
Phase 1 across ALL files before ANY Phase 2 (for multi-file).

6. ENUM VARIANT NAME COLLISIONS: Multiple enums share variant names
(Ident, Tuple, Match). Checker type annotations are authoritative
for disambiguation. Lowerer/backend must consult them.

## Differences from Go Compiler

Key differences that affect invariant translation:

- Go's any-typed Data + Kind → Lyric's nullable variant fields
- Go's map[string]T → Lyric's Dict<V> with Sym keys
- Go's *Expr (pointer) annotations → Lyric's class-based Expr
(classes are heap-allocated, always pointers in C)
- Go's if-expression fallback → Lyric's mutable variable pattern
- Go's range-copy bug → not applicable (classes are pointers)
- Go's dataAs[T] helper → direct nullable field access

## Remaining Work

To reach self-hosting, the bootstrap needs:
1. optimizer.ly — side-effect elimination, multi-return destructuring
2. monomorphizer.ly — generic specialization (required for C)
3. c_backend.ly — C code emission from LIR
4. main.ly — CLI entry point
5. --check-invariants flag — runtime invariant validation at each stage

## Test Status

Compile: lyric compile --c (all 8 files) → 0 GCC errors, 0 warnings
Tests: lyric test → 100/100 passing (lexer 31, parser 52, desugar 12, test_min 5)
Output: 8,049 lines Lyric → 30,094 lines C

