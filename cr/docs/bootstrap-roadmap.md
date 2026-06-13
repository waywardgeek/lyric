# Lyric Bootstrap Roadmap

*Created 2026-06-09. Tracks work remaining to replace the Go compiler with the self-hosted Lyric compiler.*

---

## Phase 2: Bootstrap .ly Alignment (IN PROGRESS)

Rewrite bootstrap .ly files to match current Go compiler behavior. Dependency order:

- [x] ast.ly — ConstDecl, Is/IfElse ExprKind, MatchArm.patterns, RelationSide.type_args
- [x] lexer.ly — semicolons fixed, .lyric updated
- [ ] parser.ly — ConstDecl parsing, contextual keywords (field/lock/implements), annotations (why/source), type alias parsing
- [ ] expr_parser.ly — audit against Go expr_parser.go for missing operators/constructs
- [ ] desugar.ly — audit against Go desugar.go
- [ ] checker.ly — audit against Go checker.go (144 errors were fixed; may have new gaps)
- [ ] lir.ly — audit against Go lir.go
- [ ] lowerer.ly — audit against Go lowerer.go

---

## Phase 3: main.ly + Self-Compilation Test

- [ ] Write `bootstrap/main.ly` — CLI entry point (compile, test, fmt, verify subcommands)
- [ ] File I/O stdlib — `read_file`, `write_file`, `os_args`, `os_exit` (C implementations)
- [ ] Test: bootstrap compiler can compile a simple .ly file to C and produce working output
- [ ] Test: bootstrap compiler can compile itself (self-hosting milestone)

---

## Phase 4: Post-Bootstrap Language Improvements

Once the bootstrap compiler replaces Go, these can be implemented in Lyric itself:

### Must-Fix (blocking or high-friction)

- [ ] **UTF-8 support** — lexer uses ASCII-only (`is_letter`, `is_digit`). Need proper UTF-8 decoding for identifiers, string operations, and source files with non-ASCII content
- [ ] **`lyric fmt` lexer bug** — keywords inside string literals are tokenized as keywords, breaking formatting. Showstopper for dogfooding
- [ ] **Map literal codegen** — `{"key": val}` syntax parses but lowerer has `// TODO: implement map literal lowering`
- [ ] **Module/import system** — currently all files merged into single compilation unit. Need per-directory modules with explicit imports for real projects

### Should-Fix (significant ergonomic improvements)

- [ ] **`for..in` on maps** — `for key, value in m { ... }` (iteration over map entries)
- [ ] **String methods** — `.contains()`, `.starts_with()`, `.split()`, `.replace()`, etc. via stdlib
- [ ] **Sort** — generic sort function in stdlib
- [ ] **Generic HashMap** — `map[K]V` with non-string keys (currently Dict is string-keyed only)
- [ ] **Error stack traces** — currently errors are bare strings; add source location to error interface
- [ ] **Deep `==` for structs and slices** — `==` should work on structs (field-by-field) and slices (element-by-element), not just primitives. Currently forces workarounds like `str_has_prefix`/`str_has_suffix` builtins instead of simple slice comparison. Design decision from pre-bootstrap says deep `==` for structs/tuples/slices, pointer comparison for classes/functions
- [ ] **Operator overloading** — custom `==`, `<`, etc. for user types (lower priority than deep `==`)
- [ ] **`not` keyword** — `not x` as alternative to `!x` (readability)

### Nice-to-Have (quality of life)

- [ ] **LSP server** — IDE integration (completion, go-to-definition, diagnostics)
- [ ] **Source maps** — map C output back to Lyric source for debugging
- [ ] **LLVM backend** — optimization beyond what GCC provides; eventual replacement for C backend
- [ ] **i128/i256 support** — via compiler-rt or __int128 on supported platforms
- [ ] **UFCS** (Uniform Function Call Syntax) — `x.foo()` as sugar for `foo(x)`
- [ ] **Named impls** — `as byEmail` disambiguation for multiple impls of same interface
- [ ] **Checker-side generic constraint validation** — currently only validated at monomorphization time
- [ ] **First-arg-wins type inference** — reduce need for explicit `<T>` on generic calls

---

## Phase 5: Concurrency (C Backend)

The Go backend had channels/spawn/select/lock. C backend needs:

- [ ] **pthreads-based spawn** — goroutine-style with thread pool
- [ ] **Channels** — bounded/unbounded MPMC channels via pthreads
- [ ] **Select** — multiplexed channel operations
- [ ] **Lock** — scoped mutex (already has AST/LIR support, needs C codegen)
- [ ] **Generators** — currently uses Duff's device; may need proper coroutine support

---

## Known Go Compiler Bugs (fix in either Go or bootstrap)

- [ ] `any_type.ly` — int-to-void* needs boxing in C backend
- [ ] `guarded_by.ly` — `Mutex` type undeclared in C (lock/threading not emitted)
- [ ] `interfaces.ly` — where-clause generic `count_children<P,C>` monomorphizes `children()` return to `string` instead of `[File]` slice
- [ ] `arraylist.ly` — pre-existing relation field injection bug in C backend test

---

## Bootstrap TODOs in Code

From `grep TODO bootstrap/`:

| File | Line | Issue |
|------|------|-------|
| checker.ly | 482 | Apply type args for generics |
| checker.ly | 1410 | Annotate ResolvedType (lowerer re-derives types) |
| parser.ly | 61 | Allow annotation/keyword names as identifiers |
| parser.ly | 159 | Store ConstDecl properly (now have AST type) |
| parser.ly | 443 | Store `why` annotation |
| parser.ly | 855 | Parse `source` annotation |
| parser.ly | 948 | Parse function annotations |
| expr_parser.ly | 212 | Store type_args on struct lit |
| lowerer.ly | 395 | Implement map literal lowering |

---

## Test Coverage Gaps

- **37/44 testdata tests pass** (4 skipped: channels, spawn, select, lock)
- **3 real GCC failures** in testdata (any_type, guarded_by, interfaces)
- **51/52 bootstrap parser tests** (1 failure: interface field contextual keyword)
- **8/12 bootstrap desugar tests** (contextual keyword mismatches)
- No bootstrap-level unit tests yet (tests are in testdata/ run by Go test harness)

---

## Design Decisions Log

| Decision | Rationale | Date |
|----------|-----------|------|
| C backend before LLVM | Clang gives free optimizations; C is debuggable | 2026-05 |
| No GC — SoA + arena + deterministic destruction | Compiler doesn't need GC; relations handle ownership | 2026-05 |
| ASCII-only bootstrap lexer | Simplifies C backend; UTF-8 is Phase 4 | 2026-06 |
| `source`/`fake` omitted from bootstrap LyricBlock | Verifier-only metadata, not needed for compilation | 2026-06-09 |
| Monomorphization-first, vtables later | Simpler codegen; vtables for code size optimization | 2026-05 |
