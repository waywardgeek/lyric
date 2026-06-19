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


---

## From Lyric Book review (2026-06-19, Hewitt + Bill)

### Lifetime analysis — preventing UAF after `destroy()` (CRITICAL)

**Problem**: After `a.destroy()`, any pointer to `a` is dangling. Slab `memset` makes
this look safe in practice, but it's UB. Section 11.4 of the book is explicit.
For a language pitched as "no GC, no borrow checker", this is *the* hole.

**Direction (Bill, 2026-06-19)**: Make it illegal to hold "one-way" pointers to objects
that can be destroyed. Proving no globals or refcounted instances is hard, so the rule
will be at the type system level. For class references that escape (rare), fall back to
**bidirectional pointers** — every tracked instance knows every pointer to it, and nulls
them out on destruction. Performance should still be good because escapes are rare; most
class refs stay inside the relation graph where lifetime is provable.

This is the path forward from `let ref` in TODO above — `let ref` for classes is the
local case; bidirectional pointers are the escape case.

### `Hashable` needs an `equals` method (CRITICAL for non-Sym keys)

**Problem**: `HashedList` matches entries by `hash_key()` value alone — no equality check.
For `Sym` keys this is safe because interning guarantees one entry per unique string.
For `Dict<i32, V>` or any non-Sym key, two distinct keys with colliding hashes will
match the wrong entry. Book §10.1 admits this; users will hit it.

**Direction (Bill, 2026-06-19)**: An `equals` method was originally required on `Hashable`
and may have been dropped during the multi-class-interface specialization rework. DataDraw
got this right; Lyric just needs to copy it. Bill hasn't carefully reviewed `ArrayList` /
`DoublyLinked` / `HashedList` recently — they need a pass in the context of parent/child
label specialization (still being designed; the world has never done this).

**Action**: Audit `Hashable` and `HashedList` for the missing `equals`. Restore it on the
interface. Add `equals` (or pointer-equality fallback) to the hash-bucket match path.

### `spawn` captures by pointer — silent data race (HIGH)

**Problem**: Variables captured into a `spawn` block are passed by pointer. Two `spawn`
blocks capturing the same variable race, with no compiler warning. Go specifically
avoided this. Currently users can write code that compiles, "works", and silently
corrupts under load.

**Action**: At minimum, warn at compile time when two spawns capture the same mutable
variable. Better: capture-by-copy by default for scalars and slices; capture-by-pointer
only with explicit annotation. Need to think about whether this interacts with channel
ownership.

### `select` is `sched_yield` polling — burns CPU (HIGH)

**Problem**: Generated `select` in C is a polling loop that calls `sched_yield()` between
attempts on each case. On a hot select with no traffic, this pegs a core. Book §12.4
acknowledges it.

**Action**: Move to a real wait/notify primitive. Each channel already has a pthread
mutex + condvar; `select` should register the goroutine as a waiter on all referenced
channels and block on one shared condvar. epoll/kqueue is a longer-term option but
overkill for the current channel implementation.

### `receive()` on closed channel — sentinel-value foot-cannon (HIGH)

**Problem**: When a channel is closed, `ch.receive()` returns the zero value for the
type, with no way to distinguish "real zero" from "channel closed". Book §12.3 producer
pattern uses `val >= 0` as termination — which only works because the producer happens
to send positive values. Real code needs a separate `done` channel or sentinel
discipline. Go fixed this with `(v, ok) := <-ch` in 2009.

**Action**: Change `receive()` to return `(T, bool)` where the bool is false iff the
channel was closed and the buffer is empty. This is a breaking change to every existing
channel user — do it before more code is written against the current API. Alternative
syntax to consider: `let (v, ok) = ch.receive()` vs an `ok_receive()` second method
(less breaking but two ways to do one thing).

### `assert_eq_approx` missing from stdlib (LOW)

**Problem**: `assert_eq` on floats is exact comparison. Fine for integer-valued floats,
breaks for any real numeric test.

**Action**: Add `assert_eq_approx(actual, expected, epsilon, msg?)` to stdlib. Default
epsilon = `1e-9` for f64.

### `_method_aliases` global RC bug (HIGH — tied to multi-class interface work)

(Already documented above as "BUG: Global Dict<Sym,string> freed by RC".) Note from
Bill 2026-06-19: this is tied to multi-class interface specialization by parent/child
labels. The fix lives in that workstream, not as a standalone Dict fix.

### Long-term gaps (not blocking, but flag for users)

- **No package registry.** Fine for the self-compile; will not scale to industry.
- **No incremental compilation.** 30K-line full rebuild in 0.2s buys us time.
- **No LSP server.** Editor support will become a friction point at adoption.
