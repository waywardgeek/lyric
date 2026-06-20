# Multi-Class Interface Redesign

*Status: design proposal. Not yet adopted into the spec.*

*Last updated: 2026-06-21.*

*Companion artifacts: `testdata/graph.ly`, `testdata/tree.ly` — worked-out
examples in the proposed syntax.*

*Required background: `cr/docs/lyric-language-spec.md` §1037 (Interfaces
and Multi-Class Contracts) and §1166 (Relations).*

---

## 0. TL;DR

Lyric's multi-class interfaces and relation system have a foundational
bug: relation labels are textual name prefixes applied below the type
system, but the language wants them to be load-bearing for method
dispatch. Two relations on the same parent type to the same child type
(different labels) cannot be expressed soundly today; the symptoms
include the `_method_aliases` global Dict RC bug (TODO.md), a series of
patches to `registerImplMethods` and `DesugarDestructors`, and a
working-but-fragile per-specialization rewrite in `MonoPass`.

This document proposes a redesign that:

1. Promotes relation labels to **real scopes** in their own class's
   namespace, eliminating the textual-prefix scheme and the bug class
   it enables.
2. Generalizes interface dispatch to use small **capability interfaces**
   composed via **constraint aliases**, with algorithms as free
   functions and **aggressive auto-derive** of interface satisfaction
   from fields and relation accessors.
3. Unifies fields and zero-arg methods at the call site via **UFCS**,
   making `obj.name` and `obj.name()` equivalent and `obj.name = v` the
   universal setter form.
4. **Fully monomorphizes** relation-derived and concrete-impl methods
   at desugar time, so the checker never resolves a generic interface
   call (eliminating the `void*` bug class).
5. Renames `OwningList`/`RefList` → `DoublyLinked` with `owns`/`refs`
   as orthogonal modifiers.

The user-facing relation syntax (`relation Hint P:plabel owns [C:clabel]`)
is unchanged. The migration is mechanical: `obj.label_field` becomes
`obj.label.field`; `OwningList`/`RefList` becomes `DoublyLinked`.

---

## 1. The Spec Bug

### 1.1 The shape of the ambiguity

The spec describes a `relation` declaration with the syntax:

```
relation [Hint] Parent[<Args>][:plabel] (owns|refs) (Child[<Args>][:clabel] | [Child[<Args>][:clabel]])
```

(spec §1170). The hint is a multi-class interface with **two type
parameters in (parent, child) order**. The relation's identity, however,
is a **four-tuple `(P, C, plabel, clabel)`** — the labels are required
to distinguish multiple relations between the same `(P, C)` pair.

This is the root contradiction: **the hint's vocabulary is binary;
the relation's identity is quaternary; the labels live below the
type system and cannot be named from inside the hint's method
bodies.**

The spec's example (§1037) makes the problem visible:

```lyric
interface ArrayList<P, C> {
    field P.children: [C]

    pub func add(self, child: C) {
        append(self.children, child)        // self.children — abstract name
    }
}

relation ArrayList Panel:w owns [Widget:p]
relation ArrayList Panel:b owns [Button:p]
```

The hint method `add` references `self.children` — an abstract field
name. The desugar pass textually prefixes injected names with the
parent label, producing `Panel.w_children` and `Panel.b_children` as
two separate fields on Panel. But the hint's `add` body still says
`self.children` — which has no concrete referent after desugaring.
The desugar pass works around this by producing two copies of `add`
(one prefixed with each label) and substituting the prefixed name into
each copy. This works as long as no two relations on Panel use the
same hint with the same child type — but it fails when they do.

Specifically, **two `ArrayList` relations on the same parent class
pointing to the same child class** cannot be disambiguated by any
type-system mechanism. The free-function form `array_append<Panel,
Widget>(p, w)` is one symbol; both relations would expand to the same
specialization with different field names; resolution requires a
per-relation rename map keyed on `(class, label) → abstract_name`, but
no such structure exists in the type system. The current implementation
uses a global `_method_aliases: Dict<Sym, string>` to track these
renames; the same TODO.md entry documents the RC bug where this Dict
gets freed and its memory reused by an unrelated structure.

### 1.2 Three cascading symptoms

1. **`Field vs Method Access for Relation Accessors` (spec §1216)** is
   a hand-waved special case. The spec promises `team.roster_children`
   (field) and `team.roster_children()` (method) are equivalent — but
   only for the auto-injected accessor form. Hint method bodies
   referencing `self.children` neither work as field access nor as
   method call without the textual-rewrite scheme.

