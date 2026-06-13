# Lyric Reference Counting — Design & Implementation

**Date**: 2026-06-13
**Status**: Draft — for Bill's review
**Goal**: Deterministic memory management that wins benchmarks. Zero GC pauses, minimal overhead, maximum elision.

---

## Scope

Two domains of ref counting:

1. **Class instances** not in owning relations — lifetime managed by `_rc` field
2. **Dynamic arrays (slices)** — backing data freed when last reference dies

Both domains share the same optimization strategy: **elide ref counting whenever the compiler can prove lifetime statically**.

---

## 1. Class Reference Counting

### 1.1 Mechanism

Every non-owned class gets a hidden `_rc: u32` field (injected by the memory pass, not visible in source). Owned classes (those appearing as children in `owns` relations) get NO ref counting — their lifetime is managed by their owner's destructor.

**Operations (inserted by memory pass at LIR level):**
- **Retain**: `_rc++` — on assignment, copy, capture
- **Release**: `_rc--` — on scope exit, overwrite, function return (for non-returned refs)
- **Free**: when `_rc` hits 0 → call `destroy()` if it exists, then `slab_free()`

### 1.2 Insertion Points

The memory pass walks each function body and inserts retain/release at these points:

| Site | Action |
|------|--------|
| `let x = class_expr` | retain (initial ref) |
| `let y = x` (class handle copy) | retain y |
| `x = new_val` (overwrite) | release old x, retain new_val |
| Scope exit (block/if/while/for end) | release all class locals declared in that scope |
| `return x` | NO release — ownership transfers to caller |
| Function param (class type) | NO retain on entry (caller holds ref); NO release on exit |
| `spawn` capture | retain at capture; release when spawned routine exits |

### 1.3 `trusted` Exemption

Functions/blocks marked `trusted` get NO auto-inserted retain/release. Manual `ref(x)` and `unref(x)` primitives are available inside `trusted` code only. This is for stdlib containers (ArrayList, Dict, HashedList, DoublyLinkedList).

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
- Only passed to functions as non-`mut` parameters (or `mut` but not stored)

Then: **skip all retain/release**. The object lives exactly as long as the scope. Free it at scope exit without ever touching `_rc`.

**Detection**: Walk the function body. For each `let x = ClassName {}`:
1. Track all uses of `x`
2. If `x` is only used as: local reads, function args (non-`mut` or `mut` without field-store), method calls on `x` → **elide**
3. If `x` appears in: field assignment (`other.field = x`), global assignment, spawn capture, return → **cannot elide**

### 3.2 Function Parameters

Function parameters are NEVER retained/released by the callee. The caller holds the reference. This is always safe because:
- Lyric has no closures that outlive the function
- `spawn` captures by value (which does its own retain)
- The callee cannot store the parameter anywhere that outlives the call without going through an assignment (which the memory pass handles)

This is a **zero-cost calling convention** for class references.

### 3.3 Temporary Expressions

In `foo(Bar {})`, the `Bar` instance is a temporary. It's allocated, passed to `foo`, and freed when the statement ends. No retain/release needed — just alloc before the call, free after.

### 3.4 Last-Use Optimization

If a variable's last use is as a function argument, skip the release at scope exit — the function "consumes" the reference. This avoids the retain-on-call/release-at-scope-exit pair.

```lyric
let x = Widget {}     // alloc, rc=1
process(x)            // last use of x — no release needed
                      // (process doesn't retain either, if it doesn't store x)
```

### 3.5 Dynamic Arrays — Stack Lifetime

