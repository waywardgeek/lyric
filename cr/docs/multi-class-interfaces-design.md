# Multi-Class Interfaces & Relations Design

**Date**: 2026-06-05
**Status**: Design — not yet implemented

## Motivation

Traditional single-class interfaces can't express relationships between types.
A Graph isn't a property of Node or Edge alone — it's a property of how they
relate. Writing graph algorithms (min-cut, Dijkstra, topological sort) requires
either:
- A single concrete graph type (petgraph) — not generic
- C++ concept maps (Boost BGL) — unusable ceremony

Lyric solves this with multi-class interfaces: an interface declares methods
across multiple participant types, and `impl` blocks wire them to concrete
classes via aliasing.

**Primary use cases**: graphs, doubly-linked lists, hashed relations, trees —
any data structure involving relationships between two or more types.

## Multi-Class Interface Syntax

The `func T.method(self)` syntax binds a method to a specific type parameter.
`self` is always the type before the dot.

```lyric
interface Graph<G, N, E> {
    func G.nodes(self) -> gen N
    func G.edges(self) -> gen E
    func N.out_edges(self) -> gen E
    func N.in_edges(self) -> gen E
    func E.source(self) -> N
    func E.target(self) -> N
}
```

### Free Functions

Functions without a type prefix have no receiver:

```lyric
interface Graph<G, N, E> {
    // ...methods above...
    func distance(a: N, b: N) -> f64   // no receiver, neither N is privileged
}
```

Use free functions only when there's no natural receiver.

### Default Implementations

Interfaces can provide default implementations — algorithms written once in
terms of the required methods:

```lyric
interface Graph<G, N, E> {
    // Required
    func G.nodes(self) -> gen N
    func N.out_edges(self) -> gen E
    func E.target(self) -> N

    // Default — free for all implementors
    func count_nodes(graph: G) -> u64 {
        let mut count: u64 = 0
        for _ in graph.nodes() {
            count = count + 1
        }
        return count
    }
}
```

Dijkstra, min-cut, topological sort can all live as default functions.

## Bare Relational Constraints

Instead of `where G: Graph<G, N, E>` (redundant `G:`), use bare constraints:

```lyric
func min_cut<G, N, E>(graph: G) -> (f64, ([N], [N]))
    where Graph<G, N, E>, E: Weighted
{
    for node in graph.nodes() {
        for edge in node.out_edges() {
            let w = edge.weight()
            let t = edge.target()
            // ...Stoer-Wagner
        }
    }
}
```

`where Graph<G, N, E>` asserts that G, N, E together satisfy Graph. This
grants all `func N.*` methods to values of type N, all `func E.*` methods to
values of type E, etc. The compiler resolves this from the interface declaration.

## impl Blocks with Aliasing

`impl` blocks map interface methods to concrete class methods/fields using
three forms:

### `=` — Method alias

```lyric
impl Graph<CircuitGraph, Component, Wire> {
    G.nodes    = CircuitGraph.components
    G.edges    = CircuitGraph.wires
    N.out_edges = Component.outWires
    N.in_edges  = Component.inWires
    E.source   = Wire.driver
    E.target   = Wire.receiver
}
```

### `<->` — Field binding (generates getter + setter)

```lyric
impl DoublyLinked<Folder, File> {
    P.first  <-> Folder.firstFile
    P.last   <-> Folder.lastFile
    C.parent <-> File.folder
    C.next   <-> File.nextFile
    C.prev   <-> File.prevFile
}
```

`<->` is shorthand: `P.first <-> Folder.firstFile` generates both
`P.first(self) -> C?` (getter) and `P.set_first(mut self, c: C?)` (setter)
mapped to the `firstFile` field.

### `{ body }` — Inline implementation

```lyric
impl Hashed<Graph, Edge, (Node, Node)> {
    // ...other aliases...
    C.key(self) -> (Node, Node) {
        return (self.source, self.target)
    }
}
```

Use inline implementations when the mapping requires computation, not just
field access.

## Generators

`gen T` is a generator return type — lazy iteration via `yield`:

```lyric
func G.nodes(self) -> gen N {
    let mut node = self.firstNode
    while !isnull(node) {
        yield node!
        node = node!.nextNode
    }
}
```

`for..in` works on both `[T]` (slices) and `gen T` (generators).

### Implementation Strategy

