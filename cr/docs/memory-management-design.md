# Lyric Memory Management Design

**Date**: 2026-06-12
**Status**: Draft — approved for implementation
**Scope**: Class slab allocation, reference counting, stack safety, array resize safety, copy semantics

## Overview

Lyric currently leaks all heap memory (classes, strings, dynamic arrays). This design introduces:

1. **Slab allocator** for classes (replaces malloc)
2. **Ownership-based destruction** for owned classes (extends existing relation destructors)
3. **Reference counting** for non-owned classes
4. **`trusted` keyword** for manual memory management in data structures
5. **`destroys` annotation** with compiler inference for static use-after-free prevention
6. **Safe iterators** for destroy-during-iteration
7. **Stack escape prevention** for structs/tuples
8. **`mut resize` annotation** for static array aliasing safety
9. **Go-style slice sharing** with ref-counted backing arrays
10. **Copy-on-assign** for value types (Rune model)

All memory management structures and operations are represented in the **LIR**, not the C backend, enabling future optimization passes.

---

## 1. Copy and Assignment Semantics

### 1.1 Value Types — Copy on Assign

All value types (primitives, structs, tuples, slices) use **copy-on-assign** semantics. Assignment always produces an independent copy.

```lyric
let mut x = Point { x: 0.0, y: 0.0 }
let mut y = x        // copy — y is independent
y.x = 1.0            // x.x still 0.0

let a = [1, 2, 3]
let b = a[1:3]       // copy — b owns its own backing data
```

This applies uniformly:

