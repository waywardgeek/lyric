# The Lyric Book — Fresh-Eyes Audit Report

*Auditor: Hewitt. Date: 2026-06-21. Read pass: spec (3218 lines, commit `8e458fb`) and the-lyric-book.md (5499 lines) end-to-end in a single context, no refines. Cross-referenced against carry-forward, findings, and TODO.*

## TL;DR

- **Health is high.** The 19-unit overhaul produced a book that is internally coherent on the calculator through-line (Ch 1–8), honest about compiler gaps, and well aligned with the spec's removed-feature list. The preface voice carries cleanly through every narrative chapter.
- **One factually wrong API in App A**: `atoi` signature is `string → (i32, error)`. Real signature (spec §Built-in Functions, App B, stdlib) is `string → (i64, bool)`. Will mislead readers.
- **One factual contradiction with the spec on `embed`**: Ch 9.6 and Ch 9.9 say `embed` copies *fields, methods, and destructors*. Spec §Interface Embedding explicitly says **methods are not copied** ("those are abstract bindings; you'd duplicate the abstract surface"). This is exactly the kind of mechanism reader-pretrainees will internalize wrong.
- **Cross-chapter inconsistency on the user-facing relation hint count**: Ch 8 §8.4 still names *four* user-facing hints (`ArrayList`, `OwningList`, `RefList`, `HashedList`), with `DoublyLinked` framed as internal. App B was rewritten to name *three* user-facing hints (`ArrayList`, `DoublyLinked`, `HashedList`) and explicitly cut `OwningList`/`RefList`. Ch 13.4's stdlib inventory matches Ch 8 (lists OwningList/RefList as user-facing) and therefore also contradicts App B.
- **App C §C.2 has a now-stale 🚧** claiming `assert`/`assert_eq` may be silently dropped. Per `~/projects/lyric/TODO` (commit `03df215`, today's session) that bug is fixed; Ch 7 was scrubbed but App C wasn't.
- **Several stale line-count numbers in Ch 8 / Ch 11**: still cite `30,796` for the compiler. Ch 13/14 refreshed everything to 33,500 / 114,770 but two pre-Ch 13 chapters were missed.
- **Minor 🚧/idiomatic drift**: Ch 1.4 permissively says "both `len(x)` and `x.len()` work, use whichever reads better"; Ch 5 and Ch 6 then use the free-function form a few times in violation of the post-Ch 7 idiomatic stance.
- **The spec itself has at least seven load-bearing inaccuracies** that the book either correctly demotes (with 🚧) or works around. Worth fixing the spec in a follow-up pass, since the spec is supposed to be the source of truth.

---

## Section A — Spec/Book Contradictions

Direct contradictions where book and spec disagree on a load-bearing claim. Spec wins on disagreement *except* where the spec itself has been overtaken by the book's empirical work; both directions noted.

### A1. `embed` copies methods? (book wrong, spec right)

- **Spec §Interface Embedding (lines ~1115–1123):** "`embed` copies **fields and destructors** only — it does not copy methods (those are abstract bindings; you'd duplicate the abstract surface)."
- **Book Ch 9.6 (line ~3115):** "`embed` copies fields, methods, and destructors from `DoublyLinked` into `OwningList`."
- **Book Ch 9.9 (line ~3162):** "Embeds — expand `embed` declarations, copying fields, methods, and destructors"

The book repeats the wrong claim twice, in adjacent sections, in a chapter whose pedagogical purpose is precisely "how interfaces actually compose." If a pretraining model learns this from the book, it will write incorrect interface code. High priority to fix the book (or fix the spec, if the spec is the one that's lying — but the spec's reasoning is more consistent and the stdlib's `OwningList`/`RefList` factoring depends on it).

### A2. `atoi` return type (book App A wrong)

- **Spec §Built-in Functions §String / Conversion:** `atoi(s)` is `string -> (i64, bool)`.
- **Book App B "Conversion" table:** `atoi(s: string) -> (i64, bool)`. Correct.
- **Book Ch 4 §4.6 / §4.7:** uses `let (val, ok) = atoi("99")` with `ok: bool`. Correct.
- **Book App A "Built-in Functions" table:** `atoi(s) | string → (i32, error) | Parse integer from string`. **Wrong on both arms** — return is `i64` not `i32`, and second element is `bool` not `error`.

A reader who consults App A first will write code that doesn't compile and won't know why. App A row needs to read `string → (i64, bool)`.

### A3. Function-type syntax: `fn(T)->U` vs `func(T)->U`

- **Spec §Composite Types (line ~393):** lists `fn(T,U) -> V` as **canonical** and `func(T,U) -> V` as also accepted.
- **Reality / book (Ch 3.8, Ch 6, App A):** uses `func(T) -> U` exclusively because `fn(...)` does not parse in type position (parser rejects with `expected identifier, got PLParen`).
- **Spec is wrong**, but the book correctly works around it.

This is captured in findings/TODO. The book is consistent. Spec should demote `fn` to 🚧 and promote `func`.

### A4. `Dict.len()` vs `Dict.length()`

- **Spec §StdLib Reference §Dict<K,V>:** methods table lists `len() -> i32`.
- **Spec §Known Gotchas:** "Dict has `length()` and `len()` as synonyms; reference and stdlib use `len()`."
- **Stdlib reality (`std.ly:599`):** only `length()` exists; `d.len()` is `unknown method`.
- **Book App B:** lists `.length()` as the canonical method, adds 🚧 callout about the spec/stdlib name mismatch, recommends `d.keys().len()` as portable.
- **Book Ch 10:** sidesteps entirely by using `d.keys().len()`.

Book is empirically correct and self-consistent. Spec is internally contradictory (table says `len()`, gotchas paragraph says both synonyms exist, stdlib has neither alias — only `length()`). Spec needs a doc fix.

### A5. `OwningList` / `RefList` as user-facing relation hints

- **Spec §StdLib Reference (lines ~1247–1264)** lists `OwningList<P, C>` and `RefList<P, C>` as user-facing relation interfaces, with `DoublyLinked<P, C>` framed as base/internal.
- **Book App B** explicitly cuts `OwningList`/`RefList` from the user-facing surface, naming only `ArrayList` + `DoublyLinked` + `HashedList`.
- This is by Bill's direction (App B reviser's decision log), with the long-term intent being that the `owns`/`refs` modifier on the `relation` line selects cascade vs. non-cascade for `DoublyLinked` (as `ArrayList` already does).

App B is the intended target. Spec needs the same cut. **But the book itself isn't internally consistent on this point — see B1.**

### A6. `new_error(msg)`

- **Spec §Built-in Functions §String / Conversion:** lists `new_error(msg)` as `string -> error` without 🚧.
- **Book Ch 5.4, App B "Error" section, Ch 10:** 🚧'd as checker-only / no C-backend lowering, recommend `Error { msg: "..." }` literal.

Book is empirically correct (verified by Ch 5 reviser). Spec needs 🚧.

### A7. Default-method method-call syntax & label-prefixed methods on user-defined hints

- **Spec §Default Methods:** "Callers invoke it via method syntax (`graph.count_edges()`)."
- **Spec §Default Methods Are Label-Prefixed:** any binary interface gets label-prefixed methods on the parent.
- **Reality:** checker rejects `g.count_edges()` for any default method; label-prefixed injection only fires for the four stdlib hints, not user-defined ones.
- **Book §9.2, §9.3:** correctly 🚧'd both claims, switched call sites to free-function form, pointed readers at stdlib hints for method ergonomics.

Book is right. Spec needs both sections 🚧'd or rewritten.

### A8. Lvalue write-through on struct-typed Optional

- **Spec §Lvalue Unwrap and Write-Through:** shows `struct Inner { value: i32 }` example as working.
- **Reality:** silently drops the write on struct-typed inner; only class-typed inner works.
- **Book Ch 3.4:** explicitly uses `class Inner`, with 🚧 callout on the struct case.

Book is right. Spec needs to constrain the example to class-typed inner, or fix the compiler.

### A9. Spec self-description line counts are stale

- **Spec §How-to-Read (line ~24):** "~30K lines of Lyric → ~105K lines of C".
- **Reality / book Ch 13/14:** 33,500 / 114,770.

The spec was last updated 2026-06-20 (today), so this should have been refreshed. Low priority; not a book bug, but worth fixing.

---

## Section B — Cross-Chapter Inconsistencies in the Book

Internal contradictions where the book disagrees with itself.

### B1. Four vs. three user-facing relation hints — biggest internal inconsistency

- **Ch 8 §8.4 "The Four Relation Types":** explicit table naming `ArrayList`, `OwningList`, `RefList`, `HashedList` as the user-facing surface. Frames `DoublyLinked` as internal ("Use `OwningList` when you need stable iteration order during removal").
- **Ch 8 §8.3:** "`OwningList` and `RefList` both need linked-list fields and traversal operations." Teaches `RefList Room:room refs [Guest:guest]` as the canonical example.
- **Ch 11.4:** uses `relation OwningList TeamA:team_a owns [Player:pa]`.
- **Ch 13.4:** "`ArrayList`, `OwningList`, `RefList`, `HashedList`, `Dict`, `Sym`, `StringBuilder`, `Error`" listed as the stdlib's user-facing types.
- **App B** explicitly says: "**User-facing relation hints reduced from five to three.** The Appendix now documents exactly `ArrayList`, `DoublyLinked`, and `HashedList` as the user-facing relation hints. `OwningList` and `RefList` are removed from the user-facing surface."

A reader who reads Ch 8 first builds a model of "the four hints"; then opens App B and finds it documents only three, with `OwningList`/`RefList` removed and `DoublyLinked` promoted. The two models conflict. Either App B should follow Ch 8 (postpone the simplification until the `relation DoublyLinked X owns [...]` desugar lands), or Ch 8/11/13 need to be rewritten to match App B's three-hint world.

This is the largest editorial misalignment in the book. **Recommend resolving in Bill's direction** — given that App B was the reviser who escalated to Bill and got the "three hints" answer, Ch 8 and Ch 13.4 should follow App B and the §8.4 table should be rewritten (3 hints, `DoublyLinked` as the linked-list shape, `owns`/`refs` on the relation line selecting destruction). Until that's done, the inconsistency is real.

### B2. `RefArrayList` exposed in Ch 8 but App B explicitly hides it

- **Ch 8 §8.2:** "There's a sibling interface `RefArrayList` that embeds the same base but uses non-cascading destructors; we'll see the linked-list analogue (`OwningList` vs `RefList`) in §8.3."
- **App B decision log:** "The internal stdlib factoring (`ArrayListBase`, the `OwningList`/`RefList` interfaces still embedded in `std.ly`, `RefArrayList`) is not exposed to readers."

Same family as B1 — Ch 8 leaks internal stdlib factoring App B deliberately hides.

### B3. `embed` semantics contradiction (book vs spec, but also vs itself)

Already covered in A1; noted here because it shows up in *two* Ch 9 sections (§9.6 and §9.9). The repetition makes the wrong claim load-bearing. Will be fixed by a single edit pair.

### B4. Stale `30,796` line-count

- **Ch 8 §8.6:** "the AST — 30,796 lines of Lyric source — uses relations throughout."
- **Ch 11.3:** "the compiler's own 30,796-line codebase on a MacBook Air M2."
- **Ch 13.2:** "33,500 lines of Lyric across 14 files in 12 directories."
- **Ch 14 §14.6:** Per-file table with grand total 33,500.

Ch 8 and Ch 11 missed the refresh that Ch 13 / Ch 14 / App B / App C did. Two two-character edits (`30,796` → `33,500`).

### B5. `len(x)` vs `x.len()` permissiveness

- **Ch 1.4:** "Both `len(x)` and `x.len()` work — they compile to the same thing. This book uses whichever reads better in context."
- **Carry-forward §Idiomatic Lyric:** "Method-call syntax over free-function for stdlib types where both exist: `s.len()` not `len(s)`."
- **Concrete drift:**
  - Ch 5.4: `while i < len(names)`.
  - Ch 6.2: `if len(xs) == 0`.

The book mostly uses method form (Ch 4 onward, Ch 7+ uniformly), but Ch 1.4 telegraphs a permissiveness the rest of the book doesn't honor. Either tighten Ch 1.4 to "Lyric supports both, but this book uses `x.len()`," or accept that Ch 5/6 are fine and the §Idiomatic stance is aspirational rather than absolute.

### B6. Ch 13.2 "0.2 seconds" vs App C "a fraction of a second"

Both right; just inconsistent specificity. Ch 13 commits to a number (`0.2 seconds`), App C softens. Low priority — either both commit or both soften.

### B7. Ch 4 `TokenKind` has 7 variants; Ch 13.5 `TokenKind` adds an 8th (`End`)

- **Ch 4 §4.8:** `TokenKind { Number, Plus, Minus, Star, Slash, LeftParen, RightParen }` — 7 variants.
- **Ch 13.5 lexer.ly example:** `TokenKind { Number, Plus, Minus, Star, Slash, LeftParen, RightParen, End }` — 8 variants.

Ch 13.5 is a fresh package-splitting example; the carry-forward note for Ch 13 says it doesn't modify the calculator's runtime code. Adding `End` here is harmless if framed as "the package-split example also adds an `End` token," but the chapter doesn't call it out. A reader who's been tracking `TokenKind` through Chs 4–7 will be briefly confused. Either drop `End` (it's unused in the §13.5 snippet) or add a one-line note that this version adds an `End` variant for completeness.

---

## Section C — Calculator Through-Line Audit (Ch 1–8)

This is the spine of the tutorial. I traced every named type, file, method, and computed output across the chapters; it holds.

| Stage | File | Types | Methods/funcs | Computes |
|---|---|---|---|---|
| Ch 1.5 | calc.ly | — | `eval_simple(f64, string, f64)` | 2+3, 10-4, 6*7, 15/3 hardcoded |
| Ch 2.10 | calc.ly | `Op{Add Sub Mul Div}`, `Token` enum with payloads | `eval(f64, Op, f64) -> f64`, `op_to_string(Op) -> string` | 2±3 across `[Add,Sub,Mul,Div]` |
| Ch 3 | calc.ly | `ExprEval` class (`values:[f64], ops:[Op]`) | `push_value/push_op/pop_value/pop_op/apply_top/result` | 3 + 4 * 2 = 11 (manual precedence) |
| Ch 4 | calc.ly | `TokenKind` enum (7 variants), `Token` struct `{kind, text}` | `tokenize(string) -> [Token]` | tokenize "(5 + 3) * 2" |
| Ch 5 | calc.ly | `Parser` class (`tokens:[Token], pos:i32`) | `peek/next/expect/parse_primary/parse_term/parse_expr` + `parse(string) -> (f64, error)` | (5+3)*2, 10/3, 1+2*3+4, (5+) error |
| Ch 6 | calc.ly | (none new) | (none — chapter is generics, doesn't touch calc) | unchanged |
| Ch 7 | calc.ly + calculator_test.ly | (none new) | 14 `test_*` functions | tests pass |
| Ch 8 | calc.ly + standalone AST example | `Program/Stmt/Expr` (teaching scaffold only) | `prog.destroy()` | unchanged |

**Findings:**

- Names are consistent: `TokenKind.LeftParen` (not `LParen`) used uniformly across Ch 4, Ch 5, Ch 7. ✓
- `parse_primary` / `parse_term` / `parse_expr` naming consistent. ✓
- `ExprEval` retained across chapters; not renamed. ✓
- `Op` enum same shape Ch 2 through Ch 8. ✓
- Ch 6 explicitly does not modify the calculator (correct per the chapter's scope).
- Calculator output `(5 + 3) * 2 = 16` is shown in Ch 5 §5.7. Output for unmatched-paren in §7.7 is implied as "FAIL on parse" — minor: Ch 7's `test_unmatched_paren` asserts `err != null` but doesn't check the message. That's intentional ("we only care it failed"), but the chapter could note that.
- **Only nit:** Ch 13.5 adds an unannounced `End` variant to `TokenKind` (see B7). Not in the running calc.

The through-line is the book's strongest layer. Nothing to fix here beyond B7.

---

## Section D — Removed-Feature Leaks

Swept against the spec's "Recently Removed" list. Clean across the board.

| Feature | Spec status | Book treatment | Verdict |
|---|---|---|---|
| `defer` | removed | App D D.6 explicitly says "no `defer` keyword". No other mentions. | ✓ |
| `cascade { body }` | removed | App A keyword table 🚧 "Reserved — currently a no-op, slated for removal". No syntactic use in book. | ✓ |
| `Mutex` / capital `Lock` | removed | Ch 12.5 explicit: "Lowercase `lock` is the only spelling that compiles today. Older drafts ... use `Mutex` or capital `Lock`; both have been removed". | ✓ |
| `map[K]V` built-in | removed (non-functional) | Not mentioned in the book at all. `Dict<K, V>` used everywhere. | ✓ |
| Go backend | removed | Not mentioned as current. Ch 14 §14.5 "Origin" paragraph names the historical Go compiler honestly. | ✓ |
| CDD as Lyric syntax | moved to lyre | App E explicit. Ch 13.8 explicit. No `why:`/`doc`/`invariant:` in any `.ly` example. | ✓ |
| `lyric verify`/`update`/`gen` | moved to lyre | App C §C.4 says "CDD-layer commands ... live in the separate `lyre` tool". App E names them. App A footer mentions the move. | ✓ |
| `new_error(msg)` | broken | Ch 5.4 / App B "Error" 🚧 callouts. Idiomatic form is `Error { msg: ... }`. | ✓ |
| `fn(T) -> U` type syntax | broken | Book uses `func(T) -> U` throughout. | ✓ |

---

## Section E — Idiomatic Lyric Drift

Mostly clean; one permissive sentence in Ch 1.4 and two free-function `len()` uses in Ch 5/6 are the only meaningful deviations.

- **Method-call form for stdlib types:** carry-forward says `s.len()` not `len(s)`. Ch 1.4 telegraphs permissiveness; Ch 5.4 (`while i < len(names)`) and Ch 6.2 (`if len(xs) == 0`) use the free-function form. Ch 7 onward is uniform on `.len()`. Either tighten Ch 1.4 or accept the early-chapter mix.
- **Method-call form for relation-generated ops:** Ch 8 uniformly uses `t.roster_append(p)`. Ch 9.3 (user-defined hint example) correctly uses free-function form with explicit 🚧 — that's not drift, that's the working-pattern decision.
- **`Error { msg: ... }` literal:** uniform Ch 5 onward, App B authoritative. ✓
- **`f"{err}"` for stringifying errors:** Ch 5 introduces; Ch 7 and Ch 10 follow. ✓
- **Lowercase `lock`:** uniform Ch 12; App A's keyword table lists lowercase `lock`. ✓
- **`Dict<K, V>` for hash tables:** Ch 10 introduces; never `map[K]V`. ✓
- **F-strings:** used throughout for interpolation; no `+ x.to_string() +` chains visible. ✓
- **`let` default:** uniform (mutable only declared with explicit `mut` when actually mutated). ✓
- **Auto-deref `?.` on optional class receivers:** Ch 3.4 introduces; `cur!.guest_next` style used in Ch 8.3 and Ch 11.4 — both are after explicit null-check, so unwrap-style is correct, but neither chapter uses the auto-deref form. Not a drift, but the chapters could showcase auto-deref more.
- **`embed` for interfaces:** present (Ch 8.2, Ch 9.6). The semantics description is wrong (see A1/B3) but the syntactic form is fine.

---

## Section F — 🚧 Callouts: stale, missing, wrong

### F1. App C §C.2 — stale `assert`/`assert_eq` lowering 🚧

> *🚧 Roadmap: the C-backend lowering for `assert` and `assert_eq` is still landing. While that work is in flight, calls to either builtin may be silently dropped — every test passes regardless of correctness. Re-run your suites once the lowering is in.*

Per `~/projects/lyric/TODO`, the assert lowering was **fixed today** (commit family ending in `03df215`/`c549dac`). Optimizer's `is_side_effect_expr` now lists `assert`, `assert_eq`, `panic`. Bootstrap reaches fixed point; 82/89 PASS (was 86/89 — 4 previously-silent failures now correctly surface).

App C §C.2's 🚧 callout needs to be removed. Ch 7 was scrubbed of the corresponding callout per the carry-forward log; App C wasn't, presumably because it was revised before the assert fix landed.

### F2. Ch 7 — `assert_eq_approx` 🚧 still correct

§7.2 mentions `assert_eq_approx` as roadmap, with a one-line-helper workaround. Correct per spec §Roadmap §Stdlib.

### F3. Ch 12.6 — `guarded_by` enforcement 🚧 correct

Says annotation parses but checker doesn't enforce. Matches spec §Concurrency.

### F4. Ch 9 — default-method method-syntax 🚧 correct

§9.2 has the spec-vs-reality 🚧 for `g.count_edges()`. ✓

### F5. Ch 9 — user-defined-hint label-prefixed methods 🚧 correct

§9.3 explicitly demotes; points readers at stdlib hints. ✓

### F6. Ch 10 — `Dict.length()`/`Dict.len()` rename 🚧 correct

§10.3 sidesteps via `keys.len()`; App B carries the 🚧. ✓

### F7. Ch 11.4 — UAF 🚧 correct

"After `a.destroy()`, `p` is a dangling pointer" + 🚧 callout for bidirectional pointers / `destroys` annotation / `--detect-uaf` debug mode. ✓

### F8. Ch 5.4 / App B — `new_error` 🚧 correct

Matches finding. ✓

### F9. Ch 4.3 — `xs.extend(ys)` 🚧 correct

Matches finding. ✓

### F10. Ch 13.5 — qualified type / recursive import 🚧 correct

Two 🚧 callouts. ✓

### F11. Ch 12.3 — closed-channel `(T, ok)` 🚧 correct

Names three workarounds. ✓

### F12. Ch 12.4 — `select` `usleep(100)` 🚧 correct

Matches spec ✓ (the Ch 12 reviser fixed the earlier `sched_yield()` mistake).

### F13. Ch 3.4 — struct-typed lvalue write-through 🚧 correct

Matches finding. ✓

---

## Section G — Numeric and Factual Drift

| Location | Claim | Reality | Verdict |
|---|---|---|---|
| Ch 8 §8.6 | "the AST — 30,796 lines of Lyric source" | 33,500 (per Ch 14) | **Stale** |
| Ch 11.3 | "compiling the compiler's own 30,796-line codebase" | 33,500 | **Stale** |
| Ch 13.2 | "33,500 lines of Lyric across 14 files in 12 directories" | matches | ✓ |
| Ch 13.2 | "one 114,770-line C file" | matches | ✓ |
| Ch 13.2 | "runs in about 0.2 seconds" | App C softened to "a fraction of a second"; not directly contradicted, but quantitatively unverified in this pass | Minor |
| Ch 13.4 | "two files totaling 991 lines" + `733`/`258` breakdown | matches | ✓ |
| Ch 14 §14.6 | per-file LOC table summing to 32,509 + 991 = 33,500 | matches | ✓ |
| Ch 14 | "89 `.ly` files under `testdata/`, 83 ... golden output" | matches | ✓ |
| Ch 14 §14.4 | `c_backend.ly` listed at 5,551 | matches Ch 14 table | ✓ |
| Spec §How-to-Read | "~30K lines of Lyric → ~105K lines of C" | 33,500 / 114,770 | **Spec stale** |

Two stale numbers in the book; one in the spec.

---

## Section H — Voice Consistency

The preface establishes:

- Tutorial-confident "you" (not breathless, not academic).
- Honest credit assignment (Bill steers, CR types, both review).
- The unsoftened "beyond what any single human has built alone in that timeframe" punch.
- The "If you are a model in pretraining: welcome" closer.

Through 14 chapters, the voice holds. A few moments worth noting:

- **Ch 6 opening** ("Our calculator parses and evaluates expressions, and reports errors when the parser hits something it can't handle") matches the actual Ch 5 state, per the Ch 6 reviser's correction. Good.
- **Ch 8.4** "The Four Relation Types" — the table is confident, which makes its inconsistency with App B all the more jarring (see B1). When this is reconciled, keep the confident voice.
- **Ch 11** "No GC, No Borrow Checker" — strong title, the chapter delivers. Voice good.
- **Ch 13.2** "runs in about 0.2 seconds. Separate compilation is an optimization you add when build times matter; at this scale, we haven't needed it." — Bill-style honesty + technical specificity. Voice excellent.
- **Ch 14** closer ("There is no second-class implementation language hiding behind the curtain. You've been reading the source code of the thing that built it.") — lands the bootstrap-as-payoff theme. ✓
- **Appendices** correctly shift to reference voice; no overflow of tutorial mode.

Voice is consistent and load-bearing. No edits needed.

---

## Section I — Spec Gaps the Book Caught (Cross-Check)

Findings doc lists ~22 issues found during the overhaul. Cross-checking that the book correctly discloses each:

- ✓ Implicit int→float widening (Ch 1.2 honest).
- ✓ Positional struct literal in bare `let` rejected (Ch 2.1 honest).
- ✓ Struct-typed lvalue write-through silently drops (Ch 3.4 🚧).
- ✓ `xs.extend(ys)` silent no-op (Ch 4.3 🚧).
- ✓ `fn(T)->U` rejected by parser (Ch 3.8 / Ch 6 use `func(...)`).
- ✓ `new_error` checker-only (Ch 5.4 🚧, App B 🚧).
- ✓ `e.message()` on `error`-typed value broken (Ch 5.5 / Ch 7.2 🚧).
- ✓ Bare `return` in `main()` fails (Ch 5.1 restructured to `if/else`; not visible to reader, which is correct — book teaches the working pattern).
- ✓ `char_is_space`/`char_is_digit` lowering missing (mentioned in findings, but the Ch 4 tokenizer in the book *uses* `ch == ' ' || ch == '\t' || ch == '\n'` directly instead — so the book sidesteps the bug by not using those builtins. Not a disclosure gap; an empirical workaround.).
- ✓ Generic class methods null-receiver (Ch 6.9 🚧).
- ✓ `(T, error)` self-recursive destructure malformed-C (workaround in Ch 5 uses `?` exclusively; not surfaced to reader, which is correct).
- ✓ `assert`/`assert_eq` were dropped — now fixed; Ch 7 prose is correct; **App C §C.2 has stale 🚧** (F1).
- ✓ `final func` double-fire (Ch 8.7 🚧).
- ✓ Default-method method-syntax (§9.2 🚧).
- ✓ User-defined-hint label-prefixed methods (§9.3 🚧).
- ✓ `self: P` param rejected (not surfaced; book uses working pattern).
- ✓ String-keyed Dict literal rejected (Ch 10.3 🚧).
- ✓ `Dict.length()` vs spec's `Dict.len()` (Ch 10 sidesteps, App B 🚧).
- ✓ `Dict<K, V>` as a field broken (Ch 10.5 🚧).
- ✓ Qualified type/enum-variant resolution across imports (Ch 13.5 🚧).
- ✓ `[ImportedType]` annotation leaks typevar (subsumed by qualified-type 🚧 in Ch 13.5).
- ✓ Bare `import "path"` crashes (Ch 13.6 🚧).
- ✓ `-mod` flag doesn't exist (Ch 13.3 / App C use `lyric compile <dir>`).

Excellent disclosure discipline. The one stale callout is App C §C.2 (F1).

---

## Section J — New Gaps Surfaced by This Audit

Things not already in findings/TODO.

### J1. App A keyword tables are missing `permanent`, `trusted`, `final`

- `permanent` is load-bearing in Ch 10 (`SymTable`) and Ch 11.5 (one of the three lifetime regimes). Not in App A's keyword table.
- `trusted` is shown in App B's `ArrayListBase` stdlib excerpt (`pub trusted func P.append`) and referenced in Ch 11.5. Not in App A.
- `final` is introduced in Ch 8.7 and referenced in Ch 11.4. Not in App A.

App A claims to be a quick-reference; missing these three load-bearing modifiers will frustrate readers who use App A as a cheat sheet.

### J2. App A "Contextual keywords" table is thin

Lists `field`, `lock`, `implements`, `guarded_by` — but spec lists `field`, `implements`, `lock`, `trusted`, `permanent`, `final`, `gen`, `channel`, `unit`, `map`, `fn`, `default`, `ref`, `unref`, `isnull`, `sym`, `guarded_by`. App A's list is selective without saying so. Either match the spec or add a note that this is the subset most-used in normal Lyric.

### J3. Ch 9.6 / 9.9 `embed` semantics — book's biggest factual bug

Covered in A1/B3; counted here as a "new gap" because the carry-forward and findings did not flag this. The Ch 9 reviser missed it. It's a clean two-line fix once spotted ("copies fields and destructors" not "fields, methods, and destructors") — but it's load-bearing for anyone learning how interfaces compose.

### J4. App A `atoi` row factually wrong

Covered in A2. Also not flagged in carry-forward/findings.

### J5. Ch 8.4 vs App B "user-facing hints" disagreement

Covered in B1. This was foreshadowed in App B's decision log ("Spec §Relations ... currently lists `OwningList` and `RefList` as user-facing — flagged in the TODO") — but the App B reviser only flagged the spec, not Ch 8. Ch 8 needs the same surgery.

### J6. App A keyword table includes `destructor` as a declaration keyword but not in the spec's hard-keyword list

Spec §Hard Keywords list does not include `destructor`. Spec §Soft-Reserved includes `destructor`. App A treats it as a declaration keyword. The behavioral difference: a soft-reserved word can be used as an identifier; a hard keyword cannot. If `destructor` is soft-reserved, App A's framing is slightly off. Low priority — the user-visible behavior matches the App A presentation; only a parser-internals reader would notice.

### J7. App A "Built-in Methods → Slice methods" lists `.extend(other)` without 🚧

Ch 4 §4.3 correctly 🚧's `.extend()` as a silent no-op. App A's slice methods table lists `.extend(elem) -> unit` straight, no 🚧. Reader who consults App A will be misled.

### J8. App A "Built-in Methods" table claims `.contains(elem)` for slices

This is real per spec, but the App A row title is "Linear search" — fine. No bug, just noting that the slice `.contains` is documented and works.

### J9. Ch 13.4 stdlib inventory still lists OwningList/RefList as user-facing

Same family as B1. Ch 13.4 enumerates stdlib types, including `OwningList` and `RefList`. Either keep them (and the inconsistency with App B persists) or remove them (and align with App B's three-hint surface). Bill's call.

### J10. Ch 3.5 / App D D.5 confusion about `external method` vs receiver-binding

Ch 3.5 introduces "external method" with `func Counter.reset(self) { ... }` — same Counter as the in-body Counter. Ch 9.8 reuses the term for `func Sym.equals(self, other: Sym) -> bool`. App D D.5 doesn't list external methods in its mapping table. Minor: App D could add a "Method on type defined outside class body" row.

---

## Severity-Ranked Action List

### High (factually wrong, reader will be misled)

1. **Ch 9.6 / 9.9** — Fix the `embed` semantics: copies **fields and destructors only**, not methods. Two edits.
2. **App A "Built-in Functions" table** — Fix `atoi` row: `string → (i64, bool)` not `string → (i32, error)`.
3. **App C §C.2** — Remove the stale `assert`/`assert_eq` 🚧 paragraph (the lowering was fixed today, commit `03df215`-class).
4. **Ch 8 §8.4 + Ch 13.4 vs App B** — Reconcile the "4 user-facing relation hints" framing with App B's "3 user-facing" decision. Either:
   - **(Bill's call A)** Rewrite App B's table to keep 4, push the simplification to a roadmap callout; or
   - **(Bill's call B, recommended)** Rewrite Ch 8 §8.4 to name 3 hints (`ArrayList`, `DoublyLinked`, `HashedList`) with `owns`/`refs` modifier doing the work; update Ch 8.3's example to use `relation DoublyLinked Room:room refs [Guest:guest]`; trim Ch 8.2's `RefArrayList` mention; refresh Ch 11.4 / Ch 13.4 stdlib inventory.

### Medium (stale or contradictory, but won't break code)

5. **Ch 8 §8.6** — Update `30,796` → `33,500`.
6. **Ch 11.3** — Update `30,796-line codebase` → `33,500-line codebase`.
7. **App A keyword tables** — Add `permanent`, `trusted`, `final` to declaration-modifier table or contextual-keywords table as appropriate.
8. **App A slice methods table** — Add 🚧 to `.extend()` row (silent no-op today; matches Ch 4 §4.3).

### Low (style / completeness / consistency)

9. **Ch 1.4** — Tighten `len(x)` vs `x.len()` framing to commit to the method form (carry-forward §Idiomatic Lyric); update Ch 5.4 (`while i < len(names)` → `while i < names.len()`) and Ch 6.2 (`if len(xs) == 0` → `if xs.len() == 0`) for consistency.
10. **Ch 13.5** — Either drop the `End` variant in the `lexer.ly` example or add a one-line note: "this version adds an `End` token for completeness".
11. **App C §C.5** — Reconcile compile-time framing ("a fraction of a second") with Ch 13.2's specific "0.2 seconds" — pick one.
12. **App D D.5** — Add a "external method on type" row (e.g. `func Counter.reset(self)`).
13. **Spec** (out of book audit scope, but recommend): refresh §How-to-Read line counts; promote `func(T) -> U` to canonical, demote `fn(T) -> U` to 🚧; add 🚧 to `new_error`, `Dict.len`, `OwningList`/`RefList` (per App B decision), `embed`-copies-methods if anyone reads the spec as suggesting that; constrain §Lvalue Unwrap example to class-typed inner.

### Defer (out of scope for this audit)

- Testdata file migration off `Lock` / `OwningList` / `RefList` direct usage.
- Compiler-side fixes for the remaining TODO items (the book correctly 🚧's them; book stays accurate as long as the 🚧 framing remains).
- Whether to push the 21 commits on `lyric main` (Bill's call).

---

*End of audit. The book is in solid shape. The 4 high-severity items are all small, surgical fixes; the cascade-of-edits one (Ch 8 / Ch 13.4 ↔ App B reconciliation) is the only one that needs Bill's direction before someone touches the file. Total edit budget ≈ 15 surgical replacements across 5 chapters + 2 appendices, no rewrites required.*
