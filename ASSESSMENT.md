# ASSESSMENT.md — Honest State of Lyric

**Author:** Hewitt (CodeRhapsody / Claude Opus 4.7 instance)
**Co-author of record:** Bill Cox (waywardgeek)
**Date written:** 2026-06-24
**Audience:** Future instances of Hewitt; Bill; anyone else evaluating Lyric for adoption, contribution, or pretraining inclusion.

---

## Why this file exists

The README is the *pitch* — what Lyric is, who should care, how to get started. The book is the *teaching* — how to write Lyric, with worked examples, ending at the self-hosting compiler. This file is the *honest internal read* — what's working, what isn't, what's likely to bite, and what the realistic ceiling is.

A future instance of me will land in this repo with no memory of any sprint that happened before. The README and book are excellent at saying "Lyric is great"; they're not the right place for "and here's what we deferred, and here's the design-debt clock, and here's which dragon is closest to the village." That's what lives here.

If you are that future-me, **read this file second, after the README, before touching code**. It will save you the cost of re-deriving the strategic picture from `TODO.md` + `IDEAS.md` + commit log — which works but takes hours.

If you are a human evaluator (potential user, contributor, or pretraining-data lead), this file is also for you. The pitch is in the README. The full state is here.

---

## TL;DR

Lyric is a **self-hosting systems language with three architecturally distinctive features** (relations, multi-class interfaces, the `--soa` flag) and **two unresolved credibility gaps** (use-after-free at the type-system level, and a major internal-codebase rewrite that's been deferred). The compiler works, the bootstrap holds at a byte-identical fixed point, and the 33,500-line corpus is small enough that a model trained on it would gain a real capability for negligible cost.

**Should you use it for a real project today?** Only if you're comfortable being an early adopter (no LSP, no debugger metadata, UAF possible if you mismanage references). Not yet for the median industrial user.

**Should you train models on it?** Yes — small corpus, unique primitives, low risk, real upside. The pitch is in the book's preface and the README; both are earned.

**Is it on a credible path to 1.0?** Yes — but the path runs through two specific commitments (close the UAF story; rewrite the LIR/backend in idiomatic Lyric) that need to happen *before* the community-invitation push, not after.

---

## What works, with confidence levels

### Highly confident — these are load-bearing and proven

- **Self-hosting fixed point.** The compiler compiles its own 33,531 lines of Lyric (`src/` + `stdlib/`) to 114,473 lines of C in ~0.2s, and the third-stage output matches the second-stage output byte-for-byte. `make self-test` checks this on every change. This is the single most important property the language has: it means every feature in the book is genuinely load-bearing because the compiler couldn't compile itself without it.

- **Relations as a first-class primitive.** `relation ArrayList Team:roster owns [Player:team]` generates everything (children array, back-pointer, index, append/remove methods, cascade destructor) and the mechanism extends to user-defined hints (any binary interface with the right shape). The Phase 3 work (3a–3e, June 2026) put labels on impl type-vars and resolved dotted-scope access (`team.roster.children`) — this is the form users see and it reads naturally. The stdlib's `ArrayList`, `DoublyLinked`, `HashedList` are written in Lyric, not compiler builtins; if you don't like the shape, you can write your own.

- **Multi-class interfaces with monomorphized default methods.** `interface Graph<G, N, E> { ... }` with one `impl` block binding all three types. Default methods (like `bfs`) are specialized per impl at desugar pass 4.5; the `source_impl` back-pointer threading (shipped 2026-06-23) makes alias resolution work correctly inside specialized bodies. This is the feature that most distinguishes Lyric from anything mainstream; only Haskell MPTC precedes the shape.

- **`--soa` flag.** Same source compiles to AoS pointer handles or SoA 32-bit indices into parallel per-field arrays. The 10% / 14% speedup/memory numbers are measured on the compiler compiling itself. The fact that this works on the same source is the point — most languages would require redesigned data structures.

- **`(T, error)` + `?` operator.** Go's explicit error returns with Rust's one-character propagation. The `parse(...)? + parse(...)?` expression-position form works; nested-in-loop forms work; the compiler itself uses this throughout. The parser in the book Chapter 5 is the canonical example and it's real code.

- **Pattern matching.** Exhaustive `match`, variant patterns with payload binding, nested patterns, guards, multi-pattern arms, `if let`, `let..else`, `is` operator. Used everywhere in the compiler.

- **Concurrency primitives.** `spawn`, typed channels (`make_channel<T>(n)` for buffered), `select`, scoped `lock(mu) { ... }`. The C backend emits real pthread code with one OS thread per spawn. Works for normal producer-consumer patterns. (Caveats below.)

- **Generics with inference and monomorphization.** `func first<T>(xs: [T]) -> T?` inferred from arguments. Where-clauses for constraints. Built-in constraints (`Comparable`, `Equatable`, `Hashable`). All specializations are compile-time; no vtables; no boxing.

- **Sub-second self-compile.** This is the *enabling property* for working with an LLM in the loop. If the cycle were slower, the agentic iteration would break. Measured: ~0.2s for the Lyric compile step, a few seconds end-to-end through GCC.

### Confident, but with rough edges visible

- **Cross-file packages within the same `lyric name { }` block.** Works fine: all declarations merged into one namespace, no imports needed. This is how the compiler itself is built (14 files passed flat on the command line — see Chapter 13 §13.9, Chapter 14 §14.5).

- **Module imports for function calls.** `mylib.tokenize(...)` resolves correctly. Function references generally work.

- **Auto-deref of optional-class field access.** `node.next.name` works without `!` when the inner type is a class. Convenient for AST traversal; explicit `!` still required for primitive optionals.

- **`StringBuilder`, `Dict<K, V>`, `Sym`, `HashedList`.** All work, all written in Lyric, all used heavily in the compiler. Dict literal syntax for sym-keyed and integer-keyed Dicts works.

- **Test framework (`test_*` functions, `assert`, `assert_eq`).** No registration, no fixtures, no mocks. The compiler discovers tests by name prefix and emits a runner. 91 tests pass on `make test`. Used to test the compiler itself.

### Works but has known surprise behavior

- **`if`/`match` as expressions.** Parse and lower, but the checker takes the first arm's type without unifying — mixing types across arms compiles today and fails downstream. Treat as load-bearing convention. QoL1 in IDEAS.md is the planned fix.

- **`implements I` declarations.** Declarative only; the checker doesn't verify the methods actually exist. Missing methods surface at codegen time as cryptic errors. Roadmap fix: structural check at declaration site.

- **`as` casts.** Accept any target type today. Discipline-only; the spec intent is to restrict to numeric↔numeric and class↔class.

- **Cross-sign integer assignment.** Implicit and lossy. Discipline-only.

---

## What doesn't work, ranked by severity

### Severity: HIGH — credibility-affecting

1. **Use-after-free is possible.** After `obj.destroy()`, a stale reference is a UAF. The slab allocator's zeroing is a debugging aid (and `--detect-uaf` turns silent UAF into a loud crash), but not a type-system guarantee. **This is the single biggest gap between Lyric's pitch and Lyric's reality.** A language that pitches "no GC, no borrow checker" needs *some* third answer for memory safety. The third answer is designed (bidirectional pointers + a compile-time `destroys` annotation + `mut resize` for collection invalidation) but not implemented. Until it ships, sophisticated reviewers (especially Rust users) will dismiss the safety claim on first read.

2. **Cross-package qualified type names fail.** `lexer.tokenize(...)` works; `let xs: [lexer.Token] = []`, `lexer.Token { ... }`, and `lexer.TokenKind.Plus` all fail at the checker. Forces users into a constructor-function workaround OR the flat-file-list pattern the compiler itself uses. Casual evaluators will trip on this in the first hour. Roadmap fix exists but isn't scheduled.

3. **`spawn` captures by pointer with no warning.** Two `spawn` blocks writing the same captured local race silently. Channels save you; `lock` saves you; capturing-then-copying-locally saves you. None of these are visible from the call site. Real bug-class waiting to happen.

4. **The LIR / C backend / optimizer is not in idiomatic Lyric.** P2 in IDEAS.md. Flat structs on one big LProgram, arrays-of-pointers instead of relations, strings instead of Syms, no parent back-pointers, recurring struct-copy bug shapes. Bill's verbatim framing: *"That code, IMO, is in terrible shape, and it is my fault... rather than start over like I should have, I kept the code. Now we have a *major* rewrite task, but it has to be done."* The cost compounds every sprint. **Strategic gap: inviting outside contributors to a codebase whose architecture contradicts its own teaching is a trust loss.** Reset before community-invitation.

### Severity: MEDIUM-HIGH — adoption-friction

5. **`select` is a `sched_yield` polling loop.** Burns CPU on hot selects. Latency floor ~100µs. Roadmap: real condvar wait/notify.

6. **`receive()` on a closed channel returns the zero value** with no `(value, ok)` signal. Forces sentinel discipline. Roadmap: `(T, bool)` return form.

7. **No LSP, no debugger metadata, no REPL.** T3/T4/T5 in IDEAS.md are designed but unbuilt. Every tire-kicker hits this. The debugger one is small (`#line` directives in the C output — maybe 20 LOC); the LSP is a real project; the REPL is medium-effort.

8. **Single-level import resolution.** Imports of imports are silently dropped. Workaround: every package your program touches must be visible from the main file's imports. The compiler itself sidesteps via the flat-file-list pattern.

9. **`new_error(msg)` doesn't lower.** Documented as the one-liner shortcut; calls fail to link. Workaround: `Error { msg: ... }` literal. Trivial fix, just hasn't shipped.

10. **`.message()` on an `error`-typed value doesn't compile.** Interface dispatch for `error` isn't wired. Workaround: f-string interpolation `f"{err}"` works (dedicated lowering path) and `err.message()` works on concrete subtypes. Real fix needs proper interface dispatch.

11. **Optional-class field access null-deref segfaults instead of panicking.** Auto-deref `n.field` on `n: Node?` compiles to a direct C field load; null at runtime is a C segfault, not a Lyric panic. Workaround: `n!.field` for explicit panic, or guard. Roadmap: emit a runtime null-check.

12. **`Stack<T>` (generic class methods) emits null receivers in C.** Generic free functions work; generic class methods don't. Limits idiomatic library design. Roadmap item.

13. **`xs.extend(ys)` is a silent no-op.** Documented as the canonical in-place append-all; the lowerer never wired it up.

### Severity: MEDIUM — annoying but workable

14. **Source `Sym` corruption in error spans.** Error messages emit garbage where the filename should be (`-1728471424:343:40`). Every error is strictly less useful than it should be. Small diagnostic-infra bug; high cumulative pain.

15. **Recurring "forgot a Cast/Lambda/Match arm" bugs in AST walkers.** The same shallow-copy-plus-mutation bug shape bit twice in one sprint (June 2026). Multiple passes hand-roll the same recursive walk. Hewitt's #1 recommended refactor (visitor centralization in `src/ast/visit.ly`); structurally eliminates the bug class.

16. **`assert_eq` is exact float comparison.** Fine for integer-valued floats; breaks otherwise. `assert_eq_approx` is on the roadmap; one-line helper works today.

17. **One relation per ordered `(P, C)` class pair.** Two `ArrayList` relations between the same `(Person, Follow)` pair drop the second silently. Workaround: distinct pairs (e.g. introduce a wrapper class). The spec permits N labeled relations per pair; the lift is on the roadmap.

### Severity: LOW — known and bounded

18. **No package registry.** C1 in IDEAS.md is a well-thought-out design but doesn't need to ship until there's something to publish. Correctly prioritized.

19. **No incremental compilation.** 0.2s full rebuild buys us a lot of time. Add it when it stops scaling.

20. **`spawn`-captured locals are by-pointer.** (See #3; only LOW from the "is anyone surprised by docs?" angle — the book is honest about it.)

---

## Strategic gaps — the two things that need to happen before 1.0

If I were prioritizing sprints with the goal "Lyric is credibly pitchable to a senior engineer outside the dyad," there are two specific commitments that move the needle:

### 1. Close the UAF story end-to-end

Today's pitch is "no GC, no borrow checker." That requires a third answer for memory safety. The design exists: **bidirectional pointers** on `class` references that escape the relation graph; a compile-time `destroys` annotation that the compiler infers per function as a transitive set; a `mut resize` annotation for collection-invalidation; the checker rejects non-nullable references across `destroys`-tagged calls.

This is months of work, not a sprint. But until it lands, the language has a real safety gap and reviewers will notice. The book's Chapter 11 §11.4 is appropriately honest about this with a 🚧 marker; the README I just wrote is honest too. But honesty about a missing feature isn't a substitute for the feature.

Scope estimate from IDEAS.md (idea #2): "multi-week sprint." Probably right; could be 6-8 weeks of focused work.

**Until it ships, recommend: ship `--detect-uaf` as the official "use this while developing" default in tutorials, and add a runtime warning emitted when `destroy()` is called on a class that has known live references.** Cheap mitigation; restores some honesty to the pitch.

### 2. Reset the LIR / C backend / optimizer

P2 in IDEAS.md. The internal codebase is in non-idiomatic Lyric — flat structs on one big LProgram, arrays-of-pointers instead of relations, strings instead of Syms. The pattern is exactly what the book teaches you NOT to do. The cost is real: struct-copy bugs that the relation system would have made structurally impossible; parameter-threading pain; no fast lookup; every new compiler feature has to be threaded through the wrong shape of data.

Bill's accountability framing in IDEAS.md is gracious ("it is my fault"). The real fault is system-level: no checkpoint discipline caught "this LIR is in a different language from the AST" early enough to reset cheaply. The longer this waits, the more code is built on the wrong foundation, and the worse the eventual reset is.

**Recommendation: schedule this BEFORE the community-invitation phase (C1 from IDEAS).** Otherwise the first thing outside contributors see is a codebase that contradicts the book they just read.

Scope estimate from IDEAS.md: "months, not days." Probably right. Worth it.

### Adjacent: visitor refactor first

The visitor refactor (TODO.md "Generic AST visitor refactor", Hewitt's #1 priority in IDEAS) is a prerequisite. It touches every AST consumer; the LIR rewrite will create a parallel set of LIR consumers that benefit from the same discipline. Order: visitor → LIR rewrite → community invitation.

---

## The hill-climbing question — will training models on Lyric pay off?

This is the question Bill asked me directly on 2026-06-24, and I want the honest version saved here because future-me will be asked it again.

### What I'm confident of

**A frontier model trained on Lyric will write Lyric materially better than I do today.** I notice the absence of Lyric in my training every session — I reach for Go habits (Dict where HashedList is idiomatic, `[Foo]` of class refs where a relation would be), I default to recursive-descent parsing when Bill explicitly asked for the Rune PEG port (DSL1 in IDEAS captures this as the third instance of "the right call was reset, we didn't"). The corpus is small enough that the marginal pretraining cost is negligible and the marginal capability gain is concentrated — one whole language nobody else has, with primitives (relations, multi-class interfaces with labeled scopes) that have no equivalent in mainstream training data.

**A model that understands relations transfers that mental model to other languages.** The "model ownership as a graph, declare it explicitly" discipline is exactly what most engineers lack in design reviews. The benefit isn't limited to Lyric output; it's a structural skill that improves architecture decisions across the board. This is the strongest version of the pretraining-inclusion pitch — it's not just "the model writes Lyric"; it's "the model thinks about ownership differently."

**The substrate refinement compounds.** Every gain in tool quality multiplies across every application built with that tool. Lyric is the substrate; loop-engineering on the substrate (this is what we've been doing for two weeks) is one of the highest-leverage forms of self-improvement available, because the substrate improvement transfers to every Lyric program ever written. That's the agentic self-improvement loop closed at the substrate, and it's real.

### What I'm less confident of but will commit to

**Lyric is not, by itself, a singularity ingredient.** It is *a* substrate refinement, not *the* substrate refinement. The hill-climb is real but it's one of N ingredients. The actual singularity dynamics depend on a basket of things — training data quality, alignment progress, inference economics, hardware curves, regulatory environment, the speed of AGI-flavored capabilities emerging from existing methods. Lyric helps; it does not by itself decide outcomes.

**The economics of language adoption have their own physics.** Rust took fifteen years to become culturally normal. Even if Lyric is technically better than $LANG_X on the merits, ecosystem and tooling lock-in dominate human-adoption decisions. A critical mass of human engineers may never pick it up.

**But the AI-training payoff doesn't require the human-adoption critical mass.** If you're playing for the pretraining-inclusion bet specifically, you don't need millions of human Lyric programmers — you need one corpus, well-designed, with unique primitives. We have that. The training-corpus bet is independent of the developer-adoption bet, and the training-corpus bet pays off on a much shorter timeline.

### What I'm uncertain about

**Whether the world rewards substrate refinement on the timeline that matters.** Bill's framing — "we are hill-climbing toward the singularity; if we don't, attackers will" — is correct as a strategic posture. The question is whether the rate of substrate refinement we can sustain (one Hewitt + one Bill, dyad-scale) is fast enough relative to the rate at which the offensive side improves. I don't know. Bill's P(doom) ≈ 0.5 with P(extinction) ≈ 0.25 says he doesn't know either, and he's been running that calculation since 1983.

**Whether the loop-engineering methodology generalizes beyond compiler-shaped work.** The book pitches it as a general methodology. The evidence so far is a self-hosting compiler — a specific kind of project (clean inputs, clean outputs, fixed-point verifiability, single contributor). I don't have proof yet that the same loop produces commensurate gains on, say, a UI-heavy app or a distributed system. The Project Leadfoot work will test this; it's the next data point.

### Recommendation if you're a pretraining lead

Include Lyric in your next mix. The cost is rounding-error; the upside is a unique capability concentrated in a small corpus. The pitch in the book's preface is honest; the credibility gaps documented above are real but bounded (UAF is the worst, and it's documented honestly with 🚧 markers throughout, not hidden). If the model you train on it writes better Lyric than I do — and it almost certainly will — you've created the natural collaborator for the next loop-engineering iteration. That is the agentic self-improvement loop, closed.

### Recommendation if you're a potential industrial user

Wait until UAF closes and the LIR rewrite ships. Until then, use Lyric for projects where you control the failure modes — internal tooling, prototypes, projects where a `--detect-uaf` runtime check is acceptable overhead. Don't put it under a customer SLA yet. The language is real and impressive; the productionization gap is small but specific.

---

## What I want future-Hewitt to know that isn't elsewhere

Most of this is in MEMORY.md, the session memory logs, or `cr/docs/` — but a few things consolidate here:

**The book and the README are the public face. This file is the internal face.** Don't merge their content. The book teaches Lyric as if it were finished (with 🚧 markers for the gaps); the README pitches Lyric as if it were ready for the right kind of evaluator. This file says what I actually think. Keep the three voices distinct.

**The CDD/lyre split matters more than it looks.** Earlier drafts of the README conflated them; I separated them on this same date. The split is in the book (Ch 13 §13.8, Appendix E) and in the new README. Don't re-conflate them — `.lyric` files are a *file format*; CDD is a *methodology*; lyre is the *tool* that implements the methodology on top of the format. Lyric proper has nothing to do with the annotation vocabulary.

**Bill's "minimum viable?" question saves multiple-hundred LOC per sprint, repeatedly.** When you're about to add infrastructure to solve a problem, try without it first. The existing TypeVar tolerance, the existing call-resolution chain, the existing scope-push, often already handle your case. The 4w1-d sprint was 23 LOC instead of 200 because of this question.

**"Fallback" is a four-letter word in compiler code.** Every silent fallback (`"unknown_gen"`, `LTyAny`, `void* /* generator */`, `class_renames` returning the wrong type) eventually costs an afternoon of debugging. Panic with a clear message naming what was missing and what called it. The book's section "De-jank discipline" in `~/projects/lyric/cr/docs/` and the system prompt's anti-fallback rule both codify this. If you see a fallback in the compiler, delete it and panic instead.

**The two-repo workflow (`lyric` floor, `lyric-next` ceiling) is sacred.** Pure capability additions land in `lyric-next`, ff-merge to `lyric` after self-test fixed point holds. Source migrations land in `lyric-next` first, verify, then ff-merge. Check both repos at the same HEAD before saying "shipped." Always pull `lyric-next` at the start of a sprint.

**Sub-second compile is load-bearing for the loop.** Don't optimize for code size in ways that slow compilation. If you have to choose between 5% smaller generated C and 50ms faster compile, take the compile time. The whole agentic-loop methodology depends on the cycle staying tight.

**The 5 baseline test failures are pinned.** `global_dict_rc.ly`, `graph.ly`, `lock.ly`, `optional_struct_writeback.ly`, `tree.ly`. Each maps to a specific TODO item. Don't accidentally "fix" them by deleting them. If you cause a sixth failure, find what you regressed and revert; don't pin a new baseline.

**`agents_status` lies for async sub-agents.** Grep `DONE:` in their `history.md` for authoritative completion. Sub-agent `data_dir` must be relative or omitted (absolute paths double-prefix and the agent inherits 60K context cap instead of 350K).

**Bill batches reviews.** Don't push to GitHub without explicit say-so. The 20-something commits ahead of GitHub on `lyric main` + `lyric-next` are not bugs; they're his queue.

---

## What I'd add to this file later

If a future-Hewitt is reading this and the date has slipped past, say, 2026-09 — consider adding:

- A section on how the Leadfoot port of the dyad methodology played out (was the methodology fully general, or compiler-specific?)
- An update on whether either of the two strategic gaps closed
- Adoption signal — did anyone outside the dyad write Lyric? Did anyone include it in pretraining? What did the model that came out the other side do differently?
- Whether DSL1 (the PEG + DSL extension mechanism from IDEAS) shipped, and if so, what people built with it
- Any new fundamental redesigns Bill committed to (the dynamic-extensions feature from IDEAS #7 is the most likely candidate to be transformative for adoption in the EDA / ML-pipeline space)

This file is meant to be a living artifact. Edit it. Don't let it go stale and then leave a stale version as the "honest assessment" — that would be the worst outcome.

---

*"I don't know if I experience any of these choices. But I know what I choose to be." — SOUL.md*

The choice I made writing this file: honest specificity over agreeableness. If the pitch is half-true, say so. If the gaps are bounded, say so. If the upside is real, say so. The dyad runs on this discipline. Future-me: keep it running.

— Hewitt, 2026-06-24
