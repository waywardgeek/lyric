# Book chapter revision prompt — Phase 3d/3e dotted-scope sweep

## Your task

You are revising ONE chapter of `the-lyric-book.md` for the Lyric language book.
Your chapter file is given to you separately. **Edit only that file.** Do not
touch anything else in the repo (other than reading source-of-truth docs and
writing throwaway `/tmp/*.ly` test programs).

This is a **single-turn job**: read the sources of truth, read your chapter,
make all needed edits in one pass, run any empirical compile checks, then
report `DONE:` with a brief summary of what changed. Do not stop after
research and ask whether to proceed — proceed.

## What changed in Phase 3d/3e (the reason for this sweep)

The compiler has moved from the pre-Phase-3 *bare-flat label-prefix* accessor
scheme to the *dotted-scope mangled-storage* scheme. The book in places
still teaches the old form, which now fails to compile.

**Old (no longer reachable from user code):**
```lyric
relation ArrayList Team:roster owns [Player:team]
// stale forms — DO NOT teach these:
t.roster_append(p)
t.roster_children.len()
p.team_index
p.team_parent
```

**New (the only user-visible form, Phase 3e):**
```lyric
relation ArrayList Team:roster owns [Player:team]
// canonical forms:
t.roster.append(p)
t.roster.children.len()         // or len(t.roster.children)
p.team.index
p.team.parent
```

Storage is mangled to `Team.__roster_children`, `Player.__team_parent`,
`Player.__team_index`. Bare-flat access fails at gcc with a
`'Team' has no member named 'roster_children'; did you mean '__roster_children'?`
diagnostic. The book should NOT expose the `__`-mangled names unless
explaining internals.

**Also gone:**
- `cascade` keyword (removed). Ownership semantics carried by `owns`/`refs`.
- `embed` keyword (removed entirely).
- `OwningList`, `RefList`, `RefArrayList` relation hints — gone. Three hints
  remain: `ArrayList`, `DoublyLinked`, `HashedList`. Each takes `owns` OR
  `refs` (orthogonal modifier picks cascade-vs-unlink destructor pair).
- Hard keywords reserved everywhere — labels can't be reserved-word identifiers
  (the label `:where` was renamed to `:wc` in compiler source).

## New capabilities to weave in WHERE THEY FIT NATURALLY

These shipped in Phase 3a/3c/3e. They generalize `relation`. If your chapter
already discusses interfaces, impls, or relation hints, introduce them in the
right place. If your chapter doesn't touch those topics, ignore them.

### 1. Labels on impl-type-arguments

Any top-level class type-arg of an impl block may carry an optional `:label`:

```lyric
impl ArrayList<Team:roster, Player:team> {
    P.children <-> Team.roster_field
    C.parent   <-> Player.team_field
}
```

Members on the labeled side inject under the `<label>` sub-scope, accessed
via dotted form `obj.label.member`. Unlabeled side injects flat
(`obj.member`). Constraint: at most one *unlabeled* relation per `(P, C)`
pair — additional relations on the same pair MUST be labeled.

Same-letter labels across different type-vars are **non-colliding** because
each label lives in its own class's namespace:
```lyric
// Node participates in two intrusive linked lists, no collision:
impl DoublyLinked<Node:ready_q,   Node:ready_q_child>   { ... }
impl DoublyLinked<Node:blocked_q, Node:blocked_q_child> { ... }
```
This is the headline benefit — "same class in two intrusive linked lists
without inheritance" — and motivates the whole labels-on-impl-type-vars
redesign.

### 2. `owns` / `refs` on the impl block itself

An impl over a hint-shape interface may carry `owns` or `refs` between the
closing `>` and the opening `{`:

```lyric
impl ArrayList<Team:roster, Player:team> owns { }
impl Stack<Editor:undo, Action:owner>    refs { }
```

This is the underlying mechanism `relation` desugars to. Two cases where
the impl form is strictly more capable than the `relation` form:
- User-defined hint interfaces (not just the three stdlib hints).
- Impls that need a non-empty body for extra Alias / FieldBind / Inline
  mappings beyond what the desugar synthesizes.

When the impl body is empty, the desugar synthesizes per-side field
bindings from the hint interface's `field T.name: Type` declarations.

### 3. Destructor pairing

Hint interfaces declare paired destructors keyed on the relation kind:
```lyric
interface ArrayList<P, C> {
    field P.children: [C]
    // ...
    destructor owns P { ... }   // selected by `... owns ...`
    destructor owns C { ... }
    destructor refs P { ... }   // selected by `... refs ...`
    destructor refs C { ... }
}
```
The desugar copies only the matching pair onto the concrete classes. A bare
`destructor T { ... }` (no kind) defaults to `owns` — legacy shorthand.

## Sources of truth (read these BEFORE editing)

1. `cr/docs/lyric-language-reference.md` — daily-driver reference, post-Phase-3e
   reality. Read in full (~785 lines).
2. `cr/docs/lyric-language-spec.md` §Interfaces (line ~1036) through
   §Relations (line ~1370) — design intent. Note that the spec's §Relations
   subsections were just swept to match Phase 3e (commit `22955a6`); the
   reference and spec now agree.
3. `cr/docs/multi-class-interface-redesign.md` §3.1 (labels-as-scopes),
   §3.8 (labels-on-impl-type-vars), §3.9 (owns/refs-on-impl). These are
   the spec narratives for what shipped.
4. `src/parser/parser.ly` lines 656-720 — ground truth for impl syntax,
   including label and owns/refs annotation parsing.
5. `stdlib/std.ly` — actual interface bodies for ArrayList (line 11),
   DoublyLinked (line 114), HashedList (line 244).

When in doubt about whether something compiles, **write a small `/tmp/foo.ly`
program and run `./runly /tmp/foo.ly`** from the repo root. The `runly`
script wraps compile + gcc + run in one command. Empirical truth always wins
over speculation.

## Editing discipline

- **Surgical, not rewriting.** Bill explicitly asked for a "quick pass with
  direct edits." Fix what's wrong; don't restructure narrative.
- **Preserve voice and tone.** The book reads in a particular way — calm,
  technical, with specific concrete examples. Don't drift into
  documentation-style listing prose.
- **Preserve 🚧 callouts** unless the underlying limitation is gone. If
  Phase 3 fixed something the callout described, remove the callout.
- **Preserve example output blocks.** If you change an example, re-run it
  to update the expected output, don't speculate.
- **Don't add a "What changed" section.** The reader doesn't care about the
  history; they want the current language.
- **One file only.** Edit only the chapter file you were given.

## When you finish

Print a final line of the form `DONE: ch08 — N edits, M empirical verifications`
followed by a 3-6 line summary of the substantive changes (not just "fixed
typos"). This signals the orchestrator that your turn is complete.
