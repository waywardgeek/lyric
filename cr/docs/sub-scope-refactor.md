# Sub-Scope Refactor — Design Note

**Status**: design only. Not scheduled. Created 2026-06-23 after Bill + Hewitt
review surfaced collision-class bugs and structural smells that all point
at the same missing primitive: relation labels are first-class
*sub-scopes* on the parent class, but the AST/checker/lowerer model them
as string-prefix flattened fields.

This doc captures the analysis so the next session inherits the
reasoning instead of re-deriving it.

---

## The trigger

Bill asked: graph.ly Part 3 advertises "labels match the interface
surface so the impl is empty and auto-derive does everything." But:

```
relation DoublyLinked Network:nodes owns [Person:graph]
pub func Network.nodes(self) -> gen Person {
    for p in self.nodes.children() { yield p }   // ← collision
}
```

`Network.nodes` (the user method) and `Network:nodes` (the relation
label) share a name. Inside the body, `self.nodes` resolves to the
*method itself* (registered in `info.methods`), so the dotted-scope
sugar `self.nodes.children()` → `self.__nodes_children()` never fires
— it's guarded by `!has_label && has_comb`, and `has_label` is true.

The user gets `unknown method: children` with no explanation that
the conflict is between their method name and a relation label.

Bill's call: the user-facing fix is a proper diagnostic. The
*structural* fix is to give relation labels a real home in the
class's member table.

## The three smells that all share a shape

1. **Lowerer `rename_prefix` (lowerer.ly:741-743)** reads only
   `ib_args[0].label`. The comment admits a future change "can fold
   in the child-side label too." That's a sub-scope being
   approximated by a single string field on the impl-type-arg.

2. **Dotted-scope dispatch** lives in two copy-paste sites
   (checker.ly:3723-3741 for method calls, checker.ly:4015-4035 for
   field reads). Both string-synthesize `"__" + label + "_" + member`
   and look it up. Both have the `!has_label && has_comb` guard —
   which is *exactly* the collision detector Bill is asking for, but
   it silently falls through instead of diagnosing.

3. **Two-relations-per-(P,C)-pair bug** (fixed 2026-06-23 in
   `desugar_relations`): when two DoublyLinked relations shared the
   same (hint, parent_class, child_class), the second relation's
   labels silently overwrote the first impl's slots. Root cause: the
   matcher conflated impls that share (hint, P, C) but differ in
   *labels*, because labels were a side-channel
   `ImplTypeArg.label: Sym?` instead of a primary key.

All three are the same shape: labels are second-class. They live in
strings, get sniff-tested on demand, and the structural relationship
to the parent class is rebuilt each time by string synthesis.

## The proposed model

```
struct ClassInfo {
  fields:     Dict<Sym, FieldInfo>     // user-declared + autogetter targets
  methods:    Dict<Sym, MethodInfo>    // user-declared + interface-injected
  sub_scopes: Dict<Sym, SubScope>      // NEW: one per relation label on this class
}

struct SubScope {
  label:      Sym                       // "nodes", "fwd", "routes", ...
  hint_iface: Sym                       // DoublyLinked, ArrayList, HashedList
  side:       enum { Parent, Child }    // which side this scope owns
  fields:     Dict<Sym, FieldInfo>      // first, last, parent, next, prev, ...
  methods:    Dict<Sym, MethodInfo>     // children (gen), append, remove, ...
}
```

The user-visible surface stays the same:
- `obj.label.member` resolves via `class.sub_scopes[label].{fields,methods}[member]`.
- `obj.member` resolves via `class.fields[member]` then `class.methods[member]`.
- Registering a user method `Class.X` panics if `class.sub_scopes.has(X)` —
  with the clean diagnostic.

C-backend keeps doing the flattening: emit `Class___label_member` as
the C identifier. The flattening moves from semantic-analysis time to
code-emission time — where it belongs, because that's where the
*language constraint* (C has no nested scopes) actually bites.

## Why this is the right call (and why Bill's "flatten early" was wrong
for the *right* reason at the time)