Under the hood, generators compile to an `Iterator<T>` state machine:

```lyric
interface Iterator<T> {
    func next(mut self) -> T?
}
```

- `gen T` function → compiler-generated struct implementing `Iterator<T>`
- `[T]` automatically satisfies `Iterator<T>` (index-based)
- `for x in expr` desugars to calling `.next()` until null

## Full Examples

### DoublyLinked

```lyric
interface DoublyLinked<P, C> {
    // Getters
    func P.first(self) -> C?
    func P.last(self) -> C?
    func C.parent(self) -> P?
    func C.next(self) -> C?
    func C.prev(self) -> C?

    // Setters
    func P.set_first(mut self, child: C?)
    func P.set_last(mut self, child: C?)
    func C.set_parent(mut self, parent: P?)
    func C.set_next(mut self, next: C?)
    func C.set_prev(mut self, prev: C?)

    // Default: append child to end of parent's list
    func append(parent: mut P, child: mut C) {
        match parent.last() {
            null -> {
                parent.set_first(child)
                parent.set_last(child)
                child.set_parent(parent)
                child.set_next(null)
                child.set_prev(null)
            }
            last -> insert_after(parent, last, child)
        }
    }

    // Default: insert new_child after existing
    func insert_after(parent: mut P, existing: mut C, new_child: mut C) {
        let old_next = existing.next()
        existing.set_next(new_child)
        new_child.set_prev(existing)
        new_child.set_next(old_next)
        new_child.set_parent(parent)
        match old_next {
            null -> parent.set_last(new_child)
            c    -> c.set_prev(new_child)
        }
    }

    // Default: remove child from parent's list
    func remove(parent: mut P, child: mut C) {
        match child.prev() {
            null -> parent.set_first(child.next())
            p    -> p.set_next(child.next())
        }
        match child.next() {
            null -> parent.set_last(child.prev())
            n    -> n.set_prev(child.prev())
        }
        child.set_parent(null)
        child.set_next(null)
        child.set_prev(null)
    }

    // Default: iterate children
    func children(parent: P) -> gen C {
        let mut child = parent.first()
        while !isnull(child) {
            let c = child!
            child = c.next()
            yield c
        }
    }
}
```

Usage with field bindings:

```lyric
impl DoublyLinked<Folder, File> {
    P.first  <-> Folder.firstFile
    P.last   <-> Folder.lastFile
    C.parent <-> File.folder
    C.next   <-> File.nextFile
    C.prev   <-> File.prevFile
}

// Now append, remove, insert_after, children all work on Folder/File
let folder = Folder.new("src")
let f1 = File.new("main.ly")
let f2 = File.new("lib.ly")
append(folder, f1)
append(folder, f2)

for file in children(folder) {
    println(file.name)
}
```

### Hashed Relations

```lyric
interface Hashed<P, C, K> where K: Hashable {
    func P.lookup(self, key: K) -> C?
    func P.insert(mut self, child: C)
    func P.remove(mut self, key: K) -> C?
    func P.contains(self, key: K) -> bool
    func P.entries(self) -> gen (K, C)
    func C.key(self) -> K
}
```

Usage:

```lyric
impl Hashed<Registry, Handler, string> {
    P.lookup   = Registry.findHandler
    P.insert   = Registry.addHandler
    P.remove   = Registry.removeHandler
    P.contains = Registry.hasHandler
    P.entries  = Registry.allHandlers
    C.key      = Handler.name
}
```

### Graph with Generic Algorithms

```lyric
interface Weighted {
    func weight(self) -> f64
}

interface Graph<G, N, E> {
    func G.nodes(self) -> gen N
    func G.edges(self) -> gen E
    func N.out_edges(self) -> gen E
    func N.in_edges(self) -> gen E
    func E.source(self) -> N
    func E.target(self) -> N
}

func dijkstra<G, N, E>(graph: G, start: N) -> Map<N, f64>
    where Graph<G, N, E>, E: Weighted, N: Hashable
{
    // Written once, works on any graph
    for node in graph.nodes() {
        for edge in node.out_edges() {
            let w = edge.weight()
            let t = edge.target()
            // ...relaxation
        }
    }
}

func min_cut<G, N, E>(graph: G) -> (f64, ([N], [N]))
    where Graph<G, N, E>, E: Weighted
{
    // Stoer-Wagner, written once
}

func topological_sort<G, N, E>(graph: G) -> [N]
    where Graph<G, N, E>
{
    // Kahn's algorithm, written once
}
```

