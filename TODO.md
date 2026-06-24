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

### Iteration sugar: `for x in obj` for any Iterable
- Today: must write `for x in obj.iter()` even when `obj` satisfies a generator-returning iteration interface (proposed `Iterable<P, C>`).
- Goal: any class satisfying an `Iterable`-shaped interface usable directly in `for`.
- Discovered while writing `testdata/graph.ly` and `testdata/tree.ly` — algorithm bodies read with extra `.iter()` noise everywhere.
- Found: 2026-06-21

### `let ref` on structs/classes
- Currently only meaningful for slices (skips scope-exit free)
- For classes, `let ref` should mean "borrowed handle, no RC inc/dec" — semantics TBD
- For structs, `let ref` should mean pointer-to-struct in C (actual zero-copy reference)

### `mut ref` function parameters
- Spec mentions `mut ref` for zero-copy write-through views
- Not yet implemented for function params, only for local bindings

## Known Tech Debt

### Generic AST visitor refactor
**Priority**: Hewitt's #1 recommended next sprint (see IDEAS.md). The
same shallow-copy-plus-mutation bug shape bit twice in one sprint
(2026-06-23): `deep_copy_type_expr` and `substitute_type_expr_copy`
both had stale "shallow is safe" comments predating the rich-
substitution path. Every duplicated walker is a future "forgot an
arm" bug; one centralized visitor structurally eliminates the class.

Multiple passes in desugar/checker/lir hand-roll the same recursive
walk of the AST, one match-arm per Expr/Stmt/TypeExpr variant:
- `substitute_type_params_rich_in_{block,stmt,expr,type_expr}` in
  desugar.ly
- `deep_copy_expr` / `deep_copy_block` in desugar.ly
- `rename_method_calls_in_{block,stmt,expr}` in desugar.ly
- TypeVar-leak scan in `validateAllExprsResolved` (checker.ly)
- (Probably more in lir/lowerer.ly and the optimizer.)

Each of these has — or will have — the same bug shape: an arm gets
forgotten for some Expr kind (Cast, Lambda, Match, StructLit,
IfElse, Try, TupleLit, StringInterp, ListLit, MapLit, Slice,
Unwrap, Is), the trailing `_ => {}` wildcard swallows it, and the
pass silently misses a sub-tree. The Cast/`0 as W`/W-leak bug
in default-method specialization (fixed 2026-06-23) was exactly
this shape and is unlikely to be the last.

**Right factoring** (centralize the per-variant enumeration):
- New file `src/ast/visit.ly` (or similar) exporting:
    - `for_each_subexpr(e: Expr, cb: fn(Expr))`
    - `for_each_subtypeexpr_in_expr(e: Expr, cb: fn(TypeExpr))`
    - `for_each_substmt(s: Stmt, cb: fn(Stmt))`
    - `for_each_expr_in_stmt(s: Stmt, cb: fn(Expr))`
    - `walk_exprs_deep(root, cb)` / `walk_typeexprs_deep_*` /
      `walk_stmts_deep_*` built on the above.
- Rewrite each consumer above as a one-liner over the appropriate
  `walk_*_deep`. Forgetting a variant in ONE centralized visitor
  is a real diagnostic (entire AST kind silently unwalked),
  vastly more visible than the current N×M silent drops.
- The AST is full Lyric — relation back-pointers (e.g.
  `Block:bs owns [Stmt:b]`) handle parent walks orthogonally.

Expected size: ~150 LOC for the visitor primitives + delete ~500
LOC across the consumers above. Bug-prevention value: structural
elimination of an entire class of "forgot a case" defects.

### Multi-class interface default-method alias resolution
SHIPPED 2026-06-23 (see commit log). `FuncDecl.source_impl` added; pass 4.5
sets it; checker `try_resolve_impl_alias_method` consults it FIRST in dot
dispatch (Bill: aliases must trump registry lookup to avoid silent shadowing).
No-op-rename guard prevents infinite recursion on auto-derive-style bindings.
Bug found and fixed in the same sprint: `deep_copy_type_expr` was shallow,
causing rich-substitution mutations to leak across multiple per-impl
specializations of the same default method.

