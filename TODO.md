# Lyric Compiler Bugs & TODO

## Language Bugs

### `ref`/`unref` on non-class types should be no-op at checker level
- Currently handled by C backend type guard (TyClassHandle check)
- Ideally the checker would warn and the lowerer would skip emission entirely
- Low priority since the C backend guard works
- Found: 2026-06-18

### `let` inside if-expression body doesn't work
- `let ref x = if cond { expr1 } else { let y = ...; y }` fails with parse error
- Workaround: declare the variable before the if-expression
- Found: 2026-06-18

### Scope-exit slice free on reassigned-to-borrowed slices (UAF)
- `let mut x: [T] = []; x = borrowed.fields` — memory pass sees `[]` as fresh, adds scope-exit free, but after reassignment `x` holds borrowed data → UAF
- Fixed in lyric-next with `let ref`, but the underlying memory pass issue remains: no tracking of slice reassignment
- Proper fix: either `let mut ref` (done) or memory pass should free old backing on reassign + not free borrowed at scope exit
- Found: 2026-06-18

## Deferred Features

### `let ref` on structs/classes
- Currently only meaningful for slices (skips scope-exit free)
- For classes, `let ref` should mean "borrowed handle, no RC inc/dec" — semantics TBD
- For structs, `let ref` should mean pointer-to-struct in C (actual zero-copy reference)

### `mut ref` function parameters
- Spec mentions `mut ref` for zero-copy write-through views
- Not yet implemented for function params, only for local bindings

## Known Tech Debt

### `class_renames` global map — last-writer-wins
- Multiple specializations of the same generic class overwrite each other in the global map
- Causes subtle bugs with multi-class interfaces + generics
- Known since 2026-06-09, partially mitigated by per-function ResolveClassNames pre-pass

### `inferExprType` in c_backend.ly
- Type resolution should be done by checker/lowerer, not the backend
- Backend should be a dumb emitter
- Removing it requires propagating types more thoroughly in the lowerer

### Monomorphizer doesn't specialize generic external method return types
- Known gap, causes void* in some edge cases

## BUG: Global Dict<Sym,string> freed by RC, memory reused by lowerer

**Symptom**: `_method_aliases` global Dict in checker.ly has 2 entries after checker phase.
After lowerer phase, `get_method_aliases()` returns a Dict with 30 entries — all from
`impl_method_renames`. The original 2 entries are gone.

**Root cause**: The `_method_aliases` Dict (class, heap-allocated) gets its RC decremented
to 0 somewhere between checker and lowerer phases, gets freed, and the slab allocator
reuses the same memory for the lowerer's `impl_method_renames` Dict. The global
`_method_aliases` pointer still points to the old address → now it aliases `impl_method_renames`.

**Evidence**:
- `pre-lower aliases count=2`, `post-lower aliases count=30`
- ASAN found UAF in `lower_impl_block` (fixed with `let ref` for string concat temps)
- Keys dumped at mono time show `impl_method_renames` content, not alias content
- Immediate verify after `set()` in checker succeeds (`verify=yes`)

**Workaround**: Monomorphizer uses suffix-based label stripping instead of global alias lookup.
The `_method_aliases` and `_method_labels` globals still exist but are unused by monomorphizer.

**Likely fix**: Either make the Dict `permanent`, find the RC leak (optional unwrap?),
or move aliases onto LProgram. Also need `str_substr` in stdlib.
