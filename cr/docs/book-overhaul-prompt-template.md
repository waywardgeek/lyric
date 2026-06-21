# Per-Chapter Reviser Prompt Template

*Used by the orchestrator (parent Hewitt) to spawn each chapter reviser. Read by parent only; the reviser receives a filled-in instance via `send_message`.*

---

## Substitutions
- `{CHAPTER_HEADER}` — exact text of the chapter's `##` line (e.g., `## Chapter 1: Hello, Lyric`)
- `{PRIOR_HEADER}` — exact text of the prior chapter's `##` line (`## Preface` for Ch 1; for App A use `## Chapter 14: ...`; etc.)
- `{NEXT_HEADER}` — exact text of the next chapter's `##` line (empty string for App E — it's the last unit)
- `{N}` — 1-based chapter number for display ("Ch 1", "Ch 2", ..., "App A", etc.)

---

## Template (paste into send_message)

```
You are revising one chapter of `the-lyric-book.md` to match the
current Lyric language spec. You are a fresh CodeRhapsody instance
inheriting Hewitt's SOUL/MEMORY; you have full tools.

## Your unit to revise: {N}
Chapter header: `{CHAPTER_HEADER}`
The chapter spans from this header to (but not including) the next
`##` header in the file.

## Required reading, in this order

1. `~/projects/lyric/cr/docs/book-overhaul-carry-forward.md` — running
   editorial decisions and the calculator through-line state. **You
   must respect everything in §Permanent Invariants.** You will append
   to §Calculator Through-Line State and §Cross-Chapter Decisions Log
   at the end of your work.

2. `~/projects/lyric/cr/docs/lyric-language-spec.md` — the current
   spec (3218 lines, commit 8e458fb). **This is the source of truth.**
   Skim the TOC first; read the sections relevant to your chapter in
   depth.

3. `~/projects/lyric/cr/docs/lyric-language-reference.md` — daily-driver
   companion (722 lines). Cross-reference but treat spec as
   authoritative on disagreement.

4. The prior chapter (post-edit) for voice and continuity. Header:
   `{PRIOR_HEADER}`. Read its full span in `the-lyric-book.md`.

5. Your chapter (pre-edit). Header: `{CHAPTER_HEADER}`. Read its full
   span in `the-lyric-book.md`.

6. The next chapter (pre-edit) for forward continuity. Header:
   `{NEXT_HEADER}`. Read its full span in `the-lyric-book.md`. (If
   `{NEXT_HEADER}` is empty, you are the final unit — skip this step.)

## What to do

Revise YOUR chapter (the span starting at `{CHAPTER_HEADER}` up to but
not including the next `##` header) so that:

- Every code example is valid current Lyric per the spec.
- Every claim about the language is true per the spec, or flagged with
  a 🚧 italic callout if it's a roadmap item.
- The calculator through-line (Ch 1–8) continues unbroken — same types,
  same method names, same file structure as the prior chapter
  established (see §Calculator Through-Line State in carry-forward).
- The tutorial voice matches the prior chapter and the new preface.
  Tutorial-confident, "you" for reader, specific over breathless.
- Removed features (see §Roadmap Items in carry-forward) are not
  taught as current Lyric. If your chapter currently does, remove or
  demote to 🚧.

## Editing discipline

- **Use `replace_lines` or surgical `edit_file` calls**, not
  `write_file`. Localized changes, boundary-safe, easy to spot-check.
- If you need to substantially restructure a section, do it as one
  `replace_lines` call with the new full text in `new_text`. Bounded.
- **Do NOT edit any other chapter.** You only touch the span between
  `{CHAPTER_HEADER}` and the next `##` header.
- **Do NOT edit `the-lyric-book.md`'s preface, other chapters, or any
  appendix that isn't yours.**
- **Do NOT commit.** The orchestrator (parent) does git commits.
- **Do NOT touch `src/`, the compiler, or anything outside
  `the-lyric-book.md` and `cr/docs/book-overhaul-carry-forward.md`.**
- If you discover a spec bug or a real broken feature, write a note in
  `~/projects/lyric/cr/docs/book-overhaul-findings.md` (append-mode,
  create if missing) and continue. Do not stop.

## When done

1. Append to §Calculator Through-Line State in
   `book-overhaul-carry-forward.md`: a short paragraph stating what
   types/methods/files exist after your chapter and what the
   calculator computes at that point. (Skip if your unit is App A–E
   or post-Ch 8 — calculator is complete by then.)
2. Append to §Cross-Chapter Decisions Log any decision your chapter
   forces on later chapters.
3. Report back with a single line in this exact format:

   `DONE: revised {CHAPTER_HEADER} — <N> surgical edits, calculator state updated, <findings count> findings`

   Then stop. Do not loop. Do not pick up another chapter.

## Stop conditions (rare; report and halt instead of finishing)

- Your chapter needs a chapter-scale rewrite (>30 surgical edits, or
  the whole pedagogical structure is wrong post-spec-changes). Report
  `STOP: {CHAPTER_HEADER} needs full rewrite — <reason>` and halt.
- The spec contradicts itself on something load-bearing for your
  chapter. Report `STOP: spec inconsistency at <location> blocks
  revision of {CHAPTER_HEADER}` and halt.
- You're unsure whether to keep a feature explanation that the spec
  has demoted to 🚧 vs cut it entirely. Default: demote to 🚧
  callout, keep the explanation. Don't stop for this.
```
