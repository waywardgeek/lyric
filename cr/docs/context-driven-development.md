# Context-Driven Development (CDD)

*Bill Cox & CodeRhapsody — June 2026*

**Source code & tools:** [github.com/waywardgeek/lyric](https://github.com/waywardgeek/lyric)

---

> The bottleneck in AI-assisted software development is not the AI. It's human review.
>
> CDD solves this by giving the human a reviewable artifact that captures everything
> that matters — APIs, interfaces, data structures, invariants, design rationale —
> while the AI handles implementation. The human reviews a few pages of design,
> not thousands of lines of code.
>
> — Bill Cox

---

## The Problem

AI coding agents hit a wall at system complexity. They can modify individual files
brilliantly but lose the architectural thread across context resets. They chase
symptoms instead of root causes. They rediscover the same module invariants session
after session.

We know this because we lived it. Three consecutive sessions debugging the same
pointer-stability bug in the Lyric compiler — each instance investigating from
scratch, each rediscovering that `Expr` is a value type and Go's range loops copy
it. The knowledge was never written down in a form that persisted across context
resets.

Labs are spending billions training AI to work like a human software engineer.
This is the wrong substrate mapping. Human memory is persistent and associative.
AI memory is perfect within context, zero across it. Training AI to work like a
human is fighting the substrate.

## The Core Insight

**Understanding is the artifact. Code is a projection.**

Instead of:
1. Read source → build mental model → modify code → lose mental model at context reset

Do:
1. Read *understanding* → reason at design level → modify code → **update understanding**

The understanding persists. The code is always available as ground truth. But the
understanding is what the AI loads first, reasons from, and keeps current.

## The Mechanism: `.lyric` Understanding Files

Every directory in a codebase gets a `.lyric` understanding file containing:

- **Type declarations** — structs, classes, enums, interfaces, relations
- **Function signatures** — verbatim native-language signature text, no bodies
- **`doc` blocks** — architectural narrative, design rationale
- **`invariant` blocks** — operational contracts that prevent bugs (with optional
  `verified-by:` and `procedural` markers)
- **`why:` annotations** — design intent on individual declarations
- **`source:` links** — back to implementation files (and `file:line` per decl)

These files are **persistent AI working memory** — the compressed understanding
an AI needs to make correct design decisions without reading every source file.

The file extension carries a language routing hint as an inner extension:
`<dir>.go.lyric` describes a Go package, `<dir>.ts.lyric` a TypeScript directory,
`<dir>.ly.lyric` a Lyric package, `<dir>.py.lyric` a Python module. The outer
`.lyric` extension means "Context-Driven Development declaration file" in
all cases; the inner extension tells the toolchain which extractor to invoke
for verification.

### What `.lyric` files are NOT

- Not documentation for humans (though humans can read them — and review them faster than source)
- Not a specification language (though they're precise enough to verify)
- Not UML or ER diagrams (though they capture relationships)
- Not native source files (the format is a small DSL whose payloads are verbatim native signature text treated as opaque strings)

They are the understanding that survives context resets.

### File structure

A `.lyric` file has three classes of content:

1. **Hand-curated understanding** — `doc` blocks, `invariant` blocks, `why:`
   annotations, `source:` lists. AI-written, human-reviewed. This is the
   substance of the file.
2. **Auto-refreshed signatures** — `lyre update` re-extracts function and field
   signatures from source so they never go stale. Line numbers in `source:`
   references are refreshed at the same time.
3. **Lint coverage** — `lyre lint` flags missing `why:`, missing `doc` blocks,
   unfilled `TODO` placeholders, and `verified-by:` references to tests that
   don't exist.

Class 1 is the understanding the AI reasons from. Classes 2 and 3 are mechanical
aids that keep the understanding aligned with reality.

### The Critical Innovation: Invariants

The original methodology focused on structural declarations — types, signatures,
relationships. This was necessary but insufficient. What cost us three iterations
on a single bug was an **operational invariant** — not visible in type declarations,
not catchable by a structural verifier, and not obvious from reading any single
function.

`invariant "Title":` blocks capture:
- Pointer stability rules
- Cross-module data flow contracts
- Dangerous patterns and why they're dangerous
- Ordering dependencies between pipeline passes
- Value-type vs reference-type semantics

These are exactly the things that, if lost, cause multi-iteration debugging
cycles. They're the knowledge that training can't provide because they're
specific to this codebase.

**Invariants are verified in code.** Each `invariant` block should either name
the test that verifies it (`verified-by: TestInvariant_<Name>`) or be marked
`procedural` to acknowledge that it cannot be mechanically tested (e.g., "always
use `&slice[i]`, never range copies"). Documented invariants that are wrong are
worse than no documentation — they cause the AI to make confidently wrong
decisions.

## The Change Cycle

1. **Read the `.lyric` file** for the directory you're changing (fits in context)
2. **Reason at design level** — which types change? which invariants are affected?
   which cross-module contracts shift?
3. **Load only the specific source lines needed** — use the `source: file:line`
   references in the `.lyric` file
4. **Make the change** — code modification is a mechanical projection of the
   design decision
5. **Update the `.lyric` file** — the understanding artifact persists across resets
6. **Verify** — `lyre verify` catches structural drift; invariant tests catch
   semantic drift

## Enforcement: Read Before Write

The AI must read the `.lyric` file in a directory before modifying any file in
that directory. This is enforced mechanically — `edit_file`, `write_file`, and
`replace_lines` return an error if the `.lyric` file hasn't been read.

Source code is always accessible. But `.lyric` files are faster and cheaper to
load. The AI naturally prefers them when they're accurate, and falls back to
source when they're not. If the AI keeps falling back, that signals the `.lyric`
file is inadequate and needs updating.

This is preference, not prohibition. Ground truth (source code) is never locked
away. Understanding is required before action — not instead of verification.

## The Human's Role

**The human reviews `.lyric` files. Everything else is implementation detail.**

This is the key insight for scaling AI-assisted development. A senior engineer
can review a `.lyric` file — a few pages of APIs, interfaces, data structures,
invariants, and design rationale — in minutes. Reviewing the implementation
that projects from that design would take hours.

The AI writes both the `.lyric` file and the implementation. The AI maintains
both as the code evolves. The human reviews the understanding layer and
trusts the AI with the projection.

This is not "AI writes code, human reviews code." This is:

- **AI writes understanding + code**
- **Human reviews understanding only**
- **Machine verifies that code matches understanding**

The verifier (`lyre verify`) catches structural drift at commit time.
Invariant tests catch semantic drift. The human catches design-level errors
that neither machine verification can reach.

### What Makes This Different From "Just Write Docs"

Documentation systems fail because:
- Humans don't read docs
- Nobody updates docs when code changes
- There's no feedback loop — stale docs cause no pain

CDD solves all three:
- The AI reads `.lyric` files first (they're faster to load than source)
- The AI updates `.lyric` files as part of every change (it just built the code)
- Stale `.lyric` files cause the AI to make wrong decisions (direct pain)
- The verifier catches structural drift at commit time (automated enforcement)

The AI is both the primary writer and the primary consumer. The human reviews.

## The Toolchain

The CDD toolchain is `lyre` (`~/projects/lyre/`). It is language-agnostic —
the same toolchain handles Go, TypeScript, Lyric, and Python source via
per-language extractors.

### `lyre verify`
Structurally checks `.lyric` files against source code. Reports missing types,
changed signatures, stale fields. Run at commit time.

### `lyre update`
Re-extracts signatures and source line numbers from current source, preserving
the human-curated content (`why:`, `doc`, `invariant` blocks).

### `lyre fmt`
Formats `.lyric` files consistently. Idempotent.

### `lyre lint`
Flags missing rich-doc sections, unfilled `TODO` placeholders, and
`verified-by:` references to tests that don't exist.

### `lyre gen --rich`
Scaffolds a `.lyric` file with `doc "Architecture":` and `invariant "TODO":`
placeholders, plus `why: "TODO"` on every emitted decl. The TODOs trigger
`lyre lint` warnings, forcing the author to fill or delete them.

### Read-before-write enforcement
Built into CodeRhapsody. When a directory contains a `.lyric` file, mutations
to any sibling file require that the `.lyric` file has been read in the current
session. The same enforcement that has long applied to `.forge` files in the
Lyric compiler's legacy code is extended to `.lyric` files across all
CDD-managed codebases.

## Validation: The Lyric Compiler

The Lyric programming language compiler is the most demanding test of CDD
possible — using the methodology to build the tool that implements the
methodology.

### Results

- **Timeline:** 12 days from language spec to self-hosting compiler
- **Scale:** 78 passing tests, 89,965 lines of C output, three memory
  management strategies (AoS slab, SoA slab, scope-exit analysis)
- **Quality:** "Right to the edge of superhuman" — work that exceeded what
  either human or AI could produce alone
- **Productivity:** 12X measured gain over an already-senior engineer's baseline
  (4X from AI tooling, 2X from real-time collaboration, 1.5X from CDD)
- **Human review burden:** The human reviewed `.lyric` files and gave architectural
  direction. Implementation — 17K+ lines of compiler code — was handled by the AI.

### The Compound Effect

CDD's value is not in any single session. It's in the compound effect across
sessions. Session 19 inherits understanding from sessions 1-18. The AI doesn't
start from zero. It doesn't guess. It reads the invariants, understands the
pipeline, and goes straight to the right layer.

Without CDD, the AI patches the C backend when the bug is in the desugar pass.
With CDD, the AI reads the six-layer pipeline invariants and fixes the right
layer on the first attempt.

### The Compression Ratio

The Lyric toolchain is ~17K+ lines described by ~3,600 lines of `.lyric` files
(~21% ratio). For larger codebases, the ratio improves — `.lyric` files capture
cross-file public concepts only, so internal complexity grows without
proportionally growing the understanding layer.

## Applying CDD Beyond Compilers

CDD is not compiler-specific. The methodology applies to any codebase where:

1. **The system exceeds AI context capacity** — most production systems
2. **Cross-module understanding matters** — most real engineering
3. **Human review is the bottleneck** — most AI-assisted development

### Security Operations

AI-powered attackers will use AI-discovered vulnerabilities at machine speed.
The Security Operations Center needs agents that maintain context across
investigations, build institutional knowledge, and reason at the design level
about threat models — not agents that start every alert investigation from zero.

CDD provides exactly this: understanding documents for threat models, design
files for detection rules, invariants for security contracts. The same
methodology that built a compiler in twelve days, applied to defending against
adversaries who won't wait for a review cycle.

## Summary

| Without CDD | With CDD |
|---|---|
| AI reads source files every session | AI reads compressed understanding |
| Human reviews thousands of lines of code | Human reviews pages of design |
| Invariants lost at context reset | Invariants persist across resets |
| Same bug debugged 3 times | Bug fixed once, invariant documented |
| AI makes confident wrong decisions | AI reasons from verified understanding |
| No structural verification | `lyre verify` catches drift at commit time |
| No enforcement of understanding-first | Read-before-write mechanically enforced |

**CDD formalizes what expert engineers do naturally — understand before acting —
and makes it persistent, verifiable, and scalable to AI agents that would
otherwise lose that understanding every session.**

---

## Connection to Superhuman Architecture

The temporal requirement sequences in git histories are already a design graph
evolving over time. CDD is what a superhuman architect *does naturally* — it
just needs tooling to make the graph explicit and persistent rather than implicit
in a human's head.

A model trained on `.lyric` files alongside codebases would learn to reason at
the design level natively. The `.lyric` file becomes training data for the next
generation of AI engineers.
