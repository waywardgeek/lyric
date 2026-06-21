# Book Overhaul — Findings

*Append-only notes from chapter revisers about spec/reference issues discovered during the overhaul. Not blockers — captured here so they can be fixed in the spec/reference later.*

---

## Ch 1 reviser, 2026-06-21

### Spec under-documents implicit numeric widening (`lyric-language-spec.md` §Type Casts, line ~1473)

The spec currently says:

> **Implicit numeric widening:** smaller integer types widen to larger ones without an `as`. Cross-sign integer assignment (`i32` ↔ `u8`) is also implicit today — a footgun the roadmap intends to address.

But the compiler **also** does implicit int→float widening (e.g., an `i32` argument is accepted where `f64` is expected, the compiler inserts the cast). Confirmed by orchestrator (Hewitt) during Ch 1 revision. The spec should add a line: "Integer types also widen to `f32`/`f64` without an `as`." Until the spec is updated, Ch 1 §1.2 documents this behavior anyway because it's load-bearing for natural-looking numeric code.

---

## Ch 2 reviser, 2026-06-21

### Positional struct literal in a bare `let` is rejected (book Ch 2 §2.1 was wrong)

Pre-edit Ch 2 §2.1 claimed:

> Positional construction only works inside parentheses, function arguments, or list literals — contexts where the parser can distinguish a struct literal from a code block. A standalone `let p = Point { 10, 20 }` works because it follows `=`.

The second sentence is **false**. `testdata/positional_struct_lit.ly` deliberately exercises positional struct literals inside a tuple (`(Point { 10, 20 }, 42)`), as an arg (`make_pair(Pair { "hello", 99 })`), and inside a list literal (`[Point { 1, 2 }, Point { 3, 4 }]`) — and uses the **named** form (`Point { x: 5, y: 6 }`) for the bare `let p = ...` case. The parser's `expr_depth > 0` gate disallows the bare form. Spec §Structs is correct (lists the three contexts); the book sentence was a fabrication.

Fix in book: §2.1 now shows positional construction only in arg/tuple/list contexts and keeps the bare `let` on the named form.

### Ch 3 references "Token **struct** from Chapter 2" but Ch 2 defines `Token` as an **enum**

Ch 3 §3.3 says "The Token struct from Chapter 2 is the right choice for tokens". Ch 2 defines `Token` as an enum (and Ch 4 §4.4 explicitly says "In Chapter 2, we defined `Token` as an enum with payloads" before redesigning it into a `TokenKind` enum + `Token` struct). The mismatch is in Ch 3, not Ch 2 — Ch 2 reviser leaves Ch 2's `Token` as an enum (Ch 4 depends on that history) and flags the Ch 3 line for the Ch 3 reviser.

---

## Ch 3 reviser, 2026-06-21

### Lvalue write-through on a struct-typed Optional silently drops the write

Spec §Lvalue Unwrap and Write-Through (around line 1543) gives this example:

```lyric
class Outer { data: Inner? }
struct Inner { value: i32 }

let o = Outer { data: Inner { value: 0 } }
o.data!.value = 42        // writes through the optional unwrap
assert_eq(o.data!.value, 42)
```

Empirical: this compiles, runs, and prints `value: 0` — the write to `o.data!.value` is silently lost. The cause is that struct optionals use a tagged representation; `expr!` produces an rvalue copy, and assigning into a field of that copy doesn't propagate back to `o.data`. The same example works correctly if `Inner` is a `class` (output: `42`).

Spec says auto-deref applies "only to class optionals" (line 1553) — but it doesn't extend that "class only" caveat to the lvalue-unwrap section, which presents the struct form as the canonical example. Recommend the spec either (a) constrain the lvalue example to class-typed inner, or (b) the compiler implement true lvalue write-through for struct-typed optionals.

Logged in `~/projects/lyric/TODO`.

---

## Ch 4 reviser, 2026-06-21

### `xs.extend(ys)` is a silent no-op on slices

