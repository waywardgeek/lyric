# Lyric Roadmap

*Updated 2026-06-13. Tracks completed work and remaining priorities.*

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

### Must-Fix (high friction)

- [ ] **`lyric fmt` lexer bug** — keywords inside string literals are tokenized as keywords, breaking formatting. Showstopper for dogfooding
- [ ] **String methods** — `.contains()`, `.starts_with()`, `.split()`, `.replace()`, `.trim()` etc. via stdlib
- [ ] **Generic sort** — stdlib sort function
- [ ] **Map literal codegen** — `{"key": val}` syntax parses but lowerer has `// TODO`
- [ ] **`for..in` on maps** — `for key, value in m { ... }`

### Should-Fix

- [ ] **UTF-8 support** — lexer is ASCII-only. Need proper UTF-8 decoding for identifiers and string ops
- [ ] **Error stack traces** — currently errors are bare strings; add source location
- [ ] **Generic HashMap** — `map[K]V` with non-string keys (currently Dict is string-keyed only)
- [ ] **Deep `==` for structs and slices** — field-by-field / element-by-element comparison
- [ ] **`not` keyword** — `not x` as alternative to `!x`

### Nice-to-Have

- [ ] **Operator overloading** — custom `==`, `<`, etc.
- [ ] **UFCS** — `x.foo()` as sugar for `foo(x)`
- [ ] **Named impls** — `as byEmail` disambiguation
- [ ] **First-arg-wins type inference** — reduce need for explicit `<T>`
- [ ] **Checker-side generic constraint validation** — currently only at monomorphization

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
- [ ] **i128/i256 support** — via compiler-rt or `__int128`

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