2. **`Any binary interface can serve as a hint` (spec §1170)** is too
   generous. The spec's own §1037 example declares `interface Graph<G,
   N, E>` (three type parameters); the `relation` grammar permits
   exactly one parent and one child. Three-class interfaces cannot be
   relation hints, but the spec doesn't say so.

3. **Free-function form `array_append<P, C>(p, c)` (spec §1190)** is
   keyed only on `(P, C)`. When a parent has two same-hint same-child
   relations, the free-function form is one symbol with two valid
   expansions — and nothing in the type system breaks the tie.

The deeper diagnosis: Lyric is trying to express *trait-with-
associated-state-and-labels*, parameterized by `(parent_type,
child_type, parent_label, child_label)`. Today's hint contract drops
the labels from the type signature and recovers them via textual
substitution after the fact.

### 1.3 The chosen direction

Promote labels to **real scopes** in each class's namespace. A relation
`Parent:plabel owns [Child:clabel]` defines a scope `parent.plabel` on
Parent and a scope `child.clabel` on Child. Inside each scope, the
hint's members (fields and methods) live without prefixing — the scope
*is* the prefix. Two relations on the same parent to the same child
become two distinct scopes (`panel.w` and `panel.b`); their members
never collide because they live in different scope namespaces.

The hint's method bodies are then unambiguous: `self.children` inside
`ArrayList.add` refers to the field at the *same scope* — when the
desugar pass instantiates `add` for the `Panel:w` relation, it lives
in the `:w` scope and `self.children` resolves to `panel.w.children`.

---

## 2. Architecture: Four Layers

The redesign separates four concerns that today's spec conflates:

```
┌─────────────────────────────────────────────────────────────────┐
│ STORAGE  — relations declare data structure.                    │
│            Three built-in hints (ArrayList, DoublyLinked,       │
│            HashedList) × {owns, refs} modifier.                 │
│            Labels-as-scopes; fields and methods injected into   │
│            per-relation scopes on parent and child classes.     │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ CAPABILITY  — small multi-class interfaces describe what an     │
│               algorithm needs from a data structure.            │
│               `ManyToOne<P, C>` is the universal relation-      │
│               shape; specialized capabilities (EdgeWeight,      │
│               Numeric, HasRoot) cover what's not relational.    │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ ALGORITHM  — free functions with where-clauses.                 │
│              Constraint aliases bundle capabilities into named  │
│              categories (DirectedGraph, Tree, WeightedGraph).   │
│              UFCS makes call sites read as method calls.        │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ BRIDGE  — auto-derive of interface satisfaction from fields,    │
│           methods, and relation accessors when names align.     │
│           Explicit `impl` blocks when they don't.               │
└─────────────────────────────────────────────────────────────────┘
```

Two architectural principles cut across all four layers:

**Principle 1: Desugar monomorphizes everything with concrete
bindings.** Relations and concrete impl blocks are textually
substituted at desugar time, producing concrete methods on concrete
classes. The checker sees plain typed code; it never resolves a
generic interface method call. (This eliminates the `void*` bug class
documented in the 2026-06-08 Forge handoff.)

**Principle 2: Per-use-site specialization for residual generics.**
Parametric impls (`impl<T> Iterable<Vec<T>, T>`) and algorithm-function
bodies are monomorphized at each call site by substituting the
caller-bound types into the generic body. This is again pure text
substitution, just deferred to where the types are known.

---

## 3. Storage Layer Rules

### 3.1 Labels are scopes

A relation declaration

```lyric
relation Hint Parent:plabel (owns|refs) [Child:clabel]
```

creates two scopes:

- `parent.plabel` on Parent — the parent-side view of the relation.
  Contains the hint's parent-side fields and methods (typically
  iteration accessor, count, append/remove).
- `child.clabel` on Child — the child-side view of the relation.
  Contains the hint's child-side fields and methods (typically the
  back-pointer to parent, plus link-pointers for doubly-linked).

Inside each scope, member names are **not prefixed** — the scope name
is the prefix. So a relation `Team:roster owns [Player:team]` produces:

| Access form (new)              | Access form (today's spec)        |
|--------------------------------|-----------------------------------|
| `team.roster.children`         | `team.roster_children`            |
| `team.roster.count`            | `team.roster_count`               |
| `team.roster.append(p)`        | `array_append<Team, Player>(t, p)`|
| `player.team.parent`           | `player.team_parent`              |
| `player.team.index`            | `player.team_index`               |

Scopes are first-class enough to be:

- **Iterable**: `for x in team.roster` iterates the children. (Iteration
  sugar is on TODO; explicit form `for x in team.roster.iter()` works
  today after migration.)
- **Subject of method calls**: `team.roster.append(player)`.
- **Subject of `len`**: `len(team.roster)` returns count.

### 3.2 Cross-class relations: same-letter labels are legal

When parent and child are different classes, their scope namespaces are
independent. Same-letter labels on both sides are allowed:

```lyric
relation DoublyLinked Route:a refs [Via:a]
relation DoublyLinked Route:b refs [Via:b]
```

Produces `route.a`, `route.b` scopes on Route and `via.a`, `via.b`
scopes on Via. No collision: `route.a` lives in Route's namespace,
`via.a` lives in Via's. They never share a symbol table entry.

### 3.3 Self-relations: distinct labels mandatory

When parent and child are the same class (a self-relation), both
scopes live in one namespace, and the two labels must be distinct:

```lyric
// LEGAL — distinct labels:
relation DoublyLinked Folder:children owns [Folder:parent]

