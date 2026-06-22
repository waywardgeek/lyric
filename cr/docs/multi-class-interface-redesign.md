# Multi-Class Interface Redesign

*Status: design proposal. Not yet adopted into the spec.*

*Last updated: 2026-06-21 (revision 3 — incorporates DQ1–DQ4 decisions
plus follow-up: explicit type params on decls, NO `hint interface`
keyword (one unified interface mechanism; checker validates when an
interface is used as a relation hint), no auto-projection, `embed`
removed, unlabeled relations inject flat into the class, ManyToOne →
Collection).*

*Companion artifacts: `testdata/graph.ly`, `testdata/tree.ly` — worked-out
examples in the proposed syntax.*

*Required background: `cr/docs/lyric-language-spec.md` sections
"Interfaces and Multi-Class Contracts" and "Relations".*

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

(spec "Relations" — relation grammar). The hint is a multi-class
interface with **two type parameters in (parent, child) order**. The
relation's identity, however, is a **four-tuple `(P, C, plabel,
clabel)`** — the labels are required to distinguish multiple relations
between the same `(P, C)` pair.

This is the root contradiction: **the hint's vocabulary is binary;
the relation's identity is quaternary; the labels live below the
type system and cannot be named from inside the hint's method
bodies.**

The spec's example (in "Interfaces and Multi-Class Contracts") makes
the problem visible:

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

1. **"Field vs Method Access for Relation Accessors"** is a hand-waved
   special case. The spec promises `team.roster_children` (field) and
   `team.roster_children()` (method) are equivalent — but only for the
   auto-injected accessor form. Hint method bodies referencing
   `self.children` neither work as field access nor as method call
   without the textual-rewrite scheme.

2. **"Any binary interface can serve as a hint"** is too generous. The
   spec's own "Interfaces and Multi-Class Contracts" example declares
   `interface Graph<G, N, E>` (three type parameters); the `relation`
   grammar permits exactly one parent and one child. Three-class
   interfaces cannot be relation hints, but the spec doesn't say so.

3. **Free-function form `array_append<P, C>(p, c)`** is keyed only on
   `(P, C)`. When a parent has two same-hint same-child relations, the
   free-function form is one symbol with two valid expansions — and
   nothing in the type system breaks the tie.

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
│               `Collection<P, C>` is the universal relation-      │
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

### 3.1 Labels are scopes; the empty label is flat

A relation declaration

```lyric
relation Hint Parent[:plabel] (owns|refs) [Child[:clabel]]
```

creates two namespaces of member injection on the participating classes,
parameterized by whether labels are present.

**Labeled form** (`Parent:plabel ... [Child:clabel]`) creates two
scopes:

- `parent.plabel` on Parent — the parent-side view of the relation.
  Contains the hint's parent-side fields and methods (typically
  iteration accessor, count, append/remove).
- `child.clabel` on Child — the child-side view of the relation.
  Contains the hint's child-side fields and methods (typically the
  back-pointer to parent, plus link-pointers for doubly-linked).

Inside each scope, member names are **not prefixed** — the scope name
is the prefix. So a relation `Team:roster owns [Player:team]` produces:

| Access form (labeled)          | Access form (today's spec)        |
|--------------------------------|-----------------------------------|
| `team.roster.children`         | `team.roster_children`            |
| `team.roster.count`            | `team.roster_count`               |
| `team.roster.append(p)`        | `array_append<Team, Player>(t, p)`|
| `player.team.parent`           | `player.team_parent`              |
| `player.team.index`            | `player.team_index`               |

**Unlabeled form** (`Parent ... [Child]`, no `:label` on either side)
injects all hint members **flat into the class namespace** — no scope.
This is the common case: most relations between a (P, C) pair are
unique, and labels are pure ceremony.

```lyric
relation DoublyLinked Folder owns [Folder]
```

…injects (on Folder, which is both P and C for a self-relation):

- Parent-side: `first: Folder?`, `last: Folder?`, `append(Folder)`,
  `remove(Folder)`, `iter() -> gen Folder`, `count: i32`.
- Child-side: `parent: Folder?`, `next: Folder?`, `prev: Folder?`.

The user writes `node.parent`, `node.iter()`, `node.append(child)`,
`node.first`, etc. — directly on the class, no `.label.` prefix.

**Mixed forms** (`Parent:label ... [Child]` — labeled on one side,
unlabeled on the other) are allowed. The labeled side gets a scope;
the unlabeled side injects flat.

Scopes are first-class enough at the syntactic level to be:

- **Iterable**: `for x in team.roster` iterates the children. (Iteration
  sugar is on TODO; explicit form `for x in team.roster.iter()` works
  today after migration.)
- **Subject of method calls**: `team.roster.append(player)`.
- **Subject of `len`**: `len(team.roster)` returns count.

**Scopes are syntactic, not values.** A scope is a path in the name-
resolution algorithm; it cannot be bound to a variable, passed as an
argument, or stored in a field. `let s = team.roster` is a hard error
("scope is not a value"). To pass relation data around, pass the
underlying field (`team.roster.children` is the `[Player]` slice) or
the parent class itself.

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

### 3.3 Multi-relation rule: at most one unlabeled per (P, C) pair

Multiple relations between the same `(Parent, Child)` type pair are
allowed, but **only one of them may be unlabeled**. The second must
introduce labels on at least one side to disambiguate.

For **self-relations** (where the parent and child class are the same),
the same rule applies, but the namespace collision boundary tightens
because both sides inject into one class:

```lyric
// LEGAL — unlabeled, single self-relation, P-side and C-side names
// don't collide (children/iter/count/append/remove vs. parent/next/prev):
relation DoublyLinked Folder owns [Folder]

