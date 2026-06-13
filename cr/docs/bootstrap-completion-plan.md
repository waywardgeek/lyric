# Bootstrap Compiler Completion Plan

## Goal
Port the full Lyric compiler pipeline to Lyric, producing a self-hosting bootstrap compiler
that compiles .ly files → C → executable via GCC.

## Current State (as of 440f08c)
- **Done**: AST (409), Lexer (680), Parser (1412), ExprParser (1322), Desugar (1264),
  Checker (3215), LIR types (630), Lowerer (2642) = **11,574 lines Lyric**
- **Compiles**: 0 GCC errors, 0 warnings, all 249+ Go tests pass
- **Missing**: C backend, monomorphizer, optimizer, main.ly, I/O builtins, `list_dir` runtime

## What Needs Porting

| Component | Go file(s) | Go lines | Go funcs | Est. Lyric lines | Phase |
|-----------|-----------|----------|----------|-------------------|-------|
| I/O builtins + `list_dir` | runtime + checker + c_backend | ~100 | ~3 | ~80 | 1 |
| Optimizer | optimize.go | 1,055 | 25 | ~800 | 2 |
| Monomorphizer | monomorphize.go | 1,959 | 46 | ~1,500 | 3 |
| C backend | c_backend.go | 4,038 | 94 | ~3,200 | 4 |
| main.ly (compile + test) | cmd/lyric/main.go, shared.go | 759 | 20 | ~500 | 5 |
| **Total** | | **7,911** | **188** | **~6,080** | |

## Phase Details

### Phase 1: I/O Builtins + list_dir (~80 lines, ~30 min)
Add missing OS primitives needed by main.ly:
- **`list_dir(path: string) -> [string]`** — new builtin: checker registration, C backend case,
  runtime implementation (`opendir`/`readdir`/`closedir`). Returns list of filenames.
- **`file_exists(path: string) -> bool`** — new builtin for `findRuntimeDir`/`findStdlibDir`.
- **`mkdtemp(prefix: string) -> string`** — for `lyric test` temp directories.
- **Checker**: register these in `registerBuiltins()`.
- **C backend**: add cases in `emitBuiltinCall()`.
- **Runtime**: add `lyric_list_dir()`, `lyric_file_exists()`, `lyric_mkdtemp()` to `lyric_runtime.h`.
- **Test**: write a small .ly test that calls each.

### Phase 2: Optimizer (~800 lines, ~2 hours)
Port `pkg/lir/optimize.go` → `bootstrap/optimizer/optimizer.ly`.
- 25 functions, mostly statement-level transforms on LIR.
- Works on `[LStmt]` arrays — no new types needed, uses existing LIR types.
- Key passes: side-effect temp elimination, multi-return destructuring, for-range coercion,
  optional return wrapping.
- Operates purely on LIR — no AST dependency, no checker dependency.
- **Prerequisite**: None beyond existing LIR types.

### Phase 3: Monomorphizer (~1,500 lines, ~4 hours)
Port `pkg/lir/monomorphize.go` → `bootstrap/monomorphizer/monomorphizer.ly`.
- 46 functions, 6 phases:
  0. Pre-scan to mark generic functions/classes
  1. Collect generic instantiations from function bodies
  1a. Collect from class/struct field types (new Phase 1a)
  2. Generate specialized copies (iterative for transitive generics)
  3. ValidatePostMono — no type vars survive
  4. Substitute type params in ALL signatures
  5. ResolveClassNames pre-pass with per-function ClassRenameMap
  6. RewriteImplRenames
- Heavy use of `Dict<T>` for rename maps, type maps.
- **Prerequisite**: LIR types (done), Dict (done).

### Phase 4: C Backend (~3,200 lines, ~8 hours)
Port `pkg/lir/c_backend.go` → `bootstrap/c_backend/c_backend.ly`.
- 94 functions. The largest single module.
- Core structure: `CGenerator` class with `StringBuilder` for output.
- Key subsystems:
  - Type emission (forward decls, structs, enums, optionals, slices, tagged unions)
  - Topological sort for type ordering (Kahn's algorithm)
  - Expression emission (all LExpr kinds → C expressions)
  - Statement emission (all LStmt kinds → C statements)
  - Builtin call emission (40+ builtin functions)
  - Printf/println formatting
  - Generator (Duff's device) and channel/spawn emission
  - Test runner generation
- Heavy string building — this is where `StringBuilder` (already in stdlib) pays off.
- **Prerequisite**: LIR types (done), StringBuilder (done), all builtins registered.

### Phase 5: main.ly (~500 lines, ~1 hour)
Port `cmd/lyric/main.go` → `bootstrap/main.ly`.
- Initially just `compile` and `test` commands.
- CLI arg parsing (simple switch on `args[1]`).
- Pipeline: parse → merge → merge_stdlib → desugar → check → lower → optimize →
  monomorphize → rewrite_impl_renames → emit_c → write_file.
- `lyric test`: additionally discover `test_*` functions, emit test runner, GCC compile, run.
- `loadStdlib`: uses `list_dir` + `read_file` + parse + merge.
- `findRuntimeDir`/`findStdlibDir`: uses `file_exists` + `path_join`.
- **Prerequisite**: All previous phases.

## Dependency Graph
```
Phase 1 (I/O) ──→ Phase 5 (main.ly)
                        ↑
Phase 2 (Optimizer) ────┤
Phase 3 (Monomorphizer)─┤
Phase 4 (C Backend) ────┘
```
Phases 2, 3, 4 are independent of each other but all feed into Phase 5.
Phase 1 is independent and should go first (unblocks testing).

## Execution Order
1 → 2 → 3 → 4 → 5 (serial, each builds on confidence from previous)

## Milestone: Self-Hosting Test
After Phase 5, the bootstrap compiler should be able to:
1. `./lyric compile bootstrap/*.ly -o /tmp/bootstrap.c`
2. `gcc -std=gnu11 -Iruntime /tmp/bootstrap.c -o /tmp/lyric-bootstrap`
3. `/tmp/lyric-bootstrap compile bootstrap/*.ly -o /tmp/bootstrap2.c`
4. `diff /tmp/bootstrap.c /tmp/bootstrap2.c` → identical (fixpoint!)

## Notes
- The Go→Lyric ratio is roughly 0.8x (Lyric is more concise due to `?`, `match`, closures).
- All modules should get `.lyric` UDD files documenting invariants.
- Non-exhaustive match warnings in checker.ly are benign (4 warnings currently).
- `lyric fmt` and `lyric verify/update/gen` are deferred — they're tools for the Go↔Lyric
  bridge which becomes less relevant once we're self-hosting.