| Statement | Semantics |
|-----------|-----------|
| `let y = x` | Immutable copy |
| `let mut y = x` | Mutable copy |
| `func f(x: Foo)` | Shared view (optimizer may use const pointer; no copy for non-mut) |
| `func f(mut x: Foo)` | Mutable **reference** (caller's value modified) |

**Key distinction**: `mut` on local bindings means "this binding is mutable." `mut` on parameters means "this is a reference — callee can modify the caller's value." Reference semantics live at function boundaries, not at local assignment.

**Slices as parameters**: passing a slice to a function does NOT copy — the function receives a view into the original backing data. This is critical for performance. Only assignment creates a copy.

### 1.2 `ref` — Zero-Copy View Binding

For cases where you want a reference view without copying (e.g., parsing, serialization, cryptography), use `ref` as a binding qualifier:

```lyric
let src = read_file("input.ly")
let ref token = src[start:end]        // zero-copy immutable view

// Parser example: parse fixed-width record
let ref name    = line[0:20]          // no copy — view into line
let ref address = line[20:60]         // no copy — view into line

// Mutable view for serialization/crypto (write into pre-allocated buffer)
let mut ref point = packet[16:81]
point[0] = 0x04                       // writes into packet[16]
copy_into(mut point[1:33], x_bytes)   // writes into packet[17:49]
```

`ref` is a binding qualifier like `mut`. The full binding grammar:

```
let [mut] [ref] name = expr
```

| Binding | Semantics |
|---------|-----------|
| `let x = expr` | Immutable copy |
| `let mut x = expr` | Mutable copy |
| `let ref x = expr` | Immutable view (shared, no copy) |
| `let mut ref x = expr` | Mutable view (shared, no copy, can write through) |

**Rules for `ref` bindings:**
- `let ref` is immutable by default (read-only view into shared backing data)
- `let mut ref` allows element mutation through the view
- The source data's lifetime must exceed the `ref` binding's scope (no-escape rule enforces this)
- The source cannot be passed as `mut resize` while a `ref` view exists (provenance tracking)
- `ref` on a slice increments the backing array's ref count (prevents premature freeing)

**Context disambiguation with ref counting**: `ref(x)` as a statement (inside `trusted` code) is ref counting — increment the reference count. `let ref x = ...` is a view binding. No ambiguity — different syntactic positions.

### 1.2 Class References — Handle Copy + Ref Count

Class references are u32 handles (indices into slab arrays). Assignment copies the handle and increments the ref count on the referenced instance:

```lyric
let x = Widget {}    // ref count = 1
let y = x            // copies u32 handle; ref count = 2
```

`clone(x)` deep-copies the actual object (creates a new instance with copied fields), returning a new handle. This is the only case where `clone` is needed.

### 1.3 Structs Containing Class References

When a struct is copied (by assignment or pass-by-value), the compiler auto-inserts ref increments for all transitively contained class reference fields:

```lyric
struct Pair {
    left: Widget
    right: Widget
}

let x = Pair { left: w1, right: w2 }  // ref(w1), ref(w2)
let y = x       // copies struct; compiler inserts ref(y.left), ref(y.right)
```

On scope exit, the compiler inserts decrements:

```lyric
func example() {
    let p = Pair { left: Widget {}, right: Widget {} }
}  // compiler inserts: unref(p.left); unref(p.right)
```

On reassignment, decrement old refs and increment new:

```lyric
let mut x = Pair { left: w1, right: w2 }
x = Pair { left: w3, right: w4 }  // unref(w1), unref(w2), ref(w3), ref(w4)
```

The compiler generates these copy/cleanup sequences based on type analysis — any type transitively containing a class reference field gets:
- **Copy hook**: ref increment for all embedded class references
- **Scope-exit hook**: ref decrement for all embedded class references
- **Reassignment hook**: decrement old, increment new

### 1.4 Immutable Bindings of Mutable Sources

`let y = x` where `x` is `let mut` produces a **snapshot copy** — `y` is independent of future changes to `x`. The optimizer cannot use a reference here because `x` might change, violating `y`'s immutability.

```lyric
let mut x = Point { x: 0.0, y: 0.0 }
let y = x         // snapshot copy
x.x = 1.0         // y.x still 0.0
```

---

## 2. Class Slab Allocator

### 2.1 Reference Model

All class references become `u32` indices into per-class slab arrays, replacing raw pointers.

- `null` (Lyric keyword) = `0`
- Valid references = `>= 1`
- Array index = `reference - 1` (so index 0 holds reference 1)

This means all-zeros initialization produces null references by default, which is the sensible zero value.

### 2.2 Slab Data Structure

Each class gets a global slab structure, generated in the LIR. The layout depends on the `--soa` / `--aos` compiler flag.

**AoS (Array of Structures) — default:**

```c
typedef struct {
    Counter* data;       // array of Counter structs
    uint32_t used;       // number of slots used (high-water mark)
    uint32_t allocated;  // capacity of data array
    uint32_t first_free; // head of free list (0 = empty)
} Counter_Slab;

// Each Counter struct includes a hidden `_next_free` field (u32)
```

**SoA (Structure of Arrays):**

```c
typedef struct {
    int32_t*  count;     // parallel array for each field
    uint8_t*  mu;        // (lock field)
    uint32_t* _next_free; // free list chain
    uint32_t  used;
    uint32_t  allocated;
    uint32_t  first_free;
} Counter_Slab;
```

**Future optimization**: In SoA mode, reuse an existing `u32` field as the free list pointer for dead objects (dead objects don't need their field data). This saves one array allocation per class. Defer this optimization to a later sprint.

### 2.3 Global Root Structure

A single global root struct holds all slab pointers:

```c
typedef struct {
    Counter_Slab counter_slab;
    Widget_Slab  widget_slab;
    // ... one per class
} LyricRoot;

static LyricRoot _lyric_root = {0};
```

### 2.4 Alloc / Free Functions

Auto-generated per class, inlined for performance.

**Alloc (raw — no field initialization):**

```
inline func Counter_alloc() -> u32:
    if slab.first_free != 0:
        ref = slab.first_free
        slab.first_free = slab.data[ref - 1]._next_free
        return ref
    if slab.used == slab.allocated:
        new_cap = max(slab.allocated * 2, 8)
        resize all arrays to new_cap  // realloc for AoS, per-field realloc for SoA
        slab.allocated = new_cap
    slab.used += 1
    return slab.used   // used is already 1-indexed
```

**Free (return to free list):**

```
inline func Counter_free(ref: u32):
    slab.data[ref - 1]._next_free = slab.first_free
    slab.first_free = ref
```

**Create (alloc + initialize fields):**

```
inline func Counter_create(count: i32, mu: Lock, label: string) -> u32:
    ref = Counter_alloc()
    // set fields via slab index
    return ref
```

### 2.5 Field Access

Field access rewrites in the LIR:

```
// Before (pointer-based):
obj->count

// After AoS:
_lyric_root.counter_slab.data[ref - 1].count

// After SoA:
_lyric_root.counter_slab.count[ref - 1]
```

The LIR represents this as `LExprSlabGet(class_name, field_name, ref_expr)` and `LStmtSlabSet(class_name, field_name, ref_expr, value_expr)`. The C backend emits the appropriate AoS or SoA access pattern.

---

## 3. Ownership, Destruction, and Use-After-Free Prevention

### 3.1 Owned Classes (via `owns` relations)

Lyric already generates `destroy()` methods for classes with `owns` relations. The memory management pass extends these:

1. `final` function runs (user hook — see §3.4)
2. Existing destructor body runs (cascade destroy of owned children, unlink from all relations)
3. **New**: call `ClassName_free(ref)` at the end to return the slot to the free list

No reference counting needed — ownership is the lifetime. Coding convention: a Root class recursively owns all owned classes, though this is not enforced.

### 3.2 Non-Owned Classes — Reference Counting

Classes that are NOT owned via a relation get automatic reference counting.

**Added to each ref-counted class**: a `_ref_count: u32` field (hidden, managed by the compiler).

**Compiler-inserted operations in normal code:**
- **Assignment/copy of a class handle**: increment ref count
- **Scope exit / variable goes out of scope**: decrement ref count
- **Reassignment**: decrement old, increment new
- **Return value**: ownership transfers to caller (no decrement on function scope exit for returned references)
- **When ref count hits 0**: call `destroy()` (if it exists) then `ClassName_free(ref)`

**What about owned classes?** No ref counting at all. Their lifetime is managed by their owner's destructor. `ref`/`unref` statements are ignored for owned classes.

### 3.3 The `destroys` Annotation — Static Use-After-Free Prevention

The same pattern as `mut resize` for arrays: annotate functions with what they might destroy, then statically reject access to invalidated references.

**Two levels of specificity:**

```lyric
// Level 1: destroys arbitrary instances of a class
destroys(Widget) func cleanup_all_widgets()

// Level 2: destroys only the specific parameter
destroys(w) func remove_widget(w: Widget)
```

**After a call to a `destroys(Widget)` function**, all Widget references in scope are dead:

```lyric
let a = Widget {}
let b = Widget {}
cleanup_all_widgets()     // destroys(Widget)
println(a.name)           // ERROR: a may be invalid after destroys(Widget) call
```

**After a call to a `destroys(w)` function**, only the specific binding is dead:

```lyric
let a = Widget {}
let b = Widget {}
remove_widget(a)          // destroys(a) — only a is dead
println(b.name)           // OK: b is unaffected
println(a.name)           // ERROR: a was destroyed
```

**Inference with warnings**: The compiler infers `destroys` annotations from the call graph. If a function calls `x.destroy()`, it gets `destroys(x)`. If it calls another function that `destroys(Widget)`, it inherits `destroys(Widget)`. If a public function's inferred `destroys` set changes (e.g., a new `destroy()` call is added deep in the call chain), the compiler warns — this is an API contract change.

**Liveness-based**: Like `mut resize` aliasing, the conflict is only active while the reference is **live** (has future uses). Dead references don't block anything.

**Note on relations**: Dangling `refs` to owned objects are NOT a problem. The relation system auto-generates destructors that unlink instances from ALL relationships (both `owns` and `refs`). When Widget is destroyed, it is automatically removed from every relation that references it. This is one of the core safety guarantees of the relation system.

### 3.4 `final` Function — Pre-Destroy Hook

Classes may declare a `final` function, called immediately before the auto-generated destructor runs:

```lyric
class Connection {
    fd: i32

    final func cleanup(self) {
        close_fd(self.fd)  // user cleanup before relation teardown
    }
}
```

The execution order on `destroy()` is:
1. `final` (user hook — resource cleanup)
2. Auto-generated destructor body (cascade destroy owned children, unlink from all relations)
3. `ClassName_free(ref)` (return slot to slab)

### 3.5 Safe Iterators — Destroy During Iteration

It is extremely common to iterate through children and destroy some of them. Normal iterators must prohibit this (the underlying collection is being mutated). Safe iterators explicitly allow destroying the **current element only**.

```lyric
// Normal iterator — destroy during iteration is an error
for child in parent.children() {
    child.destroy()        // ERROR: children() doesn't allow destruction
}

// Safe iterator — destroying the current element is allowed
for child in parent.children_safe() {
    if should_remove(child) {
        child.destroy()    // OK: child is the safe iterator variable
    }
    other_widget.destroy() // ERROR: only current element is destroyable
}
```

**How it works**: The safe iterator generator pre-fetches the `next` pointer before yielding the current element. This way, destroying the current element doesn't break the iteration. The yielded reference carries a `destroys(self)` annotation — the compiler knows that destroying this specific binding is permitted, but no other references of that class.

**Implementation**: Safe iterators are declared with a `safe` keyword on the generator:

```lyric
safe gen func DoublyLinked.iter_safe(self) -> gen T {
    let mut cur = self.head
    while cur != null {
        let next = cur!._next   // pre-fetch before yield
        yield cur!
        cur = next
    }
}
```

Inside the `for` body of a safe iterator, the loop variable is annotated as the one destroyable reference. Any function called with that variable can `destroys(param)` it. Any call to `destroys(Widget)` (class-level) is still an error — only the specific yielded instance may be destroyed.

**Java parallel**: Java had exactly this problem — `ConcurrentModificationException` during iteration. Java's `Iterator.remove()` was the eventual solution. Lyric's safe iterators are the same idea, but statically checked at compile time instead of throwing at runtime.

### 3.6 The `trusted` Keyword

Data structures (ArrayList, Dict, HashedList, DoublyLinkedList) manage their own element lifetimes. The compiler's automatic ref counting would double-count or interfere.

```lyric
trusted func ArrayList.push(mut self, item: T) {
    ref(item)           // manual increment
    // ... store item in backing array ...
}

trusted func ArrayList.remove(mut self, idx: i32) -> T {
    let item = self.data[idx]
    unref(item)         // manual decrement (may destroy)
    // ... shift elements ...
    return item
}
```

**Rules for `trusted` blocks/functions:**
- `ref(x)` and `unref(x)` are available (compile error outside `trusted`)
- The compiler does NOT auto-insert ref/unref for ref-counted class references
- For owned classes, `ref`/`unref` are silently ignored (no-ops)
- The programmer is responsible for correctness — the compiler trusts them

**Syntax:**

```lyric
trusted func ArrayList.push(mut self, item: T) { ... }

// Or block-level:
trusted {
    ref(widget)
    // manual memory management here
    unref(widget)
}
```

**Where `trusted` is needed**: stdlib containers that store class references (ArrayList, DoublyLinkedList, HashedList, Dict). User code almost never needs it.

### 3.7 `ref` and `unref` Semantics

```lyric
ref(x)     // increment _ref_count by 1 (no-op if owned class)
unref(x)   // decrement _ref_count by 1; if 0, destroy + free (no-op if owned class)
```

These are statement-level primitives (like `append`), not expressions.

---

## 4. Stack Safety — Structs and Tuples

### 4.1 No Escape Rule

References to stack-allocated structs and tuples **cannot escape** their scope:

- `mut` params pass by reference, but the reference is scoped to the call
- You cannot store a `mut` reference in a field, return it, or capture it in a spawn/closure
- You cannot take the address of a stack value and assign it to anything with a longer lifetime

**Enforcement**: the checker rejects any code that would store, return, or capture a `mut` reference. This is a simple syntactic check — no dataflow analysis needed.

### 4.2 Immutable Parameters — Optimizer Chooses

For non-`mut` struct/tuple parameters:
- Semantically always pass-by-value (callee gets a copy)
- The optimizer chooses the actual calling convention:
  - Small structs (≤ 16 bytes): pass by value (registers)
  - Large structs: pass by `const` pointer (no copy)
- This is invisible to the programmer — immutability makes the representation irrelevant

---

## 5. Array/Slice Memory Safety

### 5.1 The Problem

Dynamic arrays can reallocate their backing data on push/grow. If a `mut` reference into the array's elements exists (e.g., from `mut arr[i]`), reallocation invalidates that reference.

### 5.2 `mut resize` Annotation

Functions that may grow or shrink a dynamic array parameter annotate it:

```lyric
func sort(mut arr: [i32])                    // element mutation only — safe
func append_all(mut resize dst: [i32], src: [i32])  // may resize — invalidates element refs
```

`mut resize` implies `mut` (you can also mutate elements). The stdlib declares `push`, `pop`, `append`, and any resizing operation as `mut resize`.

### 5.3 Static Alias Checking Algorithm

The compiler tracks **provenance** — which base array a reference derives from — and rejects calls where a resize could invalidate a live reference.

**Provenance tracking** (within a function body):

| Expression | Provenance |
|-----------|-----------|
| `mut arr` | `arr` |
| `mut arr[i]` | `arr` (element ref) |
| `mut arr[i].field` | `arr` (transitive) |
| `let mut x = mut arr[i]` | `x` derives from `arr` |
| `mut local_var` | `local_var` (no conflict unless derived) |

**Conflict detection at each call site:**

```
for each pair of mut arguments (a, b) at a call site:
    if provenance(a) overlaps provenance(b):
        if either param is declared `mut resize`:
            ERROR: "cannot resize array while element reference exists"
```

**Within-scope tracking:**

```lyric
let mut arr = [1, 2, 3]
let mut x = mut arr[0]   // x derives from arr
push(mut resize arr, 4)  // ERROR: arr is borrowed by x
// After x goes out of scope (or is last used above this line):
push(mut resize arr, 4)  // OK: no outstanding element refs
```

**Liveness-based**: the conflict only exists while the element reference is **live** (has future uses). Once `x` is dead (no more reads/writes), `arr` is free to resize. This requires simple liveness analysis — compute last-use of each variable.

**Why this is simpler than Rust:**
- References cannot escape function scope (no lifetime parameters)
- References cannot be returned (no `'a` annotations)
- Only resize is dangerous, not all mutation (no need for shared vs exclusive borrows)
- No closures capturing references (spawn/closures take by value)
- Analysis is purely intra-procedural — no inter-procedural tracking needed

### 5.4 Subslices — Copy on Assign, View on Pass

Subslice assignment (`let a = arr[2:5]`) **copies** the elements into a new independent backing array. This is the default safe behavior — no aliasing, no shared mutation.

Subslice passing (`foo(arr[2:5])`) passes a **view** — no copy, shared backing data. The function receives a slice header pointing into the original array. The `mut resize` annotation prevents resizing while views exist.

For zero-copy local bindings, use `ref`:

```lyric
let ref view = arr[2:5]   // shared view, no copy
```

### 5.5 Scope-Based Deallocation

When a dynamic array goes out of scope, the backing array's ref count is decremented. If it hits 0, the backing data is freed:

```lyric
func example() {
    let mut arr = [1, 2, 3]
    // ... use arr ...
}  // decrement backing array ref count; free if 0
```

For arrays of arrays (`[[i32]]`), destruction is depth-first — each inner array's backing data is ref-decremented, then the outer array's.

For arrays of class references (`[Widget]`), each element is unref'd before the backing array is freed.

### 5.6 Slice Sharing in Function Parameters

When a slice is passed to a function (non-`mut`), the backing data is shared — no copy. The function receives a view into the caller's data. This is the performance-critical path (parsers, string processing).

```lyric
func count_zeros(data: [i32]) -> i32 {   // data is a view, not a copy
    let mut n = 0
    for x in data { if x == 0 { n = n + 1 } }
    return n
}
count_zeros(big_array[100:200])  // no copy — view into big_array
```

**Ref counting on backing data**: function parameters that receive slice views need the backing data to survive the call. The backing data has a ref count. Passing a slice increments it; function return decrements it. For simple cases where the caller's slice is clearly still alive, the optimizer can elide the ref count operations.

**`ref` bindings** also increment the backing data ref count and follow the same lifetime rules.

---

## 6. String Memory

Strings are `[u8]` — no unique semantics. They follow the same rules as dynamic arrays:
- Shared backing data with ref counting
- Freed when last reference goes out of scope
- `mut resize` for functions that grow/shrink strings

---

## 7. Concurrency

### 7.1 Ref Counting Across Spawn

When a `spawn` block captures a class reference, the ref count must be incremented at capture time and decremented when the spawned routine exits scope. The compiler inserts these automatically.

### 7.2 `destroys` and `mut resize` Across Spawn

The lifetime checker must treat spawn captures conservatively. If a spawned routine might `destroys(Widget)` or `mut resize` an array, the parent scope must treat those references as potentially invalid after the spawn point — unless a channel synchronization proves ordering.

### 7.3 Globals

Storing a class reference in a global variable auto-increments its ref count. Reading from a global gives a ref-counted copy. Globals are the one place where the compiler's intra-procedural analysis breaks down, so ref counting must always be applied for class references in globals, regardless of ownership.

### 7.4 Per-Slab Mutexes

The global slab is shared mutable state. With concurrent `spawn` access, alloc/free/field access are data races.

**Solution**: Per-class (per-slab) mutexes with auto-detection:
- The compiler determines which classes are ever accessed inside a `spawn` block (or transitively called from one)
- Only those classes get mutex-protected slab operations
- Classes accessed only from the main thread skip locking entirely — zero overhead
- Detection is conservative: if any code path from a `spawn` can reach a class, it gets a mutex

---

## 8. LIR Representation

All memory management operations are represented in the LIR, not generated directly in the C backend.

### 8.1 New LIR Nodes

**Types:**
- `LTyClassRef` — u32 class reference (replaces pointer types)

**Expressions:**
- `LExprSlabGet(class, field, ref)` — read field from slab
- `LExprSlabAlloc(class)` — allocate from free list / bump used
- `LExprRefCount(ref)` — read ref count (for optimizer use)

**Statements:**
- `LStmtSlabSet(class, field, ref, value)` — write field to slab
- `LStmtSlabFree(class, ref)` — return to free list
- `LStmtRefIncr(ref)` — increment class instance ref count
- `LStmtRefDecr(ref)` — decrement class instance ref count; if 0, destroy + free
- `LStmtSliceRefIncr(slice)` — increment backing array ref count
- `LStmtSliceRefDecr(slice)` — decrement backing array ref count; if 0, free backing data

### 8.2 Memory Management Pass

A new pass runs **after lowering, before optimization**:

1. **Slab structure generation**: For each class, emit the slab struct, global root entry, alloc/free/create functions
2. **Field access rewrite**: Replace `ExClassGet`/`LStmtClassSet` with `LExprSlabGet`/`LStmtSlabSet`
3. **Ref count insertion**: For non-owned classes in non-`trusted` functions, insert `LStmtRefIncr`/`LStmtRefDecr` at assignment/scope-exit/reassignment points
4. **Struct copy hooks**: For struct assignments where the struct transitively contains class references, insert ref increments for embedded handles
5. **Destructor extension**: Append `LStmtSlabFree` at end of existing destructors
6. **Slice ref count insertion**: Insert `LStmtSliceRefIncr`/`LStmtSliceRefDecr` for slice assignment/scope-exit
7. **Scope-exit cleanup**: Insert unref calls for local class references embedded in structs and arrays

### 8.3 Lifetime Checker Placement in Pipeline

The lifetime checker (the pass that infers `destroys` sets, `mut resize` conflicts, and provenance tracking) runs **after monomorphization**. Reasons:

- Generic type parameters are resolved — `ArrayList<Widget>.remove()` knows `T = Widget`
- All function bodies are concrete — no unresolved type variables
- The call graph is fully specialized — inference follows actual instantiations

Pipeline position:

```
Parse → Desugar → Check → Lower → Optimize → Monomorphize
    → Lifetime Check → Memory Management Pass → C backend
```

---

## 9. Reference Counting Cycles

### 9.1 The Problem

If class A holds a ref-counted reference to B, and B holds a ref-counted reference to A, neither reaches ref count 0. Memory leaks.

**Cycles through `owns` relations are fine** — ownership cascades destroy deterministically. Only `refs` (non-owning) relationships can form problematic cycles.

### 9.2 Solution: Static Cycle Detection

The compiler performs static cycle detection on the ref-counted class graph. If classes A and B can form a ref-counted cycle (through `refs` relationships), emit a **compiler warning**. The programmer resolves by:
- Converting one direction to an `owns` relationship (preferred)
- Using `trusted` code with manual ref/unref if the cycle is truly needed

**Future**: Consider a `weak` reference type that doesn't increment the ref count, similar to Swift's `weak` keyword. Deferred for now.

---

## 10. Implementation Plan

### Sprint 1: Slab Allocator + Field Access Rewrite
- Add `LTyClassRef`, `LExprSlabGet`, `LStmtSlabSet`, `LExprSlabAlloc`, `LStmtSlabFree` to LIR
- Write memory management LIR pass: generate slab structures, rewrite field access
- Update C backend to emit slab-based code (AoS first, SoA later)
- All existing tests should pass with index-based allocation instead of malloc

### Sprint 2: Ownership + Ref Counting + Copy Hooks
- Extend destructor pass to append `SlabFree` after cascade destroy
- Add `final` function support (pre-destroy hook)
- Add `LStmtRefIncr`/`LStmtRefDecr` to LIR
- Write ref count insertion pass for non-owned classes
- Struct copy hooks: auto-insert ref increments for embedded class references on copy
- Scope-exit cleanup: recursive unref for class references in structs and arrays
- Add `trusted` keyword to parser, checker
- Add `ref`/`unref` primitives
- Update stdlib containers (ArrayList, Dict, HashedList, DoublyLinkedList) with `trusted` + manual ref/unref

### Sprint 3: `destroys` Annotation + Safe Iterators
- Checker: infer `destroys` sets from call graph (which classes/params a function may destroy)
- Checker: after a `destroys(ClassName)` call, mark all references of that class as dead
- Checker: after a `destroys(param)` call, mark only that binding as dead
- Checker: warn when a public function's inferred `destroys` set changes
- Static cycle detection + warning for ref-counted class graphs
- Parser + checker: `safe` keyword on generator declarations
- Implement safe iterator yielding semantics (pre-fetch next, annotate loop variable as destroyable)
- Update DoublyLinkedList, HashedList with `_safe()` iterator variants

### Sprint 4: Stack Safety + Array Resize Safety
- Checker: reject escaping `mut` references (store, return, capture)
- Parser + checker: `mut resize` annotation on parameters
- Checker: provenance tracking + alias conflict detection
- Checker: liveness analysis for dead-reference cleanup
- Optimizer: non-`mut` struct pass-by-value vs const-pointer decision

### Sprint 5: Slice Ref Counting + Scope Cleanup
- Add ref count header to slice backing data
- `LStmtSliceRefIncr`/`LStmtSliceRefDecr` insertion at assignment/scope-exit
- Scope-exit free for local slices, arrays, and strings
- Subslice sharing with ref count increment
- Recursive unref for class references in arrays at scope exit (`[Widget]` cleanup)

---

## 11. Summary Table

| Type | Allocation | Deallocation | Sharing | Safety Mechanism |
|------|-----------|-------------|---------|-----------------|
| Primitives | Stack/register | Scope exit | Copy on assign | None needed |
| Struct/Tuple | Stack | Scope exit | Copy on assign | No-escape for `mut` refs |
| Class (owned) | Slab pool | Owner's destructor | u32 handle | Ownership cascade + `destroys` |
| Class (non-owned) | Slab pool | Ref count → 0 | u32 handle | Auto ref counting + `destroys` |
| Class (trusted) | Slab pool | Manual ref/unref | u32 handle | Programmer responsibility |
| Slice `[T]` | Heap backing | Scope exit (owned) | Copy on assign; shared on pass | `mut resize` + provenance |
| Slice `ref` view | Shared backing | Ref count on backing | Shared view | `mut resize` + no-escape |
| String `[u8]` | Same as slice | Same as slice | Same as slice | Same as slice |