// ILLEGAL — collision in Folder's namespace:
relation DoublyLinked Folder:a owns [Folder:a]
```

The natural pair for trees is `children` / `parent` — role names that
read as the direction-of-view from each side.

### 3.4 Auto-projection of singleton child-side scopes

A child-side scope of a many-to-one relation contains a back-pointer
field `parent: P?` plus link-pointers (for doubly-linked) or an index
(for ArrayList). When the scope name itself is referenced in a context
expecting the parent type, it **auto-projects** to the `parent` field:

```lyric
let p: Folder? = node.parent              // auto-projects to node.parent.parent
let n: Route? = via.a                     // auto-projects to via.a.parent (FPGA case)
```

The full scope handle is still accessible — `node.parent.next`,
`node.parent.prev` for explicit link-pointer access. Auto-projection
only fires when the use context demands the parent type.

Auto-projection is what makes `is_root(n) = isnull(n.parent)` read
naturally. Without it, the body would have to say `isnull(n.parent.parent)`,
which reads as a double indirection and obscures intent.

### 3.5 `DoublyLinked` with `owns`/`refs` modifier

The four built-in hints today are `ArrayList`, `OwningList`, `RefList`,
`HashedList`. `OwningList` and `RefList` are both doubly-linked; they
differ only in destruction semantics (cascade vs. unlink). The `owns`/
`refs` keyword in the relation declaration is required but largely
redundant with the hint name.

Collapse to three hint names with the `owns`/`refs` modifier as the
sole determiner of destruction:

| Today                   | Proposed                |
|-------------------------|-------------------------|
| `ArrayList ... owns`    | `ArrayList ... owns`    |
| `OwningList ... owns`   | `DoublyLinked ... owns` |
| `RefList ... refs`      | `DoublyLinked ... refs` |
| `HashedList ... owns`   | `HashedList ... owns`   |

Three hint names, two modifiers. `owns`/`refs` becomes orthogonal:
every hint can be either, with the modifier choosing destruction
behavior (cascade vs. unlink). The `_method_aliases` global Dict and
the per-relation rename map become unnecessary, because methods are
addressed by their scope path, not by a prefixed name.

Migration is a one-line sed:
`s/\bOwningList\b/DoublyLinked/g; s/\bRefList\b/DoublyLinked/g`
applied to source code. The `owns`/`refs` keyword stays as-is.

### 3.6 Hint interfaces declare members in `P.method` / `C.method` form

Today's hint interfaces declare members in a flat namespace, relying on
the desugar pass to apply textual label prefixes. Under the new design,
hint interface declarations explicitly assign each member to either the
parent-side scope (`P.method`) or the child-side scope (`C.method`):

```lyric
interface ArrayList<P, C> {
    // Parent-side scope members:
    field P.children: [C]
    pub func P.append(self, child: C) {
        append(self.children, child)
    }
    pub func P.remove(self, child: C) { ... }
    pub func P.iter(self) -> gen C { ... }
    pub func P.count(self) -> i32 { return len(self.children) }