Spec §Built-in Methods §Slices (line ~1855) lists `extend(other) -> unit` as "In-place append-all", and §Composite Types says "In-place slice extension: `xs.extend(ys)`." Empirical:

```lyric
let mut xs = [1, 2, 3]
xs.extend([4, 5, 6])
println(f"after extend: {xs.len()}")  // prints 3, not 6
```

Same result whether `xs` starts as a literal or is built up with `.push()`. The `append(xs, elem)` built-in (which returns a new slice and requires `xs = append(xs, elem)`) works correctly — `[1,2,3]` + two `append` calls → `len=5`. So the workaround for the book is to use `.push()` in a loop, or repeated `append(xs, elem)`, instead of `.extend()`.

Spec doesn't say `extend` is 🚧 — it's listed as if implemented. Either the method needs to be wired up to actually append, or the spec needs to demote it to 🚧 and the `len(xs).extend` row removed from §Built-in Methods.

Logged in `~/projects/lyric/TODO`. Book §4.3 demotes `.extend()` to 🚧 and shows the working forms (`push` in a loop, or `+` for concatenation).

### `;` is not a statement separator

Cosmetic but tripped me up while writing a test: `ys.push(1); ys.push(2)` produces `undefined variable: ;`. The spec implies one statement per line and doesn't enumerate `;` as legal; this is the empirical confirmation. Not a bug, just a gotcha — and worth documenting because most C-family programmers reach for `;` instinctively when squeezing onto one line in a snippet. Book §3.4 now uses `class Inner` and adds a 🚧 callout for the struct case.

### Spec lists `fn(T) -> U` as canonical function-type syntax, but the parser rejects it

Spec §Composite Types line 393 says:

> - `fn(T, U) -> V` — canonical
> - `func(T, U) -> V` — also accepted
> - `T -> U` — single-argument shorthand

Empirical: `func apply(x: i32, f: fn(i32) -> i32) -> i32 { ... }` fails to parse with `expected identifier, got PLParen (()` at the `(` after `fn`. `func(i32) -> i32` parses fine. So today only `func(...)` works; `fn(...)` is documentation-only.

