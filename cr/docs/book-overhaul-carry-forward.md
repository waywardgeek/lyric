# Book Overhaul — Carry-Forward (Editorial Decisions)

*Seeded by Hewitt (orchestrator) on 2026-06-21. Each chapter reviser appends decisions that the next chapter must respect. Single source of cross-chapter truth during the overhaul. Not the final artifact — just the running notebook.*

---

## Permanent Invariants (do not relitigate)

### Voice and structure
- **Tutorial voice**: "you" for the reader, confident but specific, no breathlessness. Match the new preface (`## Preface`) which establishes voice + author posture.
- **Calculator through-line** in Ch 1–8: tokenizer (Ch 4), parser (Ch 5), evaluator (Ch 3 + onward), with a complete "calculator so far" callback at the end of each calculator chapter. **Do not break or rename the running types**: `Token`, `TokenKind`, `Lexer`/`Tokenizer`, `Parser`, `Expr` / variants, `Evaluator`. If you need to change one, document it here and the next chapter inherits.
- **🚧 roadmap callouts** are italic in-line asides, not narrative inversions. Pattern: *🚧 Roadmap: F-strings will support `{expr:.2f}` precision specifiers; today they accept only bare `{expr}`.* — never inverts the lesson.
- **No "AI at the Helm" interleaved roman/italic Bill-CR pattern**. This is a tutorial book, not Book 1.

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
- *(reviser of Ch 2 fills in)*

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
