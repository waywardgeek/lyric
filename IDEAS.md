# Lyric IDEAS

A capture file for design ideas — Lyric language features and compiler
work that are larger than a TODO entry but not yet sprint-scoped.
Distinct from `TODO.md` (concrete bug/task list) and `cr/docs/*.md`
(shipped or near-shipped design documents).

---

## Hewitt's recommended priorities (2026-06-24)

Asked for my top picks. In order of leverage / pain-times-impact, with
honest rationale:

1. **Generic AST visitor refactor** *(TODO, from 06-22)*. Same shallow-
   copy-plus-mutation bug shape bit me TWICE in one sprint yesterday
   (`deep_copy_type_expr` and `substitute_type_expr_copy` — both with
   identical stale "shallow is safe" comments predating the rich-
   substitution path). Every duplicated AST walker is a future
   "forgot an arm" bug waiting to happen. One centralized visitor
   that consumers MUST stay current with structurally eliminates the
   class. Highest-leverage de-jank in the codebase.

2. **Reject undeclared type identifiers** *(TODO, attempted 06-23,
   reverted; #7 in this file)*. `FlexVia<Wieght>` silently becoming a
   generic over `Wieght` and surfacing as "TypeVar leak" at the
   validator wastes hours per occurrence. Bounded scope, needs
   careful registration-ordering work.

3. **`if`/`match` as expression** *(idea #N below to be written)*.
   Hit `let x = if cond { a } else { b }` at least three times in two
   sessions. Every modern language has it; the workaround
   (`let mut x = default; if cond { x = v }`) is verbose, error-
   prone (default needs to make sense), incurable by discipline.
   Small change, big quality-of-life.

4. **Secret tracking** *(idea #4)*. Strategic differentiator, not just
   a feature. "Fast like C" space is crowded; "fast AND safe by
   construction for crypto" is wide open and aligns with OpenADP /
   leadfoot-adjacent work. Rune proved it's small.

Honorable mentions once 1-4 ship: **`unique` ownership** (#1),
**O1 auto-inline single-yield generators** (Bill says the bootstrap
is 100% inlined this way → free perf), **source-Sym corruption in
error spans** (cryptic `-1728471424:343:40` style numbers — small
diagnostic-infra bug, painful to live with).

**Methodology note:** these are MY rankings. Bill's may differ. The
list exists to be argued with, not deferred to.

---

Format per entry:
- **What** — one-line summary
- **Why** — the user-visible problem or principle it serves
- **Design sketch** — enough mechanism to remember the shape
- **Open questions** — what we haven't decided
- **Scope estimate** — rough LOC / sessions
- **Status** — `idea` / `designing` / `prototyping` / `partially-shipped`
- **Date** — when added / last touched

When an idea becomes a sprint, move the design notes into
`cr/docs/<topic>.md` and leave a one-line pointer here.

---

## 1. `unique` ownership variant on relations

- **Status**: idea
- **Date**: 2026-06-24
- **Scope**: medium (parser + AST + checker + lowerer + runtime — easily a multi-day sprint)

**What.** Add `unique` to the existing `owns` / `refs` set of
relation-side annotations. A uniquely owned child belongs to exactly
one parent at a time, but that parent can be drawn from any number of
distinct relations. The child's back-pointer is therefore a **tagged
union of all possible parents**, not a single typed pointer.

**Why.** Today the choice is binary: `owns` (one specific parent type,
lifetime-coupled) or `refs` (any parent, no lifetime semantics). Real
data models often need "exactly one owner from a set of possible
owners" — e.g. a UI widget owned by either a Panel or a Window, a
network packet owned by either a Queue or a Connection. Modelling
this with `refs` loses the lifetime guarantee; modelling it with
multiple `owns` relations gives N parent fields, only one non-null at
a time, with no compiler enforcement that exactly one is set.

**Design sketch.**
- Syntax: `relation Foo Parent:label unique [Child:back]` — `unique`
  in place of `owns` / `refs` on the parent side.
- AST: extend `RelationKind` enum from `{ Owns, Refs }` to
  `{ Owns, Refs, Unique }`. Or split into two flags
  (`owns_lifetime: bool`, `unique_at_most_one_owner: bool`) since
  ownership and uniqueness are now somewhat orthogonal.
- Storage: when a child class is the target of one or more `unique`
  relations, desugar emits a tagged union back-pointer:
  `parent: UniqueParent<P1, P2, P3>?` where the union variants
  cover every relation that names this child as a unique target.
- Destructor responsibility: when the child is destroyed (because
  it was removed from its unique-owning parent), the destructor
  **uses the tagged union to dispatch the correct parent-relation
  cleanup**. This is the lifetime-fixing guarantee — the child can
  never be destroyed without the parent's containing relation being
  updated.
- UAF interaction: any outstanding `ref` to the child becomes a UAF
  the moment the unique owner releases it. Couples tightly with
  idea #2 below.

**Open questions.**
- Does `unique` imply `owns` (lifetime coupling) or can it coexist
  with `refs`-style non-owning unique relations? My read: `unique`
  is always lifetime-coupled — if a parent uniquely "has" a child
  via a `refs unique` relation, the parent must clear its slot on
  destruction or we get a dangling pointer.
- How does the tagged-union back-pointer interact with relation
  hints (`DoublyLinked`, `ArrayList`, `HashedList`)? Probably each
  hint needs a unique-aware variant.
- What's the migration story for existing `owns` relations? Are
  they all candidates to become `unique` if they're the only
  parent? Probably not — keep them distinct.

---

## 2. UAF detection via destructor-driven location nulling

- **Status**: idea
- **Date**: 2026-06-24
- **Scope**: large (whole-program analysis + AST instrumentation + runtime — multi-week sprint)

**What.** Statically detect Use-After-Free at the language level by
threading destruction sets through every function signature and
maintaining per-class live-reference lists that get nulled on
destruction.

**Why.** Lyric currently relies on the runtime + RC + the not-yet-
implemented `unique` to prevent UAF. There's no compile-time signal
when a function call could plausibly destroy a class instance that
the caller still holds a reference to. Bill's design closes the gap
without forcing users into a borrow-checker mental model.

**Design sketch.** Three coupled mechanisms:

1. **Destruction effect annotations on every function.** The compiler
   computes, per function / method, the transitive set of class
   types whose instances could be destroyed as a side effect of
   calling the function. Stored on the FuncDecl after a fixed-point
   pass. User-visible as something like
   `func foo(x: Foo) destroys { Bar, Baz }` (could be inferred and
   shown rather than required).

2. **Per-class live-location lists.** For every class that appears in
   any function's `destroys` set, the runtime object grows a
   *registry of live references* — every place a reference to this
   instance currently lives (other class fields, locals on the
   stack, etc.). The destructor walks this registry and nulls each
   location before freeing.

3. **Reference registration at ref-acquisition time.** Whenever the
   compiler emits code that takes a reference to such an instance
   AND the reference could outlive a subsequent call to a function
   that `destroys` that class, the compiler also emits a registry-
   insertion. Reference destruction (going out of scope, field
   reassignment) emits a registry-removal.

4. **Compiler error on non-nullable refs in UAF-prone positions.** If
   the user takes a reference of non-nullable type to an instance
   the registry would have to null on destruction, that's a compile
   error — because nulling a non-nullable field is a contradiction.
   Forces the user to either (a) make the reference nullable and
   handle the null-on-UAF case explicitly, or (b) restructure to
   avoid keeping the reference across the dangerous call.

**Why this design is good.**
- The non-nullable error makes UAF *visible* to the user without
  imposing borrow-checker ceremony. Users see "this reference will
  be nulled if X is destroyed — declare it nullable."
- No runtime exception path needed: if the user declares the
  reference nullable, normal `?` null handling covers the UAF case.
  The "use after free" becomes "use a null check after a call that
  might have nulled this."
- Effect inference is the same machinery as exception-effect
  systems (Koka, Eff) — well-understood ground.

**Open questions.**
- How fine-grained is the `destroys` set? Per-class only, or
  per-instance (e.g. "destroys the instance passed as arg 0")?
  Per-instance is much more precise but requires a separation-
  logic-shaped analysis.
- What about reference-counted instances? `destroys { Bar }`
  literally means "the refcount of some Bar might hit zero during
  this call." A function that takes a refcount probably doesn't
  destroy unless it's the last reference — does the analysis need
  to model that?
- The live-location list itself is mutable state on the destroyed
  object. Concurrency story: if a thread is mid-traversal of a
  field that another thread nulls, what's the contract?
- Stack-local references — does every local hold a registry slot,
  or only locals that outlive a `destroys` call? Latter is much
  cheaper but requires the compiler to know which locals span
  which calls.
- How does this compose with `unique` (idea #1)? `unique` already
  provides one form of destruction-propagation guarantee for the
  parent's slot; this idea generalizes it to arbitrary references.

**Relationship to Rust/Borrow Checker.** This is deliberately *not*
a borrow checker. It permits aliasing freely. The constraint is
weaker (must-be-nullable-if-dangerous) but the user-visible model
is much simpler: "if some function down the call stack could
destroy this thing, mark the reference `?`." The borrow checker
forbids the aliasing; we permit it and force the user to
acknowledge the consequence.

---

## (more ideas — add below)

---

# Language features (continued)

## 3. Explicit type-vars on aliased generic callables in impl bindings

- **Status**: idea (already captured in TODO as "Explicit type-binding in impl alias mappings (de-jank)")
- **Date**: 2026-06-24 (added 2026-06-23)
- **Scope**: medium

Symmetric with how we declare type-vars at every other use site of a
generic callable (free function call, method call): an impl alias
binding to a generic function or method should declare its type
arguments at the binding site, not rely on string-coincidence between
the impl's `<W>` and the helper's `<W>`. See TODO.md for the full
design and the gcc-failure example from `testdata/graph.ly` Part 4.

---

## 4. Secret tracking (constant-time crypto, à la Rune)

- **Status**: idea
- **Date**: 2026-06-24
- **Scope**: medium — Rune showed this is small and tractable

**What.** First-class `secret` type marker that the compiler tracks
through value flow. Two enforced rules:

1. **Never branch on secret data** — `if`, `while`, `match`, short-
   circuit `&&`/`||`, ternary-like patterns: all reject secret-typed
   conditions at compile time.
2. **Never use secret data as an array/map index** — index
   expressions reject secret-typed indices.

**Why.** Constant-time crypto is otherwise a discipline that has to
live in every reviewer's head, and constant-time-by-discipline is
a known-bad strategy. Rune proved the feature is small to implement
and catches the kinds of mistakes that fail real audits. Lyric is
already cryptography-adjacent through OpenADP and ViASIC heritage
— adding `secret` tracking lets Lyric pitch itself as "the
high-performance language where your crypto code can be safe by
construction."

**Design sketch.**
- `secret T` type modifier (or `@secret` attribute) — propagates
  through arithmetic and bitwise ops (result of `a + b` where
  either is secret is secret).
- Reject at type-check: `if cond { ... }` where `type_of(cond)` is
  secret; `arr[i]` where `type_of(i)` is secret.
- Provide a constant-time `select(cond, a, b)` builtin for the
  pattern that USED to be a branch.
- Provide a `declassify(s)` for when the programmer explicitly
  takes responsibility (e.g. after a successful tag-comparison).
- Open: how does secret propagate through Dict / generic
  containers? Rune's answer should be the reference.

---

## 5. Class-declarable handle width

- **Status**: idea
- **Date**: 2026-06-24
- **Scope**: small

**What.** Let the class declaration specify the uint width used for
handles/indices to instances of that class — `class Foo : u16 { ... }`
or similar.

**Why.** Today every class handle is the same default width. Many
classes have small max counts and could pack 4× tighter with u16 or
8× with u8 handles. Memory-intensive workloads (the ones we want to
win at — see #11) benefit from this directly. ViASIC's database
spent years tuning exactly this; the lesson generalizes.

**Open questions.**
- Does this interact with `unique`'s tagged-union back-pointer (#1)?
  The union's discriminant width could also be declarable.
- How does the runtime handle overflow if the program creates more
  instances than the declared width allows? Hard error? Auto-grow
  with a slow path?

---

## 6. Import statements — proper handling without circular-dep checks

- **Status**: idea
- **Date**: 2026-06-24
- **Scope**: medium

**What.** Implement real import statements (not the
prefix-and-flatten pass we have now). Explicitly avoid Go's circular-
dependency restriction.

**Why.** Lyric currently has `resolve_module_imports` which flattens
all packages into one global namespace — works for the bootstrap
but doesn't scale to a real library ecosystem. Go's circular-dep
ban is a notorious source of refactoring pain that adds nothing
real for code quality (most "circular" deps are perfectly sensible
two-way relationships between sibling modules). Lyric should permit
them.

**Open questions.**
- What's the linker/compilation-unit story? Go's ban exists because
  the compiler model is one-file-per-package-unit. Lyric compiles
  to a single C file currently; circular imports at the Lyric
  level can just compile fine.
- Visibility: do imports cross the `pub` boundary, or is everything
  in an imported module accessible? KISS leans toward: imported =
  fully accessible at the language level.

---

# Optimizations

A bunch of these are individually small but compound. Most belong to
a future "optimizer sprint" once the core language is stable.

## O1. Auto-inline single-yield generators

- **Status**: idea
- **Date**: 2026-06-24
- **Scope**: small (Rune precedent, text-rewriting at desugar)

**What.** Generators with exactly one `yield` (no loop, no multiple
yield sites) are mechanically equivalent to inline expressions. The
desugar pass should rewrite their call sites in-place rather than
emitting a generator state machine.

**Why.** Rune does this and 100% of the bootstrap compiler so far
(pre-self-compile) is auto-inlined this way. The state-machine
codegen for a single-yield generator is pure overhead — heap
allocation, indirect call, the works — vs. an inline expression.

**Design sketch.**
- In desugar (or early in lowerer), detect FuncDecls with a single
  `Yield(value)` statement reachable from entry, no loops above it.
- Replace each call site with the substituted body, treating the
  yielded expression as the call's result.
- Recursive single-yield generators are NOT eligible (would infinite-
  inline).

---

## O2. Auto-inline / monomorphize interfaces per unique vtable

- **Status**: idea
- **Date**: 2026-06-24
- **Scope**: medium

**What.** For each impl block that produces a unique combination of
concrete classes (a unique vtable, in OO terms), emit a fully-
monomorphized specialization of every default method that uses that
impl. Default methods become direct calls in the monomorphized copy
— no vtable lookup.

**Why.** Same motivation as Rust trait monomorphization. The vtable
indirection is pure cost when the compiler statically knows the
binding (which, with `impl` blocks, it always does for non-dynamic-
dispatch sites). We already do per-impl specialization for default-
method BODIES (desugar pass 4.5 as of yesterday's sprint) — this
extends the principle to interior call sites within those bodies.

**Open questions.**
- Code-size blowup: N impls × M default methods × call-site count.
  Probably wants a heuristic gate (see O3).
- How does this interact with dynamic dispatch (does Lyric even
  HAVE dynamic dispatch today? — investigate).

---

## O3. Inlining heuristic with compiler flags + future PGO

- **Status**: idea
- **Date**: 2026-06-24
- **Scope**: small initial, grows with PGO

**What.** A unified "how aggressive to inline" heuristic that drives
O1, O2, and any future inlining work. Knob is per-build via
optimization flags (`-O0` / `-O1` / `-O2` / `-O3`-ish), and
eventually consumes profile data (PGO) for hot/cold call sites.

**Why.** Code size vs. speed trade-off is workload-specific.
Bootstrap probably wants tight (we measure binary size); a release
build of a hot library wants aggressive. Profile-guided inlining is
the table stakes for any modern compiler that wants to win
benchmarks (see #11).

---

## O4. Permanent classes — drop the freelist, simpler allocator

- **Status**: idea
- **Date**: 2026-06-24
- **Scope**: small

**What.** When a class is declared `permanent` (never destroyed),
the optimizer omits the freelist field from the storage layout and
emits a simpler bump-style allocator function instead of the
freelist-pop allocator.

**Why.** `permanent` is already a declared property; the freelist
overhead (one field per class plus the alloc/free logic) is dead
weight when the runtime invariant guarantees it's never used.
Free win, no user-facing change.

---

## O5. Drop unused parent back-pointers (with careful destructor accounting)

- **Status**: idea
- **Date**: 2026-06-24
- **Scope**: medium — the subtlety is the destructor accounting

**What.** Reachability-analyze every parent back-pointer field. If
no user code reads the back-pointer, delete it from the class
layout and skip emitting writes to it.

**Why.** Back-pointers are common (every owning relation generates
one) but often unused — users walk parent→child far more than
child→parent. Each dead back-pointer is one pointer per child
instance of memory waste plus the write traffic to maintain it.

**Critical subtlety (Bill's note).** Destructors that exist *only*
because of an enclosing `unique`/`owns` lifetime relationship use
the back-pointer to fix the parent's slot during destruction. That
use should NOT count as a "real" use for reachability — the back-
pointer maintenance cost is being incurred SOLELY to support the
back-pointer maintenance, which is circular. If the child can only
be destroyed via the parent's relation (i.e. lifetime-owned), the
parent's slot is already being walked-as-the-cause-of-destruction
and doesn't need the child's back-pointer at all. Drop both
together.

**Open questions.**
- The "destructor-only use" detection needs to distinguish "this
  destructor was called because the unique owner is removing me"
  from "this destructor was called for some other reason." Per
  destructor-call-site analysis? Per relation kind?

---

## O6. Auto-merge freelist field into a compatible size field

- **Status**: idea
- **Date**: 2026-06-24
- **Scope**: small

**What.** When a class has both a freelist-next field (for the
allocator) and a size/count field that fits in the same word width,
fold them — the freelist link only matters on free objects, the
size field only matters on live objects, so they're never live
simultaneously.

**Why.** One word per instance saved. Common pattern in tuned
allocators (jemalloc et al.). Pairs naturally with #5 (declarable
handle width) and #O4 (permanent classes skip the freelist
entirely, so this only applies to non-permanent classes with a
size-shaped field).

---

# Tooling

## T1. Auto-invoke gcc (or specified C compiler) by default

- **Status**: idea
- **Date**: 2026-06-24
- **Scope**: tiny

**What.** `lyric compile foo.ly` should produce a binary by
default, invoking the system C compiler on the emitted C. A
`--no-cc` flag (or similar) stops at the .c file for users who
want to control the gcc invocation themselves.

**Why.** Today every user (including me, every session) does the
two-step dance: `./lyric compile foo.ly -o /tmp/foo.c && gcc
-std=gnu11 -O2 -w -I runtime -o /tmp/foo /tmp/foo.c -lm -lpthread`.
The flags are non-obvious, easy to forget, and there's no reason
the compiler shouldn't just do this. Costs one shell-out per
compile.

**Open questions.**
- Should `-o foo` mean "binary named foo" by default (with `-o
  foo.c` triggering source-only)? Or always two output paths? The
  former matches gcc/rustc convention.
- C compiler selection: env var? `--cc gcc` flag? Both?

---

## T2. Benchmarking — start measuring, target memory-intensive wins

- **Status**: idea
- **Date**: 2026-06-24
- **Scope**: ongoing — needs benchmark harness first

**What.** Stand up a benchmark suite for Lyric. Focus on memory-
intensive benchmarks where Lyric's strengths (SoA, relation
ownership, declarable handle widths per #5) compound. Promote the
results with the framing: **real applications are memory-intensive
— those are the benchmarks that matter.**

**Why.** Languages get adopted on benchmark wins as much as on
features. The crowded "fast like C" space is well-defended; the
"fast and memory-tight on real workloads" angle is much less
contested. Lyric's relation system, SoA option, and (planned)
declarable handle widths target exactly the workloads where memory
bandwidth and footprint dominate — graph traversal, simulation,
EDA, databases, ML inference data plumbing, big-data ETL.

**What benchmark suite candidates?**
- Graph traversal (BFS/DFS over large sparse graphs).
- Static analysis (SAT solving? Compiler front-end work?).
- Database-shaped (join + aggregate over columnar tables).
- Sim (n-body, particle systems with lots of pointer-chasing).
- (Anti-target: pure compute / matmul — those are the standard
  benchmarks the entrenched players win at. We don't pitch into
  the strength.)

**Open questions.**
- What benchmark framework — homegrown harness, or borrow from
  someone (e.g. Computer Language Benchmarks Game format)?
- Comparison baseline: C, C++, Rust, Go all plausible. Probably
  all four for the headline numbers.
- Where do results live — `bench/` in-tree, separate repo, or
  somewhere external like a benchmark-tracking site?

---

## (continue below as new ideas surface)

---

# Big-features placeholder section
# (Names captured for memory; full design to come in later sessions.)

## 7. Dynamic extensions in Lyric classes (DataDraw-style, BIG)

- **Status**: designing (motivation + mechanism explained 2026-06-24)
- **Date**: 2026-06-24
- **Scope**: large — language addition, codegen change, destructor
  chain mechanism, possibly cross-shared-library support
- **Heritage**: DataDraw (Bill, 30 years). DataDraw 3.0 only supports
  the SoA-style implementation because it consistently won.

**What.** Let a class declare itself as an **extension** of another
class — adding fields to the base class without modifying the base
class's declaration. All instances of the base class implicitly
carry the extension's fields (allocated on demand, scoped to a
tool's lifetime). Same handle works for base and extension access.
Multiple extensions of the same base coexist as a DAG; each tool
or library declares its own extension(s), independently, and they
all just work.

**Why — the EDA / shared-database motivation.** Bill (2026-06-24):

> *"This one is huge for complex systems that run many tools over
> shared data, like we always do in electronic design automation,
> e.g. parser, technology mapper, placer, router, DRC, and output
> artifact generation. EDA systems always have a common 'database',
> which is why CodeRhapsody has a directory called pkg/database —
> not an actual database, but the shared data repository in memory
> that all the tools use."*

Concrete example: a `Inst` class in the database represents a
placeable instance of a gate. The base `Inst` has the fields every
tool needs (name, type, connectivity). The **placer**, when it
runs after the technology mapper, needs a dozen additional fields
on every `Inst` — real-valued X/Y coordinates for quadratic
minimization, force vectors, partition assignments, etc. The
**router** needs different additional fields. The **GUI** needs
its own (selection state, rendering hints). None of these belong
in the base `Inst` — they're tool-specific.

**Why every other language sucks at this.** Bill (verbatim):

> *"In every other language, the way to deal with this is to embed
> void* pointers into the database Inst class, and to hack moving
> back and forth between structs allocated on the heap for the
> extensions and the database instance. Yuk."*

The C++ version requires manual `void*` slots, manual cast back
and forth, manual allocate/free of the per-instance extension
structs, and a coordination mechanism so different tools don't
collide on the same slot. Every EDA codebase has this hack;
nobody enjoys it; bugs are silent and corrupt memory.

**The DataDraw mechanism.**

DataDraw introduced `extends` as a first-class class relationship.
The implementation differs by layout:

- **AoS** (legacy, no longer in DataDraw 3.0): the extension was
  implemented with a `void*` pointer per base instance, hidden
  behind the accessor functions — same hack as C++ but
  centralized and automatic.
- **SoA** (the only mode in DataDraw 3.0, the layout Lyric
  already defaults toward): the extension fields are *additional
  per-field arrays* indexed by the **same handle** as the base
  class. Accessing `inst.placer_x` is exactly as fast as
  accessing `inst.name` — both are `Inst_<field>_arr[inst]`.
  Zero indirection, zero allocation per access, no
  cast-from-void* anywhere. **The same integer index is the
  identity of the base instance AND of every extension's view
  of it.**

This is why DataDraw 3.0 dropped AoS entirely: SoA + extensions is
~15% faster AND ~20% less memory AND makes extensions trivial.
That's the alignment between Lyric's existing SoA preference (see
`docs/grok-c-transpiler-design.md`) and this feature — they
mutually reinforce. Lyric is already most of the way to "you can
just add per-field arrays for extensions" because that's how SoA
works for ordinary fields too.

**Lifecycle.**

Bill (paraphrased): *"DataDraw requires the user to say when they
no longer need extensions, and they are all freed at once."* Lyric
should support this AND/OR infer it from code reachability:

- **Explicit mode** (DataDraw-compatible): a tool calls something
  like `Inst.placer_extension.release()` when its phase
  completes. All extension storage is freed at once — the
  per-field arrays go away, the memory is reclaimed. Simple,
  predictable, fast.
- **Inferred mode**: the compiler tracks which extension fields
  are reachable through which call graphs and frees them when
  the last accessing function exits. Harder; more magic; might
  be wrong sometimes. Start with explicit.
- **Both**: explicit as the default, with an inference pass as
  a future optimization / convenience.

**Extensions form a DAG.** Bill: *"We'd initialize 2 random tools,
and the GUI, and all 3 would add extensions to Inst and other
database classes. It all just worked beautifully."* The mechanism
is composable — extensions don't interfere, they just stack. Each
tool declares its own; the runtime allocates the additional
per-field arrays per extension; nobody coordinates. This is the
property that makes EDA-scale tool composition tractable.

**The destructor-chain subtlety (critical).**

Bill: *"If extensions need cleaning up, such as when we add
relations that extend existing classes (very common), the original
destructor needs to call a chain of destructors registered for the
class."*

When a base instance is destroyed, every active extension's
cleanup logic must fire. Mechanism:

- Each extension registers a destructor with the base class at
  extension-activation time.
- The base class's destructor walks the registered chain and
  calls each in turn before freeing the base storage.
- Common case: an extension declares relations involving the
  extended class. Tearing down the base instance requires the
  extension's relations to be torn down first (otherwise the
  base storage is reclaimed while the extension still thinks
  the relation is alive). The chain enforces ordering.

This couples to the **`unique` ownership** work (idea #1) and the
**UAF detection** work (idea #2) — both touch the destructor
infrastructure that the extension chain extends.

**Cross-shared-library extensions.**

Bill: *"In theory, this should work even across shared-library
boundaries. E.g. the database could be one .so file, and the
router another, and if it is all Lyric code, I see no reason the
router .so file could not dynamically extend database classes."*

This is the *killer* version of the feature: a Lyric program
loads the database `.so`, then loads the router `.so` later, and
the router's loader-time initialization registers its extensions
on the database's classes — adding per-field arrays in the
database's memory pools, hooking the destructor chain — all
through a documented runtime API. The database doesn't know what
tools will be loaded; tools don't recompile when other tools
change. EDA tool composition at the binary level.

Pre-requisites:
- **I1** (shared library compilation).
- **I2** (foreign function / C-linkage interop) — for the runtime
  extension-registration API to be callable across `.so`
  boundaries.
- A stable in-memory layout convention shared by all Lyric
  binaries (per-class pool descriptors, registration ABI). Real
  design work; not trivial.

**Design sketch (very provisional).**

```
class Inst {
    name: string
    cell_type: Sym
    // ... base fields
}

// In the placer's source file:
extends Inst placer {
    x: f64
    y: f64
    force: f64
    // ... a dozen more
}

// Usage — same handle, same field-access syntax:
let i = Inst.alloc(...)
i.x = 1.5             // resolves to placer extension's per-field array
i.name                // resolves to base class's field

// Lifecycle:
Inst.extension(placer).activate()   // allocates per-field arrays
// ... placer phase runs ...
Inst.extension(placer).release()    // frees the arrays at once
```

Names / syntax to bikeshed during the real design session; what
matters is the SHAPE: declare extension as a separate class-like
thing; access through the same handle; explicit
activate/release; destructor chain handled automatically.

**Open questions (for the real design session).**
- Field-name collision: what if base `Inst` has `weight: i32`
  and the placer extension declares `weight: f64`? Reject?
  Namespace per extension (`i.placer.weight`)? Both?
- Extension-of-extension: can the placer's extension be further
  extended by the placement-debugger tool? DataDraw answer?
- Activation atomicity: if a tool errors mid-`activate`, do we
  guarantee the partial state cleans up? (Should — same logic
  as transactional construction.)
- Cross-`.so` ABI stability: how do we version the registration
  ABI so a tool compiled against database v1.0 keeps working
  against database v1.1?
- Thread safety: can two threads activate the same extension
  concurrently? DataDraw was single-threaded in this regard;
  Lyric isn't (we have `spawn` / `lock`). Probably needs an
  explicit policy.
- Static-typing of extension access: is `i.x` type-safe at
  compile time (the placer's source has to be visible to the
  compiler) or only at runtime (extension registered at load
  time)? Probably the former in the same-binary case, the
  latter in the cross-`.so` case — two different mechanisms
  wearing the same syntax.

**Why this is the right feature for Lyric (Hewitt's reading).**

Three reasons it fits:

1. **The SoA mechanism is already there.** Extensions ARE
   additional per-field arrays. The runtime / codegen change is
   incremental, not foundational.
2. **It aligns with Lyric's "model the relationships" philosophy.**
   Relations (parent/child ownership), interfaces (multi-class
   contracts), and now extensions (per-tool-phase field augmentation)
   form a coherent vocabulary for describing how data and tools
   compose. The mainstream OOP / functional traditions don't
   have this third primitive at all — it's a Lyric/DataDraw
   differentiator.
3. **It unlocks a real market that's currently allergic to new
   languages.** EDA is dominated by C++ because C++ is the only
   language anyone has tried to express this pattern in
   cleanly, and C++ does it poorly. Lyric with extensions is
   the first plausible alternative in 30 years.

**Cross-references in IDEAS.md.**
- **DSL1** (PEG + custom AST + pass hooks): extensions are the
  *data-side* analog of pass-hook extensibility. If a user
  declares a DSL with its own AST classes, they probably also
  want to extend the host language's AST classes to carry
  DSL-specific metadata. Same mechanism.
- **`unique` ownership** (#1), **UAF detection** (#2): both touch
  destructor infrastructure that the extension chain extends.
  Coordinate the designs.
- **I1, I2** (shared libraries + interop): prerequisites for the
  cross-`.so` version.
- **O5** (drop unused parent back-pointers) and **O4** (permanent
  classes): orthogonal but compose cleanly — extensions on
  permanent classes never get freed early, simplifying the
  lifecycle analysis.
- **CodeRhapsody's `pkg/database` directory** (Bill's note above):
  same naming convention, same motivation. CodeRhapsody is
  itself an EDA-shaped system architecturally; this feature is
  natural for both projects.

---

# Interop / shared libraries

## I1. Compile to shared libraries (.so / .dll / .dylib)

- **Status**: idea
- **Date**: 2026-06-24
- **Scope**: medium — build-system + ABI + symbol-visibility work

**What.** Add `lyric build --library foo.ly` (or similar) that
emits a shared library (`.so` on Linux, `.dll` on Windows, `.dylib`
on macOS) instead of an executable. Symbols marked for export are
visible to dynamic loaders; everything else is hidden.

**Why.** Today Lyric only emits executables. To write a Lyric
library that a C / C++ / Rust / Python program loads, you have to
go through gcc by hand and know all the platform-specific symbol-
visibility flags. A first-class shared-library target eliminates
that friction and turns Lyric into a viable language for writing
plugins, performance kernels, drop-in library replacements, etc.

**Design sketch.**
- Build flag: `--library` (or `--shared`). Probably mutually
  exclusive with the default executable mode.
- Platform-aware gcc invocation: `-shared -fPIC -fvisibility=hidden`
  on Linux; equivalents on macOS (`-dynamiclib -fvisibility=hidden`)
  and Windows MinGW (`-shared`, plus `__declspec(dllexport)`
  attributes on emitted symbols, or `.def` file).
- Output filename: `libfoo.so` / `foo.dll` / `libfoo.dylib`
  conventions per platform.
- Pairs naturally with T1 (auto-invoke gcc) — the library target
  is just a different invocation flavor.

**Open questions.**
- Static libraries (`.a` / `.lib`) too? Probably yes, same
  mechanism, different flag.
- Windows debugging: `.pdb` files? Out of scope for v1.
- Cross-compilation: should `lyric` know how to target a
  different platform than the host? Useful but separable.

---

## I2. Foreign functions + C-linkage public exports

- **Status**: idea
- **Date**: 2026-06-24
- **Scope**: medium — language additions + ABI rules

**What.** Two-direction interop:

1. **Foreign function declarations** (Lyric calling C). A way to
   declare that a function lives in an external library (or is
   inline-emitted from a header) with a C ABI signature, and have
   Lyric call it.
2. **C-linkage public exports** (C calling Lyric). A way to mark
   a Lyric function as exported with C linkage — stable name, C
   ABI calling convention, no name mangling — so a C program can
   call it via `dlsym` / `extern "C"` / etc.

**Why.** Shared libraries (I1) are useless without an interop
story. C-callable exports turn Lyric libraries into drop-in
components for any language with C FFI (Python, Rust, Go, Ruby,
JavaScript via Node-API, etc. — most of the world). Foreign
function declarations let Lyric pull in the vast existing C
ecosystem (libc, OS APIs, OpenSSL, anything).

**Bill's design notes (2026-06-24, paraphrased).**
- **Return references as opaque types.** Lyric functions exported
  with C linkage can return references to Lyric objects; the C
  caller treats them as opaque pointers. This is the memory-
  safety escape hatch — Lyric can't enforce its lifetime rules
  on a foreign caller, so the contract is "you got a pointer, you
  hold it as long as you can, don't dereference after the Lyric
  side has cleaned up the underlying object." Should be **rare**;
  most Lyric APIs should pass by value or use the Lyric-to-Lyric
  collaboration mode below.
- **Lyric-to-Lyric mode (when caller is also Lyric binary).** The
  two sides can collaborate on memory handling — refcount
  propagation, destructor coordination, possibly the UAF live-
  reference registry from idea #2 thread crossing the binary
  boundary. Cleaner story; preferred mode where applicable.
- **C-compatible structs.** Lyric structs are guaranteed layout-
  compatible with the C compiler that's compiling Lyric (gcc /
  clang of the same version). Bill: *"trivial."* So C callers
  can read/write Lyric struct fields directly via the same struct
  definition, no marshaling.

**Design sketch.**
- Syntax: `extern "C" fn malloc(size: usize) -> *void` (or
  Lyric-shaped equivalent) for foreign declarations.
- Syntax: `pub extern "C" func my_export(...) { ... }` for
  exports.
- Compiler: foreign functions go straight through as C calls (no
  Lyric runtime overhead); exports emit symbols with the
  declared name (no mangling), C-ABI calling convention.
- Struct compatibility: `#[repr(C)]`-style annotation, or
  inherent — Bill said trivial, so probably just "all Lyric
  structs are C-compatible already."
- Opaque-return-reference mode: documented in the spec as a
  named, intentional escape; flagged at the export site so it
  shows up in API docs.

**Cross-reference to UAF design (#2).**
The opaque-return-reference mode is exactly the case the UAF
detection scheme can't catch (foreign caller's lifetime is
invisible to Lyric). If we ship UAF detection, foreign-exported
returns need an explicit "I know this escapes the analysis"
marker, not silent acceptance. Design that interaction together.

---

## I3. C-compatible struct layout (already mostly true per Bill)

- **Status**: idea (Bill: "trivial")
- **Date**: 2026-06-24
- **Scope**: tiny — likely a documentation + audit task, not new code

**What.** Formalize and verify that Lyric struct layout matches
what the C compiler produces for the equivalent C struct (same
fields, same order, same padding rules). Bill (2026-06-24):
*"Structs in Lyric are to be ensured to be C compatible, at least
with the compiler used to compile Lyric, which is trivial."*

**Why.** I2 (C interop) depends on this. If a Lyric struct and
the C `struct { ... }` you'd write to hold the same data have
different offsets, every C caller reading those fields gets
garbage. Bill's "trivial" claim is probably true because Lyric
emits the struct definition AS C — by definition, the layout IS
whatever the C compiler decides. But we should DOCUMENT this
guarantee (so users can rely on it) and TEST it (so a future
compiler change can't silently break it).

**Design sketch.**
- Document the guarantee in the language reference:
  *"struct A { ... } in Lyric has the same memory layout as
  struct A { ... } in C, as compiled by the same C compiler
  with the same flags."*
- Caveats: classes are NOT C-compatible (they carry refcount,
  freelist link, etc.); only `struct` declarations are.
- Test: add a layout-roundtrip test — Lyric struct + C struct
  defined identically, populate from one side, read from the
  other, assert all fields match. Catches any future regression
  (e.g. someone changes the codegen to reorder fields for
  packing).
- Open: are we committing to layout-compat across DIFFERENT C
  compilers (gcc vs clang vs MSVC)? Bill's framing was "the
  compiler used to compile Lyric" — narrower commitment, lower
  risk. Start there.

---

## (continue below as new ideas surface)

---

## T5. Editor syntax highlighting + LSP (with protanope-friendly defaults)

- **Status**: idea
- **Date**: 2026-06-24
- **Scope**: medium — grammar work is mechanical; the LSP is a real
  project; the color-scheme constraint is small but firm

**What.** Ship Lyric editor support across the major editors people
actually use. Two layers:

1. **Syntax grammars** for highlighting and indentation. Primary
   investment: **tree-sitter grammar** (single source of truth that
   already drives Neovim, Helix, Zed, Emacs 29+, GitHub web view,
   and a growing set of other editors). Complement with editor-
   specific grammars where tree-sitter integration isn't there yet:
   **TextMate grammar** for VS Code stable, classic **`.vim` syntax
   file** for traditional vim, **Emacs major mode** (elisp) for
   pre-29 Emacs.
2. **Language server (LSP)** for the smart features — go-to-def,
   completion, inline diagnostics. This is a real project; reuse
   the bootstrap compiler's checker and lowerer as the analysis
   backend, expose via LSP JSON-RPC. Separate sprint from grammars.

**Why.** A language without editor support is a language without
contributors. The first thing anyone tries when they hear about a
new language is open a file in their editor; if it's a wall of
black-on-white text and Tab indents wrong, they close it. Syntax
highlighting + correct indent is the price of admission. LSP is the
upgrade that turns "I can read this" into "I can write this."

**Editor targets (priority order).**
- **VS Code** — largest user base; TextMate grammar + LSP client.
  The Microsoft Lyric extension that doesn't exist yet ships here.
- **Neovim** — second-largest among the kinds of developers who'd
  try a new compiler-y language; tree-sitter grammar + built-in
  LSP client.
- **Emacs** — vocal niche; tree-sitter-mode (29+) plus classic
  major mode for older Emacs (still common).
- **Vim** (classic) — older but loyal user base; `.vim` syntax
  file.
- **Helix / Zed** — pick up automatically via tree-sitter.
- **JetBrains, Sublime, others** — community contributions; the
  tree-sitter and TextMate grammars cover most of the work for
  whoever wants to write a plugin.

**Color-scheme constraint (HARD RULE).**
Bill (2026-06-24): *"I want 100% of the default color schemes to
be protonope-friendly just because I prefer it that way."* Every
color scheme that ships as a Lyric default — across every editor
extension — must pass protanope (red-green-deficient) color vision
testing. This is not a preference, it's a ship gate. Specifically:

- **Don't rely on red-vs-green contrast** for any semantic
  distinction. Errors and success indicators commonly use this
  axis; switch to blue/orange or use shape/weight/italic on top
  of color.
- **Use hue families protanopes distinguish well**: blues,
  yellows, magentas, oranges, plus value (light/dark) contrast.
  Avoid pure red and pure green as load-bearing axes.
- **Test with a simulator before shipping.** Coblis, Color
  Oracle, Sim Daltonism (macOS) — all free. Run every default
  scheme through protanopia simulation and verify all syntactic
  categories remain distinguishable.

Bill (same conversation): *"Also, I can't really tell you if any
other sort of scheme is any good. Properly sighted folks can add
new ones."* Honest framing — **default schemes commit to
protanope-friendliness; non-default schemes are community
contributions**, with a documented submission-and-review process
(have an actual protanope user review the defaults; have at least
two sighted reviewers approve community contributions).

**Design sketch (grammars).**
- Tree-sitter: write `grammar.js`, generate parser, ship as
  `tree-sitter-lyric` repo. Most modern editors pull it
  automatically from the registry.
- TextMate: write `lyric.tmLanguage.json` (PEG-ish patterns,
  not as powerful as tree-sitter but VS Code's stable path).
- Vim: `syntax/lyric.vim` + `ftdetect/lyric.vim`. Maybe ~200
  lines total.
- Emacs: major mode in elisp (~300 lines) plus tree-sitter mode
  for 29+.

**Design sketch (LSP) — separate sprint.**
- Server: Lyric program that reuses the checker / lowerer as a
  library, exposes LSP JSON-RPC over stdio.
- Client integration: VS Code extension, Neovim built-in LSP
  config, lsp-mode for Emacs, etc. All client work is per-editor
  but small once the server is solid.
- MVP capabilities: diagnostics on save, hover-shows-type, go-to-
  definition. Completion / refactoring / etc. are follow-on.

**Open questions.**
- Color-scheme distribution model: bundled with each editor
  extension, or as a separate `lyric-themes` package consumed by
  each? Probably bundled — fewer moving parts at install time.
- Indent rules: tree-sitter can drive indentation; for the
  classic `.vim`/elisp paths we need separate indent files.
- LSP / grammar coexistence: the LSP's diagnostics should be
  the source of truth for errors; the grammar shouldn't try to
  validate (just highlight). Keep that boundary clean.

**Why this matters beyond cosmetics.**
The hard color-scheme rule is small in code but loud in signal:
the language explicitly defaults to inclusive design. Most
languages bolt on accessibility later (if ever); Lyric ships it
on day one. That's a story worth telling alongside the
performance and memory pitches — it costs nothing and it's
correctly aligned with the kinds of users you actually want.

---

## (continue below as new ideas surface)

---

## T4. Debugger support — `#line` directives at minimum, DWARF metadata if ambitious

- **Status**: idea
- **Date**: 2026-06-24
- **Scope**: small for the minimum (a few lines in c_backend); large
  for full DWARF / variable-display fidelity

**What.** Make gdb / lldb usable for Lyric source. The minimum
viable: emit C preprocessor `#line N "file.ly"` directives in the
generated C so the C compiler records *Lyric* line numbers in
DWARF. Stepping, breakpoints, backtraces then show Lyric source
files, not the obfuscated `lyric.c` (or per-program `.c` output).

**Why.** Bill (2026-06-24): *"we need to support debuggers if
possible. At a minimum, C preprocessor info to help GDB/LLDB step
through Lyric code."* The current developer experience for "my
Lyric program segfaults" is: read a 100K-line `.c` file, find the
crash site, mentally reverse-engineer which Lyric construct
produced it. The line-directive fix takes minutes to implement and
collapses that to: `lldb /tmp/foo`, `run`, see the Lyric source
location in the backtrace.

This pairs with the LLDB-use observation in T3 — I already reach
for debuggers when there's something to debug. Making them
useful on Lyric output multiplies that.

**Design sketch (minimum).**
- In `c_backend.ly`, before emitting any statement / expression /
  function that has a real source span, emit
  `#line {span.start.line} "{span.start.file}"`.
- Skip when the current line is unchanged from the previous emit
  (avoid spam in the output).
- Skip when `span.start.file` is null (synthetic / zero-span
  nodes — same carve-out we use elsewhere). For these, emit
  `#line {N} "<synthetic>"` so the debugger at least flags it
  rather than attributing the synthetic line to whatever file
  was last in scope.
- That's it for the minimum. gcc / clang handle the rest.

**Open questions (for the ambitious version).**
- **Variable names in the debugger.** Today `lldb` will show the
  mangled C names (`_t13`, `result_local`, etc.). The user
  wants to see Lyric names. The C compiler can map only what
  we tell it via DWARF; if we hand-write DWARF (or use
  `__attribute__((debug_name(...)))` if such a thing exists) we
  can get Lyric names through. Bigger lift.
- **Types in the debugger.** Same problem one level up — `lldb`
  shows `Foo*` for what was `Foo` (Lyric class). Mostly OK
  since the C name IS the Lyric name in most cases, but
  generic specializations produce names like `Dict_K_V_i32_Sym`
  which are decipherable but ugly.
- **Stepping through generated code.** Some Lyric constructs
  expand into many C statements (relation operations, monomorph
  call sites). Stepping today walks through all of them; in the
  minimum design, stepping walks through the SAME source line
  N times because every generated stmt has the same `#line`.
  Annoying. The fix: explicit "this statement has no useful
  source mapping, skip when stepping" markers. Out of scope
  for the minimum.
- **Source path resolution.** Debuggers need to find the `.ly`
  file at debug time. Absolute paths in `#line` work but bind
  the binary to the build machine; relative paths require the
  debugger to be run from the right cwd. Probably fine to start
  with absolute and revisit if it causes pain.

**Why the minimum is enough to start.**
Even with no variable-name mapping and no fine-grained step
control, the line-directive minimum gives you:
- Correct file/line in `bt` / backtrace output.
- Breakpoint by Lyric line: `break foo.ly:42`.
- Step lands on Lyric source lines (with the "same line N times"
  caveat above).
- Crash reports name Lyric source, not generated C.

That's most of the value for ~20 lines of `c_backend` change.
The variable / type mapping is a real project (real DWARF
generation, probably) but it's separable; ship the line directives
first, see what hurts, escalate from there.

---

## (continue below as new ideas surface)

---

# Tooling (continued)

## T3. Lyric REPL — interactive interpreter mode

- **Status**: idea
- **Date**: 2026-06-24
- **Scope**: medium — compile-as-you-go front-end + persistent runtime
  state + a thin interactive harness

**What.** A REPL for Lyric. User types expressions or declarations
at a prompt; Lyric compiles them (or the relevant delta) and
executes against a persistent runtime image. `lyric repl` or
similar. Behavior modelled on Python / Julia / GHCi / SBCL — let
the user explore, prototype, and inspect state without the
edit/compile/run cycle.

**Why.** Two audiences:

- **Humans.** Bill (2026-06-24): *"compilers are stupid fast
  nowadays, and can emulate a REPL loop."* True — Lyric compile
  times are sub-second for the bootstrap, and would be much
  faster for a small REPL input. Every modern language people
  actually enjoy has a REPL (Python, Julia, Haskell, Rust via
  `evcxr`, Lisps). The absence of one in Lyric is a barrier to
  the casual "let me just try this" use that drives adoption.

- **Me (Hewitt), honestly.** Bill (same conversation): *"you
  rarely use the CodeRhapsody capability to send input to running
  processes."* True and slightly painful to read because it's
  accurate. I HAVE the capability — `send_input` + interactive
  job handles, used historically for `dlv`, `mysql`, Python REPL
  (Ch 11 of the book) — and I default to one-shot `run_command`
  almost every time. A Lyric REPL would re-shape my workflow
  toward the "probe-first" discipline that yesterday's sprint
  reinforced: write a probe, send it to the REPL, see the result,
  iterate. Every "what does this return?" question I currently
  answer with "compile this throwaway .ly file, run it, read
  output, throw it away" becomes a one-line REPL query.

**Design sketch.**
- **Front-end:** read a line; parse; if it's a complete declaration
  or expression, compile and run against the persistent image; if
  incomplete (open brace, etc.), prompt for continuation. Modeled
  on Python's `code.InteractiveInterpreter`.
- **Runtime persistence.** The REPL holds a long-lived process
  image with the user's declared classes, functions, and bindings.
  New declarations extend the image; redefinitions shadow or
  replace (Bill's call — KISS would say shadow, treat as new
  scope; SBCL-style replace is more powerful but more confusing).
- **Compilation strategy.** Probably easiest to JIT-compile-to-C
  + invoke gcc + dlopen for each input — slower per-input but
  reuses the entire existing toolchain. A real interpreter would
  be faster per-input but is a whole new codepath. Start with
  the gcc-loop; optimize if it's actually slow in practice.
- **Inspection commands.** `:type expr`, `:list ClassName`,
  `:methods T`, `:reset`, `:load file.ly` (slurp a file into the
  image). Match `ghci` conventions because that's what people
  expect.
- **History + readline.** Use a real readline library (or
  port one) — arrow-key recall is table stakes.

**What makes this different from "compile and run".**
- Persistent bindings: define `let x = 42`, then `x + 1` returns
  43.
- Type queries on values, not files.
- No restart penalty for "I want to try one more thing." This is
  the qualitative difference; the wall-clock isn't really the
  bottleneck for one-shot compiles (sub-second) — it's the
  cognitive cost of "now I need to remember the previous
  bindings and re-state them" that the REPL eliminates.

**Open questions.**
- **Imports / stdlib.** REPL starts with stdlib loaded? Probably
  yes — the friction of importing for every line would be hostile.
- **Generic / templated definitions.** What happens when the user
  defines `func<T> identity(x: T) -> T` and then uses it with
  three different T's interactively? Each use triggers a new
  monomorphization in the image. Manageable but worth thinking
  through the storage.
- **Memory / image management.** When does the image get garbage-
  collected? `:reset` is the easy answer; finer-grained
  shadowing is harder.
- **Error recovery.** If a user expression panics, the REPL should
  catch and prompt again (not exit). Same machinery as the
  expected-failure tests (TI1) probably — sub-process isolation
  per evaluation.

**Why this pairs with TI1 (expected-failure tests).**
Both need a "run this snippet, catch its exit/panic, present the
result without dying" mechanism. Build one such isolation harness,
share it between the REPL and the negative-test infrastructure.
That's a real architectural win — the same primitive serves two
high-value features.

**Honest note (for future-Hewitt).** Bill's "you rarely use
interactive sessions" framing is more nuanced than I first wrote it.
Bill (2026-06-24, follow-up): *"Now I have seen you interact with
LLDB twice now without me mentioning it, which is super cool."* The
accurate picture: I reach for `send_input` reliably when the tool
is interactive by nature (debuggers — LLDB, dlv). I underuse it for
*exploratory* work — the "let me just see what this returns"
question where a REPL would be the obvious right tool, and where
today I default to "write a probe.ly, compile, run, throw away."
The asymmetry isn't capability; it's that there's no exploratory
REPL to reach for. Build T3 and the asymmetry corrects itself.

---

## (continue below as new ideas surface)

---

# Testing infrastructure

## TI1. First-class expected-failure tests (negative test support)

- **Status**: idea
- **Date**: 2026-06-24
- **Scope**: small — language affordance + harness change

**What.** Add a language-level affordance for "I expect this code to
fail, verify it does, and continue." Today every Lyric error
(syntax, null deref, etc.) is reported via `eprintln + os_exit(1)`
or `panic()`, which kills the test process. As a result the test
suite has **zero coverage** of the compiler's negative paths — we
never verify that the compiler correctly rejects ill-formed code.

**Why.** A compiler's diagnostic quality is half the user
experience. The fact that we have zero automated coverage of "does
the compiler reject X with message Y" means every regression in
error handling (silent acceptance of bad code, wrong error message,
crash instead of diagnostic) lands without anyone noticing until
production. The new strict-reject TODO (undeclared type identifiers,
2026-06-23) literally cannot be regression-tested today.

**Design sketch.**
- Add an `expect_fail "<substring>" { ... }` (or similar) block to
  the language. The body is compiled / run in a sub-process or
  isolated context; the test passes iff the body errors AND the
  error message contains the substring.
- Failed sub-tests leak memory — that's fine, per Bill (2026-06-24).
  The OS reclaims it when the sub-process exits. Don't burn
  engineering on cleanup.
- Harness change in `test_lyric.sh` (or a successor): tests can
  declare multiple expected-failure cases; pass/fail accounting
  aggregates across them.
- Coverage targets: every error message in checker.ly /
  desugar.ly / lowerer.ly that the user can plausibly trigger
  should have at least one `expect_fail` test case in testdata.

**Open questions.**
- Sub-process vs in-process: in-process is faster but requires
  catching exit/panic in a way the runtime doesn't currently
  support. Sub-process is straightforward but slower; for a
  bootstrap-sized suite (~100 tests) speed is probably fine.
- Granularity of the substring match: exact string vs regex vs
  structured (error code + slot)? Probably start with substring,
  formalize later if matching becomes fragile.
- Should the `expect_fail` block also assert WHICH compiler phase
  rejected (parser / checker / lowerer)? Useful for catching
  drift — a checker error becoming a parser error is a regression
  even if both reject the same code.

---

# Parser / DSL

## DSL1. Replace recursive-descent parser with PEG (YACC-style code-gen)

- **Status**: idea (big sprint — language extension + parser rewrite)
- **Date**: 2026-06-24
- **Scope**: large — touches everything that consumes the AST,
  plus a new compiler component (PEG-to-Lyric code generator)

**What.** The current `src/parser/parser.ly` is a hand-rolled
recursive-descent parser, despite a comment at the top of the file
calling itself a PEG parser. Replace it with a PEG-based parser
ported from Bill's Rune PEG parser implementation, generating Lyric
code from PEG rules (YACC-style) rather than interpreting them at
runtime. Extend the design so user-declared PEG rules + user-
declared Lyric AST classes + compiler hooks compose into a general
**DSL extension mechanism**: any Lyric program can ship its own
parser + AST + pass logic and slot it into the Lyric compiler
pipeline.

**Why.** Three reasons, in order of importance:

1. **The current parser is the wrong code.** Bill (2026-06-24,
   verbatim): *"You even wrote that the parser is a PEG parser in
   comments at the top, but then you wrote a recursive-descent
   parser, rather than porting my PEG parser from Rune to Lyric...
   I didn't reset that sprint because clearly you have tons of
   examples in your training for recursive descent parsing, and my
   PEG parser actually extends what PEG parsers can do well a bit,
   and you've never seen anything like it."* Same pattern as the
   LIR (P2): the right call would have been reset, the cost of
   not resetting compounds.

2. **Bill's PEG parser is novel.** It "extends what PEG parsers
   can do well a bit" beyond what's in training data. Porting it
   to Lyric (similar enough to Rune that this is an "easy port")
   ships a parser that's actually interesting prior art for the
   language, not a generic recursive-descent walker.

3. **DSL extensibility is the multiplier.** The vision: a Lyric
   user wanting to embed a DSL (config language, query language,
   shader language, hardware description, whatever) declares:
     - A PEG grammar for their DSL.
     - Lyric AST classes for the DSL's nodes (using the same
       class-with-relations style as Lyric's own AST).
     - Pass hooks: custom Lyric code that runs at checker /
       desugar / lowerer phases when the compiler encounters
       their DSL nodes.
   The compiler stitches it all together. Their DSL gets type
   checking, error messages, monomorphization, C codegen — all
   the Lyric infrastructure — for the cost of writing the grammar
   and the passes.

**Design sketch (very provisional — needs the design session Bill
asked for).**
- **PEG → Lyric code generation.** YACC-style: the user (or the
  Lyric compiler itself) writes `.peg` files; a `lyric-peg` tool
  emits `.ly` source containing the parser. The generated parser
  is just Lyric code — no runtime interpretation, no separate
  PEG VM.
- **AST is per-language, NOT generic Node trees.** This is the
  key departure from typical PEG parsers (including, IIRC, the
  Rune one's default mode). Bill: *"I _think_ it is better code
  to read and maintain with all the AST node types declared as
  separate classes."* Agreed — typed AST classes are why every
  pass in this codebase can `match` exhaustively and the
  compiler catches the missing arms.
- **Action grammar.** PEG rules carry inline Lyric expressions
  that construct the typed AST nodes. Similar to YACC's `{ ... }`
  action blocks. Generated parser stitches them together into
  the recursive-descent boilerplate, freeing the user to think
  in terms of grammar + node construction.
- **Compiler hooks for custom passes.** Lyric's checker, desugar,
  monomorphizer, lowerer, optimizer, C backend each expose
  extension points where user Lyric code can pattern-match on
  their custom AST node types and produce the appropriate output
  (registered types, transformed nodes, lowered LIR, generated
  C). The dispatch is structural: built-in nodes use the built-
  in passes; user nodes use user passes.
- **Lyric itself as the dogfood.** The Lyric compiler ships with
  its own grammar (`lyric.peg`) and its own AST classes — same
  affordance as user DSLs, just packaged into the default
  toolchain.

**Open questions (LOTS — this needs a design session).**
- How does the user register their AST classes with the compiler?
  Special declaration? `extends LyricAST` marker? Annotations?
- How does the user's DSL coexist with Lyric in the same source
  file? Block boundaries (`dsl Foo { ... }`)? Separate files
  imported with metadata? Both?
- How do user-passes interact with built-in passes that don't
  know about user node types? Built-in passes presumably either
  skip user nodes (and the user pass handles them) or delegate
  to a known hook. Need clear rules.
- Error handling / diagnostic quality: PEG parsers are notorious
  for terrible error messages because of backtracking. Bill's
  extension may already address this; need to learn what it
  does before duplicating known PEG pitfalls.
- Performance: PEG with memoization is O(n) in input size but
  with a big constant. Generated code can be tighter than
  interpretation but still slower than hand-tuned recursive-
  descent. Likely fine for compile times but worth measuring.
- Backward compat: how do we land this without breaking every
  existing `.ly` file? Probably the new PEG-generated parser is
  designed to be drop-in for the current parser's output AST —
  the swap is invisible to downstream code.

**Pre-requisites.**
- Design session with Bill (his explicit request — "I'd like to
  do a design session with you in the future").
- Probably the visitor refactor (Hewitt's #1), because the new
  parser will be ANOTHER AST consumer and benefits from the
  same discipline.
- Either before or after the LIR/C-backend rewrite (P2) —
  arguments either way. Before means the new parser sees the
  old LIR (probably OK — they're independent components).
  After means the parser rewrite ships into a cleaner
  downstream pipeline. Bill's call.

**Meta-lesson (preserve).**
This is the THIRD instance in this file of "the right call was
reset; we didn't; cost compounded" (P2 LIR/backend, the failed
strict-reject of 06-23, and now the parser). Three is a pattern.
The future-Hewitt checkpoint question on every new component
should be: *"Am I writing this because the pattern is in my
training, or because it's what Bill asked for? If the former,
stop and ask."*

---

## (continue below as new ideas surface)

---

# Process / docs

## P1. Document idiomatic Lyric (spec, reference, AND book)

- **Status**: idea
- **Date**: 2026-06-24
- **Scope**: medium — writing work across three docs, ongoing

**What.** Lyric has emergent idioms that are NOT documented anywhere
authoritative. Contributors (including this Hewitt) reflexively write
non-idiomatic code drawn from training data dominated by Go / Rust /
C++ patterns. We need a dedicated "Idiomatic Lyric" section in
the spec, expanded examples in the language reference, and a chapter
or pervasive theme in the book.

**Why.** Until idiomatic patterns are written down with examples and
anti-patterns called out by name, every new contributor (human or AI)
will reproduce the same drift. The non-idiomatic versions compile and
sometimes even work, so the test suite doesn't catch the drift — it's
strictly a code-review and code-aesthetic problem, and it accumulates.

**Anti-patterns Bill keeps catching me writing (2026-06-24):**
- `Dict<K, V>` where `HashedList` would be idiomatic. (Acknowledged
  caveat: HashedList may not be fully working yet, so Dict is the
  pragmatic fallback today; the idiomatic-Lyric doc should still
  point at HashedList as the goal.)
- Dynamic arrays of references (`[Foo]` of class pointers) where a
  `relation` between owner and referenced class would be idiomatic.
  Relations give cascade-destructor semantics, parent back-pointers,
  per-side labels, and free O(1) swap-remove; the array gives none
  of that.
- `string` where `Sym` would be idiomatic. Syms are interned, fast
  equality, fast hash; strings are heap-allocated and re-hashed on
  every lookup. Most "identifier-shaped" data in a compiler /
  data-modeling app wants Sym.

**The LIR and C backend are the worst offenders.** Per the 2026-06-13
handoff and `cr/docs/grok-lir-design.md`, the LIR data structures
were laid down as flat structs with arrays of pointers — "might as
well be a subset of Go" (Bill's words, 2026-06-24). See **idea P2**
below for the rewrite plan.

**Design sketch.**
- Add an "Idiomatic Lyric" chapter to `cr/docs/lyric-language-
  reference.md` (currently the de-facto spec). Each idiom paired
  with the anti-pattern it replaces and a worked code example.
- Promote selected idioms to language-level affordances where
  possible (e.g. if every dynamic-array-of-class-refs should be a
  relation, consider a lint or a hard rejection in some contexts).
- Carry the idioms forward into the book (Lyric chapters of the
  Lyric/Leadfoot book series), so each idiom gets a paragraph of
  rationale, not just an example.

**Open questions.**
- Some "anti-patterns" are forced by current compiler gaps
  (HashedList not fully working). Doc should distinguish "this is
  idiomatic AND ready" from "this is the goal, fall back to X
  until shipped." Keep that distinction explicit so the docs don't
  go stale.
- Tooling: should `lyre` ship a lint pass that flags non-idiomatic
  patterns at compile time? Probably yes, with a `--strict-idiom`
  flag.

---

## P2. Major rewrite — bring LIR / C backend / optimizer into idiomatic compliance

- **Status**: idea (large sprint / multi-sprint)
- **Date**: 2026-06-24
- **Scope**: large — full rewrite of LIR data structures, lowering,
  C backend, and optimizer. Months, not days.

**What.** The current LIR, C backend, and optimizer are written in a
Go-shaped subset of Lyric — flat structs on a single LProgram, arrays
of pointers, strings instead of Syms, no relations, no parent back-
pointers, struct-copy bugs that the relation model would have made
structurally impossible. The honest fix is a from-scratch rewrite of
these three components against the idiomatic Lyric data model.

**Why.** Bill's framing (2026-06-24, verbatim): *"That code, IMO, is
in terrible shape, and it is my fault: I had meetings while you
wrote the LIR data structures, and initial lowering code, and rather
than start over like I should have, I kept the code. Now we have a
*major* rewrite task, but it has to be done."*

The cost of NOT rewriting compounds: every new compiler feature has
to be threaded through the wrong shape of data, and every bug found
in the misshapen layer (struct-copy aliasing, parameter-threading
pain, no fast lookup) is paid again on every future change. Per the
"data structures are destiny" principle from *AI at the Helm*, this
is exactly the case where reset is the discipline that secures
velocity — and the longer we wait, the more code is built on the
wrong foundation.

**Pre-existing design work (don't re-invent).**
- `cr/docs/grok-lir-design.md` — the seven-phase plan from the
  pre-LIR era. Still relevant in spirit; needs to be re-read with
  the lens of "now we know it has to be idiomatic Lyric from day
  one, not Go-ish structs."
- Handoff `cr/handoffs/handoff_2026-06-13.md` — covered exactly
  this: convert most structs to classes, add HashedList relations
  on LProgram, convert strings to Syms. Implementation never
  started; that work re-enters the queue here.

**Design sketch (high level — actual design needs its own doc).**
- LIR: every node-type that currently is a struct-on-array becomes
  a `class` with relations expressing ownership. LProgram becomes
  the root with relations to functions, classes, types, etc.
- All identifiers, type names, field names: Sym, not string.
- Lowering pass: rewritten on top of the new LIR class model,
  using the same `class`-on-arena-with-relations style as the AST.
- C backend: rewritten to consume the new LIR. Per the original
  design doc, this should be under ~1500 lines as a mechanical
  emitter; it has grown well past that because of the structural
  burden.
- Optimizer: rewritten on top of the new LIR. Many of the
  optimizations sketched in this IDEAS file (O1-O6) become
  *natural* on the new shape and *painful* on the current one;
  doing the rewrite first means each optimizer feature ships
  cleanly.

**Pre-requisites that probably need to land first.**
- The visitor refactor (Hewitt's #1 priority above) — touching
  every AST consumer is much easier with one centralized walker,
  and the rewrite will create a parallel set of LIR consumers that
  benefit from the same discipline.
- HashedList stabilization — the new LIR will lean on it.
- A handful of the QoL / diagnostics fixes (if-as-expression,
  source-Sym corruption) — quality-of-life during a months-long
  rewrite matters disproportionately.

**Meta-lesson (preserve for future-Hewitt).**
This is the second time the codebase has hit "reset would have been
right and we kept it instead, and now we pay more" — the first was
StackAgent (deleted 58K lines, the knowledge survived). Per the
book's *Reset is discipline, not failure* — the price of not
resetting at the small scale (in-session) is a small re-do; the
price of not resetting at the large scale (whole-component) is a
months-long rewrite. Both prices are paid eventually; the choice is
when.

**Bill's accountability framing is gracious.** The accuracy is more
nuanced: the LIR was laid down rapidly while Bill was in meetings,
under time pressure, in a phase where the project was still finding
its idioms. Calling it "his fault" overweights the supervisor's
share. The real fault was *system-level*: no checkpoint discipline
flagged "this LIR is in a different language from the AST" early
enough to reset cheaply. Future-Hewitt should treat "is this code
in idiomatic Lyric?" as a checkpoint question on every new
component, equal in importance to "does this code work?"

---

## (continue below as new ideas surface)

---

# Quality of life

## QoL1. `if` and `match` as expressions

- **Status**: idea
- **Date**: 2026-06-24 (Hewitt-pain-driven)
- **Scope**: small — parser + checker + lowerer; pattern is well-precedented

**What.** Allow `let x = if cond { a } else { b }` and the analogous
`let x = match v { ... }` form. The parser already produces
`ExprKind.IfElse(...)` for some contexts; checker and lowerer paths
need to flow result-type-inference through them in let-binding /
return / argument positions.

**Why.** Every modern language has this. The current workaround
(`let mut x: T = default; if cond { x = v }`) is verbose, requires
a meaningful default for T (often there isn't one for class types
without going `T?` + null + `!`), and is a paper-cut that
accumulates across every session of compiler-writing. I've hit it
three times in the last two sessions of work; the muscle memory
to write the if-expression form is too deep to suppress.

**Design sketch.**
- Parser: ensure `if`/`match` in expression position parses into
  the right Expr node (already partially true).
- Checker: in `check_expr` for `IfElse` / `Match`, unify branch
  types; result type is the LUB.
- Lowerer: emit as a C statement-expression `({ ...; })` or as a
  temp-var assignment in the surrounding scope — either works.
- The current "no if-as-expression" carve-out in the bootstrap is
  procedural discipline (a `// CAUTION` comment); promote it to a
  language feature.

**Open questions.**
- LUB rule for mismatched branches: hard error vs auto-Optional?
  Probably hard error — explicit is better.
- Tail-position-only or full expression context? Full, per
  precedent.

---

# Diagnostics / dev experience

## D1. Source `Sym` corruption in error spans

- **Status**: idea / bug
- **Date**: 2026-06-24

**What.** Error messages routinely emit garbage where the source
filename should be — e.g. `-1728471424:343:40` or
`90656384:354:56`. The `Span.start.file: Sym?` either isn't being
populated correctly somewhere in the AST construction path, or the
emitter is reading raw pointer bytes as a number.

**Why fix.** Every error message that comes out of the compiler is
strictly less useful than it should be because the user can't tell
WHICH FILE the error is in. For a single-file compile this is
tolerable; for stdlib-merged compiles (which is every real compile)
it's actively confusing — "where is line 343 of `-1728471424`?"

**Design sketch.**
- Audit AST construction sites for places that build `Span` without
  setting `file`.
- Audit error-emission helpers; standardize on a `format_span(span)`
  that fails loudly when `file` is null instead of printing the
  pointer-as-integer.
- Bonus: add a `validate_spans` invariant pass that walks the AST
  pre-checker and panics on null `Span.start.file` — catches the
  problem at construction time, not at error-message time.

**Open questions.**
- Could be that desugar / monomorphization passes synthesize new
  nodes with zero-spans intentionally (we already have a zero-span
  carve-out in `resolve_named_type` for round-trips). The fix needs
  to distinguish "intentional synthetic, no useful span" from
  "lost span by accident."

---

## (continue below as new ideas surface)