Concrete implementation:

```lyric
impl Graph<CircuitGraph, Component, Wire> {
    G.nodes     = CircuitGraph.components
    G.edges     = CircuitGraph.wires
    N.out_edges = Component.outWires
    N.in_edges  = Component.inWires
    E.source    = Wire.driver
    E.target    = Wire.receiver
}

let circuit = CircuitGraph.load("cpu.spice")
let order = topological_sort(circuit)
let (cut_val, (partA, partB)) = min_cut(circuit)
```

## Named impl Blocks (Disambiguation)

When a type participates in multiple relations of the same interface with
identical type signatures, `impl` blocks need labels to disambiguate:

```lyric
impl Hashed<UserStore, User, string> as byEmail {
    P.lookup   = UserStore.findByEmail
    P.insert   = UserStore.addByEmail
    P.remove   = UserStore.removeByEmail
    P.contains = UserStore.hasEmail
    P.entries  = UserStore.emailEntries
    C.key      = User.email
}

impl Hashed<UserStore, User, string> as byUsername {
    P.lookup   = UserStore.findByUsername
    P.insert   = UserStore.addByUsername
    P.remove   = UserStore.removeByUsername
    P.contains = UserStore.hasUsername
    P.entries  = UserStore.usernameEntries
    C.key      = User.username
}
```

At call sites, qualify with the label:

```lyric
byEmail.insert(store, user)
byUsername.insert(store, user)

let found = byEmail.lookup(store, "bob@example.com")
let also  = byUsername.lookup(store, "bob")
```

**Labels are only required when ambiguous** — two impls of the same interface
with the same type parameters. Most impls won't need them.

## Uniform Function Call Syntax (Future)

`C.foo(args)` and `foo(self: C, args)` could be treated as equivalent — the
compiler wouldn't distinguish method calls from free function calls where the
first argument matches. Not required for initial implementation but the design
is compatible. Don't write code that cares about the distinction.

## Implementation Plan

### Phase 1: Generators
- Add `gen T` type to AST, parser, checker
- Add `yield` statement to AST, parser, checker
- Transpiler: `gen T` → Go iterator pattern (or channel-based)
- LIR: generator state machine lowering
- C backend: struct-based state machine

### Phase 2: Multi-Class Interface Declarations
- Extend `InterfaceDecl` AST to support `func T.method(self)` syntax
- Parser: `func` + identifier + `.` + identifier = typed method
- Checker: validate type params match interface type params
- Store per-type method maps on interface declaration

### Phase 3: impl Blocks
- New `ImplBlock` AST node: `impl Interface<T1, T2, ...> { mappings }`
- Parser: `=` (alias), `<->` (field binding), `{ body }` (inline)
- Checker: validate mapping signatures match interface declarations
- Registry: store impl blocks for constraint resolution

### Phase 4: Bare Relational Constraints
- Extend `where` clause parser for bare `Interface<T1, T2, T3>` (no `:`)
- Constraint solver: when `where Graph<G, N, E>`, grant all `func N.*`
  methods to type param N, etc.
- Lookup: find matching `impl` block to resolve concrete types at call sites

### Phase 5: Default Implementations
- Allow function bodies in interface declarations
- Transpiler: emit as standalone generic functions parameterized by interface types
- At call sites, resolve to the default unless overridden in impl block

### Phase 6: Transpiler & Backend Support
- Go backend: multi-class interfaces → multiple Go interfaces + glue
- C backend: vtable structs per interface, dispatch functions
- LIR: interface dispatch lowering

### Phase 7: Relations — Parsing & Field Injection
- `relation` as first-class AST node
- Parser: `relation Interface Parent[:label] (owns|refs) [Child[:label]]`
- `field T.name: Type` declarations in interfaces
- Desugar: relation → impl block, inject fields into concrete classes with label prefixing
- Checker: validate relation types match interface type params

### Phase 8: Relations — Default Methods & Destructors
- Inject default methods (append, remove, iterate) with field name rewriting
- Auto-generate destructor bodies: cascade destroy for `owns`, unlink for children
- Append/prepend semantics for destructor code composition

