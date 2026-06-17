# Lyric Reference Counting — Design & Implementation

**Date**: 2026-06-13 (original), 2026-06-17 (revised)
**Status**: Design finalized, implementation in progress
**Goal**: Deterministic memory management that wins benchmarks. Zero GC pauses, minimal overhead, maximum elision.

---

## Scope

Two domains of ref counting:

1. **Class instances** not in owning relations — lifetime managed by `_rc` field
2. **Dynamic arrays (slices)** — backing data freed when last reference dies

Both domains share the same optimization strategy: **elide ref counting whenever the compiler can prove lifetime statically**.

---

## 1. Class Reference Counting

### 1.1 The Core Model: Owned vs Borrowed References

Every reference to an RC-eligible class instance is either **owned** or **borrowed**.

**Owned references** — the holder must decrement `_rc` when done:
1. **Return from any call** — includes constructors, factory functions, everything. Calling a constructor IS calling a function; no special case needed.
2. **Field/global read** — copying a ref out of another object's storage. Must retain on read (the source object might be destroyed while we still hold the ref).
3. **Copy from local** — `let y = x` where x is still live afterward. Must retain.

**Borrowed references** — the holder does NOT touch `_rc`:
4. **Function parameters** — the caller holds an owned ref for the duration of the call. Zero-cost calling convention: no retain on call, no release on return.

