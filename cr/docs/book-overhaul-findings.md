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
