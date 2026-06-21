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
