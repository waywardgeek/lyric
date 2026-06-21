# Book Overhaul — Carry-Forward (Editorial Decisions)

*Seeded by Hewitt (orchestrator) on 2026-06-21. Each chapter reviser appends decisions that the next chapter must respect. Single source of cross-chapter truth during the overhaul. Not the final artifact — just the running notebook.*

---

## Permanent Invariants (do not relitigate)

### Voice and structure
- **Tutorial voice**: "you" for the reader, confident but specific, no breathlessness. Match the new preface (`## Preface`) which establishes voice + author posture.
- **Calculator through-line** in Ch 1–8: tokenizer (Ch 4), parser (Ch 5), evaluator (Ch 3 + onward), with a complete "calculator so far" callback at the end of each calculator chapter. **Do not break or rename the running types**: `Token`, `TokenKind`, `Lexer`/`Tokenizer`, `Parser`, `Expr` / variants, `Evaluator`. If you need to change one, document it here and the next chapter inherits.
- **🚧 roadmap callouts** are italic in-line asides, not narrative inversions. Pattern: *🚧 Roadmap: F-strings will support `{expr:.2f}` precision specifiers; today they accept only bare `{expr}`.* — never inverts the lesson.
- **No "AI at the Helm" interleaved roman/italic Bill-CR pattern**. This is a tutorial book, not Book 1.

### Idiomatic Lyric — use the preferred form everywhere

The spec usually presents both a low-level and an idiomatic form for an operation. **The book teaches the idiomatic form by default**, and only shows the lower-level form when explaining what the idiomatic form expands to.