// LEGAL — labeled, role names express the direction-of-view:
relation DoublyLinked Folder:children owns [Folder:parent]

// LEGAL — multiple self-relations require distinct labels:
relation DoublyLinked TreeNode:left owns [TreeNode:lparent]
relation DoublyLinked TreeNode:right owns [TreeNode:rparent]

// ILLEGAL — two unlabeled relations on same (P, C) pair:
relation ArrayList Team owns [Player]
relation ArrayList Team owns [Player]   // ERROR: ambiguous

// ILLEGAL — labels on a self-relation collide in one namespace:
relation DoublyLinked Folder:a owns [Folder:a]   // ERROR
```

The compiler enforces "at most one unlabeled" and "labels don't
collide in any class's namespace" at the checker (Phase 1).

For the existing three hints, the P-side and C-side member names never
overlap, so a single unlabeled self-relation always works. A future
hint that injects same-named members on both sides would force labels
in self-relations — captured as a design constraint on new hints, not
a language rule.

### 3.4 (No auto-projection)

A previous draft proposed *auto-projection*: a child-side scope name
in a context expecting the parent type would silently resolve to the
scope's `.parent` field, so `via.a` would mean `via.a.parent` when
assigned to a `Route?`-typed variable. **This is rejected.** Implicit
unwrap is the same class of magic as the current spec's segfaulting
auto-deref on optional class receivers, and it costs more in surprise
than it pays in keystrokes. Adopters write `.parent` explicitly:
`via.a.parent`, not `via.a`. The unlabeled flat form (§3.1) already
gives clean reading for the common single-relation case (`node.parent`
is just the back-pointer field, no scope at all).

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
addressed by their scope path (when labeled) or their flat class name
(when unlabeled), not by a prefixed name.

Migration is a one-line sed:
`s/\bOwningList\b/DoublyLinked/g; s/\bRefList\b/DoublyLinked/g`
applied to source code. The `owns`/`refs` keyword stays as-is.

### 3.6 Hint interfaces declare members in `P.method` / `C.method` form

Interfaces that are used as relation hints declare each member as
belonging to either the parent-side (`P.method`) or child-side
(`C.method`). The desugar pass uses this annotation to decide which
class each member injects into:

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

When the desugar pass instantiates this for a labeled relation
(`relation ArrayList Foo:bar owns [Baz:qux]`), it substitutes P=Foo
and C=Baz throughout, placing parent-side members into `Foo.bar` scope
and child-side members into `Baz.qux` scope. For an unlabeled relation
(`relation ArrayList Foo owns [Baz]`), members inject flat into Foo
and Baz directly.

The body of `append` references `self.children` — under substitution,
this refers to the **same scope's** `children` field, which becomes
`foo.bar.children` (labeled) or `foo.children` (unlabeled). Within a
single relation's instantiation, all abstract names resolve
unambiguously to the same namespace.

### 3.7 Any interface may be a relation hint; the checker enforces fit

There is **no special "hint interface" keyword**. One mechanism: an
interface declares methods and `field T.name: Type` requirements.
Field declarations get *injected* into the participating class(es)
when the interface is bound — whether the binding comes from a
`relation` declaration or from a generic where-clause. The injection
rule is the same in both cases (one field per concrete class per
distinct binding); only the binding mechanism differs.

An interface may be used as the hint in a `relation` declaration iff
the checker validates that it fits the relation-hint shape:

1. **Exactly two type parameters**, in `(parent, child)` order. Zero,
   one, or three+ is an error.
2. **Each member is annotated with its side** via the `P.member` /
   `C.member` declaration form. Members on the parent's type variable
   land on the parent side; members on the child's type variable land
   on the child side. A member that names neither type variable
   (e.g., `func default_name() -> string` with no `P.` / `C.` prefix)
   is an error in hint position.
3. **Member bodies reference only same-side state.** A parent-side
   method body may reference parent-side fields and methods; same for
   child-side. Cross-side references inside a body are an error
   (they're ill-defined under per-relation injection because there's
   no way to name "the other side's instance" from inside one side's
   method).

When these conditions hold, the desugar pass can mechanically
instantiate the interface for any `relation Hint P:l owns [C:m]`
declaration, substituting P and C and routing members to the right
scope (or flat, for unlabeled relations).

When they don't hold, the checker emits a diagnostic at the
`relation` declaration site, citing the interface declaration site
and the specific failure:

```
error: cannot use MyContract as a relation hint
   at example.ly:42:  relation MyContract Foo:bar owns [Baz:qux]
   note: MyContract is declared at contracts.ly:10
   reason: an interface used as a relation hint must have exactly 2
           type parameters (parent, child); MyContract has 3 type
           parameters <A, B, C>.
   suggestion: split MyContract into a 2-parameter interface (for use
               as the relation hint) plus a 3-parameter capability
               interface, or use a different hint.