**Returning references:**
- Returning an **owned** ref → transfers ownership to caller, skip scope-exit decrement (net zero).
- Returning a **borrowed** ref (a function parameter) → must increment before returning (creating a new owned ref for the caller, since the caller's original ref will be released independently).

**Restriction:** Users cannot directly call the destructor of an RC-eligible class. Only the RC machinery triggers destruction when `_rc` hits 0. The checker enforces this.

### 1.2 Move Semantics

When a reference is assigned and the source is **dead afterward** (never used again), the assignment is a **move**, not a copy:

```lyric
let x = make_widget()    // owned, rc=1
let y = x                // x is never used again → MOVE
                          // no retain on y, no release on x
                          // y inherits x's ownership
```

vs:

```lyric
let x = make_widget()    // owned, rc=1
let y = x                // x IS used again → COPY, retain y
do_something(x)          // both release at scope exit
```

Move detection requires a liveness check: is the source variable used after this point? If not, it's a move. This eliminates the most common inc/dec pairs — most locals are created, used briefly, and passed along.

**Inlining mental model:** If the compiler inlined all non-recursive function calls, there would be zero RC overhead for parameter passing. The only reason RC exists is because separate function scopes can't see each other's lifetimes. Parameters are the one case where we CAN see the lifetime (caller is provably alive for the call duration), so we skip bookkeeping.

### 1.3 Temp Variable Tracking

In LIR, everything is single-assignment temps. Each temp has exactly one def and a clear last-use point:

```
_t1 = SlabAlloc(Matrix)         // rc = 1
StSlabSet(_t1, "rows", _t3)
_t5 = Call(determinant, _t1)    // last use of _t1
// Insert StRefDecr(_t1) here   // rc → 0 → free
StVarDecl(volume, _t5)
```

The memory pass can:
1. For each temp of RC class type, find its **last use** in the statement sequence
2. Insert `StRefDecr` right after that last use
3. Skip if the temp is returned or stored in a field (ownership transferred)

**Stack allocation optimization (future):** Temp arrays of size computed on the fly that do not resize can be allocated on the stack via GCC VLAs/alloca. No malloc/free needed. The same stack-based temp tracking covers both class refs and array temps.

### 1.4 Delta Folding

Within a single function, if you can compute the **net delta** to each `_rc` at compile time, you only need to emit the net effect. Most common pattern: alloc (+1) followed by scope-exit release (-1) with no intermediate copies = just emit the decrement. No intermediate inc/dec pairs.

Field stores are the exception — the retain must happen eagerly because the reference escapes into another object's lifetime that may outlive the current scope.

### 1.5 Insertion Points (Memory Pass)

| Site | Action |
|------|--------|
| Return from any call (including constructors) | Caller owns result (rc already 1 from alloc) |
| `let y = x` (copy, x still live) | Retain y |
| `let y = x` (move, x dead after) | Transfer ownership, no inc/dec |
| `x = new_val` (overwrite) | Release old x, retain new_val (unless fresh) |
| Field read (`let m = obj.field`) | Retain m (copies ref from heap) |
| Field store (`obj.field = x`) | Retain x (escape into another lifetime) |
| Scope exit | Release all owned refs declared in that scope |
| `return x` (owned ref) | Transfer ownership, skip release |
| `return x` (borrowed ref / param) | Increment before returning |
| Function param | Nothing (borrowed, zero-cost) |
| `spawn` capture | Retain at capture; release when spawned routine exits |

### 1.6 `permanent` Classes

Classes marked `permanent` skip ALL RC instrumentation — no `_rc` field, no alloc init, no inc/dec. Used for compiler-internal classes (AST nodes, LIR nodes, CGen, etc.) that live for the entire compilation and are freed by slab destruction, not individual RC. Currently 34 compiler classes are permanent.

### 1.7 Owned Classes

Classes that are children in `owns` relations skip RC entirely. Their lifetime is managed by the parent's destructor. `is_owned` is set on `LClassDecl` during `resolve_class_types()`.

### 1.8 `trusted` Exemption and `ref`/`unref` Primitives

Functions/blocks marked `trusted` get NO auto-inserted retain/release. Inside `trusted` code, two primitive statements are available:

```lyric
trusted func Dict.set(self, key: Sym, value: V) {
    // ... find or create entry ...
    ref value          // manually increment rc — we're storing it
    entry.value = value
}

trusted func Dict.remove(self, key: Sym) {
    // ... find entry ...
    unref entry.value  // manually decrement rc — we're releasing it
    // ... remove entry from table ...
}
```

**`ref x`** — increment `_rc` on x. Used when storing a reference into a container's internal storage (the container is taking ownership of an additional reference).

**`unref x`** — decrement `_rc` on x. If `_rc` hits 0, triggers destruction + slab free. Used when removing a reference from a container's internal storage.

These are **statements**, not expressions. The checker rejects `ref`/`unref` outside `trusted` context. The lowerer emits `StRefIncr`/`StRefDecr` directly.

**Relation-generated code** is implicitly `trusted` — the compiler does zero automatic RC there. When a child is inserted into a relationship, the generated insert/append method calls `ref child` once.

### 1.9 Breaking Reference Cycles via Non-Owning Back-Pointers

Lyric's relation system generates parent back-pointers on child classes (e.g. `Node.graph`, `Edge.node`). With naive RC, these create cycles: Graph refs Node, Node refs Graph → neither ever reaches rc=0.

The solution: **back-pointers are non-owning — no `ref` on the parent**. Only the forward direction (parent→child) holds a counted reference. The back-pointer is a raw handle that is valid only as long as the parent is alive.

This is safe because Lyric's ownership model guarantees the parent outlives its children:
- The parent's destructor destroys all owned children (via relation-generated teardown)
- Children cannot exist without a parent — insertion into a relation is the only way to set the back-pointer
- If a child is removed from a relation, its back-pointer is cleared

Example:
```
relation Graph owns Node      // Graph.nodes: HashedList<Node>, Node.graph: Graph
relation Node refs Edge        // Node.edges: [Edge], Edge.node: Node

// Generated trusted code for Graph.add_node(child):
//   ref child          // forward ref: Graph holds counted ref to Node
//   child.graph = self // back-pointer: NO ref on Graph (non-owning)

// When Graph is destroyed:
//   for each node in self.nodes:
//     unref node       // rc→0 → destroy Node
//                      // Node's destructor does NOT unref node.graph (non-owning)
```

Without non-owning back-pointers, every parent↔child pair would be a cycle that RC alone can never collect. This convention eliminates cycles by construction — no cycle detector needed.

---

## 2. Slice/Array Reference Counting

### 2.1 Backing Array Header

Every dynamically allocated slice backing array gets a ref count. The backing array layout:

```
[ rc: u32 | cap: u32 | data... ]
     ^header          ^slice.data points here
```

The `rc` field lives at `((uint32_t*)slice.data)[-2]` (or a separate side-table — see §2.3).

**Alternative (simpler for Phase 1):** Don't ref-count backing arrays at all. Just free every slice on scope exit (copy semantics — every `let b = a` copies the data). This loses sharing but is dead simple and sufficient for benchmarks. Ref-counted sharing is a Phase 2 optimization.

### 2.2 Insertion Points

| Site | Action |
|------|--------|
| `let a = [1,2,3]` | alloc backing, rc=1 |
| `let b = a` | **copy data** (Phase 1) or rc++ (Phase 2) |
| Scope exit | rc-- (free if 0) or just free (Phase 1) |
| `func f(x: [T])` | no-op (view, caller holds ref) |
| `return arr` | ownership transfer, no free |

### 2.3 Implementation Strategy

**Phase 1 (speed-first):** Every local slice gets `free()` at scope exit. No ref counting on backing arrays. Copies are real copies. This is correct and fast for the compiler itself (which allocates tons of short-lived arrays).

**Phase 2 (sharing):** Add rc header to backing arrays. Subslice assignment shares backing + increments rc. `ref` bindings increment rc. Free only when rc hits 0.

---

## 3. Lifetime Elision — The Speed Play

The core optimization: **skip retain/release when the compiler can prove the object outlives all references to it**. This is where we beat Go's GC and Rust's borrow checker overhead.

### 3.1 Stack-Local Classes (No Escape)

If a class handle is:
- Allocated in the current scope
- Never stored in a field of another object
- Never stored in a global
- Never captured by `spawn`
- Only passed to functions as parameters (zero-cost convention covers this)

Then: **skip all retain/release**. The object lives exactly as long as the scope. Free it at scope exit without ever touching `_rc`.

### 3.2 Function Parameters

Function parameters are NEVER retained/released by the callee. The caller holds the reference. This is always safe because:
- Lyric has no closures that outlive the function
- `spawn` captures by value (which does its own retain)
- The callee cannot store the parameter anywhere that outlives the call without going through a field store (which emits an eager retain)

This is a **zero-cost calling convention** for class references. Even for recursive calls — the call stack itself acts as a "holds" guarantee. The original caller's owned ref sits at the bottom of the stack and can't be released until the recursive chain fully unwinds.

### 3.3 Temporary Expressions

In `foo(Bar {})`, the `Bar` instance is a temporary. It's allocated, passed to `foo`, and freed when the statement ends. No retain/release needed — just alloc before the call, free after (last-use release on the temp).

### 3.4 Last-Use Optimization

If a variable's last use is as a function argument, skip the release at scope exit — insert the release right after the call instead. This keeps the object alive only as long as needed.

```lyric
let x = Widget {}     // alloc, rc=1
process(x)            // last use of x
                      // release x here (rc→0→free)
```

### 3.5 Dynamic Arrays — Stack Lifetime

For slices allocated on the stack that never escape:
- `let a = [1, 2, 3]` — allocate, use, free at scope exit
- Even when passed to functions: the backing data survives the call (caller's scope)
- No ref counting needed — just scope-exit free

**Future optimization:** Temp arrays whose size is computed on the fly and that do not resize can be allocated on the stack via GCC VLAs/alloca. Zero malloc/free overhead.

---

## 4. LIR Representation

### 4.1 LIR Nodes (implemented)

`LStmtKind` RC variants:
- `StRefIncr` — increment `_rc` on a class handle (`LRefIncrData`: handle + class_name)
- `StRefDecr` — decrement `_rc`, free if 0 (`LRefDecrData`: handle + class_name)
- `StSliceRetain` — increment `_rc` on all class handles in a slice

Scope-exit free variants:
- `StSlabFree` — free a class handle via slab
- `StSliceFree` — free a slice's backing memory

### 4.2 LType Metadata (implemented)

`LType` has class metadata fields populated by `resolve_class_types()`:
- `class_decl: LClassDecl?` — link to the class declaration
- `is_permanent: bool` — class uses `permanent` keyword, skips RC
- `is_owned: bool` — class is child in an `owns` relation, RC managed by parent

`resolve_class_types()` must be called post-lowering AND post-monomorphization.

### 4.3 LProgram Flags (implemented)

- `slab_mode` / `slab_mode_soa` — AoS vs SoA slab allocation
- `detect_uaf` — sets `_rc = UINT32_MAX` on free, checks on every field access
- `rc_free` — enables actual `slab_free` calls when rc drops to 0 (opt-in)
- `owned_classes: Dict<Sym, bool>` — tracks which classes are relation children

### 4.4 C Backend Emission (implemented)

```c
// StRefIncr (AoS)
p->_rc++;

// StRefIncr (SoA)
_lyric_slab_Widget._rc[h]++;

// StRefDecr (AoS, with --rc-free)
if (--p->_rc == 0) {
    destroy_Widget(p);   // if has destructor
    lyric_slab_free_Widget(p);
}

// --detect-uaf: on free, _rc = UINT32_MAX; on access, check for poison
if (p->_rc == UINT32_MAX) { fprintf(stderr, "UAF: ..."); abort(); }

// StSliceFree
free(arr.data);
```

---

## 5. Struct Copy Hooks

When a struct contains class reference fields (transitively), assignment must retain all embedded references, and scope exit must release them.

**Example:**
```lyric
struct Pair {
    a: Widget    // class handle
    b: Widget
}
let p = Pair { a: w1, b: w2 }  // retain w1, retain w2
let q = p                       // retain q.a, retain q.b
```  // release p.a, p.b, q.a, q.b

**Implementation:** The memory pass inspects each struct type. If it transitively contains `TyClassHandle` fields, it generates retain/release sequences at copy/scope-exit points.

**Elision:** If the struct is stack-local and never escapes, the embedded refs don't need retain/release either (same escape analysis as §3.1).

---

## 6. Memory Pass Implementation

### 6.1 Scope Stack

The memory pass uses a scope stack. Each call to `slab_rewrite_stmts` is implicitly a scope. Per-scope tracking:

- `owned_refs: Dict<Sym, RefInfo>` — locals that own references (name → type, live flag)
- At scope exit: release all owned refs that are still live

RefInfo per variable:
- `type`: the LType (for emitting the right StRefDecr)
- `live`: bool — set false when moved (consumed by assign where source is dead afterward)

### 6.2 Statement Processing

| Statement | Action |
|-----------|--------|
| `StVarDecl` init from call/alloc result | Push owned ref (no retain — call returns rc=1) |
| `StVarDecl` init from field read | Emit retain, push owned ref |
| `StVarDecl` init from local var | Liveness check: source dead? → move (mark source not-live). Source still used? → emit retain, push owned ref |
| `StAssign` overwrite | Release old, same logic as VarDecl for new value |
| `StSlabSet`/`StClassSet` (field store) | Emit retain on the stored value (eager — cannot defer) |
| `StReturn` | Skip release for returned ref; release all others |
| `StTempDef` with RC class type | Track temp, insert StRefDecr after last use |
| Scope exit | Release all still-live owned refs |

Function params never enter the owned_refs map — they're invisible to RC.

---

## 7. Escape Analysis Algorithm

Per-function, per-variable (fixed-point iteration for inter-procedural):

```
for each local variable v of class or slice type:
    escapes = false
    for each use of v:
        if use is:
            field_store(other.field = v)  → escapes = true
            global_store(GLOBAL = v)      → escapes = true
            spawn_capture                 → escapes = true
            return v                      → escapes = true (but don't free — transfer)
            struct/class literal field    → escapes = true
            func_arg                      → ok (zero-cost convention)
            method_call on v              → ok
            local read/write              → ok
    if !escapes:
        mark v as "scope-managed" — free at scope exit, no retain/release needed
```

---

## 8. Implementation Phases — Ordered for Benchmark Impact

### Phase 1: Scope-Exit Free for Slices ✅ DONE
Escape analysis, `StSliceFree` at scope exit. 138 slice frees inserted.

### Phase 2: Scope-Exit Free for Strings ✅ DONE
`TyString` locals from f-strings/concat only. `LYRIC_STR` cap=0 marks static strings. 30 string frees.

### Phase 3: Class Reference Counting 🔧 IN PROGRESS

**Done:**
- `_rc` field on non-permanent, non-owned classes (AoS + SoA)
- `StRefIncr` / `StRefDecr` LIR nodes and C backend emission
- 34 compiler classes marked `permanent`
- `owned_classes` tracking from relation declarations
- `--detect-uaf` poison-on-free + access checks
- `--rc-free` flag for opt-in live frees
- `resolve_class_types()` for LType metadata

**Remaining:**
- Implement owned-vs-borrowed model (replace escape workaround)
- Move semantics (liveness-based elision)
- Temp last-use tracking and release
- Fix: StSlabSet statements generated by ExClassAlloc expansion need RC retains
- Test: `make test` (81/81), `make self-test` (fixed point), `--rc-free --detect-uaf` self-compile

### Phase 4: Struct Copy Hooks
Retain/release for class references embedded in structs. Deferred until Phase 3 is solid.

### Phase 5: `trusted` + `ref`/`unref` Primitives
Parser + checker support for `trusted` keyword. Manual ref/unref inside trusted blocks. Update stdlib containers. Relation-generated code marked `trusted` with explicit ref on insert, no ref on back-pointers.

---

## 9. Interaction with Existing Passes

**Pipeline:**
```
Parse → Desugar → Check → Lower → Optimize → Monomorphize
    → Memory Pass (slab rewrite + ref counting + slice free) → C backend
```

The memory pass (`memory.ly`) already runs after monomorphization. Ref counting and slice freeing are extensions to the same pass. No new pass needed.

**Slab interaction:** Ref-counted classes live in the same slab as owned classes. The `_rc` field is just another field in the slab. `StSlabFree` is called when `_rc` hits 0, returning the slot to the free list — same as owned destruction.

---

## 10. What We're NOT Doing (Yet)

- **Cycle detection** — deferred. The compiler's class graph is acyclic. Back-pointer convention (non-owning) prevents cycles.
- **`destroys` annotation** — deferred.
- **`mut resize` / provenance tracking** — deferred.
- **Slice ref counting (sharing)** — Phase 1 uses copy + free. Sharing is Phase 2.
- **Weak references** — deferred indefinitely.
- **Per-slab mutexes** — deferred until concurrency is hardened.
- **User-callable destructors on RC classes** — forbidden by design. Checker must reject.

---

## 11. Benchmark Strategy

**Target:** Self-compile (15K Lyric → 105K C) as the primary benchmark.

**Current:** SoA 0.20s / 295 MB, AoS 0.22s / 344 MB.

**Expected after Phase 3 (class ref counting with elision):**
- Most class refs in the compiler are permanent (skip RC) or stack-local (scope-exit free, no retain/release)
- Only StringBuilder and Dict variants (~256 sites) currently RC'd
- With move semantics and last-use optimization, expect minimal actual inc/dec operations
- The slab free list gets populated → alloc speedup from reuse

**Key metric:** Run `time lyric compile ...` and `vmmap` / `leaks` before and after each phase.
