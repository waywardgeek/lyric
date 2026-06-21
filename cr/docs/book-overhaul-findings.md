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