```

```
error: cannot use Display as a relation hint
   at example.ly:42:  relation Display Widget:w owns [Panel:p]
   note: Display is declared at ui.ly:5
   reason: Display has methods that name no relation side
           (`func to_string(self) -> string` is unscoped).  An
           interface used as a relation hint must annotate every
           member with `P.` or `C.`.
```

```
error: cannot use Pairing as a relation hint
   at example.ly:42:  relation Pairing Foo owns [Baz]
   note: Pairing is declared at pairing.ly:8
   reason: Pairing.P.merge body references `self.other` where `other`
           is a C-side field.  Methods of an interface used as a
           relation hint may reference only same-side members.
```

Why this approach beats a special keyword:

- **No action-at-a-distance** — a `relation` declaration cannot
  silently change anything about the interface; the interface's own
  declaration is unchanged.
- **One interface mechanism** — `interface` is the only keyword;
  every interface declares contract + optional field injection.
- **Checker catches misuse at the use site**, with a clean message
  pointing back at both the relation and the interface. The cost is
  three diagnostic rules in the checker, not a new keyword in the
  language.

The companion non-hint case (interfaces used purely as capabilities,
never as relation hints) just works — they declare methods and
optional fields; consumers satisfy them by providing those members.
The `Collection`, `EdgeWeight`, `HasRoot`, and `Numeric` interfaces
in §5 and §6.2 are examples.

### 3.8 Labels live on impl type variables; relations are sugar

A relation declaration carries one label per side
(`relation ArrayList Team:roster owns [Player:team]`). Each label
says "the members this interface puts on this class go into the
`<label>` scope on that class." Because each side has its own class
and its own scope namespace, the label naturally belongs to the
**type variable that names the participating class**, not to the
impl block as a whole.

This is reflected in surface syntax: an impl declaration can
optionally label any of its top-level class type-arguments.

```lyric
impl ArrayList<Team:roster, Player:team> {
    P.children <-> Team.roster_field
    C.parent   <-> Player.team_field
    C.index    <-> Player.team_idx
}
```

Members declared on `P` (the parent type variable) inject into the
`roster` scope on `Team`; members on `C` inject into the `team`
scope on `Player`. Omit the label and the side injects flat into
the class namespace (§3.1).

Labels are *per class type-variable*, not per impl block. Same-
letter labels on different type-vars in the same impl are fine
(`<Route:a, Via:a>`) because each label lives in its own class's
namespace (§3.2). An earlier draft of this design carried a single
`ImplBlock.label` slot to which the parent-side label was routed
and the child-side label was threaded through a separate side-
channel in `desugar_destructors`; per-type-var labels eliminate
that asymmetry.

**A relation is then a trivial desugar to a labeled impl:**

```
  relation ArrayList Team:roster owns [Player:team]
      ↓
  impl ArrayList<Team:roster, Player:team> {
      // field-binds synthesized from the hint interface's
      // field declarations, using each side's label
  }
  + owns flag on each side's destructor pair (§3.5)