For slices allocated on the stack that never escape:
- `let a = [1, 2, 3]` — allocate, use, free at scope exit
- Even when passed to functions: the backing data survives the call (caller's scope)
- No ref counting needed — just scope-exit free

**Detection**: Same escape analysis as classes. If the slice is never stored in a field, global, or spawn capture → just free at scope exit.

---

## 4. LIR Representation

### 4.1 New LIR Nodes

Add to `LStmtKind`:
```
StRefIncr    // increment _rc
StRefDecr    // decrement _rc; if 0, destroy + free
StSliceFree  // free slice backing array
```

Add to `LStmt` fields:
```
ref_incr: LRefIncrData?    // { handle: LValue }
ref_decr: LRefDecrData?    // { handle: LValue, class_name: string }
slice_free: LSliceFreeData? // { slice: LValue }
```

**Why LIR, not C backend:** The memory pass already does slab rewrite at LIR level (`memory.ly`). Ref counting is the same kind of transformation — rewrite the LIR, let the C backend be a dumb emitter. This also enables future optimization passes (elision, motion) to operate on the LIR.

### 4.2 C Backend Emission

```c
// StRefIncr
_lyric_slab_Widget._rc[h]++;              // SoA
_lyric_slab_Widget.data[h]._rc++;         // AoS (block-based, pointer)

// StRefDecr
if (--_lyric_slab_Widget._rc[h] == 0) {   // SoA
    Widget_destroy(h);                      // calls slab_free internally
}

// StSliceFree
free(arr.data);                            // Phase 1: unconditional free
// Phase 2: if (--((uint32_t*)arr.data)[-1] == 0) free(backing);
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

## 6. Implementation Phases — Ordered for Benchmark Impact

### Phase 1: Scope-Exit Free for Slices (biggest benchmark win)

The compiler currently leaks every dynamic array. Freeing them at scope exit will dramatically reduce RSS.

**Work:**
1. Add `StSliceFree` to LIR (`lir.ly`)
2. In `memory.ly`: for every `StVarDecl` with a slice type, record it. At block/function exit, emit `StSliceFree` for each.
3. In `c_backend.ly`: emit `free(var.data)` for `StSliceFree`
4. Handle nested scopes (if/while/for/match — variables declared inside die at block exit)
5. Handle `return` — don't free returned slices
6. Handle reassignment — free old backing before assigning new

**Expected impact:** The compiler allocates millions of short-lived arrays (string processing, AST building). Freeing them should cut RSS by 50%+ and improve cache behavior → faster.

**Test:** Self-compile under Valgrind or `leaks` to verify no use-after-free.

### Phase 2: Scope-Exit Free for Strings

Strings are `[u8]`. Same treatment as slices.

**Work:** String literals are `const char*` (no free). Only dynamically built strings (f-strings, concat, StringBuilder output) need freeing. The memory pass must distinguish:
- `ValLitString` → no free (static data)
- Computed strings (from `ExFormat`, `ExCall` returning string, etc.) → free at scope exit

### Phase 3: Class Reference Counting

**Work:**
1. Add `_rc` field to non-owned classes (injected in memory pass, not source)
2. Add `StRefIncr` / `StRefDecr` to LIR
3. Insert retain at assignment, release at scope exit/overwrite
4. `StRefDecr` emission: decrement, if 0 → call destroy + slab_free
5. Escape analysis for elision (§3.1-3.5)

**Test:** `destroy_shared.ly` already tests cascade destruction. Add test for ref-counted class going to zero.

### Phase 4: Struct Copy Hooks

Retain/release for class references embedded in structs. Deferred until Phase 3 is solid.

### Phase 5: `trusted` + `ref`/`unref` Primitives

Parser + checker support for `trusted` keyword. Manual ref/unref inside trusted blocks. Update stdlib containers.

---

## 7. Escape Analysis Algorithm (for elision)

Per-function, per-variable:

```
for each local variable v of class or slice type:
    escapes = false
    for each use of v:
        if use is:
            field_store(other.field = v)  → escapes = true
            global_store(GLOBAL = v)      → escapes = true  
            spawn_capture                 → escapes = true
            return v                      → escapes = true (but don't free — transfer)
            func_arg (non-mut)            → ok (callee can't store it)
            func_arg (mut)                → check if callee stores it (conservative: escapes = true)
            method_call on v              → ok
            local read/write              → ok
    if !escapes:
        mark v as "scope-managed" — free at scope exit, no retain/release
```

For Phase 1 (slices), the conservative approach is fine: just free everything at scope exit that isn't returned. The only danger is double-free from aliasing, which copy-on-assign semantics prevents.

---

## 8. Interaction with Existing Passes

**Pipeline:**
```
Parse → Desugar → Check → Lower → Optimize → Monomorphize
    → Memory Pass (slab rewrite + ref counting + slice free) → C backend
```

The memory pass (`memory.ly`) already runs after monomorphization. Ref counting and slice freeing are extensions to the same pass. No new pass needed.

**Slab interaction:** Ref-counted classes live in the same slab as owned classes. The `_rc` field is just another field in the slab. `StSlabFree` is called when `_rc` hits 0, returning the slot to the free list — same as owned destruction.

---

## 9. What We're NOT Doing (Yet)

- **Cycle detection** — deferred. The compiler's class graph is acyclic. Warn on cycles later.
- **`destroys` annotation** — deferred to Sprint 3 of the memory management plan.
- **`mut resize` / provenance tracking** — deferred to Sprint 4.
- **Slice ref counting (sharing)** — Phase 1 uses copy + free. Sharing is Phase 2.
- **Weak references** — deferred indefinitely.
- **Per-slab mutexes** — deferred until concurrency is hardened.

---

## 10. Benchmark Strategy

**Target:** Self-compile (15K Lyric → 101K C) as the primary benchmark.

**Current:** SoA 0.20s / 295 MB, AoS 0.22s / 344 MB.

**Expected after Phase 1+2 (slice/string free):**
- RSS should drop dramatically (maybe 100-150 MB) — freed memory gets reused by malloc
- Time might improve from better cache locality (less memory pressure)
- Or might slow slightly from free() calls — measure and optimize

**Expected after Phase 3 (class ref counting with elision):**
- Most class refs in the compiler are stack-local (AST nodes, LIR nodes created and consumed in same function)
- With good elision, <5% of refs need actual retain/release
- The slab free list gets populated → alloc speedup from reuse

**Key metric:** Run `time lyric compile ...` and `vmmap` / `leaks` before and after each phase.