    // Child-side scope members:
    field C.parent: P?
    field C.index:  i32
}
```

When the desugar pass instantiates this for `relation ArrayList
Foo:bar owns [Baz:qux]`, it substitutes P=Foo and C=Baz throughout,
placing parent-side members into `Foo.bar` scope and child-side
members into `Baz.qux` scope.

The body of `append` references `self.children` — under substitution,
this refers to the **same scope's** `children` field, which becomes
`foo.bar.children`. Within a single relation's instantiation, all
abstract names resolve unambiguously to the same scope.

### 3.7 Per-class field injection on type variables

When an interface declares a field on a type variable that is *not*
participating in a relation declaration in its own body, the field is
injected on the bound concrete class **once**, regardless of how many
relations involve that type variable.

```lyric
interface EdgeWeight<E, W: Numeric> {
    field E.weight: W = W.zero()        // per-class field injection
    pub func E.weight(self) -> W { return self.weight }
}
```

A concrete class `Via` that satisfies `EdgeWeight<Via, f64>` gains a
single `weight: f64` field, not one field per relation Via participates
in. This eliminates the "duplicated weight under different prefixes"
bug shape that would arise from naively applying per-relation prefixing
to interface fields.

The distinction from hint interfaces: hint interface fields are
declared on type variables that the hint expects to be relation
participants, and they land in the per-relation scope. Non-hint
interface fields land directly on the concrete class. The compiler
distinguishes the two cases by whether the interface is being used as
a relation hint (auto-detected from any `relation X ...` declaration
referencing the interface) or as a capability interface (no relation
declarations reference it).

If the spec wants to be explicit, the keyword `hint interface` could
mark hint-interface declarations. The implicit detection works for the
existing three hints and is simpler.

---

## 4. Method System Rules

### 4.1 Methods are class-scoped functions

A method declaration has the form:

```lyric
[pub] func ClassName.method_name([self,] args) -> ReturnType { ... }
```

Either at module top level, inside an `impl` block, or inside a class
body (where the `ClassName.` is implicit because the receiver is
unambiguous from the enclosing class).

A class scope is a namespace. Methods of `Widget` live in `Widget.*`.
Static methods (no `self` parameter) live in the same scope and are
called via the long form `Widget.factory(args)`. Instance methods
(with `self`) are also called via the long form `Widget.method(widget,
args)` but typically via UFCS shorthand (§4.3 below).

### 4.2 Field/method unification

A field declaration `name: T` on a class auto-defines two methods in
the class scope:

- A getter: `func Class.name(self) -> T` that returns the field.
- A setter callable via assignment: `obj.name = v` (the assignment
  form desugars to a setter method call).

A user-declared method `func Class.name(self) -> T` IS the getter; no
separate field is implied. If the user declares both a field `name: T`
and a method `func Class.name(self) -> T`, it is a **compile error**
(naming collision). The user must rename one — fields use one name,
computed properties use another, never the same name for both.

This eliminates today's spec §1216 special case ("field == zero-arg
method, for relation accessors only") by generalizing it to the
universal rule: fields and zero-arg methods are interchangeable at the
call site for any class scope, not just for relation-injected accessors.

### 4.3 UFCS at call sites

For instance methods (with `self`), the call site forms

```lyric
widget.to_string()            // method-syntax form
widget.to_string              // method-syntax form, zero-arg sugar
Widget.to_string(widget)      // function-syntax form
```

are equivalent. UFCS lets the user pick whichever reads better at the
call site.

For static methods (no `self`), only the long form is meaningful:

```lyric
Widget.default_name()         // no receiver to use UFCS on
```

### 4.4 Receiver explicitness depends on context

| Where declared                              | `ClassName.` prefix? |
|---------------------------------------------|----------------------|
| Inside `class Widget { ... }` body          | Implicit             |
| Inside `impl Iface<Widget> { ... }` single-class | Implicit         |
| Inside `impl MultiClassIface<...> { ... }`  | Use interface type-var name |
| Free-standing at module top level           | Required             |

For multi-class impl blocks, the method is declared with the interface's
type-variable name in the receiver position:

```lyric
impl Graph<Net, Route, Via> {
    pub func G.nodes(self) -> [Route] { return self.routes }
    pub func N.outgoing(self) -> gen Via { for v in self.a { yield v } }
}
```

This makes the abstract-to-concrete mapping visible at every method
declaration: reading "`G.nodes` is implemented as `self.routes`" tells
the reader which interface slot this method satisfies.

### 4.5 Type arguments on method calls are a parse error

Method calls do not take explicit type arguments:

```lyric
net.bfs(alice)                // OK
net.bfs<Net, Route, Via>(alice)   // PARSE ERROR
```

The method's type parameters are always determined by the receiver
type, the argument types, and the where-clause constraints. Explicit
type arguments at the call site are redundant and a footgun (user
writes the wrong ones, behavior is unexpected). The error message
guides the user to the free-function form if they need explicit type
arguments:

> error: method calls don't take explicit type parameters; the
> receiver determines the binding. If you need to pin a non-receiver
> type parameter, use the free-function form: `bfs(net, alice)`.

Free-function calls retain explicit type arguments for the legitimate
case where a type parameter appears only in the return type or where-
clause constraints (e.g., `make_vec<i32>()`, `parse<f64>("3.14")`).

### 4.6 No free-function form of relation-derived methods

Today's spec offers both `team.array_append(player)` and
`array_append<Team, Player>(t, p)` (§1190). The free-function form is
a holdover from the textual-prefix scheme and provides no expressive
power that UFCS doesn't. It is removed in this redesign.

After migration: `team.roster.append(player)` is the only call form;
the free-function form is no longer accepted.

---

## 5. Interface System Rules

### 5.1 Capability interfaces

A capability interface is a small, single-purpose multi-class interface
that declares a minimal contract. The universal capability for
relation-shaped storage is:

```lyric
interface ManyToOne<P, C> {
    pub func P.iter(self) -> gen C
    pub func P.count(self) -> i32
    pub func C.parent(self) -> P?
}
```

Every relation hint (`ArrayList`, `DoublyLinked`, `HashedList`)
automatically satisfies `ManyToOne<P, C>` because each provides
iter+count on the parent side and a parent back-pointer on the child
side.

Specialized capabilities cover what `ManyToOne` doesn't:

```lyric
interface Numeric<T> {
    pub func T.zero() -> T              // static factory
    pub func T.one() -> T               // static factory
    pub func T.add(self, other: T) -> T
    pub func T.mul(self, other: T) -> T
}

interface EdgeWeight<E, W: Numeric> {
    pub func E.weight(self) -> W
}

interface HasRoot<T, N> {
    pub func T.root(self) -> N?
}
```

Capability interfaces declare what algorithms need, not what data
structures provide. They are intentionally minimal — composition
happens at the constraint-alias layer (§5.2), not by inflating
individual interfaces.

### 5.2 Constraint aliases

A constraint alias is a named bundle of constraints with no methods of
its own:

```lyric
constraint DirectedGraph<G, N, E> =
    ManyToOne<G:nodes, N:graph> +
    ManyToOne<N:out,   E:source> +
    ManyToOne<N:in,    E:target>

constraint WeightedDirectedGraph<G, N, E, W> =
    DirectedGraph<G, N, E> + EdgeWeight<E, W>

constraint Tree<T, N> =
    HasRoot<T, N> + ManyToOne<N:children, N:parent>