```

Both labels ride on the impl's type-args. No special "this label
belongs to the parent side" routing — the type-var IS the side.

**Why this matters beyond relations.** Type variables in an impl
declaration already designate the participating classes (that's
what makes multi-class interfaces work — the same interface puts
default fields and methods on more than one class via its
type-vars). Labels-per-type-var generalize the "inject this
bundle of fields/methods onto this class, in a named sub-scope"
capability to *any* multi-class interface, whether or not it's a
relation hint.

The canonical motivating example is older than Lyric. Around 1990,
C++ users hit the question: "How do I make a class participate in
two intrusive linked lists at once?" Single inheritance from a
`LinkedListChild` mixin worked for the first list and collided on
the second. Multiple inheritance partially worked but offered no
way to disambiguate which `next`/`prev` field belonged to which
list, and dragged in vtable layout pain that wasn't even relevant
to the problem. The feature the community actually wanted was
*"inject the same default-field bundle on this class twice, under
distinct names."* Per-class-type-variable labels on impl
declarations are exactly that feature, three decades late. Two
instances of the same interface on the same class, under distinct
labels, give two non-colliding bundles of injected members:

```lyric
impl DoublyLinked<Node:ready_q,   Node:ready_q_child>   { ... }
impl DoublyLinked<Node:blocked_q, Node:blocked_q_child> { ... }
```

`node.ready_q.next` and `node.blocked_q.next` are different fields
on the same class, with no naming collision and no inheritance.
The relation surface syntax (`relation DoublyLinked Node:ready_q
owns [Node:ready_q_child]`) is the convenient daily-driver
spelling; per-type-var labels on impls are the underlying
mechanism that makes "non-colliding double injection" cleanly
expressible at all.

**Implementation footprint** of moving from `ImplBlock.label: Sym?`
(single slot) to per-type-var labels is small and local. The impl
block's type-argument list changes from `[TypeExpr]` to a wrapper
`[ImplTypeArg]` where each entry pairs a `TypeExpr` with an
optional `Sym?` label. The parser learns to read `:label` after
each top-level type-arg in an `impl` declaration. The relation→impl
desugar drops each side's label into the corresponding type-arg.
The checker's existing label-prefix registration consults the
per-type-arg label instead of the impl-block-wide label. The
previous `type_param_to_label` side-channel in `desugar_destructors`
is deleted. Net change is mildly LOC-positive only because the
side-channel disappears.

**Backward-removed:** the obsolete `impl X<...> as <label> { ... }`
form (a single-label-on-the-whole-impl parser path, never used by
user code; only ever populated by the relation→impl desugar
internally) is removed in favor of the per-type-var form.

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

This eliminates today's spec "field vs method access for relation
accessors" special case by generalizing it to the universal rule:
fields and zero-arg methods are interchangeable at the call site for
any class scope, not just for relation-injected accessors.

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
net.bfs(alice)                    // OK
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

**Constructor and static-method calls are not method calls.**
`Class<T>(args)` and `Class<T>.static_method(args)` retain explicit
type arguments — there is no receiver to pin the type parameter from,
so the user supplies it:

```lyric
let d = Dict<Sym, i32>()              // OK — constructor
let ch = make_channel<i32>(10)        // OK — free function (already legal today)
let s = StringBuilder<i32>.empty()    // OK — static method on parameterized class
```

The distinguishing rule: type arguments live on **the receiver/class
name**, not on the method name. `Foo<T>.bar(...)` is fine; `foo.bar<T>(...)`
is not.

Free-function calls also retain explicit type arguments for the
legitimate case where a type parameter appears only in the return
type or where-clause constraints (e.g., `make_vec<i32>()`,
`parse<f64>("3.14")`).

### 4.6 No free-function form of relation-derived methods

Today's spec offers both `team.array_append(player)` and
`array_append<Team, Player>(t, p)`. The free-function form is
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
interface Collection<P, C> {
    pub func P.iter(self) -> gen C
    pub func P.count(self) -> i32
    pub func C.parent(self) -> P?
}
```