When Bill made the flatten-early call, the cost was theoretical: "we
have to flatten for C anyway, save the complexity." The cost is no
longer theoretical: it's three bug classes plus user-facing
diagnostic blockers. Specifically:

- **Collision detection costs less with sub-scopes.** It's a
  `sub_scopes.has(name)` check at class registration, not a
  reverse-lookup synthesis from the user-method side.
- **Dotted-scope dispatch costs less.** Two copy-paste string-synthesis
  sites collapse into one structural lookup.
- **Wave 2 relation equivalence becomes natural.** The syntax
  `G:nodes/N:graph = Net:routes/Route:net` is a sub-scope-to-sub-scope
  alias by construction.
- **`__name`-shadowing-by-user-field bug class disappears.** A user
  class declaring a field literally named `__nodes_children` today
  silently shadows the relation-injected accessor; with sub-scopes,
  the `__`-prefixed names live inside the sub-scope namespace and
  can't collide with the class's own member tables.
- **Future label-aware mono behavior** (e.g. specializing algorithms
  per-label) requires structural label identity. Today's lowerer has
  to invent it from strings.

The "C must flatten" argument still holds — but flattening is a
*code-emission* concern, not a *semantic-analysis* concern. Move the
flattening to `c_backend.func_name()`-style mangling and the upstream
phases benefit from clean structure.

## Scope estimate

Sites to touch (rough count):
- `ClassInfo` definition + Registry insertion (1 module).
- `Checker.register_class` — populate sub_scopes from relations;
  diagnose collisions inline.
- The two dotted-scope dispatch sites in `checker.ly` — replace
  string-synthesis with `class.sub_scopes[label].lookup(member)`.
- `desugar_relations` — emit sub-scope members instead of flat
  label-prefixed fields. (Most of this is moving existing logic to a
  new container; the *content* of the synthesis stays.)
- `Lowerer.lower_impl_block` — rename_prefix becomes structural;
  per-side labels both consulted.
- `c_backend` — extend `func_name` mangling to
  `<Receiver>___<label>_<name>` when target is in a sub-scope.
- Testdata: regression suite for sub-scope correctness, collision
  diagnostics, multi-label-per-pair, user-field-not-shadowing.

Net delta: probably 400-600 LOC, several sessions. The bootstrap
fixed-point check is the safety net — sub-scope refactor must
self-compile.

## Migration strategy

1. Add `sub_scopes` field on `ClassInfo`; populate from
   `desugar_relations` (additive, no behavior change yet).
2. Add collision diagnostic in `register_class` (the user-visible
   improvement; ships independently as a Wave-N-collision-msg sprint
   without the full refactor).
3. Switch dotted-scope dispatch to consult `sub_scopes` first, fall
   back to string synthesis (compat shim).
4. Migrate `desugar_relations` field/method injection from flat
   names to sub-scope members.
5. Drop the string-synthesis fallback; flatten only in c_backend.
6. Remove `ImplTypeArg.label` (now redundant — labels live on
   ImplBlock indirectly via the sub-scopes they target).

Stages 1-3 can land independently and each holds the bootstrap.
Stages 4-6 require careful sequencing with the bootstrap fixed-point.

## Open questions for the next design session

- Should `SubScope` be a Lyric class or a struct? A class lets us
  attach methods (`scope.lookup_member(s)`); a struct keeps it pure
  data. Probably class, to match the rest of the checker registry.
- Sub-scopes on the child side too? `Edge` in `[Edge:a]` has its
  own injected fields (`__a_next`, `__a_prev`, `__a_parent`).
  Symmetrically the child's `a` is a sub-scope. Bill's call.
- What's the user-visible name? Today the spec uses "label". The
  refactor introduces "sub-scope" as the implementation primitive.
  Probably keep "label" as the user-facing term and document
  sub-scope as the implementation note.

---

*Owner: next session. Don't start without re-reading this doc and the
two dotted-scope dispatch sites in checker.ly.*