- **Method-call syntax for relation-generated methods.** When a `relation` (e.g. `relation ArrayList Team:roster owns [Player:team]`) generates child-iteration, child-add, child-remove, etc., use the method form: `team.append_player(p)`, `for player in team.players()`, `team.remove_player(p)`. The free-function form (`append_player(team, p)`) is the same thing under the hood (UFCS), but the book teaches the method form. The spec's relations section should confirm this preference; if the spec only shows the free-function form, the method form is still preferred — flag a spec doc gap in `book-overhaul-findings.md` and use the method form in the book.
- **Method-call syntax over free-function for stdlib types** where both exist: `s.len()` not `len(s)`, `list.append(x)` not `append(list, x)`, etc. — unless the spec explicitly recommends the free-function form for a specific operation.
- **F-strings** for any string interpolation that has more than one variable: `f"{name}={value}"` not `name + "=" + value.to_string()`.
- **`let` (immutable) by default**, `let mut` only when actual mutation happens.
- **`?.`** (auto-deref / Optional chaining) when accessing fields on an Optional class receiver, not a manual null-check + dereference.
- **`new_error(msg)`** for error values, never a bare string-as-error (the checker has a hole that allows it; don't exploit the hole).
- **Lowercase `lock`** as the mutex keyword (never `Mutex` or capital `Lock`).
- **`Dict<K, V>`** from stdlib for dictionaries (never `map[K]V` — that's a removed built-in).

If a code example in the current chapter uses a non-idiomatic form, replace it with the idiomatic form unless the chapter's specific pedagogical point requires the lower-level form (in which case, leave it but add a one-line `*Idiomatic Lyric: `team.append_player(p)` does the same thing.*` callout).

### Lyric-language facts (current per spec at `cr/docs/lyric-language-spec.md`, 3218 lines, commit `8e458fb`)
- **Lyric vs `.lyric` vs lyre**: Lyric is the language. `.lyric` files are declaration-only Lyric (no function bodies). **lyre** is a separate toolchain (Go/Python/TypeScript/Lyric) that reads `.lyric` files and verifies them against implementations. CDD annotations (`why:`, `doc`, `invariant:`, `source:`, `fake:`) are **lyre features, not Lyric features** — never document them as Lyric syntax. The new preface already establishes lyre via the "A sibling artifact: lyre" subsection; chapters should not duplicate that pitch but may reference `.lyric` mode in passing.
- **Lowercase `lock`** is the canonical mutex keyword. **Do not use `Mutex` or `Lock` (capital)** — both removed (spec §Recently Removed).
- **No `defer` keyword** — removed (spec §Recently Removed). Use scope-exit destructors / explicit cleanup.
- **No `map[K]V` built-in** — non-functional. Use `Dict<K, V>` from stdlib instead (spec §Standard Library Reference, line ~2750).
- **No `cascade { body }` statement** — removed. Ownership cascade is automatic through `owns` relations.
- **No Go backend** — removed; only C backend.
- **`embed` keyword on interfaces** — still current (spec line 145 lists it under "Added in Lyric"). The multi-class-interface-redesign proposes to remove it in a future phase, but **for the book, `embed` is current Lyric**. If you mention `embed`, do not flag it 🚧.
- **Multi-class interface redesign** is committed as a separate roadmap doc (`cr/docs/multi-class-interface-redesign.md`, commit `f8b1271`). The book's existing 🚧 callouts on the redesign are correct as-is. **Do not adopt the redesign's syntax** in any chapter — current Lyric is the subject.
- **Function annotations on `.ly` source** — 🚧 roadmap. If currently shown as implemented anywhere, flag 🚧.
- **Strings are byte slices**, not UTF-8 codepoint sequences. UTF-8 handling is 🚧. Honest framing in Ch 4.
- **`error` interface**: spec §Interfaces line 1145 — `error` is a built-in interface. `new_error()` constructor replaces string-as-error (the 6/20 sweep already patched Ch 5 for this).
- **Numeric tower**: `u8`–`u64` and `i8`–`i64` implemented; `u128`/`u256` and floats 🚧 (spec §Design Lineage).
- **Auto-deref of Optional class receivers**: `x?.field` is current Lyric (spec §Auto-Deref). On null, it segfaults today; spec marks 🚧 for panic behavior.
- **`spawn` data race** known gotcha (captures by pointer). Document honestly in Ch 12.

### Reference doc note
- `cr/docs/lyric-language-reference.md` (722 lines) is the daily-driver companion. Cross-check examples against it but treat spec as authoritative when they disagree.

---

## Calculator Through-Line State (Ch 1–8)

*To be filled in by Ch 1 reviser and updated by each subsequent reviser. Track: which types exist, which methods, which file names, what the running example computes by the end of the chapter.*

### After Preface
- Nothing yet. Calculator first appears in Ch 1.5 "A First Real Program: The Calculator."

### After Ch 1
- **File:** `calc.ly` — single file, no module.
- **Types introduced:** none (only primitives `f64` and `string`).
- **Functions:** `eval_simple(a: f64, op: string, b: f64) -> f64` — dispatches on `op` ∈ {`"+"`, `"-"`, `"*"`, `"/"`}; returns `0.0` for any other string. `main()` calls it four times.
- **What the calculator computes:** four hardcoded binary expressions (2+3, 10-4, 6*7, 15/3), printed via `println` + f-strings.
- **Known gaps deliberately left for later chapters:** (1) `op` as `string` permits typos → flagged for Ch 2's enum rewrite; (2) division by zero crashes → flagged for Ch 5; (3) no precedence, no parser → flagged for Ch 2+ (tokenizer Ch 4, parser Ch 5).
- **Ch 2 inheritance:** Ch 2 renames `eval_simple` → `eval` and replaces `op: string` with `op: Op` (enum `Add Sub Mul Div`). Ch 2 also previews a `Token` enum (later redesigned in Ch 4). Ch 2's opening sentence already references "the calculator from Chapter 1 takes two numbers and an operator string" — keep that hook intact.

### After Ch 2
- **File:** still `calc.ly` — single file, no module split yet.
- **Types introduced:** `Op` (enum `Add Sub Mul Div`) and `Token` (enum with variants `Number(value: f64)`, `Operator(op: Op)`, `LeftParen`, `RightParen`). Also teaching-only types that don't carry into later chapters: `Point`, `Rect`, `Color`, `Shape` (`Circle(radius: f64) | Rect(w: f64, h: f64)`), and a hand-rolled `Option` (`Some(value: Shape) | None`) used only to motivate the built-in `T?` introduced in Ch 3.
- **Functions:** `eval(a: f64, op: Op, b: f64) -> f64` (replaces Ch 1's `eval_simple` with the same dispatch but typed on `Op`). Helper `op_to_string(op: Op) -> string`. `main()` iterates `[Add, Sub, Mul, Div]` and prints `f"{a} {sym} {b} = {result}"` for each.
- **What the calculator computes:** the same four binary expressions as Ch 1 (now with `a=2.0, b=3.0`), printed with operator symbol via `op_to_string`. Output: `2 + 3 = 5`, `2 - 3 = -1`, `2 * 3 = 6`, `2 / 3 = 0.666667`.
- **Known gaps deliberately left for later:** (1) division by zero still crashes → Ch 5 fixes via `Result`/`?`/`new_error`; (2) `Token` is introduced as a teaching example for enum payloads but isn't *used* by `main` yet — it's there to motivate the Ch 4 tokenizer and the Ch 5 parser; (3) `as` is permissive today (see 🚧 in §2.9); (4) `match`-as-expression branch unification is not enforced (see 🚧 in §2.3).
- **Ch 3 inheritance:** Ch 3 keeps the `Op` enum and the standalone `eval(a, op, b)` function from §2.10 (it's still called from `ExprEval.apply_top`). Ch 3 introduces the `ExprEval` *class* with two slice-backed stacks (`values: [f64]`, `ops: [Op]`) plus `push_value`, `push_op`, `pop_value`, `pop_op`, `apply_top`, `result`, and uses the built-in `T?` optional for `pop_*` returns. Ch 3 should **not** redefine `Op` or `eval`.
- **Forward-reference hazard for the Ch 3 reviser:** the *pre-edit* Ch 3 §3.3 currently says "The Token **struct** from Chapter 2" — but Ch 2 defines `Token` as an *enum*, and Ch 4 §4.4 explicitly relies on that ("In Chapter 2, we defined `Token` as an enum with payloads ... So we redesign"). The Ch 4 redesign is what introduces the `TokenKind` enum + `Token` struct pair. Ch 3 reviser should fix the §3.3 line to read "The Token enum from Chapter 2" (or drop the example, since Token isn't used in Ch 3's code). Logged in `book-overhaul-findings.md`.

### After Ch 3
- *(...)*

### After Ch 4
- ...

### After Ch 5
- ...

### After Ch 6
- ...

### After Ch 7
- ...

### After Ch 8
- The full calculator (tokenize → parse → evaluate) is complete by the end of Ch 8.

---

## Cross-Chapter Decisions Log

*Append decisions that future chapters must respect. Format:*

*`## Decision (Ch N): <short title>`*
*`Decided to <X> because <why>. Next chapters: <impact>.`*

(Empty at seed time.)

## Decision (Ch 1): Calculator source file is `calc.ly`
Decided to give the running calculator a single, stable filename — `calc.ly` — starting at Ch 1.5, because every subsequent chapter compiles and runs it. Next chapters: use `calc.ly` until the calculator is split into a multi-file package (likely Ch 6 or later when classes/relations land); when that happens, document the file split in this log.

## Decision (Ch 1): Honest framing for implicit int→float widening
Decided to document implicit int→float widening in §1.2 even though the spec's §Type Casts only explicitly mentions integer-to-integer widening. Behavior is real (orchestrator-confirmed) and load-bearing for natural-looking numeric code; spec gap filed in `cr/docs/book-overhaul-findings.md`. Next chapters: rely on implicit int→float widening freely in examples; the spec will be tightened to document it.

---

## Roadmap Items the Book Should Mark 🚧

This is the consolidated list from the spec's §Roadmap and §Recently Removed sections, so chapter revisers don't have to re-derive it. If you find one of these claimed as implemented in your chapter, demote to 🚧.

### Type system (🚧)
- Numeric widths beyond u8–u64 / i8–i64 (no `u128`, `u256`, floats yet)
- Branch-type unification in if-expressions
- Restricted `as` casts (no narrowing without explicit syntax)

### Operators (🚧)
- Bitwise operator precedence fix (currently below `==`/`<`/`&&`/`||`)
- `~` operator
- Compound assignment operators (`+=`, `-=`, etc.) beyond what's implemented

### Lexer / literals (🚧)
- UTF-8 in strings (currently bytes)

### Imports / modules (🚧)
- Various import-related features (consult spec §Imports)

### Function annotations (🚧)
- `.ly` source function annotations (currently `.lyric` declaration-layer only)

### Memory safety (🚧)
- Bidirectional pointers as escape hatch
- `destroys` annotation for UAF prevention
- `mut resize` annotation
- Safe iterators across destroy-during-iteration

### Stdlib (🚧)
- `Hashable.equals` restoration (HashedList currently matches by hash alone)
- 1-to-1 relation hint
- Iteration sugar (`for x in obj` without `.iter()`)
- Planned I/O library (spec §I/O Library — Planned, line ~2951)

### Removed (do NOT teach as current Lyric)
- CDD annotations (`why:`, `doc`, `invariant:`, `source:`, `fake:`) → moved to lyre
- `lyric verify` / `lyric update` / `lyric gen` → moved to lyre
- Three-zone `.lyric` file layout → moved to lyre
- `defer` keyword
- `cascade { body }` statement
- `Mutex` / `Lock` (capital) type — use lowercase `lock`
- `map[K]V` built-in — use `Dict<K, V>`
- Go backend

## Decision (Ch 2): `Token` stays an enum in Ch 2, becomes a `TokenKind` enum + `Token` struct in Ch 4
Decided to keep Ch 2's `Token` definition as an *enum with payloads* (`Number(value: f64) | Operator(op: Op) | LeftParen | RightParen`) because the chapter's pedagogical point in §2.4 is "enums can carry data" and Ch 4 §4.4 explicitly hangs its tokenizer redesign on this earlier shape ("In Chapter 2, we defined `Token` as an enum with payloads. So we redesign..."). Next chapters: Ch 3 must say "Token enum from Chapter 2" (not "struct"); Ch 4 keeps its planned `TokenKind` enum + `Token` struct redesign untouched; the running calculator does not depend on Ch 2's `Token` at runtime, only conceptually.

## Decision (Ch 2): The hand-rolled `Option` in §2.5 is a teaching scaffold only
Decided to keep the `enum Option { Some(value: Shape) | None }` in §2.5 as a deliberate scaffold for teaching nested patterns *before* the reader has met Lyric's built-in `T?`. The forward pointer is to **Chapter 3** (where `T?` lands), not Chapter 6 — Ch 3 already uses `f64?` heavily, so promising the reader they'll have to wait until Ch 6 was misleading. Next chapters: do not redefine `Option`; do not use the §2.5 `Option` in any later calculator code.

## Decision (Ch 2): Bare `let p = Point { 10, 20 }` is rejected — positional struct literals require an expression-depth context
Decided to fix §2.1 to match parser reality: positional struct literals only work inside parens, function arguments, or list literals. The previous claim that a bare `let p = Point { 10, 20 }` "works because it follows `=`" was a fabrication contradicted by `testdata/positional_struct_lit.ly`. Next chapters: when introducing new structs in code examples, use the named-field form (`Point { x: 10, y: 20 }`) for bare `let` assignments; positional is fine inside `(...)`, arg lists, and `[...]`.