### Phase 9: Relations — Ref Counting (C Backend)
- `_refcount` field on classes not owned by any relation
- `ref(x)` / `unref(x)` explicit statements
- Suppress auto ref/unref in default code blocks
- `ref()`/`unref()` emission in C backend for insert/remove

### Phase 10: Relations — Dead Field Elimination (LIR)
- Collect all field reads across entire program
- Prove back-pointers unused: single owner + destructor only from owner
- Eliminate dead fields and all writes to them
- Simplify destructors when back-pointer eliminated

## Relations

Relations are the high-level user-facing concept that connects multi-class
interfaces to ownership, lifetime management, and automatic code generation.
A relation declaration says: "these classes are connected via this data
structure pattern, and this class owns that class."

### Syntax

```lyric
relation DoublyLinked Root owns [Func]
relation DoublyLinked Root:out_edges owns [Edge:out]
relation Hashed<string> Registry owns [Handler:by_name]
```

The general form:

```
relation Interface Parent[:label] (owns|refs) [Child[:label]]
```

- **Interface**: which multi-class interface provides the data structure
  (DoublyLinked, Hashed, Tree, etc.)
- **Parent/Child**: concrete classes bound to the interface's type params
- **owns/refs**: ownership semantics (see Lifetime below)
- **Labels**: optional, used to prefix auto-injected field names for
  disambiguation

### Default Fields

Interfaces declare default fields using `field T.name: Type` syntax:

```lyric
interface DoublyLinked<P, C> {
    field P.first: C?
    field P.last: C?
    field C.prev: C?
    field C.next: C?
    field C.parent: P?    // back-pointer, always declared

    // Default methods use these fields
    func append(parent: mut P, child: mut C) { ... }
    func remove(parent: mut P, child: mut C) { ... }
    func children(parent: P) -> gen C { ... }
}
```

When a relation is declared, the compiler:

1. Generates an `impl` block mapping the interface to the concrete classes
2. Injects fields into the concrete classes, prefixed by label if present
3. Injects default methods (append, remove, iterate, destroy)

Example — `relation DoublyLinked Node:out_edges owns [Edge:out]` generates:

| Interface field | Injected as          |
|----------------|----------------------|
| `P.first`      | `Node.out_edges_first: Edge?` |
| `P.last`       | `Node.out_edges_last: Edge?`  |
| `C.prev`       | `Edge.out_prev: Edge?`        |
| `C.next`       | `Edge.out_next: Edge?`        |
| `C.parent`     | `Edge.out_parent: Node?`      |

Label prefixing rule: `{label}_{field_name}`. Unlabeled relations use bare
field names. The default method bodies are rewritten to use the prefixed names.

### Lifetime & Ownership

There are two ownership modes:

**`owns`** — Parent owns child's lifetime. The child lives until its destructor
is called. No reference counting on the child. The parent's auto-generated
destructor cascades destruction to all owned children.

**`refs`** — Parent references but does not own child. The child's lifetime is
managed elsewhere (by another owner, or by reference counting if no owner
exists).

**Root rule**: A class not owned by any relation is **reference counted**.
A class owned by at least one relation survives until its destructor is called
(no ref counting overhead).

A class can be owned by multiple relations. Example:

```lyric
relation DoublyLinked Chip:pins owns [Pin:chip]
relation DoublyLinked Net:connections owns [Pin:net]
```

Pin is owned by both Chip and Net. When either owner destroys a Pin, it must
unlink from the other owner's list — this requires the back-pointer.

### Auto-Generated Destructors

Each `owns` relation appends cleanup code to the parent's destructor. The
interface provides the cleanup implementation. For DoublyLinked:

```
// Auto-appended to ~Node():
while !isnull(self.out_edges_first) {
    destroy(self.out_edges_first!)   // cascades to Edge destructor
}
```

The Edge destructor in turn has auto-appended code to unlink from its parent:

```
// Auto-appended to ~Edge():
DoublyLinked_remove<Node, Edge, out>(self.out_parent, self)
```

Destructors are composed from all relations a class participates in. Each
relation appends its own cleanup block. The interface's `destroy` default
method provides the template; the label selects which fields to use.

**Append/prepend semantics**: Interface default code blocks can specify whether
they are appended or prepended to a function body. Destructor cleanup is always
appended (user code in the destructor runs first, then auto-cleanup).

