# Forge Phase 1 — Implementation Plan

*All changes from phase1-language-review.md, ordered by dependency and priority.*

---

## Change 1: `as` Cast Syntax (replaces `<T>x`)

**What**: Replace prefix `<T>(expr)` with postfix `expr as T`. `T` can be a type literal or variable/expression (cast to its type).

### Files to modify

**Lexer** (`pkg/parser/lexer.go`):
- Add `TAs` token kind. `as` is a new keyword.

**Parser** (`pkg/parser/expr_parser.go`):
- Remove `parseAngleCast()` (or whatever handles `<T>(expr)` prefix).
- Add `as` as a postfix operator in the Pratt parser at a precedence between comparison and additive (Rust puts it above comparison). In `parseInfix()`, when `TAs` is seen, parse the right side as a type expression.
- Produce `ExprCast` AST node with same `CastExpr{Target, Operand}` structure.

**Checker** (`pkg/checker/checker.go`):
- No change — already validates `ExprCast` regardless of syntax.

**Lowerer** (`pkg/lir/lowerer.go`):
- No change — already handles `ExprCast` → `LExprCast`.

**C backend** (`pkg/lir/c_backend.go`):
- No change — emits `((target_type)value)`.

**Bootstrap lexer** (`bootstrap/lexer/lexer.fg`):
- Add `TAs` to `TokenKind` enum and keyword map.

**Bootstrap parser** (`bootstrap/parser/expr_parser.fg`):
- Mirror the Go parser changes.

**Testdata**: Update `testdata/advanced.fg`, `testdata/features.fg`, and any other files using `<T>()` syntax.

**Estimated effort**: 2-3 hours.

---

## Change 2: `?` Operator Returns `T`, Not `T?`

**What**: After `let x = foo()?`, `x` should be `T`, not `T?`. This matches Rust/Swift semantics.

### Files to modify

**Checker** (`pkg/checker/checker.go`):
- In the `ExprTry` case: currently sets `ResolvedType` to the optional of the success type. Change to set it to the success type directly (`T`, not `T?`).

**Lowerer** (`pkg/lir/lowerer.go`):
- In the `?` lowering: currently produces an optional result. Change to produce the unwrapped `T` directly. The error check + early return stays the same; the difference is the final temp holds `T` not `T?`.
- Remove `hoistNestedTry`'s optional wrapping if applicable.

**Bootstrap checker** (`bootstrap/checker/checker.fg`):
- Mirror Go checker change.

**Bootstrap lowerer** (`bootstrap/lowerer/lowerer.fg`):
- Mirror Go lowerer change.

**Testdata**: Update `testdata/try_operator.fg`, `testdata/nested_try.fg`, `testdata/errors.fg`. Remove all `x!` unwraps after `?` calls.

**Bootstrap .fg files**: Remove all `x!` after `?` in lexer.fg, parser.fg, expr_parser.fg, checker.fg, lowerer.fg, desugar.fg. This will be a substantial cleanup (dozens of sites).

**Estimated effort**: 3-4 hours.

---

## Change 3: `is` Operator

**What**: `expr is Variant` returns `bool`. Sugar for `match expr { Variant(_) => true, _ => false }`.

### Files to modify

**AST** (`pkg/ast/expr.go`):
- Add `ExprIs` kind. Data: `IsExpr { Operand Expr, VariantName string }`.

**Lexer** (`pkg/parser/lexer.go`):
- Add `TIs` token kind and keyword.

**Parser** (`pkg/parser/expr_parser.go`):
- Add `is` as an infix operator at comparison precedence. RHS is an identifier (variant name), not an expression.

**Checker** (`pkg/checker/checker.go`):
- Validate operand is an enum type, variant name exists on that enum. Set `ResolvedType = TypeBool`.

**Lowerer** (`pkg/lir/lowerer.go`):
- Lower `ExprIs` to tag comparison: `LExprBinOp(LExprVariantTag(operand), ==, variant_tag_constant)`.

**C backend**: No change — already handles tag comparisons.

**Bootstrap**: Add `ExprIs` to ast.fg, `TIs` to lexer.fg, parser support in expr_parser.fg, checker and lowerer support.

**Testdata**: Add `testdata/is_operator.fg` with comprehensive tests.

**Estimated effort**: 3-4 hours.

---

## Change 4: If-Expression

**What**: `if cond { a } else { b }` usable as an expression. Both branches required, same type.

### Files to modify

**AST** (`pkg/ast/expr.go`):
- Add `ExprIf` kind. Data: `IfExpr { Cond *Expr, Then *Expr, Else *Expr }`. Or reuse `StmtIf` and mark it as expression context.

**Parser** (`pkg/parser/expr_parser.go`):
- In expression parsing context, when `if` is encountered, parse as `ExprIf`. Distinguish from statement `if` by context (expression position vs statement position). The `exprDepth` counter or parse context already provides this.

**Checker** (`pkg/checker/checker.go`):
- Check both branches, verify same type. Set `ResolvedType` to that type. `else` branch is required (error if missing).

**Lowerer** (`pkg/lir/lowerer.go`):
- Lower to: declare temp, emit `LStmtIf` with assignments to temp in each branch, result is temp. Similar to how `match` expressions are lowered.

**C backend**: No change — uses existing if/temp pattern.

**Bootstrap**: Add `ExprIf` to ast.fg, parser support, checker/lowerer support.

**Testdata**: Add `testdata/if_expr.fg` with tests including nested if-expressions, type mismatch errors, missing else errors.

**Estimated effort**: 3-4 hours.