```

Constraint aliases are pure naming substitution. The compiler expands
them inline wherever they appear in a `where` clause; they do not
introduce a new type or interface. The `+` operator combines
constraints; constraint aliases can reference other constraint aliases
(e.g., `WeightedDirectedGraph` uses `DirectedGraph`).

Why constraint aliases instead of interface-composing-interface (`interface
Foo<...> where Bar<...> + Baz<...>`):

- Constraint aliases have no methods, no impl-block logic, no method-
  body context — just a name → constraint-list substitution.
- They compose freely (alias-of-alias-of-alias works trivially).
- They don't tempt users to put method bodies "inside" a constraint-
  aggregator that pretends to be an interface.
- One mechanism for naming constraint bundles, instead of two.

Spec cost: one new keyword (`constraint`), one new top-level declaration
kind, ~5 lines of spec text.

### 5.3 Labels in where-clauses

A `where` clause references constraint instantiations. When a class
participates in multiple relations to the same other class, the
constraint instantiation carries labels to disambiguate which relation
satisfies the constraint:

```lyric
constraint DirectedGraph<G, N, E> =
    ManyToOne<G:nodes, N:graph> +
    ManyToOne<N:out,   E:source> +
    ManyToOne<N:in,    E:target>
```

`ManyToOne<N:out, E:source>` reads as "a `ManyToOne` whose parent
scope is named `out` on N and whose child scope is named `source` on
E." This is the syntactic mechanism for binding capability-interface
slots to specific relation labels.

When a user's data has matching labels (`relation X N:out refs
[E:source]`), the constraint auto-satisfies. When labels don't match,
the user provides an explicit binding (§5.5).

### 5.4 Aggressive auto-derive of interface satisfaction

An interface method `func T.name(self, ...) -> R` is auto-satisfied
by, in priority order:

1. **An explicit `impl` block.** Always wins.
2. **A unique name+signature match in the class scope.** "Class scope"
   includes declared methods, field-derived getters/setters, and
   relation accessor scopes. If exactly one candidate matches, use it.
3. **No match or multiple matches** → compile error. The user must
   provide an explicit impl to disambiguate.

The compiler does not attempt auto-widening, type coercion, or fuzzy
name matching. The match must be by name and exact signature.

For the FPGA `Route:a` / `Route:b` case, neither label matches the
constraint's `:out` or `:in`. Multiple candidates exist (a and b);
neither label matches the expected name. Auto-derive fails; the user
provides an explicit binding.

For the Network `Person:out` / `Person:in` case, exactly one
candidate matches `:out` and exactly one matches `:in`. Auto-derive
succeeds; no impl block needed.

### 5.5 Explicit `impl` for disambiguation and overrides

When auto-derive fails, the user provides an explicit `impl` block.
Two forms:

**Long form** with method bodies:

```lyric
impl ManyToOne<Route:out, Via:source> {
    pub func P.iter(self) -> gen Via { for v in self.a { yield v } }
    pub func P.count(self) -> i32 { return len(self.a) }
    pub func C.parent(self) -> Route? { return self.a }
}
```

**Alias form** for pure label-renaming:

```lyric
impl ManyToOne<Route:out, Via:source> = ManyToOne<Route:a, Via:a>
```

The alias form is sugar for the long form with delegation bodies. The
compiler synthesizes the delegated methods at desugar time.

When the user wants to satisfy a constraint alias (e.g.,
`DirectedGraph<Net, Route, Via>`), they can use a **grouped
impl block on the constraint alias**:

```lyric
impl DirectedGraph<Net, Route, Via> {
    ManyToOne<Net:nodes, Route:graph> = ManyToOne<Net:routes, Route:net>
    ManyToOne<Route:out, Via:source>  = ManyToOne<Route:a, Via:a>
    ManyToOne<Route:in,  Via:target>  = ManyToOne<Route:b, Via:b>
}
```

Each body entry is itself an impl-alias. The grouped form desugars to
the same set of top-level impls; the grouping is purely visual,
expressing "these bindings together satisfy DirectedGraph for this
type triple."

### 5.6 Static methods are class-scoped functions without `self`

The presence or absence of `self` as the first parameter distinguishes
instance methods from static methods. No new keyword required:

```lyric
class Widget {
    name: string
    func to_string(self) -> string { ... }      // instance — has self
    func default_name() -> string { return "anon" }   // static — no self
}
```

Static methods are called via the long form `Widget.default_name()`.
Interfaces can require static methods:

```lyric
interface Numeric<T> {
    pub func T.zero() -> T              // static requirement
    pub func T.add(self, other: T) -> T // instance requirement
}
```

In generic code, static calls on type variables resolve via the
where-clause constraints:

```lyric
pub func sum(xs: [T]) -> T where Numeric<T> {
    let mut total: T = T.zero()         // static call on type variable
    for x in xs { total = total.add(x) }
    return total
}
```

The monomorphizer specializes `T.zero()` to `i32.zero()` (or whatever
T binds to) at use sites.

### 5.7 Interface body type-var scoping

Type parameters declared in an interface header are in scope throughout
the interface body — for methods, non-method functions, fields, and
nested type references. Non-method functions declared inside an
interface body inherit the interface as an implicit where-clause
constraint:

```lyric
interface Graph<G, N, E> {
    pub func G.bfs(self, start: N) -> [N] { ... }         // method

