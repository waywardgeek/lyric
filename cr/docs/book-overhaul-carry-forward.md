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
- **`Error { msg: "..." }`** for error values — the stdlib class literal. The spec lists `new_error(msg)` as a shortcut, but the C backend doesn't lower it yet (Ch 5 reviser verified empirically; logged to TODO). Until that's fixed, use the class literal. Never bare-string-as-error (checker hole; don't exploit).
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

### Empirical verification recipe (compile + run a snippet)
The compiler emits C source, not a binary. To compile + run a snippet end-to-end:

```bash
cd ~/projects/lyric                              # must be project root for stdlib autoimport
./lyric compile /tmp/snippet.ly -o /tmp/snippet.c
gcc -std=gnu11 -O2 -w -x c /tmp/snippet.c -o /tmp/snippet_bin -I runtime -lm -lpthread
/tmp/snippet_bin
```

Notes:
- **Must `cd` to the lyric project root** — otherwise stdlib types (`StringBuilder`, `Dict`, etc.) fail to resolve with `checker: stdlib type X not found`.
- The `-o` from `lyric compile` is a **`.c` file**, not a binary. If you give it a path without `.c`, gcc will refuse to read it as C (treats as linker script); use `-x c` or rename.
- gcc flags taken from `test_lyric.sh` (the canonical test runner): `-std=gnu11 -O2 -w -I runtime -lm -lpthread`.
- Statements must be one per line — `;` is **not** accepted as a statement separator (`a; b` is a checker error).

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
- **File:** still `calc.ly` — single file, no module split yet.
- **Types introduced (carrying forward):** `ExprEval` class with fields `values: [f64]` and `ops: [Op]`. Teaching-only types in this chapter that **don't** carry forward: `Node` (linked-list demo for auto-deref), `Outer`/`Inner` (lvalue-unwrap demo, both classes now), `Counter` (visibility + external-method demo), `Point` (mut-param demo — note this is a fresh `struct Point { x: i32, y: i32 }`, distinct from Ch 2's `Point` which had `f64` fields), `Stack` (concrete-typed stack motivating Ch 6's `Stack<T>`).
- **Methods on `ExprEval`:** `push_value(self, v: f64)`, `push_op(self, op: Op)`, `pop_value(self) -> f64?`, `pop_op(self) -> Op?`, `apply_top(self) -> bool`, `result(self) -> f64?`. `apply_top` pops `b` then `a`, calls `eval(a!, op!, b!)` (the Ch 2 free function), and pushes the result.
- **Functions:** `eval(a, op, b)` from Ch 2 is still the dispatch core — `ExprEval` does not redefine it. `main()` builds an `ExprEval`, feeds it `3 + 4 * 2` token-by-token, applies `*` then `+`, and prints `3 + 4 * 2 = 11`. `op_to_string` is no longer called (the chapter has moved past the "iterate all ops" demo).
- **What the calculator computes:** one expression, `3 + 4 * 2 = 11`, with manual precedence dispatch (the caller decides which `apply_top` happens first). The point is that `ExprEval` carries state across calls, not that it parses.
- **Language features introduced and load-bearing for later chapters:** classes (heap, by-reference), methods with `self`, optionals (`T?`, `isnull`, `expr!`), auto-deref of optional class receivers, lvalue write-through (class case), external methods (`func T.method(self)`), `pub` visibility, `mut` parameters on structs, lambda values and `func(T) -> U` type syntax.
- **Known gaps deliberately left for later:** (1) precedence is still done by hand at the call site — Ch 4 introduces a tokenizer, Ch 5 a parser; (2) `Stack` is `f64`-only and gets generic in Ch 6; (3) struct-typed lvalue write-through silently drops the write today (flagged 🚧 in §3.4 and in findings); (4) `fn(T) -> U` type syntax doesn't parse despite the spec listing it — the book uses `func(T) -> U`, which works (also in findings).
- **Ch 4 inheritance:** Ch 4 already opens with "We ended Chapter 3 with a calculator that evaluates expressions — but only when we feed it values and operators by hand," which matches the §3.10 wrap-up. Ch 4 introduces strings/slices and the tokenizer that produces values for `ExprEval` to consume. Ch 4 does **not** depend on `Stack`, `Counter`, `Node`, `Outer/Inner`, or `Point` from Ch 3 — those are teaching scaffolds only.

### After Ch 4
- **File:** still `calc.ly` — single file, no module split yet.
- **Types introduced (carrying forward):** `TokenKind` (flat enum, `Number | Plus | Minus | Star | Slash | LeftParen | RightParen`) and `Token` (struct, `{ kind: TokenKind, text: string }`). **This is the redesign Ch 2 promised** — Ch 2's `Token` enum-with-payloads is replaced by this enum+struct pair. The variant names `LeftParen` / `RightParen` are preserved exactly from Ch 2 (not abbreviated to `LParen`/`RParen`) so `match` arms read the same way.
- **Functions:** `tokenize(input: string) -> [Token]` — a 70-line scanner driven by a `while pos < input.len()` loop, slicing `input[start:pos]` for number text. No `Lexer` *class*; the chapter chooses a free function because there's no per-tokenization state worth bundling beyond the local `pos`. (A `Lexer` class with line/column tracking is a natural extension for Ch 5 or later when error messages need source positions.) The chapter also exercises `make_pair() -> (i32, string)` and reuses Ch 3's `ExprEval` only by reference in §4.9.
- **What the calculator computes:** `tokenize("(5 + 3) * 2")` returns a 7-element `[Token]`. The chapter's `main()` prints each token's kind and text; it does **not** yet feed them into an evaluator. Ch 5 builds the parser that bridges `tokenize` → `ExprEval`.
- **Language features introduced and load-bearing for later chapters:** byte-level string indexing (`s[i] -> u8`), character literals (`'A'`, `'\n'`, `'\x41'`), slice expressions (`s[lo:hi]`, `s[:hi]`, `s[lo:]`, `s[:]`), slice `push`/`pop`/`+`/`for x in xs`, the `append(xs, x)` built-in, `let ref` for zero-copy views, `StringBuilder`, triple-quoted strings (`""" ... """`), f-string brace doubling (`{{`/`}}`), tuples (`(T, U)` + `._0`/`._1` + destructuring), and the `atoi`/`itoa`/`char_to_string`/`parse_float` conversion built-ins.
- **Known gaps deliberately left for later:** (1) `xs.extend(ys)` is documented in the spec as the in-place append-all method, but the compiler silently drops the call — Ch 4 §4.3 demotes it to 🚧 and uses a `push` loop / `append` built-in instead; (2) UTF-8 is 🚧 — `string` is bytes today, and the chapter says so honestly with a roadmap callout in §4.1; (3) the tokenizer skips unknown characters silently — Ch 5 adds errors at the lexer/parser boundary.
- **Ch 5 inheritance:** Ch 5's pre-edit text already references `TokenKind.LParen` and `TokenKind.RParen` in §5.6 (`Parser.parse_primary` and `Parser.expect(TokenKind.RParen)?`). **The Ch 5 reviser must rename these to `LeftParen` / `RightParen`** to match Ch 2 / Ch 3 / Ch 4. Ch 5 also calls `str_to_float(tok!.text)` — that's correct (stdlib `str_to_float: string -> f64`, distinct from the C-backend builtin `parse_float: string -> (f64, bool)`); leave it alone.

### After Ch 5
- **File:** still `calc.ly` — single file, no module split yet.
- **Types introduced (carrying forward):** `Parser` class with fields `tokens: [Token]` and `pos: i32`. Teaching-only class that does **not** carry forward: `Item` (used in the §5.4 `try`-in-a-loop demo) and `ParseError` (used in §5.5 to motivate custom error types).
- **Methods on `Parser`:** `peek(self) -> Token?`, `next(self) -> Token?`, `expect(self, kind: TokenKind) -> (Token, error)`, `parse_primary(self) -> (f64, error)`, `parse_term(self) -> (f64, error)`, `parse_expr(self) -> (f64, error)`. Recursive-descent precedence: `parse_expr` calls `parse_term` for `+`/`-`, `parse_term` calls `parse_primary` for `*`/`/`, and `parse_primary` calls `parse_expr` for parenthesized sub-expressions (mutual recursion). `expect`'s returned token is discarded by `?` at the call site in `parse_primary` — we just want the error-propagation behavior, not the token.
- **Functions:** `parse(input: string) -> (f64, error)` ties tokenizer + parser together: `let tokens = tokenize(input); let parser = Parser { tokens: tokens, pos: 0 }; return parser.parse_expr()`. `main()` iterates `["(5 + 3) * 2", "10 / 3", "1 + 2 * 3 + 4", "(5 + )"]` and prints either the result or the error for each. Output: `(5 + 3) * 2 = 16` / `10 / 3 = 3.33333` / `1 + 2 * 3 + 4 = 11` / `(5 + ) => error: unexpected token: )`.
- **What the calculator computes:** the full pipeline finally works end-to-end — `parse("(5 + 3) * 2")` returns `(16.0, null)`. The malformed `"(5 + )"` reaches `parse_primary`, which sees `)` where it expects a number or `(`, and returns an `Error { msg: "unexpected token: )" }`. The `?` in `parse_term` propagates it up through `parse_expr` and out through `parse`.
- **Language features introduced and load-bearing for later chapters:** `(T, error)` tuple-return shape; tuple destructure `let (val, err) = f()` and `let (_, err) = f()` (`_` discards); the `?` operator (works in `let`-statements, expression position, both sides of a binary expression, and inside loops — when it fires, the *enclosing function* returns, not just the loop iteration); the `error` interface (any class with `pub func message(self) -> string` satisfies it via structural subtyping); the stdlib `Error { msg: string }` class as the default concrete implementation; and `f"{err}"` for stringifying any `error` value (the f-string lowerer special-cases the error type).
- **Idiomatic error construction:** the book uses the stdlib literal `Error { msg: "..." }` everywhere. The spec lists a free-function shortcut, `new_error(msg)`, but it's checker-only — the C backend doesn't lower it, so it's demoted to 🚧 in §5.4. Logged in `~/projects/lyric/TODO`. **Ch 6+ should keep using `Error { msg: ... }` until the lowering lands.**
- **Known gaps deliberately left for later:** (1) `e.message()` directly on an `error`-typed value doesn't compile — interface dispatch for `error` isn't wired up in the C backend (TODO); the book teaches `f"{err}"` instead. Calling `.message()` on a concrete error class (e.g. `ParseError`) does work. (2) The calculator still reports errors as plain text with no source position — §5.5 motivates a `ParseError { line, col, msg }` class but the calculator's `Parser` keeps the simple `Error { msg }` form. **Ch 6's pre-edit opening claims the calculator "handles errors, and reports line and column numbers" — that's aspirational for the post-Ch 6 state; the Ch 6 reviser should either soften that opening or actually add `Lexer`-tracked line/col to the parser as a Ch 6 exercise.** (3) Bare `return` inside `main()` fails to compile (C backend emits `int main(...)` and rejects `return;`); the §5.1 `main` was refactored to `if/else` to avoid this. Logged in `~/projects/lyric/TODO`. (4) `char_is_space`/`char_is_digit` C-backend lowering is missing — Ch 4's tokenizer doesn't compile today. Ch 5's parser was verified end-to-end against a hand-built `[Token]` literal as a workaround. Logged in `~/projects/lyric/TODO`.
- **Ch 6 inheritance:** Ch 6 introduces generics. The calculator's `Parser` class and its `(f64, error)` returns remain `f64`-specific; Ch 6 may generalize the evaluator (`ExprEval<T>` for any numeric `T`) or the stack types from Ch 3, but the **parser** stays `f64` for now (parser/AST genericization is a much bigger surgery than Ch 6 is sized for). The `error` type, the `Error { msg: ... }` literal, and the `?` operator are all generic-friendly already — `(T, error)` parameterizes naturally over `T`.

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

## Decision (Ch 3): Lvalue write-through is taught on class-typed inner only; struct case is 🚧
Decided to rewrite §3.4's lvalue-unwrap example so the inner type is a `class Inner { value: i32 }` rather than a `struct Inner { value: i32 }`. Empirical test against the current compiler: `o.data!.value = 42` on a struct-typed inner Optional silently writes to a temporary copy and `println(o.data!.value)` still prints `0` — confirmed end-to-end (compile + run) in the Ch 3 revision session. Spec §Lvalue Unwrap and Write-Through (line ~1543) shows the struct version as if it worked; that's a spec/compiler gap. Logged in `book-overhaul-findings.md` and in `~/projects/lyric/TODO`. Next chapters: model "mutable inner state" with a class, or pull-mutate-replace for structs; do not show struct-typed lvalue write-through as a current Lyric pattern.

## Decision (Ch 3): `Point` is re-introduced as a fresh `i32` struct in §3.7 (not Ch 2's `f64` `Point`)
Decided to leave §3.7's `struct Point { x: i32, y: i32 }` mut-param demo as-is, even though Ch 2 already defined a `Point` with `f64` fields. The two `Point`s are pedagogical scaffolds in independent chapters — neither is carried into the calculator. Next chapters: if `Point` is reintroduced again, declare it fresh; do not assume either Ch 2's or Ch 3's `Point` is in scope. The calculator through-line uses `ExprEval` (class), `Op` (enum), `Token` (enum, Ch 2 / redesigned Ch 4), and `eval` (free function) — no `Point` at all.

## Decision (Ch 4): `Token` redesigned into `TokenKind` enum + `Token` struct; paren variants stay `LeftParen`/`RightParen`
Decided to land the redesign Ch 2 flagged: `Token` becomes a `struct { kind: TokenKind, text: string }` paired with a flat `TokenKind` enum (`Number | Plus | Minus | Star | Slash | LeftParen | RightParen`). The split lets the lexer carry source text (and, later, line/column info) without forcing the parser to live inside a payload pattern-match. Paren variant names stay long-form (`LeftParen`, `RightParen`) to match Ch 2's enum and avoid the "is it `L` or `Left`?" naming hazard. Next chapters: use `TokenKind.LeftParen` / `TokenKind.RightParen` everywhere. Ch 5's pre-edit text uses the abbreviated `LParen`/`RParen` in `Parser.parse_primary` and `Parser.expect`; the Ch 5 reviser must rename to `LeftParen`/`RightParen`. Ch 7's pre-edit test file uses `TLParen`/`TRParen` (a parallel naming scheme used by a hand-written test); the Ch 7 reviser should consider aligning that too.

## Decision (Ch 4): `.extend()` on slices demoted to 🚧; teach `push` loop / `append` built-in instead
Decided to demote `xs.extend(ys)` to a 🚧 callout in §4.3, even though the spec lists it as the canonical in-place append-all method, because the compiler silently drops the call (verified empirically: `let mut xs = [1,2,3]; xs.extend([4,5,6]); println(xs.len())` prints `3`). Two working alternatives in the chapter: (a) `push` in a loop, (b) re-bind with the `append(xs, x)` built-in. Logged in `cr/docs/book-overhaul-findings.md` and `~/projects/lyric/TODO`. Next chapters: do not show `.extend()` as a current Lyric idiom; if you need in-place append-all, use a `push` loop. When the compiler bug is fixed, this decision can be revisited.

## Decision (Ch 4): UTF-8 framing is a 🚧 roadmap callout in §4.1, not silent
Decided to keep §4.1's UTF-8 status as an explicit italic *🚧 Roadmap: ...* callout rather than the pre-edit's casual "no built-in UTF-8 decoding — if you need Unicode, work with bytes directly" framing. The roadmap callout names the specific operations that are planned (`\u{NNNN}` escapes, `for c in s.chars()`, code-point `char_at`, Unicode-aware case) and reassures the reader the byte-level API stays. The honest "strings are bytes today, code points are 🚧" message is now front-of-chapter, in the same voice as every other 🚧 callout. Next chapters: rely on byte-level string ops freely; don't pretend Unicode methods exist.

## Decision (Ch 4): No `Lexer` class — `tokenize` stays a free function
Decided to skip wrapping the tokenizer in a `Lexer` class. The 70-line `tokenize(input: string) -> [Token]` function has no per-tokenization state worth bundling beyond a local `pos` cursor — making it a class would just add `self.` noise. The class form becomes worthwhile when the lexer needs to carry line/column tracking and produce structured error messages, which is a natural Ch 5 (or later) extension when errors land. Next chapters: if you want a `Lexer` class for source positions, wrap `tokenize` then; Ch 4's free function form is intentional.

## Decision (Ch 5): `Error { msg: ... }` literal is the canonical error-construction form; `new_error(msg)` demoted 🚧
Decided to use the stdlib `Error { msg: "..." }` class literal everywhere in Ch 5 (and going forward) for error construction. The spec lists `new_error(msg): string -> error` in §Built-in Functions §String / Conversion as a one-liner shortcut, and the checker types it correctly (`src/checker/checker.ly:3609`), but the C backend has no lowering for it — `lyric compile` succeeds but the generated C calls `new_error(LYRIC_STR(...))` with no declaration, so gcc fails to link. Verified empirically: `Error { msg: "..." }` compiles + runs end-to-end; `new_error(...)` doesn't. §5.4 introduces the literal form and adds a 🚧 callout about `new_error`. Logged in `~/projects/lyric/TODO`. Next chapters: keep using `Error { msg: ... }`; do not show `new_error(...)` as a current Lyric idiom until the lowering lands. (Carry-forward's prior note that "`new_error(msg)` is the idiom for error values" was based on spec-text only and was empirically false — corrected here.)