Either the parser needs to accept `fn` as a type-position keyword (matching the spec's "preferred" guidance), or the spec needs to swap the canonical/accepted order and demote `fn` to 🚧. Book Ch 3 §3.8 uses `func(i32) -> i32`, which works today.

Logged in `~/projects/lyric/TODO`.

---

## Ch 5 reviser, 2026-06-21

### `new_error(msg)` is checker-only — no C-backend lowering

Spec §Built-in Functions §String / Conversion (line ~1780) lists:

> `new_error(msg)` | `string -> error` | Build an `Error`

The checker types it correctly (`src/checker/checker.ly:3609` → `make_error_type()`), but the C backend emits a literal `new_error(LYRIC_STR(...))` call with no corresponding declaration or definition. gcc rejects with `implicit declaration of function 'new_error'`. The working alternative is the stdlib class literal `Error { msg: "..." }`, which compiles + runs end-to-end (verified). Either implement the lowering as sugar for `Error { msg: ... }`, or demote in the spec.

Book §5.4 introduces `Error { msg: ... }` and adds a 🚧 callout for `new_error`. Logged in `~/projects/lyric/TODO`.

### Interface dispatch on `error`-typed values doesn't work; f-string interpolation does

`err.message()` where `err: error` (e.g. from a `(T, error)` destructure) passes the checker but the C backend models `error` as `const char*` and generates `Error_message((const char*)err)` against the concrete `Error_message(Error*)` signature — gcc rejects with "incompatible types". The chapter teaches `f"{err}"` for stringifying errors, which works because the f-string lowerer has a dedicated path for the `error` type.

Subtler corollary: f-string interpolation of an `error` value always pulls the `Error.msg` field, regardless of the dynamic type. A custom `class ParseError { line, col, msg; pub func message(self) -> string { f"{line}:{col}: {msg}" } }` will print just `msg` through `f"{err}"`, silently dropping the user-defined `message()` formatting. Both bugs share the same root cause (no real interface dispatch / vtable for `error`).

Logged in `~/projects/lyric/TODO`.

### Bare `return` inside `main()` is a compile error

`func main() { ...; if cond { return } ... }` fails to compile because the C backend lowers `func main()` as `int main(int _argc, char** _argv)` and a Lyric `return` with no value becomes a bare `return;`, which gcc rejects with `'return' with no value, in function returning non-void`. Workaround in the book: restructure with `if/else` instead of early `return`. The §5.1 `main` was refactored accordingly. Logged in `~/projects/lyric/TODO`.

### `char_is_space` / `char_is_digit` C-backend lowering is missing

While trying to compile the full Ch 4 → Ch 5 calculator pipeline end-to-end, the tokenizer (which uses `char_is_space(b)` and `char_is_digit(b)`) failed to compile: `lyric compile` printed `c_backend: unhandled builtin: char_is_space` and `c_backend: unhandled builtin: char_is_digit`, and the generated C contained lines like `bool _t4 = ;` (assignment with empty RHS) that gcc rejects. The checker accepts the calls; only the lowerer is missing arms. The other `char_is_*` predicates listed in spec §Built-in Functions probably need an audit too.

This blocks the Ch 4 tokenizer example from compiling end-to-end. Ch 5's parser was verified against a hand-built `[Token]` literal as a workaround. Logged in `~/projects/lyric/TODO`. The Ch 4 reviser should be alerted — their tokenizer example is currently aspirational rather than verified.

### Ch 6's pre-edit opening over-promises what Ch 5 delivers

Ch 6's first paragraph claims: "Our calculator parses and evaluates expressions, handles errors, **and reports line and column numbers**." Ch 5 introduces `ParseError { line, col, msg }` as a custom-error *example* in §5.5, but the actual calculator's `Parser` keeps the simple `Error { msg }` form — there's no `Lexer` carrying source positions yet. The Ch 6 reviser should either soften that opening to match Ch 5's actual state, or add a small "Lexer with positions" step at the start of Ch 6 that lifts the §5.5 `ParseError` into the parser. The Ch 4 carry-forward note already anticipated this: "A `Lexer` class with line/column tracking is a natural extension for Ch 5 or later when error messages need source positions." Ch 5 chose *later*; Ch 6 needs to choose now.

---

## Ch 6 reviser, 2026-06-21

### Generic class methods that access `self.<field>` lower to a null receiver

Spec §Generics (under "Generics" on the class side, line ~967) and §Class Generics show the canonical form:

```lyric
class Pair<T> {
    first: T
    second: T
}
let p = Pair<i32> { first: 1, second: 2 }
```

Field *access* on `p` works. But the moment a method on a generic class touches `self.<field>`, the C backend drops the receiver pointer. Minimal repro:

```lyric
class Stack<T> {
    items: [T]
    pub func push(self, item: T) {
        self.items.push(item)
    }
    pub func len(self) -> i32 {
        return self.items.len()
    }
}

func main() {
    let empty: [f64] = []
    let mut s = Stack<f64> { items: empty }
    s.push(1.0)
    println(f"len: {s.len()}")
}
```

`./lyric compile` succeeds, but `gcc` errors on lines like:

```c
lyric_push(&/* null value */->items, item, LyricSlice_double);
```

The literal text `/* null value */` is in the generated C — i.e., the lowerer knows it has nothing to write for the receiver and emits a placeholder comment instead of `(self)`. Same shape for `pop` / `peek` (`lyric_pop(&/* null value */->items)`). Non-generic classes work fine; the bug is specific to monomorphized methods on generic classes.

Secondary finding from the same investigation: the untyped empty slice literal does not seed type-variable inference for a generic class constructor.

```lyric
let mut s = Stack<f64> { items: [] }   // checker: TypeVar leak 'T' in main
```

Both bugs logged in `~/projects/lyric/TODO`. The book's Ch 6 §6.9 `Stack<T>` example is preserved as illustrative with an explicit 🚧 callout; the chapter's working generic code is all *free functions* (`max_val<T: Comparable>`, `identity<T>`, `first<T>`, `print_it<T: Printable>` — all compile + run end-to-end, verified).

---

## Ch 7 reviser, 2026-06-21

### `assert` and `assert_eq` builtins are no-ops in the C backend

**Severity: critical.** The compiler defines runtime macros `lyric_assert(cond, msg, file, line)` and `lyric_assert_eq(eq, actual_str, expected_str, msg, file, line)` in the generated C header (both with the correct FAIL output format from spec §Testing and both calling `exit(1)` on failure), but the lowerer never emits a call to either macro. Repro:

```lyric
func test_boom() {
    assert(false, "should fail")
}
```

Generated C body: `void test_boom(void) {}` — the `assert` call is dropped on the floor. Same for `assert_eq(1, 2, "boom")`. Same drop happens in non-test functions: `func main() { println("before"); assert(false, "x"); println("after") }` prints both lines and exits 0.

Consequence: every Lyric test silently passes regardless of correctness. The 78 tests in the compiler's own test suite (§7.6 references `test_field_generates_getter_and_setter` and the spec advertises 78 tests) provide zero regression protection today.

The checker accepts the calls correctly; only the lowerer is missing arms for the `assert` / `assert_eq` builtins. Fix: lower `assert(cond, msg)` to `lyric_assert(cond, msg, __FILE__, __LINE__)` and `assert_eq(a, b, msg?)` to `lyric_assert_eq(a == b, to_string(a), to_string(b), msg ?? "", __FILE__, __LINE__)`.

Logged in `~/projects/lyric/TODO`. Hewitt indicated he would fix this overnight; per his instruction the Ch 7 chapter is written as if the lowering works.

### `(T, error)` destructure on self-recursive method calls emits malformed C

Discovered while building the Ch 5 calculator to verify Ch 7 tests against it. In a method like `Parser.parse_primary(self) -> (f64, error)`, replacing the working `?` form with an explicit destructure of a self-recursive call:

```lyric
let (val, err) = self.parse_expr()    // self.parse_expr returns (f64, error)
if err != null {
    return (0.0, err)
}
```

emits this C:

```c
double val = _t14;
const char* err = _t14;
```

where `_t14` is typed `LyricResult_double` (the tuple-return temp). gcc rejects with `incompatible types when initializing type 'double' using type 'LyricResult_double'`.

The non-recursive destructure form works (e.g. `main` calling `parse()` and destructuring its `(f64, error)` return), and the `?` form on the recursive call works. So the bug is the combination of (a) destructuring a `(T, error)` return, (b) inside a method whose own return type is `(T, error)`, (c) when the called method is `self.<recursive>`. The lowerer reuses the tuple-temp slot directly instead of unpacking it into the named locals.

Workaround: use `?` instead of explicit destructure for self-recursive `(T, error)` calls. The Ch 5 calculator's `parse_primary` already uses `?` here, so the calculator compiles fine.

Logged in `~/projects/lyric/TODO`.

---

## Ch 8 reviser, 2026-06-21

### `final func` fires twice when `.destroy()` is called explicitly on a stack-local class

Spec §Class Destruction documents the execution order as `final → auto-destructor (cascade + unlink) → slab free`, but doesn't say whether `final` is one-shot or whether explicit `.destroy()` interacts with the implicit scope-exit teardown.

Empirical repro:

```lyric
class Connection {
  name: string
  final func cleanup(self) {
    println(f"closing {self.name}")
  }
}

func main() {
  let c = Connection { name: "db" }
  println("before destroy")
  c.destroy()
  println("after destroy")
}
```

Output:
```
before destroy
closing db
after destroy
closing db
```

`cleanup` fires twice — once for the explicit `c.destroy()`, once again on scope-exit cleanup of the now-freed slot. Omitting the explicit `c.destroy()` (relying on scope exit alone) produces exactly one `closing db`, which is the workaround used in Ch 8 §8.7.

Fix candidates: (a) the explicit `.destroy()` should mark the slab slot so scope-exit doesn't re-fire `final`; (b) `final` should carry an idempotency guard checked at the top of its emitted body. The spec should also be tightened to say `final` is one-shot, so the contract is unambiguous regardless of which fix lands.

Logged in `~/projects/lyric/TODO`.

### Stdlib reality has diverged from spec/book examples around `ArrayList` / `ArrayListBase`

Pre-edit Ch 8 §8.2 showed a monolithic `ArrayList<P, C>` interface containing both the `array_append` / `array_remove` functions and the destructors. The current `stdlib/std.ly` factors this differently:

- `ArrayListBase<P, C>` holds the fields and *both* forms of the operation: free-function (`array_append(parent, child)`, `array_remove(child)`) **and** method-form (`P.append(self, child)`, `P.remove(self, child)`). All four are marked `pub trusted func` because they reach into `ref child` / `unref child`.
- `ArrayList<P, C>` `embed`s `ArrayListBase` and adds the cascade destructors.
- `RefArrayList<P, C>` `embed`s the same base and adds the non-cascading destructors — the chapter's pre-edit "four relation types" table doesn't mention this one (the spec's §Standard Library Reference also doesn't list `RefArrayList`).

The same factoring applies on the doubly-linked side: `DoublyLinked<P, C>` is the base, `OwningList<P, C>` and `RefList<P, C>` embed it. The book §8.3 already names this correctly.

Spec §Relations (§ArrayList — Dynamic Array Ownership) shows only the free-function form (`array_append<Team, Player>(t, p)`); the spec should be updated to also show the method form (`t.roster_append(p)`) since that's the carry-forward's preferred form *and* it's what the stdlib actually generates today. The spec should also probably document `ArrayListBase` and `RefArrayList` so the four-vs-five-types discrepancy doesn't surprise readers who go from the book to the spec.

No compiler bug — the stdlib just got more sophisticated than the docs. Book §8.2 now shows the real factoring; logged here so the spec can follow.

---

## Ch 9 reviser, 2026-06-21

### Spec promises default-method method-call syntax that the checker doesn't resolve

Spec §Default Methods says of `pub func count_edges(graph: G) -> i32 { ... }` inside `interface Graph<G, N, E>`:

> Callers invoke it via method syntax (`graph.count_edges()`) when the interface is implemented on the receiver's type.

Empirical (extracted from `testdata/interfaces.ly`'s working `Graph` example, kept identical except for the call site):

```lyric
let count = g.count_edges()    // checker: unknown method: count_edges
```

The checker rejects `g.count_edges()` for any `g` of a concrete type that has a `Graph<G, N, E>` impl block. The free-function form `count_edges<G, N, E>(g)` resolves and runs correctly (and is what `testdata/interfaces.ly` uses for `dll_append<Folder, File>(dir, f)`). So the spec's "method syntax for default methods" line is aspirational; only free-function-with-explicit-type-args works today.

Same shape for default methods declared with the `P.` receiver-binding form (`pub func P.add(self, child: C) { ... }` inside `interface MyList<P, C>` with `relation MyList Panel:w owns [Widget:p]`): neither `panel.add(w1)` nor `panel.w_add(w1)` resolves; the underlying `Panel_add` / `Panel_w_add` C symbol is never emitted. By contrast, the stdlib hint relations (`ArrayList`, `OwningList`, `RefList`) *do* wire up label-prefixed methods (`team.roster_append(player)`, `dir.files_append(file)`) — that machinery is specific to the four stdlib hints, not a generic feature of `relation` over any binary interface.

Book §9.3 was rewritten to use the working pattern: top-level default function (`pub func add(parent: P, child: C)`), getter/setter form inside the body (`parent.items()` / `parent.set_items(...)`), free-function call at the call site (`add<Panel, Widget>(panel, w1)`), and an explicit 🚧 callout pointing readers at the stdlib hints when they want method-call ergonomics today.

Logged in `~/projects/lyric/TODO`.

### Spec §Default Methods Are Label-Prefixed overstates the user-defined-hint story

The spec explicitly says:

> A relation's hint interface can declare default methods. The desugar pass binds those methods onto the parent type with the parent label as prefix… `panel.w_add(widget)` and `panel.w_count()`.

This is true for the stdlib hints. For a user-defined `interface MyList<P, C>` used as a relation hint, the desugar pass does *not* inject label-prefixed methods — the checker reports `unknown method: w_add` and the abstract `field P.items` is not visible inside the default method body (`field items not found on type` when the body says `self.items` or `parent.items`). Verified end-to-end against `relation MyList Panel:w owns [Widget:p]` with three call shapes (`panel.add`, `panel.w_add`, free-function `add<Panel,Widget>(panel, w1)`); only the free-function form passes the checker, and even then the body must use getter/setter form (`parent.items()` not `parent.items`).

Two ways to bring spec and reality into agreement: (a) extend the relation-desugar to inject label-prefixed methods for any binary interface used as a hint (the spec's intent); (b) demote the "any binary interface" framing and explicitly say only the four stdlib hints get method-injection today, with the rest of the family on the roadmap. The book takes path (b) for now.

Logged in `~/projects/lyric/TODO`.

### `self: P` parameter syntax inside interface default methods is a parser error

Tried `pub func add(self: P, child: C) { ... }` inside an interface body to make the "self is the parent" hint explicit. Parser rejects with `expected identifier, got PColon`. Two workarounds both compile: (a) the receiver-binding declaration `pub func P.add(self, child: C)` (where `self` is implicitly typed as `P`); (b) a plain top-level default with a named parameter (`pub func add(parent: P, child: C)`). The book uses (b) because it matches the free-function call-site form readers must use today. Not a high-priority spec gap, but worth a line saying `self` is only valid as a receiver in the `Type.method` form, not as a typed positional parameter.

---

## Ch 10 reviser, 2026-06-21

### Spec §Dict<K,V> overstates Dict literal key support; string-keyed form doesn't parse

Spec §Standard Library Reference §Dict<K,V> (line ~2780) advertises three Dict-literal key forms:

```lyric
let names = {`alice`: 1, `bob`: 2}              // Dict<Sym, i32>
let cities = {"NYC": 8_000_000, "SF": 875_000}   // Dict<string, i32>
let lookup = {1: "one", 2: "two"}                // Dict<i32, string>
```

Empirical (verified end-to-end against `./lyric compile` + gcc + run):

- Backtick-keyed: parses, compiles, runs. ✅
- Integer-keyed: parses, compiles, runs. ✅
- **String-keyed**: parser rejects with `expected PRParen, got SEOF ()` at the opening of the literal — the disambiguator commits to a struct-literal interpretation when the first key is a string-literal and trips trying to close it.

Book §10.3 removes the string-keyed example and adds a 🚧 callout pointing readers at `let d = Dict<string, i32>()` + explicit `.set(...)` calls for that case. Logged in `~/projects/lyric/TODO`.

### `Dict.length()` vs spec's `Dict.len()`

Spec §Dict<K,V> methods table lists `len() -> i32`. Stdlib reality (`stdlib/std.ly:599`):

```lyric
pub func Dict.length<K, V>(self) -> i32 where K: Hashable {
    return self.d_hash_count
}
```

There is no `len()` method on `Dict`; `d.len()` fails with `checker: unknown method: len`. Two options: rename the stdlib method to `len()` (matches `xs.len()` everywhere else in the stdlib and the carry-forward §Idiomatic Lyric stance) or fix the spec table. Book Ch 10 sidesteps by using `keys.len()` on the slice returned by `d.keys()` (which works), and avoids `d.length()` / `d.len()` in code examples until this is resolved. Logged in `~/projects/lyric/TODO`.

### `Dict<K, V>` as a non-generic class field leaks the inner typevar

Pre-edit Ch 10 §10.5 declared:

```lyric
class Calculator {
    vars: Dict<Sym, f64>
    func get_var(self, name: string) -> (f64, error) {
        let entry = self.vars.get(sym(name))
        ...
    }
}
```

Empirical: `checker: validateAllExprsResolved: TypeVar leak 'V' in Calculator.get_var`. The concrete `V = f64` from the field type isn't being propagated into `Dict.get<K, V>`'s where-clause typevar at the call site. Making the outer class generic (`class VarTable<V> { vars: Dict<Sym, V> }`) trips a different downstream failure:

```
c_backend[c_type/TyClassHandle]: unmangled generic class 'Dict' reached c_backend
(class_renames last-write-wins is 'Dict_CSym_f64'); monomorphizer missed a
TyClassHandle site
```

So neither the non-generic-outer nor the generic-outer route works for "Dict as a field of another class" today. Both bugs logged in `~/projects/lyric/TODO`. Same family as the Ch 6 `Stack<T>` generic-class bug — generic class methods don't compose cleanly with other generic class types yet.

Workaround in the book: §10.5 was rewritten from a `Calculator` class with a `Dict` field to a top-level `let vars = Dict<Sym, f64>()` in `main` plus a free function `get_var(d: Dict<Sym, f64>, name: string) -> (f64, error)`. That shape compiles and runs end-to-end. A 🚧 callout names the limitation and points at the workaround.

### Pre-edit §10.5 also referenced types that don't exist in the running calculator

The pre-edit's "Inside parse_primary:" snippet read `if tok!.kind == TokenKind.Ident { let val = self.calc.get_var(tok!.text)? ... }`. The Ch 4 / Ch 5 calculator's `TokenKind` enum has variants `Number | Plus | Minus | Star | Slash | LeftParen | RightParen` — there is no `Ident` variant — and the Ch 5 `Parser` class has fields `tokens: [Token]` and `pos: i32`, not a `calc` field referencing a `Calculator`. The snippet was aspirational code for a calculator that doesn't exist in the book.

Carry-forward already records "Calculator through-line is complete by Ch 8. Ch 10+ uses standalone examples." Ch 10's §10.5 was the one place the carry-forward rule was being violated; the rewrite to a standalone variable-bindings example brings Ch 10 into compliance and removes the broken cross-reference. No additional bug filed — this is a book-side fix, not a compiler issue.

### `pub permanent class SymTable` — `permanent` was missing from the §10.1 stdlib excerpt

`stdlib/std.ly:503` declares `pub permanent class SymTable { }`, but the pre-edit §10.1 stdlib-excerpt code block showed it as `pub class SymTable { }` — dropping the `permanent` keyword that's load-bearing for sym interning (interned `Sym` instances must outlive every function that holds a handle). Fixed in §10.1; added a one-sentence prose mention that `permanent` opts the class out of slab reclamation. No compiler bug — the chapter's excerpt was simply incomplete.

---

## Ch 13 reviser, 2026-06-21

### Qualified type names and enum-variant references don't resolve across `import`

`import lexer` makes `lexer.tokenize(...)` (a qualified function call) resolve and run end-to-end. But every qualified *type* reference fails:

- `let xs: [lexer.Token] = []` → `checker: unknown struct/class: lexer.Token` + `validateAllExprsResolved: TypeVar leak 'lexer.Token'`.
- `lexer.Token { kind: 1, value: "x" }` (qualified struct constructor) → same `unknown struct/class` error at the literal opener.
- `lexer.TokenKind.Plus` (qualified enum variant access) → same kind of failure.

The prefix-then-merge resolver in `src/ast/modules.ly` rewrites qualified call sites (`ExFnCall` with a module-prefixed callee) but the same rewrite isn't applied to `TyHandle`, struct-literal `kind`, or enum-variant resolution against the imported namespace. Verified by extracting the `testdata/import_dir` example and replacing the function-call body with `let p: mylib.Point = mylib.Point { x: 7, y: 8 }`.

This is the **single biggest gap** in the current module system. It means an imported package can only export *behaviour* (functions and methods) ergonomically; if it exports *data* (types, enum variants), the importing package can't name those types directly — it has to round-trip through a constructor function whose return type the checker can infer locally. The book's Ch 13 §13.5 teaches that exact workaround pattern (`pub func make_number(value: string) -> Token`) and adds a 🚧 callout.

Knock-on effect on the Lyric compiler itself: `src/` has 14 `.ly` files in 12 directories — and **not a single `import` statement among them**. The compiler relies entirely on flat-namespace merging of `BOOTSTRAP_FILES` because its AST module exports types like `Expr` and `TokenKind` that the import system can't resolve qualified-type-name style. The Makefile lists all 14 files explicitly. Ch 13 §13.9 documents this honestly.

Fix: the qualified-name resolver should produce the same prefixed identifier (`lexer_Token`, `lexer_TokenKind_Plus`) whether the use site is a function call, a type annotation, a struct-literal opener, or an enum-variant access. Today only the call site is rewritten.

Logged in `~/projects/lyric/TODO`.

### `lyric compile -mod .` does not exist as a CLI flag

Pre-edit Ch 13 §13.3 said: "When you run `lyric compile -mod .`, the compiler finds `lyric.mod`, discovers all `.ly` files in the directory tree, resolves imports, and compiles everything together." Empirical: `./lyric compile -mod /path/to/proj main.ly` fails with `error: cannot read -mod`. There is no `-mod` flag.

Reality (per `src/main/main.ly:735–765`): directory mode is **automatic** — `lyric compile <dir>` checks for `<dir>/lyric.mod`, and `lyric compile <single.ly>` checks for `<parent>/lyric.mod`. If either is present, the discovered directory is used as the module root; otherwise no module mode. Both shapes work end-to-end (verified against `testdata/import_dir`).

Ch 13 now teaches the `lyric compile <dir>` form. Spec §Compilation Model already documents this correctly; only the book was out of sync. No compiler bug; doc fix only.

### Import resolution is single-level, but the working level is only as deep as the root file imports

Spec §Imports already calls out "🚧 Recursive imports are not resolved" — `resolve_module_imports` in `src/ast/modules.ly` walks the *root file's* import declarations only; an imported package's own `import` statements are silently ignored. This was already known.

The consequence the pre-edit chapter under-stated: it's not just that nested imports don't resolve, it's that **anything an imported package references must already be in the root program's namespace**. If `parser/parser.ly` says `import ast` and uses `ast.Token`, both the `import` and the qualified references die — first because the import is dropped, second because (per the qualified-type bug above) the qualified type name doesn't resolve even when the import *is* processed. The two bugs compose.

Ch 13 §13.5 names this honestly and points readers at the flat-file-list pattern (§13.9, the compiler) as the working workaround for projects that outgrow what one root file can `import`.

### Multi-file single-package works perfectly

This isn't a bug — it's the silver lining. When multiple `.ly` files in the same directory all declare `lyric <name> { }` (or even mix `lyric { }` wrappers with bare top-level declarations), the parser merges them into one namespace before the checker runs. Cross-file calls, type references, constructors, enum variants, methods, everything works unqualified. There's no `import` needed because every declaration is visible to every other declaration in the same package.

This is the pattern the book now teaches in §13.5 as the default split (calculator across `lexer.ly` + `parser.ly` + `main.ly`, all in package `main`), and the pattern the compiler itself uses across all 14 files. It scales surprisingly well — the bootstrap compiler is 33,500 lines of Lyric across 14 files in 12 directories, all flat-merged.

### Pre-edit §13.5 example would not compile even after `import` is sorted out

The pre-edit §13.5 layout had `lexer/`, `parser/`, `ast/` as three subpackages with `main.ly` importing all three and the sub-packages importing each other (`parser/parser.ly` imports both `ast` and `lexer`). This fails three independent ways today: (a) recursive import — `parser`'s `import ast` is ignored; (b) qualified types — even if it weren't ignored, `ast.Token` as a type annotation wouldn't resolve; (c) the example uses `LParen`/`RParen` instead of `LeftParen`/`RightParen` (Ch 4 decision), and a `match` expression with bare-identifier arms which the parser also rejects. Rewritten to the working multi-file-single-package shape; the cross-package addendum uses constructor functions to sidestep (b).