### Back-Pointer Elimination

Back-pointers (`C.parent: P?`) are always declared but may be eliminated as a
dead-field optimization in LIR:

1. Walk all LIR — collect reads of every injected field
2. If `parent` is never read by user code, AND
3. The child has exactly one owner, AND
4. The child's destructor is only called from the owner's destructor (not
   called directly or from another owner's destructor)
5. → Eliminate the back-pointer field and all writes to it

Multiple owners make back-pointers mandatory: when Net destroys a Pin, it must
unlink from Chip's list, which requires `Pin.chip_parent`.

### Reference Counting Rules

In the C backend, reference counting follows DataDraw/Rune conventions:

**Default code blocks suppress auto ref/unref.** Inside auto-generated method
bodies (insert, remove, destroy), pointer assignments do NOT automatically
increment or decrement reference counts. This prevents back-pointers from
creating reference cycles.

Explicit `ref(x)` and `unref(x)` statements are available for when the
programmer (or the interface's default implementation) intentionally wants to
adjust counts:

```lyric
// Inside DoublyLinked.append default implementation:
func append(parent: mut P, child: mut C) {
    // ... link child into parent's list ...
    child.set_parent(parent)   // no ref bump — auto-generated code
    ref(child)                 // explicit: one ref for ownership
}

func remove(parent: mut P, child: mut C) {
    // ... unlink child from parent's list ...
    child.set_parent(null)     // no ref drop — auto-generated code
    unref(child)               // explicit: drop the ownership ref
}
```

This way, a child linked into a doubly-linked list has exactly one reference
count bump (from the ownership claim), not three (parent pointer + prev + next
would all bump if auto-counted).

**User code follows normal rules.** Only default code blocks (generated from
interface implementations) suppress auto ref/unref. Regular user code gets
automatic reference counting as expected.

## C Backend: Relations & Memory Management

The C backend is where relations have the most impact. Go has garbage
collection, so ownership and ref counting are irrelevant there — but the
auto-generated insert/remove/destroy methods are still valuable.

### Struct Layout

Default fields are injected directly into C struct definitions:

```c
typedef struct Node Node;
typedef struct Edge Edge;

struct Node {
    // User-declared fields
    const char* name;
    // Injected by relation DoublyLinked Node:out_edges owns [Edge:out]
    Edge* out_edges_first;
    Edge* out_edges_last;
};

struct Edge {
    // User-declared fields
    int32_t weight;
    // Injected by relation
    Edge* out_prev;
    Edge* out_next;
    Node* out_parent;   // back-pointer (may be eliminated)
    // Reference counting (only if not owned, or owned by multiple)
    uint32_t _refcount;
};
```

### Reference Counting Implementation

```c
static inline void edge_ref(Edge* e) {
    if (e) e->_refcount++;
}

static inline void edge_unref(Edge* e) {
    if (e && --e->_refcount == 0) {
        edge_destroy(e);  // calls destructor, then free
    }
}
```

Classes owned by exactly one relation have no `_refcount` field — their
lifetime is entirely managed by the owner's destructor.

### Destructor Composition

```c
void node_destroy(Node* self) {
    // User destructor body (if any) runs first

    // Auto-appended: cascade destroy owned out_edges
    while (self->out_edges_first) {
        edge_destroy(self->out_edges_first);
    }

    free(self);
}

void edge_destroy(Edge* self) {
    // User destructor body (if any) runs first

    // Auto-appended: unlink from owner's DoublyLinked list
    // (uses out_parent back-pointer to find the parent)
    if (self->out_parent) {
        doublylinked_remove_out(self->out_parent, self);
    }

    free(self);
}
```

### Insert/Remove (No Auto Ref/Unref)

```c
// Generated from DoublyLinked.append with label "out"
void doublylinked_append_out(Node* parent, Edge* child) {
    // All pointer writes — NO ref count changes (default code block)
    child->out_parent = parent;
    child->out_prev = parent->out_edges_last;
    child->out_next = NULL;
    if (parent->out_edges_last) {
        parent->out_edges_last->out_next = child;
    } else {
        parent->out_edges_first = child;
    }
    parent->out_edges_last = child;
    // Explicit ref: one count for ownership
    edge_ref(child);
}
```

### Interface Dispatch via Vtables

The C backend implements interfaces using function pointer tables (vtables).
Each interface gets one vtable struct type. Each impl block generates one
static vtable instance populated with auto-generated getters/setters. Default
methods are compiled **once** against the vtable — no per-relationship code
duplication.

```c
// One vtable struct per interface
typedef struct DoublyLinked_vtable {
    void* (*P_first)(void* self);
    void* (*P_last)(void* self);
    void* (*C_parent)(void* self);
    void* (*C_next)(void* self);
    void* (*C_prev)(void* self);
    void (*P_set_first)(void* self, void* child);
    void (*P_set_last)(void* self, void* child);
    void (*C_set_parent)(void* self, void* parent);
    void (*C_set_next)(void* self, void* next);
    void (*C_set_prev)(void* self, void* prev);
} DoublyLinked_vtable;

// Default methods compiled ONCE — vtable provides the indirection
void dll_append(DoublyLinked_vtable* vt, void* parent, void* child) {
    void* old_last = vt->P_last(parent);
    if (old_last == NULL) {
        vt->P_set_first(parent, child);
    } else {
        vt->C_set_next(old_last, child);
    }
    vt->C_set_prev(child, old_last);
    vt->C_set_next(child, NULL);
    vt->P_set_last(parent, child);
    vt->C_set_parent(child, parent);
}

// Auto-generated getters/setters per impl — one cast + field access
static void* folder_get_files_first(void* self) {
    return ((Folder*)self)->files_first;
}
static void folder_set_files_first(void* self, void* child) {
    ((Folder*)self)->files_first = (File*)child;
}

// One static vtable instance per impl
static DoublyLinked_vtable dll_folder_files = {
    .P_first     = folder_get_files_first,
    .P_set_first = folder_set_files_first,
    // ...
};
```

**SoA variant**: getters/setters index into arrays instead of casting pointers.
Same vtable shape, different getter bodies:

```c
static void* node_soa_get_first(void* self_idx) {
    uint32_t idx = (uint32_t)(uintptr_t)self_idx;
    return (void*)(uintptr_t)node_out_edges_first[idx];
}
```

**Inlining**: Since vtable instances are static constants, the compiler can
devirtualize and inline speed-critical paths. For hot inner loops, profile-
guided optimization or `__attribute__((always_inline))` on getters can
eliminate the indirect call overhead entirely.

**Binary size**: One copy of each default method per interface (not per
relation). The per-impl cost is only the tiny getters/setters and the vtable
struct. Duplicate function removal (linker ICF or a post-process pass) can
further deduplicate identical getter bodies across impls with the same layout.

### Dead Field Elimination

After LIR lowering, a pass identifies back-pointer fields that are never read:

1. Collect all field reads across the entire program
2. For each `C.parent` injected by a relation:
   - If never read in user code
   - If child has single owner
   - If child destructor only called from owner's destructor
3. Remove the field from the struct, remove all writes to it
4. Simplify the destructor (no need to unlink from parent — parent is the
   only one who calls destroy, and it walks its own list)

This optimization is critical for cache performance in hot inner loops where
the back-pointer wastes a cache line slot.

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Method binding syntax | `func T.method(self)` | Reading order matches reasoning — receiver type first |
| Constraint syntax | `where Graph<G, N, E>` | Bare relational, no redundant `G:` prefix |
| Field binding | `<->` operator | Generates getter+setter, cuts boilerplate in half |
| Method alias | `=` operator | Simple, distinct from field binding |
| Generators | `gen T` + `yield` | Lazy iteration, no allocation, natural for linked structures |
| Free functions in interfaces | `func name(args)` (no type prefix) | For operations with no natural receiver |
| Default fields | `field T.name: Type` in interfaces | Auto-injected into classes via relations; reduces boilerplate |
| Label prefixing | `{label}_{field}` | Single label per relation, applied uniformly to all injected fields |
| Back-pointers | Always declared, dead-code eliminated | Conservative: always correct, optimized away when provably unused |
| Ref counting scope | Suppressed in default code blocks | Prevents cycles from back-pointers; explicit ref/unref for ownership |
| Root class lifetime | Ref counted if unowned | Classes not owned by any relation need automatic lifetime management |
| Destructor composition | Append cleanup per relation | Each relation appends its own unlink/cascade; user code runs first |
| UFCS | Deferred | Design is compatible; don't write code that distinguishes |