---

## Change 5: Fix `forge fmt` String Literal Bug

**What**: Lexer tokenizes keywords inside string literals as keywords, breaking the formatter.

### Files to modify

**Formatter lexer** (`pkg/formatter/`):
- The issue is likely in the formatter's own lexer or its reuse of the parser lexer. The fix: when inside a string literal state (between `"` delimiters, or inside `"""` triple-quotes, or inside f-string `{}`), emit string content tokens, not keyword tokens.
- Check if the formatter uses the same lexer as the parser or has its own. If same lexer, the bug may be there.

**Actual root cause**: Look at `pkg/parser/lexer.go` `scanToken()` — does it check for string state before keyword matching? The fix is likely in `scanString()` or the main scan loop.

**Estimated effort**: 1-2 hours.

---

## Change 6: Spec & Reference Cleanup (DONE)

Already completed in this session:
- Removed stale Go backend references
- Updated `?` operator semantics
- Added `as` cast syntax
- Added `is` operator
- Added if-expression
- Updated Known Gotchas
- Updated bootstrap status (0 errors, 0 warnings, 100/100 tests)

---

## Change 7: Map Type and Map Literals

**What**: Add `map[K]V` as a built-in type with literal syntax `{"key": value}` and `{:}` for empty.

### Files to modify

**AST** (`pkg/ast/expr.go`, `pkg/ast/ast.go`):
- Add `ExprMapLit` kind. Data: `MapLitExpr { Entries []MapEntry }` where `MapEntry { Key, Value Expr }`.
- Add `TypeMap` to type expressions: `MapType { Key, Value TypeExpr }`.

**Lexer** (`pkg/parser/lexer.go`):
- No new tokens needed — `{`, `}`, `:` already exist. `map` needs to be a keyword (add `TMap`).

**Parser** (`pkg/parser/parser.go`, `expr_parser.go`):
- Parse `map[K]V` in type positions: when `TMap` followed by `[`, parse key type, `]`, value type.
- Parse `{key: value, ...}` as map literal in expression position. Disambiguate from blocks: `{` followed by `expr :` is a map literal; `{:}` is empty map.
- Empty map `{:}` already partially handled — extend to full map literals.

**Checker** (`pkg/checker/checker.go`):
- Type-check map literals: all keys must be same type, all values must be same type. Infer `map[K]V` from entries.
- Validate key type is comparable.
- Index expression `m[key]` on map type returns `V?`.
- Index assignment `m[key] = value` — new statement form or reuse existing.
- Add `delete` builtin.

**Lowerer** (`pkg/lir/lowerer.go`):
- Lower `ExprMapLit` to: allocate map, insert each entry.
- Lower map index to runtime lookup call.
- Lower map index-assign to runtime insert call.

**C backend** (`pkg/lir/c_backend.go`):
- Emit map as a generic hash table struct. Options: (a) use existing `HashedList` runtime machinery, (b) add a new `forge_map` runtime type in `grok_runtime.h`.
- `forge_map_new()`, `forge_map_get()`, `forge_map_set()`, `forge_map_delete()`, `forge_map_len()`.
- Map iteration: emit as a for loop over buckets.

**Runtime** (`grok_runtime.h`):
- Add generic map implementation. Open-addressing hash table with FNV-1a for string keys, identity hash for integers. Monomorphizer specializes per `K,V` pair.

**Bootstrap**: Add `TMap` to lexer.fg, map type parsing to parser, map lit to expr_parser. Not needed immediately in bootstrap .fg files (they use Dict<V>).

**Testdata**: Add `testdata/maps.fg` with construction, lookup, insert, delete, iteration, type inference, nested maps.

**Estimated effort**: 6-8 hours. This is the largest single change — touches parser, checker, lowerer, C backend, and runtime.

---

## Change 8: Stdlib Additions (Future)

These are needed before `main.fg` but not blocking the immediate compiler changes:

### 7a. File I/O stdlib
- `File.open()`, `File.create()`, `File.read_all()`, `File.write_string()`
- `Reader`/`Writer` interfaces
- C backend: `fopen`/`fclose`/`fread`/`fwrite`

### 7b. Sort
- `sort<T>(slice: [T], less: fn(T, T) -> bool)` generic sort
- C backend: qsort wrapper with closure context

### 7c. String→number parsing in stdlib .fg files
- Move `atoi`/`parse_float`/`itoa` from hardcoded builtins to stdlib .fg declarations

---

## Recommended Order

| # | Change | Deps | Effort | Priority |
|---|--------|------|--------|----------|
| 1 | `?` returns `T` | None | 3-4h | **Critical** — affects all error handling |
| 2 | `is` operator | None | 3-4h | **High** — needed for compiler code |
| 3 | `as` cast syntax | None | 2-3h | **High** — cleaner syntax |
| 4 | If-expression | None | 3-4h | **Medium** — ergonomic improvement |
| 5 | `forge fmt` fix | None | 1-2h | **Medium** — needed for dogfooding |
| 6 | Spec cleanup | None | Done | ✅ |
| 7 | `map[K]V` + literals | None | 6-8h | **High** — needed for compiler tables |
| 8 | Stdlib I/O | None | 4-6h | **Later** — before main.fg |

Changes 1-5 and 7 are independent and can be done in any order. I'd recommend doing `?` first since it has the biggest impact on existing code — the bootstrap .fg files will get significantly cleaner.

**Total estimated effort: 19-27 hours across changes 1-5 and 7.**

After these changes, all bootstrap .fg files should be rewritten to use the new syntax (Phase 2).
