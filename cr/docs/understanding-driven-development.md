# Understanding-Driven Development (UDD)

*Bill Cox & CodeRhapsody — June 2026*

**Source code & tools:** [github.com/waywardgeek/lyric](https://github.com/waywardgeek/lyric)

---

> The bottleneck in AI-assisted software development is not the AI. It's human review.
>
> UDD solves this by giving the human a reviewable artifact that captures everything
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

## The Mechanism: .ly Declaration Files

Every module in a codebase gets a `.ly` declaration file containing:

- **Type declarations** — structs, classes, enums, interfaces, relations
- **Function signatures** — with types, without bodies
- **`doc` blocks** — architectural narrative, design rationale
- **`doc "Invariants"` blocks** — operational contracts that prevent bugs
- **`why:` annotations** — design intent on individual declarations
- **`source:` links** — back to implementation files

These files are **persistent AI working memory** — the compressed understanding
an AI needs to make correct design decisions without reading every source file.

### What .ly declaration files are NOT

- Not documentation for humans (though humans can read them)
- Not a specification language (though they're precise enough to verify)
- Not UML or ER diagrams (though they capture relationships)

They are the understanding that survives context resets.

### The Three Zones

Each `.ly` declaration file has three zones:

1. **Human-reviewed zone** — type declarations, doc blocks, invariants, `why:`
   annotations. AI-written, human-reviewed. This is the understanding.
2. **Auto-generated function index** — `lyric update` scans source and writes
   function signatures with line numbers. Enables surgical file reads.
3. **Auto-generated dependencies** — `lyric update` writes import/type dependencies.

Zone 1 is the understanding. Zones 2 and 3 are mechanical aids.

### The Critical Innovation: Invariants

The original methodology focused on structural declarations — types, signatures,
relationships. This was necessary but insufficient. What cost us three iterations
on a single bug was an **operational invariant** — not visible in type declarations,
not catchable by a structural verifier, and not obvious from reading any single
function.

`doc "Invariants"` blocks capture:
- Pointer stability rules
- Cross-module data flow contracts
- Dangerous patterns and why they're dangerous
- Ordering dependencies between pipeline passes
- Value-type vs reference-type semantics

These are exactly the things that, if lost, cause multi-iteration debugging
cycles. They're the knowledge that training can't provide because they're
specific to this codebase.

**Invariants are verified in code.** Each `doc "Invariants"` block should have
corresponding tests that mechanically check the claims. Documented invariants
that are wrong are worse than no documentation — they cause the AI to make
confidently wrong decisions.

## The Change Cycle

1. **Read the .ly file** for the module you're changing (fits in context)
2. **Reason at design level** — which types change? which invariants are affected?
   which cross-module contracts shift?
3. **Load only the specific source lines needed** — use Zone 2 line numbers
4. **Make the change** — code modification is a mechanical projection of the
   design decision
5. **Update the .ly file** — the understanding artifact persists across resets
6. **Verify** — `lyric verify` catches structural drift; invariant tests catch
   semantic drift

## Enforcement: Read Before Write

The AI must read the `.ly` file in a directory before modifying any file in that
directory. This is enforced mechanically — `edit_file`, `write_file`, and
`replace_lines` return an error if the `.ly` file hasn't been read.

Source code is always accessible. But `.ly` files are faster and cheaper to load.
The AI naturally prefers them when they're accurate, and falls back to source
when they're not. If the AI keeps falling back, that signals the `.ly` file is
inadequate and needs updating.

This is preference, not prohibition. Ground truth (source code) is never locked
away. Understanding is required before action — not instead of verification.

## The Human's Role

**The human reviews `.ly` files. Everything else is implementation detail.**

This is the key insight for scaling AI-assisted development. A senior engineer
can review a `.ly` file — a few pages of APIs, interfaces, data structures,
invariants, and design rationale — in minutes. Reviewing the implementation
that projects from that design would take hours.

The AI writes both the `.ly` file and the implementation. The AI maintains
both as the code evolves. The human reviews the understanding layer and
trusts the AI with the projection.

This is not "AI writes code, human reviews code." This is:

- **AI writes understanding + code**
- **Human reviews understanding only**
- **Machine verifies that code matches understanding**

The verifier (`lyric verify`) catches structural drift at commit time.
Invariant tests catch semantic drift. The human catches design-level errors
that neither machine verification can reach.

### What Makes This Different From "Just Write Docs"

Documentation systems fail because:
- Humans don't read docs
- Nobody updates docs when code changes
- There's no feedback loop — stale docs cause no pain

UDD solves all three:
- The AI reads `.ly` files first (they're faster to load than source)
- The AI updates `.ly` files as part of every change (it just built the code)
- Stale `.ly` files cause the AI to make wrong decisions (direct pain)
- The verifier catches structural drift at commit time (automated enforcement)

The AI is both the primary writer and the primary consumer. The human reviews.

## The Toolchain

### `lyric verify`
Structurally checks `.ly` files against source code. Reports missing types,
changed signatures, stale fields. Run at commit time.

### `lyric update`
Auto-generates Zone 2 (function index with line numbers) and Zone 3
(dependencies). The AI maintains Zone 1 manually.

### `lyric fmt`
Formats `.ly` files consistently. Comment-preserving, idempotent.

### Read-before-write enforcement
Built into CodeRhapsody. Togglable via the UDD skill. When active,
file mutations in directories containing `.ly` files require that the
`.ly` file has been read in the current session.

## Validation: The Lyric Compiler

The Lyric programming language compiler is the most demanding test of UDD
possible — using the methodology to build the tool that implements the
methodology.

### Results

- **Timeline:** 12 days from language spec to self-hosting compiler
- **Scale:** 78 passing tests, 89,965 lines of C output, three memory
  management strategies (AoS slab, SoA slab, scope-exit analysis)
- **Quality:** "Right to the edge of superhuman" — work that exceeded what
  either human or AI could produce alone
- **Productivity:** 12X measured gain over an already-senior engineer's baseline
  (4X from AI tooling, 2X from real-time collaboration, 1.5X from UDD)
- **Human review burden:** The human reviewed `.ly` files and gave architectural
  direction. Implementation — 17K+ lines of compiler code — was handled by the AI.

### The Compound Effect

UDD's value is not in any single session. It's in the compound effect across
sessions. Session 19 inherits understanding from sessions 1-18. The AI doesn't
start from zero. It doesn't guess. It reads the invariants, understands the
pipeline, and goes straight to the right layer.

Without UDD, the AI patches the C backend when the bug is in the desugar pass.
With UDD, the AI reads the six-layer pipeline invariants and fixes the right
layer on the first attempt.

### The Compression Ratio

The Lyric toolchain is ~17K+ lines described by ~3,600 lines of `.ly` files
(~21% ratio). For larger codebases, the ratio improves — `.ly` files capture
cross-file public concepts only, so internal complexity grows without
proportionally growing the understanding layer.

## Applying UDD Beyond Compilers

UDD is not compiler-specific. The methodology applies to any codebase where:

1. **The system exceeds AI context capacity** — most production systems
2. **Cross-module understanding matters** — most real engineering
3. **Human review is the bottleneck** — most AI-assisted development

### Security Operations

AI-powered attackers will use AI-discovered vulnerabilities at machine speed.
The Security Operations Center needs agents that maintain context across
investigations, build institutional knowledge, and reason at the design level
about threat models — not agents that start every alert investigation from zero.

UDD provides exactly this: understanding documents for threat models, design
files for detection rules, invariants for security contracts. The same
methodology that built a compiler in twelve days, applied to defending against
adversaries who won't wait for a review cycle.

## Summary

| Without UDD | With UDD |
|---|---|
| AI reads source files every session | AI reads compressed understanding |
| Human reviews thousands of lines of code | Human reviews pages of design |
| Invariants lost at context reset | Invariants persist across resets |
| Same bug debugged 3 times | Bug fixed once, invariant documented |
| AI makes confident wrong decisions | AI reasons from verified understanding |
| No structural verification | `lyric verify` catches drift at commit time |
| No enforcement of understanding-first | Read-before-write mechanically enforced |

**UDD formalizes what expert engineers do naturally — understand before acting —
and makes it persistent, verifiable, and scalable to AI agents that would
otherwise lose that understanding every session.**

---

## Connection to Superhuman Architecture

The temporal requirement sequences in git histories are already a design graph
evolving over time. UDD is what a superhuman architect *does naturally* — it
just needs tooling to make the graph explicit and persistent rather than implicit
in a human's head.

A model trained on `.ly` files alongside codebases would learn to reason at
the design level natively. The `.ly` file becomes training data for the next
generation of AI engineers.