Every relation hint (`ArrayList`, `DoublyLinked`, `HashedList`)
automatically satisfies `Collection<P, C>` because each provides
iter+count on the parent side and a parent back-pointer on the child
side.

Specialized capabilities cover what `Collection` doesn't:

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
    Collection<G:nodes, N:graph> +
    Collection<N:out,   E:source> +
    Collection<N:in,    E:target>

constraint WeightedDirectedGraph<G, N, E, W> =
    DirectedGraph<G, N, E> + EdgeWeight<E, W>

constraint Tree<T, N> =
    HasRoot<T, N> + Collection<N:children, N:parent>
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
    Collection<G:nodes, N:graph> +
    Collection<N:out,   E:source> +
    Collection<N:in,    E:target>
```

`Collection<N:out, E:source>` reads as "a `Collection` whose parent
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
impl Collection<Route:out, Via:source> {
    pub func P.iter(self) -> gen Via { for v in self.a { yield v } }
    pub func P.count(self) -> i32 { return len(self.a) }
    pub func C.parent(self) -> Route? { return self.a }
}
```

**Alias form** for pure label-renaming:

```lyric
impl Collection<Route:out, Via:source> = Collection<Route:a, Via:a>
```

The alias form is sugar for the long form with delegation bodies. The
compiler synthesizes the delegated methods at desugar time.

When the user wants to satisfy a constraint alias (e.g.,
`DirectedGraph<Net, Route, Via>`), they can use a **grouped
impl block on the constraint alias**:

```lyric
impl DirectedGraph<Net, Route, Via> {
    Collection<Net:nodes, Route:graph> = Collection<Net:routes, Route:net>
    Collection<Route:out, Via:source>  = Collection<Route:a, Via:a>
    Collection<Route:in,  Via:target>  = Collection<Route:b, Via:b>
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
pub func<G, N, E> ParseGraph(spec: string) -> G? where Graph<G, N, E> { ... }
```

with G, N, E as the interface's type parameters and `Graph<G, N, E>`
as the implicit constraint. Callers invoke via explicit instantiation:
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

### 5.9 Error message shape

Three failure modes that show up at the `impl`/auto-derive boundary,
with the diagnostic shape each should produce. Concrete examples
serve as the bar for the implementation.

**Auto-derive ambiguity** — multiple candidates match an interface
method's name+signature in the class scope:

```
error: cannot auto-derive Graph<Net, Route, Via>:
   method N.outgoing(self) -> gen Via is ambiguous on Route
     candidates:
       Route.a.iter (from relation :a)
       Route.b.iter (from relation :b)
   note: provide an explicit impl block to choose.
   suggestion:
       impl Graph<Net, Route, Via> {
           Collection<Route:out, Via:source> = Collection<Route:a, Via:a>
       }
```

**Constraint not satisfied** — at the use site of a generic
algorithm, the receiver's type doesn't provide the required surface:

```
error: net.bfs(alice): constraint DirectedGraph<Network, Person, ?> not satisfied
   missing: Collection<Person:out, ?:source>
     no method Person.outgoing nor relation accessor matching :out on Person.
   note: bfs is declared at graph.ly:114 with
         where DirectedGraph<G, N, E>.
   suggestion: add `relation DoublyLinked Person:out refs [Follow:source]`,
               or provide an explicit impl block.
```

The error originates at the user's call site (`net.bfs(alice)`), not
inside the synthesized generic body, so the user reads the failure in
their own code.

**Field/method collision** — Phase 1's universal unification rule:

```
error: Counter declares both `count: i32` (field) and `count(self) -> i32` (method).
   field is at counter.ly:3
   method is at counter.ly:7
   note: field and zero-arg method names share a namespace; rename one.
```

These are the three commonest failure modes; others (e.g., destructor
collision, hint-interface arity mismatch) follow the same pattern:
quote the offending name, cite the two declaration sites, suggest the
canonical fix.

---

## 6. Generic System Rules

### 6.1 Type parameters on function declarations are explicit

Every generic type parameter on a function declaration must be declared
explicitly inside `<>` between the `func` keyword and the function
name (or, for class-scoped methods, between `func` and the receiver
class name):