    pub func ParseGraph(spec: string) -> G? { ... }       // non-method
}
```

The `ParseGraph` non-method function desugars to:

```lyric
pub func ParseGraph(spec: string) -> G? where Graph<G, N, E> { ... }
```

with G, N, E as inferred type parameters and `Graph<G, N, E>` as the
implicit constraint. Callers invoke it via explicit instantiation:
`ParseGraph<Net, Route, Via>("a -> b")` returns `Net?`.

### 5.8 Interface where-clauses are dropped (in favor of constraint aliases)

The spec syntax `interface Foo<...> where Bar<...> + Baz<...>` — an
interface composing other interfaces in its header — is removed. Use
constraint aliases instead. The interface itself declares methods only;
constraint composition lives in the constraint-alias namespace.

The motivation for this is small-spec hygiene: interface where-clauses
and constraint aliases are nearly identical mechanisms (both bundle
constraints under a name). Picking one mechanism keeps the spec
smaller and removes the temptation to write methods inside a constraint-
aggregator interface.

---

## 6. Generic System Rules

### 6.1 Implicit type parameter declaration

A function's generic type parameters are inferred from its signature
(receiver type + argument types + return type) and its where-clause.
Any identifier used as a type that is not in lexical scope as a class
or built-in is treated as a generic type parameter:

```lyric
// Explicit (legal):
pub func bfs<G, N, E>(g: G, start: N) -> [N] where DirectedGraph<G, N, E> { ... }

// Implicit (equivalent — preferred):
pub func bfs(g: G, start: N) -> [N] where DirectedGraph<G, N, E> { ... }
```

The compiler scans the signature and where-clause, collects identifiers
that don't resolve to in-scope declarations, and treats them as
generic type parameters. Order of declaration = order of first
appearance in the signature scan, used for explicit instantiation
(`bfs<Net, Route, Via>(...)`).

When the user wants to pin the parameter order for explicit
instantiation (rare), the explicit `<G, N, E>` form remains available.

Naming rule: **a type identifier is a generic parameter iff no
declaration with that name is in scope.** No case-based rule (no
"single capital letter is a type var"). This works for any naming
convention; `Pair` and `Net` resolve as classes (in-scope), `G` and
`T` resolve as type parameters (not in scope).

### 6.2 Numeric constraint

`Numeric<T>` is a built-in constraint covering the integer and
floating-point types (`i8` through `i64`, `u8` through `u64`, `f32`,
`f64`). It declares `zero`, `one`, `add`, `mul` (with the static-method
forms for `zero` and `one`, instance forms for `add` and `mul`). The
compiler provides built-in impls for each numeric type.

Algorithms parameterize over `W: Numeric` when they want polymorphism
over the weight type:

```lyric
interface EdgeWeight<E, W: Numeric> {
    pub func E.weight(self) -> W
}

