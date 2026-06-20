# Lyric Roadmap

*Updated 2026-06-20. Tracks completed work and remaining priorities.*

For language-level bugs and surgical fixes, see `TODO.md` (especially the "Spec / Reference Cleanup Findings" section added 2026-06-20).

---

## Completed

### Self-Hosting (Phases 1-3) ✅
- Bootstrap compiler written in Lyric (~15K lines across 14 .ly files)
- Three-stage self-compilation with fixed point verification
- Go compiler retired to `legacy/go-compiler/`
- 76/76 tests passing, 101K lines generated C

### Memory Management ✅
- **AoS slab allocator** — block-based, pointer handles, `lyric_next` free list
- **SoA slab allocator** — parallel arrays, uint32_t handles, `--soa` flag
- **Deterministic destruction** — cascade delete through ownership relations, slab free on destroy
- **Benchmarks** (self-compile): SoA 0.20s / 295 MB vs AoS 0.22s / 344 MB

### Language Features ✅
- Module/import system (directory-based, `lyric.mod`)
- String `+` concat, slice `+` concat, `.extend()`
- `as` cast, `is` operator, if-expressions
- `if let` / `let..else`
- Multi-pattern match, match guards, nested enum match, destructuring
- `mut` parameters (Swift `inout` pattern)
- F-strings, triple-quote strings
- Generators (Duff's device state machine)
- `Dict<K,V>` with `Hashable` where-clause, `Sym` keys
- Multi-class interfaces, `impl..for`, default impls, `embed`
- Relations (owns/refs) with field injection and back-pointers
- `spawn` with auto-capture-by-reference, channels, `select`, `lock(mu)`

---

## Phase 4: Language Ergonomics

### Type system soundness (spec/reference audit, 2026-06-20)

- [ ] **Tighten `null` assignability** — currently assignable to every type; should be `T?` / class / interface / error only
- [ ] **Tighten `as` cast** — currently accepts any target type; restrict to numeric ↔ numeric (or document as wide-cast and add other safety checks)
- [ ] **Make narrowing / cross-sign integer assignment explicit** — today `i32 ↔ u8` is silent
- [ ] **Enforce `implements I` method completeness** at check time (today: declarative only)
- [ ] **Unify branch types for `if` / `match` as expressions** (today: first branch wins, others unchecked)

### Operators (spec/reference audit, 2026-06-20)

- [ ] **Add `~` (unary bitwise NOT)** — lexer has no token; LIR `UnBitNot` is phantom
- [ ] **Add compound bit assigns**: `%= &= |= ^= <<= >>=`
- [ ] **Fix bitwise precedence** — promote `& | ^ << >>` above non-integer ops (above `== != < <= > >=` and above `&& ||`). Today inherits C's broken precedence (`a & 1 == 0` mis-parses). Bitops take integers and return integers; arithmetic tier, not boolean tier
- [ ] **Hex / octal / binary integer literals** (`0xFF`, `0o755`, `0b1010`)
- [ ] **Integer type suffixes** (`123u64`)

### Phantom / obsolete to remove (spec/reference audit, 2026-06-20)

- [ ] **Remove `cascade { body }` from the language** — obsolete from earlier design; today's cascade lives in `owns` / `refs` on relations; current code path is a no-op
- [ ] **Remove `defer` / `StDefer` / `LDeferData`** — no lexer token, no parser path, no use site
- [ ] **Remove `Mutex` (capital M) recognition** in `lowerer.lower_named_type` — standardize on lowercase `lock`
- [ ] **Rename `KNil` / `Nil` → `KNull` / `Null`** — kind name is historical; today the lexer emits it for `null`
- [ ] **Decide `map[K]V` fate** — parses but C backend emits stub `void*`; either implement or remove from grammar
- [ ] **Decide Go-style stdlib aliases fate** — `fmt.Println`, `strconv.Itoa`, etc. are hardcoded in `c_backend.emit_call_expr` and emit working code. Officially document as legacy or remove

### Imports (spec/reference audit, 2026-06-20)

- [ ] **Bare `import "path"` form** — null-derefs in `modules.ly:36`; reject in parser OR derive alias from path basename
- [ ] **Recursive import resolution** — imports of imports today silently dropped
- [ ] **`pub`-filtering on imports** — all declarations currently accessible
- [ ] **Circular import detection**
- [ ] **Parse `lyric.mod` content** — today only file existence matters
- [ ] **Recursive subdirectory `.ly` discovery** in module mode

### Desugar — error on silent skips (spec/reference audit, 2026-06-20)

- [ ] Error on undefined `embed Iface`
- [ ] Error on bad `relation Hint Parent ...` hint (today silently skipped)
- [ ] Fix `Lock()` Stmt and `Match()` Expr enum-variant name collisions causing shallow-copy fallbacks in `deep_copy_*`

### Function annotations (entire table currently roadmap)

The function-annotation table in the spec (`requires:`, `ensures:`, `raises:`, `concurrent:`, `requires_lock`, `excludes_lock`, `spawns:`, `pure:`) is not parsed today. Only `guarded_by(name)` on class fields is implemented.

- [ ] **`requires:` / `ensures:`** — pre/postconditions (initially design-doc; later runtime)
- [ ] **`raises: E1, E2`** — named error conditions
- [ ] **`requires_lock` / `excludes_lock`** — runtime enforcement of `guarded_by`
- [ ] **`concurrent:` / `spawns:` / `pure:`** — concurrency / purity contracts
- [ ] **`destroys` annotation** — compiler-inferred mark on functions that may destroy class instances; static UAF prevention
- [ ] **`mut resize` annotation** — prevent element-access during slice resize

### Larger numeric tower

- [ ] **`i128 i256 u128 u256 f128`** — register in checker, add LType variants, emit via `__int128` / compiler-rt

### Must-Fix (high friction)

- [ ] **`lyric fmt` lexer bug** — keywords inside string literals are tokenized as keywords, breaking formatting. Showstopper for dogfooding
- [ ] **String methods** — `.contains()`, `.starts_with()`, `.split()`, `.replace()`, `.trim()` etc. via stdlib
- [ ] **Generic sort** — stdlib sort function
- [ ] **`for..in` on Dicts** — `for key, value in d { ... }` (today: `for k in d.keys()`)

### Should-Fix

- [ ] **UTF-8 support** — lexer is ASCII-only; `string` operations are byte-oriented. Need: `\u{NNNN}` escapes, code-point iteration (`for c in s.chars()`), Unicode-aware case ops, normalization. Keep `string` as type name; UTF-8 sits on top
- [ ] **`Hashable.equals`** — interface today requires only `get_hash`; add `equals` to make hash tables work correctly with custom keys
- [ ] **Error stack traces** — currently errors are bare strings; add source location
- [ ] **Deep `==` for structs and slices** — field-by-field / element-by-element comparison
- [ ] **`not` keyword** — `not x` as alternative to `!x`
- [ ] **Generic lambdas** — lambda type_param_names is always empty today

### Nice-to-Have

- [ ] **Operator overloading** — custom `==`, `<`, etc.
- [ ] **UFCS** — `x.foo()` as sugar for `foo(x)`
- [ ] **Named impls** — `as byEmail` disambiguation
- [ ] **First-arg-wins type inference** — reduce need for explicit `<T>`
- [ ] **Checker-side generic constraint validation** — currently only structural at monomorphization; user-defined constraints accepted but minimally validated
- [ ] **Block comments** `/* */`
- [ ] **`.ly` formatter** — today `lyric fmt` handles `.lyric` only

---

## Phase 5: Reference Counting

Deterministic memory management for unowned classes. The slab allocator handles owned objects via relations; ref-counting covers the rest.

- [ ] **Ref-count field injection** — compiler-inserted `_rc` field on unowned classes
- [ ] **Automatic retain/release** — compiler inserts `_rc++` on assignment, `_rc--` on scope exit / overwrite
- [ ] **Cycle detection strategy** — weak references, or accept leak-on-cycle with documentation
- [ ] **Interaction with slab allocator** — ref-counted objects can live in slabs; free returns to free list when `_rc` hits 0
- [ ] **`ref` / `mut ref` bindings** — zero-copy views (from memory management design doc)
- [ ] **`trusted` blocks** — opt out of safety checks (chosen over `unsafe` to avoid Rust baggage)

---

## Phase 6: Concurrency (C Backend Polish)

AST/LIR/C backend support exists for spawn/channels/select/lock. Needs hardening and real-world testing.

- [ ] **Thread pool** — currently raw `pthread_create` per spawn; need bounded thread pool
- [ ] **Channel correctness** — stress-test bounded/unbounded MPMC channels
- [ ] **Select fairness** — current polling loop; consider proper blocking select
- [ ] **Generator coroutines** — Duff's device works but fragile; evaluate stackful coroutines
- [ ] **Race detector** — optional instrumentation mode

---

## Phase 7: Tooling & Ecosystem

- [ ] **LSP server** — completion, go-to-definition, diagnostics
- [ ] **Source maps** — map C output back to Lyric source for debugging
- [ ] **LLVM backend** — optimization beyond GCC; eventual C backend replacement
- [ ] **Package manager** — dependency resolution, versioning
- [ ] **i128/i256 support** — via compiler-rt or `__int128` (also listed under Phase 4)
- [ ] **Per-test timing** in test runner output (or remove `(0.1ms)` from documented examples)
- [ ] **`assert_eq_approx`** for floats
- [ ] **Test filtering** (`lyric test --filter pattern`)
- [ ] **Rewrite `lyric.lyric`** (stale — describes the old Go-compiler `pkg/` layout)
- [ ] **Rewrite `stdlib/stdlib.lyric`** header (references `pkg/ast/stdlib.go`)

---

## Known Bugs

- [x] `any_type.ly` — was int-to-void* boxing; now passes (fixed by earlier sessions)
- [x] `interfaces.ly` — was where-clause generic return type; now passes (fixed by earlier sessions)
- [x] `arraylist.ly` — was relation field injection; now passes (fixed by earlier sessions)
- [x] `collectUsedTemps` walker — ExMakeSlice/ExFormat were already handled; added ExMakeMap (2026-06-16)
- [x] **Slice methods `is_empty`, `first`, `last`, `remove`, `index_of`, `clear`, `reverse`** — were missing from lowerer dispatch, c_backend emit_builtin, optimizer `is_side_effect_expr` list, and checker `check_list_method`. All added 2026-06-16. New test: `testdata/slice_methods.ly`.
- [ ] External methods not registered in global scope
- [ ] `LTyError` → `const char*` wrong (should be `lyric_string`)
- [ ] Several bootstrap TODOs in checker/parser (see `grep TODO src/`)
- [x] **Selective stdlib merge misses class literals in function bodies** — Fixed 2026-06-16: added StructLit arm to `ast_collect_call_names_expr` in `src/ast/ast.ly`.
- [x] **Lock field emits incompatible C** — Fixed 2026-06-16: `lock` is now a lowercase builtin type handled by checker `check_struct_lit` and lowerer `lower_named_type`. `Lock` → `lock` standardized everywhere.
- [ ] **Interface method mangling last-writer-wins** — monomorphizer uses `orig.name` instead of `mangle_name(orig.name, types)` for interface methods; C backend also needs child type in emitted name. Multi-class interface methods (e.g. `MyList.add()`) don't get properly emitted. (Known, confirmed 2026-06-14)

---

## Design Decisions Log

| Decision | Rationale | Date |
|----------|-----------|------|
| C backend before LLVM | Clang gives free optimizations; C is debuggable | 2026-05 |
| No GC — slab + deterministic destruction | Compiler doesn't need GC; relations handle ownership | 2026-05 |
| Monomorphization-first, vtables later | Simpler codegen; vtables for code size optimization | 2026-05 |
| AoS pointer slab as default | Simpler codegen, proven correct first | 2026-06 |
| SoA as opt-in `--soa` | Better perf but more complex codegen; let user choose | 2026-06 |
| `trusted` over `unsafe` | Avoids Rust baggage | 2026-06 |
| `mut` over `&` for pass-by-reference | Communicates intent, no pointer types in surface language | 2026-06 |
| Block scoping over variable renaming | Bill's directive — cleaner generated C | 2026-06 |
| Ref-counting for unowned classes | Deterministic, no GC pause, fits slab model | 2026-06 |