```lyric
// Free function:
pub func<G, N, E> bfs(g: G, start: N) -> [N] where DirectedGraph<G, N, E> { ... }

// Class-scoped method:
pub func<G, N, E> G.bfs(self, start: N) -> [N] where DirectedGraph<G, N, E> { ... }

// Multi-class impl method (type params inherited from the impl header):
impl Graph<Net, Route, Via> {
    pub func G.nodes(self) -> [Route] { return self.routes }
}
```

This is the same explicit-declaration rule as today's spec. The
redesign does **not** change it. A draft considered inferring type
parameters from the signature scan; the original spec rationale ("a
typo silently becomes a type variable") held up under review and the
inference proposal was rejected.

**Call-site type inference is unchanged.** Callers still write
`max(a, b)` and `identity(42)` without explicit type arguments —
the compiler infers from argument types. Explicit type arguments at
the call site remain optional for free functions
(`bfs<Net, Route, Via>(net, start)`) and forbidden for method calls
(see §4.5).

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

- **The "field == zero-arg method" special case** (spec section
  "Field vs Method Access for Relation Accessors"). It
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

Concrete edits to `cr/docs/lyric-language-spec.md`. Section references
use **names**, not line numbers (line numbers drift across edits).

### Additions

- **New section "Capability Interfaces and Constraint Aliases"** —
  small multi-class interfaces; constraint alias syntax
  (`constraint Name<...> = ... + ...`); labels in where-clauses.
- **New section "Interfaces as Relation Hints"** — the unified
  `interface` mechanism; checker rules that validate an interface for
  use as a relation hint (2 type params, `P.member`/`C.member` form,
  no cross-side body references); diagnostic shapes when validation
  fails.
- **New section "Methods as Class-Scoped Functions"** — declaration
  rules, receiver-explicitness contexts, UFCS at call sites,
  field/method unification with collision-is-error.
- **New section "Static Methods"** — distinguished by absence of `self`;
  call via long-form `Class.method(args)`; interfaces may require them;
  static calls on type variables resolve at monomorphization.
- **New section "Auto-Derive of Interface Satisfaction"** — three
  priority rules (explicit impl > unique name match > ambiguous
  error); examples for each.
- **New section "Desugar and Monomorphization"** — desugar handles
  concrete bindings via text substitution; monomorphizer handles per-
  use-site specialization; checker sees only concrete code.
- **New section "Numeric Constraint"** — built-in constraint covering
  all numeric types; declared methods (`zero`, `one`, `add`, `mul`);
  algorithm-parameterization examples.
- **New subsection "Error Message Shape"** — at least the three
  worked examples from §5.9 of this doc.

### Edits to existing sections

- **"Interfaces and Multi-Class Contracts"** — drop the `where` clause
  on the interface header (use constraint aliases); update impl-block
  syntax to use type-variable-name receiver form for multi-class impls;
  document the grouped-impl-on-constraint-alias sugar.
- **"Relations"** — replace "labels prefix injected names" with the
  two-rule structure: labeled = scope, unlabeled = flat injection;
  document the "at most one unlabeled per (P, C) pair" rule; remove
  the field-method-equivalence special case (subsumed by the universal
  field/method unification rule); rename `OwningList`/`RefList` to
  `DoublyLinked` with `owns`/`refs` modifier; update the hint table.
- **"Field vs Method Access for Relation Accessors"** — generalized
  to all class scopes; no longer a special case.
- **Hint table** — three hints (`ArrayList`, `DoublyLinked`,
  `HashedList`); cross-table with `owns`/`refs` modifiers. Hints are
  declared as ordinary interfaces; the checker validates fit at the
  `relation` declaration site (no special keyword).
- **Free-function form of relation-derived methods**
  (`array_append<P,C>(p, c)` etc.) — removed. Only UFCS method form
  remains.
- **"Generics and Type Variables"** — type-parameter declaration on
  function signatures is unchanged (explicit only); the redesign adds
  the multi-class impl-header form (`impl<G, N, E> Graph<...>`) and
  the constraint-alias form (`constraint Name<...> = ...`).

### Removals

- **"Short-form receiver" inside impl blocks** — replaced by the
  receiver-explicitness rule (§4.4 of this doc).
- **`embed` keyword** — the only consumers were `OwningList` and
  `RefList`, both of which fold into `DoublyLinked` with the
  `owns`/`refs` modifier (§3.5 of this doc). No other interface uses
  `embed` today. Delete from lexer, parser, AST, desugar.
- **Auto-projection of child-side scopes** — explicitly rejected
  (§3.4 of this doc). Any earlier spec text proposing implicit
  `scope` → `scope.parent` conversion is dropped.
- **Any text suggesting `_method_aliases` or textual prefixing as
  the dispatch mechanism** — removed.

---

## 9. Implementation Phases

Proposed order to build, each phase shippable and testable.

### Phase 0 (prerequisite): Field/method collision audit

Before Phase 1 lands the universal field/method unification rule,
sweep the bootstrap source (`src/`, `stdlib/`, `testdata/`) for
existing classes that declare a field and a method with the same
name (e.g., `count: i32` alongside `func count(self) -> i32`). Each
collision needs a manual rename decision (cached-field vs. computed-
method semantic distinction).

This is real work, not sed. Allocate one focused session. Block
Phase 1 on its completion.

### Phase 1: Methods as class-scoped functions + UFCS

- Implement field-auto-getter/setter.
- Implement zero-arg-method-as-property sugar.
- Implement assignment-as-setter sugar (`obj.field = v`).
- Field/method collision-is-error — applies to **user-declared names
  only** in this phase. Relation-injected names retain today's
  flat-prefix scheme until Phase 3 reorganizes them into scopes;
  the collision check on relation-injected names lands in Phase 3.
  (Without this carve-out, every existing relation that injects a
  `roster_children` field plus a `roster_children()` getter would
  trip the new rule immediately.)
- Migrate existing test data and stdlib to the new form (sed-grade
  changes for getters; manual review for the collisions identified
  in Phase 0).

**Why first**: smallest, most self-contained, no dependencies on the
other phases. Pays for itself immediately in code readability.

### Phase 2: `DoublyLinked` rename + `owns`/`refs` modifier orthogonal

- Add `DoublyLinked` as a hint name accepting both `owns` and `refs`.
- Mark `OwningList` and `RefList` as deprecated aliases (warning, not
  error).
- Migrate stdlib, ast.ly, testdata via sed.
- Eventually remove the old names.
- Delete the `embed` keyword and its lexer/parser/AST/desugar paths
  (its only consumers were `OwningList` and `RefList`).

**Why second**: also small, mostly mechanical. Independent of phases
3-5. Cleans up the hint table before the bigger labels-as-scopes work.

### Phase 3: Labels-as-scopes for storage layer

- Implement the checker's relation-hint shape validation: 2 type
  params, `P.member`/`C.member` form on every member, no cross-side
  body references. Emit clean diagnostics at the `relation`
  declaration site (the three error shapes in §3.7) when validation
  fails.
- Rewrite hint interface declarations (`ArrayList`, `DoublyLinked`,
  `HashedList`) to use `P.method` / `C.method` receiver form.
- Rewrite desugar pass to emit relation-injected fields and methods
  into per-relation scopes on parent and child classes (labeled
  relations) or flat into the class namespace (unlabeled relations).
- Implement scope iteration (`for x in scope`), `len(scope)`.
- Enforce the multi-relation rule: at most one unlabeled per `(P, C)`
  pair; labels don't collide in any class's namespace.
- Extend the field/method collision check (carved out in Phase 1) to
  cover relation-injected names now that they live in proper scopes.
- Delete `_method_aliases` and `_method_labels` globals; delete the
  per-relation rename map in `DesugarDestructors`; delete the
  `registerImplMethods` dual-registration patch.

**Why third**: this is the architectural change. Phase 1 and 2 set
the table; this phase rewrites the storage layer.

### Phase 3.5: `lyric migrate` tool

A standalone driver subcommand that rewrites a `.ly` source tree from
the pre-Phase-3 syntax to the post-Phase-3 syntax:

- `obj.label_field` → `obj.label.field` (labeled relations).
- `obj.label_field` → `obj.field` (when the user's relation can be
  expressed unlabeled — exactly one relation on its `(P, C)` pair).
- `array_append<P, C>(p, c)` → `p.append(c)` (and similar for
  `dll_append`, `hash_insert`, etc.).
- `OwningList`/`RefList` → `DoublyLinked` (covered by Phase 2 sed,
  but the tool absorbs it for one-shot migration).

Implementation: reuses the compiler's relation table to disambiguate
underscores (which are label separators vs. ordinary snake_case). Sed
cannot do this reliably; a parser-aware tool can.

Block migrating the bootstrap compiler on shipping this tool. The
bootstrap source is ~30K lines; manual migration is not viable.

### Phase 4: Capability interfaces and constraint aliases

- Implement `constraint Name<...> = A + B` declarations.
- Implement labels-in-where-clauses syntax (`Collection<N:out, E:source>`).
- Implement aggressive auto-derive (three priority rules from §5.4).
- Implement the grouped-impl-on-constraint-alias sugar (§5.5).
- Implement the three error-message shapes from §5.9.
- Add `Collection` to the stdlib (auto-satisfied by all three hints).

**Why fourth**: depends on labels-as-scopes (Phase 3) for the
constraint instantiations to work. Lays the groundwork for algorithm
libraries.

### Phase 5: Static methods + Numeric constraint

- Implement `Numeric<T>` as a built-in constraint with built-in impls
  for the numeric types (`i8`–`i64`, `u8`–`u64`, `f32`, `f64`).
- Implement static-call resolution on type variables (`T.zero()` when
  `T: Numeric`).
- Document the `func T.method(self?, args)` syntax (presence of
  `self` is the only distinction between instance and static).

**Why fifth**: enables generic numeric algorithms. Smaller than
Phase 4. Could be done concurrently with Phase 4.

### Phase 6: Parse-error on method-call type args + constructor exception

- Implement parse-error on `obj.method<T>(...)`.
- Confirm `Class<T>(args)` and `Class<T>.static_method(args)` still
  parse (the type args live on the class name, not the method name —
  see §4.5).

**Why last**: smallest semantic change. Pure spec hygiene; the
existing behavior of accepting `obj.method<T>(...)` is rare in
practice and the parse-error is a five-minute change once the rest of
the redesign is in.

---

## 10. Examples

Two worked-out examples in `testdata/`, exercising the design end-to-
end:

- **`testdata/graph.ly`** — multi-class capability interfaces,
  constraint aliases, two consumer cases (FPGA antifuse router with
  neutral labels requiring an explicit bridging impl; follower
  network with semantic labels auto-deriving everything). Demonstrates
  the multi-relation-same-pair case that breaks today's spec, and the
  use of labeled relations + labeled constraint signatures.
- **`testdata/tree.ly`** — self-relation case (Folder owning Folders)
  using the unlabeled-flat-injection form. Demonstrates that a
  single self-relation needs no labels, constraint subsetting
  (algorithms that need only the node-level capability vs. the full
  tree wrapper), and that algorithm authors choose whether their
  constraints carry labels (graph.ly does; tree.ly doesn't).

Both files compile under the proposed design (assuming the
implementation phases land). They do not compile under today's spec.

---

## 11. Deferred / Open Questions

### Decided in the 2026-06-21 review (kept here for the record)

- **Implicit type parameter inference on function declarations** —
  *rejected.* Explicit `<T>` declaration on decls is required (the
  current spec rule survives); call-site inference is unchanged.
  See §6.1.
- **Special `hint interface` keyword** — *rejected.* One unified
  `interface` mechanism. Any interface may be used as a relation
  hint provided it passes the checker's shape validation (2 type
  params, members in `P.method`/`C.method` form, no cross-side
  references in bodies). The checker emits clean diagnostics at the
  `relation` declaration site when the fit fails. See §3.7.
- **Auto-projection of child-side scopes** — *rejected.* No implicit
  `scope` → `scope.parent` conversion. See §3.4.
- **`embed` keyword** — *removed.* Its only consumers (`OwningList`,
  `RefList`) fold into `DoublyLinked` with the `owns`/`refs` modifier.
  See §3.5 and Phase 2 in §9.
- **Explicit `static` keyword** — *rejected.* Presence/absence of
  `self` is the only distinction between instance and static methods.
  See §5.6.
- **Interface where-clauses retained as sugar** — *rejected.* Use
  constraint aliases instead. See §5.8.
- **`ManyToOne` naming** — *renamed to `Collection`.* The natural
  English word for "P has a bunch of C's"; doesn't oversell
  cardinality.

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

### Still-open questions for spec-writing time

- **Whether `field T.name` in capability (non-hint) interfaces should
  be explicitly marked** (e.g., `inject field T.name`) vs. implicit
  (`field T.name`). Today's syntax is ambiguous between "this
  interface declares a field requirement" and "this interface
  injects a field into the bound class." Under the unified-interface
  model (§3.7), the distinction is whether the field is referenced by
  a method body — in which case it must be injected so the body can
  read/write it. Explicit marking would document intent at the point
  of use, but isn't strictly necessary.

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