## Decision (Ch 5): Stringify errors with `f"{err}"`, not `err.message()`
Decided to teach `f"{err}"` (and `println(f"...: {err}")`) as the way to display an error value of interface type `error`. The natural method-call form `err.message()` fails to compile today: the C backend models `error` as `const char*` (the legacy bare-string representation), so a call `Error_message(e)` against the concrete `Error_message(Error*)` signature is rejected by gcc with "incompatible types". The f-string lowerer has a dedicated path for the `error` type that does the right thing. Verified empirically. Caveat (also logged): the f-string path always pulls the `Error.msg` field directly, so a custom `class ParseError { ... pub func message(self) -> string { f"{line}:{col}: {msg}" } }` will print just its `msg` field through `f"{err}"`, not the formatted "L:C: msg" form — i.e. user-defined `message()` overrides are bypassed by both the call form (compile error) and the f-string form (silently ignored). Both bugs share the same root cause (no real interface dispatch for `error`) and are logged in `~/projects/lyric/TODO`. Next chapters: stringify errors with `f"{err}"`; if you write a custom error class, you can still call `.message()` on it directly (concrete-type dispatch works), but don't promise readers that user-defined `message()` is what gets called when the value is held as `error`.

## Decision (Ch 5): Bare `return` inside `main()` is avoided; use `if/else` instead
Decided to refactor §5.1's `main()` example from early-`return`-on-error to an `if/else` shape because bare `return` inside `main()` fails to compile. The C backend lowers `func main()` as `int main(int _argc, char** _argv)` (to accept the CLI args the runtime injects) but a Lyric `return` with no value lowers to a bare `return;`, which gcc rejects with `'return' with no value, in function returning non-void`. Logged in `~/projects/lyric/TODO`. The fix is on the compiler side — either lower bare `return` in `main` as `return 0;`, or wrap the user's `main` in a void-returning shim. Next chapters: write `main()` examples with `if/else` rather than `if ... { return }` early-exits until the compiler fix lands. `return` from non-main functions is unaffected.