### Explicit type-binding in impl alias mappings (de-jank)
Currently impl alias mappings like

```
impl<W> WeightedDirectedGraph<FlexNet<W>, FlexRoute<W>, FlexVia<W>, W> {
    N.outgoing_edges = FlexRoute.all_outgoing
}
```

work by NAME COINCIDENCE only — `FlexRoute.all_outgoing` (whose return
type uses `W` from the class) happens to thread W through because of
string matching, NOT because of any type-level binding declared in the
alias. If the helper used `V` instead of `W` in its declaration the
impl wouldn't compile, but conversely: if the helper used a different
SHAPE (e.g. function-level `<U>`), the impl wouldn't know what to
bind U to.

Bill's design (2026-06-23): the alias RHS should carry explicit
type-args so the binding is in the source, not implicit. Two
candidate syntaxes both expressing "specialize this callable for W":

```
N.outgoing_edges = FlexRoute<W>.all_outgoing       // class-instantiation form
N.outgoing_edges = FlexRoute.all_outgoing<W>       // function-side type-arg form
```

The class-instantiation form is more honest when V belongs to the
class (`class FlexRoute<V> { ... }`), since V is a class type-param
not a function one. The function-side form matches existing stdlib
patterns (`Dict.set<K, V>`). Pick one and require it.

Tied to this: the user-side restructure of graph.ly Part 4 currently
lives in tree as `class FlexNet<W>` / `FlexRoute<W>` / `FlexVia<W>`
with the alias `= FlexRoute.all_outgoing` (no explicit binding —
relies on the W string-match coincidence). Once explicit binding
lands, the alias becomes `= FlexRoute<W>.all_outgoing` (or fn-side
equivalent) and we can also rename the class-side param to `V` as a
test that the compiler doesn't conflate by name.