pub func G.total_weight(self) -> W where WeightedDirectedGraph<G, N, E, W> {
    let mut sum: W = W.zero()
    for n in self.nodes.iter() {
        for e in n.out.iter() { sum = sum.add(e.weight) }
    }
    return sum
}
```

A user with `weight: f32` instantiates the algorithm at W=f32; a user
with `weight: f64` instantiates at W=f64. No auto-widening; the user
chooses explicitly.

### 6.3 Desugar monomorphizes concrete bindings

The desugar pass performs **text substitution** for every case where
type bindings are concrete:

| Case                                  | Desugar action                          |
|---------------------------------------|-----------------------------------------|
| `relation Hint Foo:bar owns [Baz:qux]` | Substitute P=Foo, C=Baz throughout Hint's body; emit fields and methods into `Foo.bar` and `Baz.qux` scopes. |
| `impl Iface<Widget>` (concrete)        | Substitute interface type vars with Widget; emit concrete methods on Widget. |
| `impl Graph<Net, Route, Via>` (multi-class concrete) | Substitute G=Net, N=Route, E=Via throughout; emit concrete methods. |
| `interface X { pub func default_method(...) ... }` | Extract default method as top-level generic function with where-clause constraint `X<...>`. |

After desugar, the checker sees a code base with:
- Concrete methods on concrete classes (from relations and concrete
  impls).
- Generic free functions with where-clauses (from algorithm
  declarations and extracted default methods).

The checker performs ordinary type-checking on the concrete methods.
For generic free functions, the monomorphizer (a separate pass)
specializes per use site.

### 6.4 Monomorphizer specializes residual generics

At each call site of a generic function, the monomorphizer:

1. Resolves the type parameters from the receiver, argument types, and
   where-clause constraints.
2. Substitutes the resolved types into the generic body.
3. Resolves interface method calls in the body via the impl block (or
   auto-derived equivalent).
4. Emits a concrete specialization of the function.

This is again pure text substitution, just deferred to the use site.
The checker only sees concrete code; generic resolution does not
happen during type checking.

### 6.5 Error positioning for failed constraint resolution

When auto-derive fails to satisfy an interface method during
monomorphization, the error message originates at the **user's call
site** (e.g., `net.bfs(alice)`), with secondary location pointers to
the failing constraint (e.g., "constraint `OutEdges<Net, Follow>`
not satisfied; no method `outgoing(Net) -> gen Follow` and no
matching relation accessor"). Errors do not originate inside the
synthesized free-function form of the algorithm body — that location
is invisible to the user.

---

## 7. What This Eliminates

The redesign deletes several pieces of today's machinery:

- **The `_method_aliases: Dict<Sym, string>` global** in `checker.ly`
  and its companion `_method_labels`. Their job (mapping label-prefixed
  names back to abstract names for resolution) is unnecessary once
  methods live in scopes. The RC bug documented in TODO.md goes away
  with the data structure.

- **Per-relation textual rename map in `DesugarDestructors`** (the
  2026-06-09 patch). Destructor bodies no longer need to know about
  labels because the bodies execute inside the relation's scope, where
  abstract names resolve naturally.

- **The `registerImplMethods` patch from 2026-06-08** (register both
  the abstract name and the label-prefixed name). Method registration
  is just per-class-scope, no dual registration needed.

- **The "field == zero-arg method" special case** (spec §1216). It
  becomes the universal rule, not a relation-accessor-only sugar.

- **The free-function form of relation-derived methods**
  (`array_append<P, C>(p, c)`). UFCS form is the only public API.

- **`OwningList` and `RefList` hint names**. Replaced by `DoublyLinked`
  with the `owns`/`refs` modifier.

- **Interface where-clauses on the interface header** (in favor of
  constraint aliases).

- **The textual-prefix dispatch mechanism** as a whole. Labels are
  scopes; scope members live in the scope's namespace; resolution is
  by ordinary name lookup, not by reconstructing names from a global
  map.

What this earns: the entire bug class around `_method_aliases`,
per-specialization rename inconsistency, multi-relation-same-pair
ambiguity, and the `void*` returns from late generic resolution
(documented in the 2026-06-08 Forge handoff) is eliminated by
construction, not by patching the existing machinery.

---

## 8. Spec Text Changes

Concrete edits to `cr/docs/lyric-language-spec.md`:

### Additions

- **§new "Capability Interfaces and Constraint Aliases"** — small
  multi-class interfaces, constraint alias syntax (`constraint Name<...>
  = ... + ...`), labels in where-clauses.
- **§new "Methods as Class-Scoped Functions"** — declaration rules,
  receiver explicitness contexts, UFCS at call sites, field/method
  unification with collision-is-error.
- **§new "Static Methods"** — distinguished by absence of `self`; call
  via long-form `Class.method(args)`; interfaces may require them;
  static calls on type variables resolve at monomorphization.
- **§new "Auto-Derive of Interface Satisfaction"** — three priority
  rules (explicit impl > unique name match > ambiguous error);
  examples for each.
- **§new "Generic Type Parameter Inference"** — implicit declaration
  from signature scan; explicit form remains available; ordering
  semantics.
- **§new "Desugar and Monomorphization"** — desugar handles concrete
  bindings via text substitution; monomorphizer handles per-use-site
  specialization; checker sees only concrete code.
- **§new "Numeric Constraint"** — built-in constraint covering all
  numeric types; declared methods (`zero`, `one`, `add`, `mul`);
  algorithm-parameterization examples.

### Edits to existing sections

- **§1037 "Interfaces and Multi-Class Contracts"** — drop "where" on
  the interface header (use constraint aliases); update impl-block
  syntax to use type-variable-name receiver form for multi-class
  impls; document the grouped-impl-on-constraint-alias sugar.
- **§1166 "Relations"** — replace "labels prefix injected names" with
  "labels are scopes"; remove the field-method-equivalence special
  case (subsumed by the universal field/method unification rule);
  rename `OwningList`/`RefList` to `DoublyLinked` with `owns`/`refs`
  modifier; update the hint table.
- **§1216 "Field vs Method Access for Relation Accessors"** —
  generalized to all class scopes; no longer a special case.
- **§1170 hint table** — three hints (`ArrayList`, `DoublyLinked`,
  `HashedList`); cross-table with `owns`/`refs` modifiers.
- **§1190 free-function form** — removed. Only UFCS method form
  remains.

### Removals

- **§1097 "short-form receiver" inside impl blocks** — replaced by the
  receiver-explicitness rule (§4.4).
- **Any text suggesting `_method_aliases` or textual prefixing as
  the dispatch mechanism** — removed.

---

## 9. Implementation Phases

Proposed order to build, each phase shippable and testable:

### Phase 1: Methods as class-scoped functions + UFCS

- Implement field-auto-getter/setter.
- Implement zero-arg-method-as-property sugar.
- Implement assignment-as-setter sugar (`obj.field = v`).
- Field/method collision-is-error.
- Migrate existing test data and stdlib to the new form (sed-grade
  changes for getters; manual review for collisions).

**Why first**: smallest, most self-contained, no dependencies on the
other phases. Pays for itself immediately in code readability.

### Phase 2: `DoublyLinked` rename + `owns`/`refs` modifier orthogonal

- Add `DoublyLinked` as a hint name accepting both `owns` and `refs`.
- Mark `OwningList` and `RefList` as deprecated aliases (warning, not
  error). 
- Migrate stdlib, ast.ly, testdata via sed.
- Eventually remove the old names.

**Why second**: also small, mostly mechanical. Independent of phases
3-5. Cleans up the hint table before the bigger labels-as-scopes work.

### Phase 3: Labels-as-scopes for storage layer

- Rewrite hint interface declarations to use `P.method` / `C.method`
  receiver form.
- Rewrite desugar pass to emit relation-injected fields and methods
  into per-relation scopes on parent and child classes.
- Implement scope iteration (`for x in scope`), `len(scope)`.
- Implement auto-projection of singleton child-side scopes.
- Delete `_method_aliases` and `_method_labels` globals; delete the
  per-relation rename map in `DesugarDestructors`; delete the
  registerImplMethods dual-registration patch.
- Migrate all user code from `obj.label_field` to `obj.label.field`.

**Why third**: this is the architectural change. Phase 1 and 2 set
the table; this phase rewrites the storage layer.

### Phase 4: Capability interfaces and constraint aliases

- Implement `constraint Name<...> = A + B` declarations.
- Implement labels-in-where-clauses syntax (`ManyToOne<N:out, E>`).
- Implement aggressive auto-derive (three priority rules).
- Implement the grouped-impl-on-constraint-alias sugar.
- Add `ManyToOne` to the stdlib (auto-satisfied by all three hints).

**Why fourth**: depends on labels-as-scopes (Phase 3) for the
constraint instantiations to work. Lays the groundwork for algorithm
libraries.

### Phase 5: Static methods + Numeric constraint

- Implement `Numeric<T>` as a built-in constraint with built-in impls
  for the numeric types.
- Implement static-call resolution on type variables.
- Document the `func T.method(self?, args)` syntax (presence of `self`
  is the only distinction).

**Why fifth**: enables generic numeric algorithms. Smaller than
Phase 4. Could be done concurrently with Phase 4.

### Phase 6: Implicit generic parameter declaration + parse-error on method-call type args

- Implement signature-scan inference of type parameters.
- Implement parse-error on `obj.method<T>(...)`.

**Why last**: cosmetic improvements over the existing explicit form.
No semantic change, just spec hygiene.

---

## 10. Examples

Two worked-out examples in `testdata/`, exercising the design end-to-end:

- **`testdata/graph.ly`** (312 lines) — multi-class capability
  interfaces, constraint aliases, two consumer cases (FPGA antifuse
  router with neutral labels requiring explicit binding; follower
  network with semantic labels auto-deriving everything). Demonstrates
  the multi-relation-same-pair case that breaks today's spec.
- **`testdata/tree.ly`** (204 lines) — self-relation case (Folder
  owning Folders), constraint subsetting (algorithms that need only
  the node-level capability vs. the full tree wrapper), auto-projection
  of singleton scopes (`n.parent` returns `N?` directly).

Both files compile under the proposed design (assuming the
implementation phases land). They do not compile under today's spec.

---

## 11. Deferred / Open Questions

### Deferred to follow-up

- **`for x in obj` iteration sugar.** Currently requires `for x in
  obj.iter()`. Sugar would let any object satisfying an `Iterable`-
  shaped interface be used directly in `for`. Added to TODO.md.
- **`Hashable.equals` restoration.** Currently `HashedList` matches by
  hash value alone, which works for `Sym` (interned) but not for other
  Hashable types. Until restored, algorithm examples use linear-search
  `[N]` slices for visited-tracking. Tracked in TODO.md (separate
  pre-existing item).
- **1-to-1 relation hint.** Currently a single-owning link (like
  `Filesystem.root: Folder?`) is a plain field, not a relation. A
  hypothetical `OneToOne` hint would close that gap; deferred until
  evidence demands it.
- **Iteration sugar interaction with SoA.** Scope iteration over the
  parent-side relation needs to work both in AoS (slice iteration) and
  SoA (parallel-array iteration). Implementation detail, not a design
  question.

### Open questions for spec-writing time

- **Explicit `static` keyword on declarations** as a marker, or
  rely solely on presence/absence of `self`? Current proposal: no
  keyword. Alternative: optional `static` for readability.
- **Naming `ManyToOne` differently** (e.g., `Container`, `Aggregate`,
  `Collection`). The current name describes the cardinality but not
  what you do with it. Bike-shed candidate.
- **Whether `field T.name` in non-hint interfaces should be explicitly
  marked** (e.g., `inject field T.name`) vs. implicit (today's syntax).
- **Whether to retain interface where-clauses** as a sugar that
  desugars to constraint aliases. Current proposal: drop them. Open
  if migration cost is prohibitive.

### Out of scope for this redesign

- The relation kinds beyond the current three (your future `LinkedList`,
  `TailLinked`, etc.). The design supports them as additional hints
  declared in stdlib; no language changes required.
- Safe iterators, reverse iterators, snapshot iterators. These are
  stdlib additions on top of the existing scope iteration; no language
  changes required.
- Algorithm libraries beyond the graph and tree sketches. Implement on
  top of the design; no language changes required.

---

## 12. Acknowledgments

This design grew out of a single multi-hour design conversation with
Bill on 2026-06-20–21, working from the spec bug surfaced by the
labels-below-the-type-system observation. The example files
`testdata/graph.ly` and `testdata/tree.ly` were iterated through
multiple drafts during the conversation as the design evolved.

The capability-interface idiom draws on Haskell typeclass hierarchies
and Rust trait composition; the relation-first worldview is Bill's
from 30 years of EDA work (ViASIC, DataDraw, etc.); the synthesis is
specific to Lyric's storage-first/relations-as-primary-abstraction
language design.

---

*End of document.*