Scope: parser (alias-RHS type-args), checker (resolve parametric
callable, thread bindings through `try_resolve_impl_alias_method`),
possibly desugar (impl's per-binding rich substitution context),
monomorphizer (specialize alias targets with explicit args). Easily
200+ LOC; warrants its own sprint. Without it, graph.ly Part 4
ALSO fails at gcc — `FlexVia_dst_route` returns `int` instead of
`FlexRoute_i32*` because monomorphization can't see the binding.

### Reject undeclared type identifiers in user signatures (de-jank)
Currently `resolve_named_type` silently promotes any unknown identifier
to a TypeVar (line 1234 of checker.ly: "Unknown type — treat as type
variable (matches Go checker behavior)"). This masks user typos
(`FlexVia<Wieght>` becomes a synthetic generic over Wieght) and lets
undeclared free vars surface only at the validator stage as cryptic
"TypeVar leak" messages.

Bill's call (2026-06-23): a Named identifier in a type position should
error with a clear "undefined type identifier" message unless it is
a primitive, a registered type, or a declared type-param (function's
own `<T>`, receiver class's `<U>`, or a where-clause variable).

Attempted in-sprint: a `validate_func_signature` pass in `register_func`
broke ~25 tests including stdlib usages. The implementation needs to
handle: (a) registration ORDER (Dict.set<K, V> may be checked before
Dict's class type-params are populated in the registry), (b)
relation-decl-synthesized helpers, (c) interface-field-derived
getter/setter synthesis spans, (d) maybe more. Estimate: 100-200 LOC
of careful sequencing once registration order is understood.

Coupled with that work: the SYNTAX for declaring type-params on an
external method of a CONCRETE class is `pub func Class.method<T>(...)`
(type-params AFTER method name); for an external method whose
RECEIVER is itself a type-var the form is `pub func<T> T.method(...)`.
Both forms exist in the codebase (stdlib for the former, tree.ly for
the latter); the parser accepts both. Once the strict-reject lands,
the error message should suggest the right form based on receiver kind.

### Multi-class interface default-method alias resolution (original entry)Default methods declared on a multi-class interface and specialized
into concrete classes (desugar pass 4.5) currently FAIL to resolve
alias-form impl bindings inside the specialized body. Example:

```lyric
interface DG<G, N, E> {
    pub func N.outgoing_edges(self) -> [E]
    pub func G.bfs(self, start: N) -> [N] {
        // ...
        for e in n.outgoing_edges() { ... }   // ← unknown method
    }
}
impl DG<Net, Route, Via> {
    N.outgoing_edges = Route.outgoing_a    // alias-form
    // ...
}
```

After desugar pass 4.5 specializes bfs for `<Net, Route, Via>`,
the specialized body's `n.outgoing_edges()` (n: Route) errors with
"unknown method: outgoing_edges" because Phase 1.5b only registers
the interface abstract NAME on the concrete class when the impl
is empty-body / direct-match — alias-form bindings register the
TARGET name (`Route.outgoing_a`), not the interface name.

**Design (Bill 2026-06-23):** desugar should not substitute method
call names (it lacks types) — instead, attach the per-impl alias
rename table to each specialized FuncDecl as a back-pointer to the
source ImplBlock. The checker, when resolving `expr.member` inside
a function whose `source_impl != null`, consults the impl's
bindings first: for each `IfaceTypeVar.member = ConcreteClass.alias`
binding whose (substituted) class matches the receiver and whose
member matches the call, rewrite to the alias target. Handles both
method-alias and field-alias (field auto-getter) forms uniformly.

Add `FuncDecl.source_impl: ImplBlock?` (null for normal funcs).
Set in desugar pass 4.5 when deep-copying default-method bodies.
Consult in checker method-resolution + field-access dispatch
before falling through to normal lookup.

This unblocks `testdata/graph.ly` Parts 2-4. Without it, graph.ly
remains baseline FAIL.

### Built-in constraints hard-coded in checker
- `Comparable`, `Equatable`, `Hashable`, and (planned) `Numeric` are
  recognized by string-match in `satisfies_constraint` (zero callers
  today) and were previously enforced by `validate_where_clauses`
  (now disabled — call commented out in `check_program`).
- They should be defined as **marker interfaces** (zero methods) in
  `stdlib/std.ly`, e.g. `interface Numeric<T> { }`, so:
    1. They appear in `iface_decls` like any other interface.
    2. `satisfies_constraint` becomes a structural lookup (iface +
       type → built-in dispatch table for primitive admission)
       instead of a string-match.
    3. `grant_relational_methods` no longer needs the marker-skip
       branch — empty method set falls through naturally.
    4. `validate_where_clauses` can be re-enabled with the standard
       `iface_decls.has(name)` check covering all four uniformly.
- Until this lands, `where Numeric<W>` compiles because
  `grant_relational_methods` skips cleanly and the validator is
  off — but typos like `where Comparible<T>` silently succeed.

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


---

## Spec / Reference Cleanup Findings (2026-06-20)

The spec and reference were rewritten this session against an exhaustive read of the compiler source, stdlib, and testdata. The items below are language-level discrepancies discovered during that audit. Most should be small surgical fixes; a few are roadmap-shaped.

### Type system

- [ ] **`null` is assignable to every type** (including primitives). Spec promise is `T?` / class / interface / error only. Tighten `is_assignable` Nil case in checker.
- [ ] **`as` cast accepts any target type** (not just numeric ↔ numeric). Either tighten `check_cast` to numeric ↔ numeric, or change semantics to "wide cast" and document.
- [ ] **Cross-sign integer assignment is implicit and lossy**. `numeric_widens` allows i32 ↔ u8 silently. Should require explicit `as` for narrowing / cross-sign.
- [ ] **`implements I` is declarative only** — `check_implements` just appends to a list; it does NOT verify the class has the interface's methods. Add method-presence checks.
- [ ] **`if` / `match` as expressions don't unify branch types**. Today the checker returns the first branch's type without comparing. Add unification.
- [ ] **Lambdas cannot be generic** (`type_param_names` always empty). Either implement or document explicitly.

### Operators

- [ ] **Add `~` (unary bitwise NOT).** Lexer has no token; LIR `UnBitNot` exists but is unreachable.
- [ ] **Add compound bit assigns**: `%= &= |= ^= <<= >>=`.
- [ ] **Fix bitwise-operator precedence** — promote `& | ^ << >>` above `==`, `!=`, `<`, `<=`, `>`, `>=`, `&&`, `||`. Currently inherits C's broken precedence (`a & 1 == 0` parses as `a & (1 == 0)`). Bitwise ops take integers and return integers — arithmetic tier, not boolean tier.

### Literals & lexer

- [ ] **Hex / octal / binary integer literals** (`0xFF`, `0o755`, `0b1010`).
- [ ] **Integer type suffixes** (`123u64`).
- [ ] **`\u{NNNN}` Unicode escapes** (precondition for UTF-8 layer).
- [ ] **Block comments** `/* ... */`.

### UTF-8

- [ ] **Full UTF-8 string layer.** Today `string` is a byte slice. Need: code-point iteration (`for c in s.chars()`), Unicode-aware case ops, normalization, `\u` escapes. Keep `string` as the type name; UTF-8 sits on top.

### Obsolete / phantom syntax to remove

- [ ] **Remove `cascade { body }` from the language.** Obsolete from an earlier design — we use `owns` / `refs` on relations for cascade semantics. Today the parser accepts it and the lowerer treats it as a plain block (no-op). Remove from lexer / parser / AST / checker / lowerer.
- [ ] **Remove `defer` from LIR / AST.** No lexer token, no parser path, no use site. `StDefer` + `LDeferData` are phantom kinds.
- [ ] **Remove `Mutex` (capital M) recognition** in `lowerer.lower_named_type`. Standardize on lowercase `lock`. Bill confirmed.
- [ ] **Rename `KNil` TokenKind / `Nil` ExprKind to `KNull` / `Null`** for honesty. The kind name is historical (from when `nil` was the keyword); today it's emitted for `null`.
- [ ] **`KNil` switch branch in lexer that never fires** — confirm and remove if dead.
- [ ] **`map[K]V`** literal parses but the C backend emits `void* /* map */` and `make_map` returns NULL with "not supported". Either implement or remove the syntax.
- [ ] **Remove the case-sensitive parser hack at `src/parser/expr_parser.ly:865-877`.** `is_pattern_let_ahead()` keys off ASCII A-Z (`first < 65 || first > 90`) to disambiguate variant-pattern let-else (`let Foo(x) = ... else { ... }`) from regular let. This is the **only** place in the compiler where identifier case carries semantic weight — and the spec promises case is not part of the language. The disambiguation can be done without case: at let-statement level, `let Ident (` followed by a parseable pattern is unambiguously a pattern-let (a regular let cannot have a call-shaped LHS). Drop the A-Z check; rely on `peek().kind == PLParen` alone.
- [ ] **`check_return` doesn't verify return-value type against declared return type** (`src/checker/checker.ly:2706-2724`). It type-checks the expression in isolation and propagates type hints to `[]`/`null` literals, but never calls `is_assignable(value_type, declared_return)`. Result: `return (null, "empty")` against `(Item?, error)` compiles cleanly even though `string` is not an `error`. The lowerer happily emits the wrong-typed bytes into a multi-value return and the C backend produces UB. Fix: add the `is_assignable` check; expect to need to migrate ~half a dozen testdata files (e.g. `testdata/try_loop.ly:10`) that exploit the hole.
- [ ] **Optional-class field access null-deref is a C segfault, not a Lyric panic.** When `x: T?` for class `T`, `x.field` type-checks (the checker auto-unwraps Optional in `resolve_field_type` at `checker.ly:4017-4019`), and the lowerer emits a direct field access on the class handle. If the handle is null at runtime, the C backend segfaults — there is no Lyric-level panic. Fix: emit an explicit `is_null` check before the field access on optional-class receivers, with a Lyric panic message ("nil deref on x.field at file:line") matching `expr!`'s behavior.

### Imports

- [ ] **Bare `import "path"`** form null-derefs in `modules.ly:36` when `imp.alias` is null. Either parser should reject, or derive alias from path basename, or document as unsupported.
- [ ] **Recursive import resolution**. Today only the root file's imports are processed; imports of imports are silently dropped.
- [ ] **`pub`-filtering on imports**. All declarations are accessible regardless of `pub`. Implement.
- [ ] **Circular import detection.**
- [ ] **Parse `lyric.mod` content.** Today only the file's existence matters.
- [ ] **Recursive subdirectory `.ly` discovery in module mode.**

### Desugar / silent skips

- [ ] **Error on undefined `embed Iface`.** Today silently skipped.
- [ ] **Error on bad `relation Hint Parent ...` hint.** Today silently skipped if hint isn't an interface or has < 2 type params.
- [ ] **`Lock()` Stmt and `Match()` Expr enum-variant name collisions** cause shallow-copy fallbacks in `deep_copy_stmt` / `deep_copy_expr`. Fix when enum-variant naming gets scope-qualified, OR rename one of the colliding kinds.

### Function annotations (all currently roadmap)

The entire function-annotation table (`requires:`, `ensures:`, `raises:`, `concurrent:`, `requires_lock`, `excludes_lock`, `spawns:`, `pure:`) is not parsed today. Spec marks them roadmap; need a separate sprint to implement.

- [ ] **Implement `destroys` annotation** — compiler-inferred mark on functions that may destroy class instances; static UAF prevention.
- [ ] **Implement `mut resize` annotation** — prevent element-access during resize.

### Larger numeric tower

- [ ] **Add `i128 i256 u128 u256 f128`** — register in checker, add LType variants, emit via `__int128` / compiler-rt.

### Built-ins not in spec/reference

- [ ] **Decide on Go-style stdlib aliases** (`fmt.Println`, `fmt.Printf`, `fmt.Sprintf`, `fmt.Errorf`, `errors.New`, `strconv.Itoa`, `strings.ToUpper/ToLower/Contains`) — they're hardcoded in `c_backend.emit_call_expr` and actually emit working code. Either officially keep as legacy aliases (and document) or remove.

### Test runner

- [ ] **Add per-test timing** to test runner output (or remove `(0.1ms)` from documented examples). Today the C runner just prints `PASS  name` / `FAIL  name`.
- [ ] **Add `assert_eq_approx`** for floating-point comparison with tolerance.
- [ ] **Test filtering** (`lyric test --filter pattern`).

### Stale internal docs

- [ ] **`lyric.lyric`** is heavily stale — describes the old Go-compiler layout (`pkg/ast/`, `pkg/checker/`, ...) and claims "Bootstrap: rewrite Lyric compiler in Lyric (IN PROGRESS)". Rewrite for the current `src/` layout.
- [ ] **`stdlib/stdlib.lyric`** header references `pkg/ast/stdlib.go`. Update to point at the self-hosted equivalents.

### Documentation cleanups (already done in this session — listed for the record)

- [x] **CDD layer removal** — `why:`, `doc`, `invariant:`, `verified_at:`, `source:`, `fake:`, three-zone files. Removed from spec/reference (they live in `lyre`).
- [x] **Remove `lyric verify` / `lyric update` / `lyric gen`** from spec/reference toolchain section (Bill: moved to `lyre`).
- [x] **Drop Go-backend bullets** from spec generators / unions section (Go backend retired).
- [x] **Drop `f128 / i128 / ...` from "current primitive types"** in spec/reference; moved to roadmap.
- [x] **Fix "Both null and null accepted" reference typo** — was a copy-paste, real answer is just `null`.
- [x] **Document `T -> U` single-arg function type** and `func(T) -> R` synonyms.
- [x] **Document context-driven enum-variant disambiguation.**
- [x] **Document `recv` / `length` / `starts_with` / `ends_with` synonyms.**
- [x] **Publish the operator precedence table** in both spec and reference.
- [x] **Document inferred move semantics.**
- [x] **Document auto-generated `destroy` for every non-permanent class.**
- [x] **Document `permanent` + relation-owned warning.**
- [x] **Honest current state of `implements`, `as`, `if`/`match` branch unification.**
- [x] **Honest current state of imports** (single-level, no pub filtering, no cycle detection).
- [x] **Document `--soa`, `--detect-uaf`, `--rc-free`, `--no-rc`, `--lir-dump`** compile flags.
