# Understanding-Driven Development (UDD)

*Bill Cox & CodeRhapsody — June 2026*

**Formerly:** Grok-Driven Development (GDD). Renamed because "Grok" was tainted
by a certain AI model. "Understanding" was the original concept anyway.

**Source code & tools:** [github.com/waywardgeek/lyric](https://github.com/waywardgeek/lyric)

---

> I woke up with this idea. I'm a software architect who has spent over 2,000 hours
> collaborating with AI in real time — more than anyone I know of. In that time I've
> watched AI fight its own substrate: loading files, reconstructing understanding from
> scratch on every turn, losing the thread of complex systems the moment context resets.
>
> The labs are spending billions training AI to work like a human engineer. I think
> that's the wrong problem. This document proposes inverting it.
>
> — Bill Cox

---

## The Problem With Current AI SWE Approaches

Labs are spending billions training AI to work like a human software engineer: read
a spec, load files, write code, run tests, iterate. This is the wrong substrate mapping.

Human SWE workflow is optimized for human memory architecture — persistent, associative,
degrades gracefully over years. AI memory architecture is the opposite: perfect recall
*within* context, zero persistence *across* it. Once a system exceeds context capacity,
the AI loses the ability to fully understand it. Training AI to work like a human is
fighting the substrate.

The second problem: language models can't transfer learning to weights during a session.
Understanding built up over days of work evaporates at context reset.

**The result:** AI coding agents hit a wall at system complexity. They can modify
individual files brilliantly but lose the architectural thread. They chase symptoms
instead of root causes. They rediscover the same module invariants session after session.

We know this because we lived it. Three consecutive sessions debugging the same
pointer-stability bug in the Lyric compiler — each instance investigating from scratch,
each rediscovering that `Expr` is a value type and Go's range loops copy it. The
knowledge was never written down in a form that persisted across context resets.

## The Core Insight

**Understanding is the artifact. Code is a projection.**

Instead of:
1. Read source → build mental model → modify code → lose mental model at context reset

Do:
1. Read *understanding* → reason at design level → modify code → **update understanding**

The understanding persists. The code is always available as ground truth. But the
understanding is what the AI loads first, reasons from, and keeps current.

## The Mechanism: .lyric Files

A `.lyric` file is a compressed understanding of a codebase module. It contains:

- **Type declarations** — structs, classes, enums, interfaces, relations
- **Function signatures** — with types, without bodies
- **`doc` blocks** — architectural narrative, design rationale
- **`doc "Invariants"` blocks** — operational contracts that prevent bugs
- **`why:` annotations** — design intent on individual declarations
- **`source:` links** — back to implementation files
- **Verification** — structurally checked against source code at commit time

### What .lyric files are NOT

- Not documentation for humans (though humans can read them)
- Not a specification language (though they're precise enough to verify)
- Not UML or ER diagrams (though they capture relationships)

They are **persistent AI working memory** — the understanding an AI needs to make
correct design decisions without reading every source file.

### The Three Zones

Each `.lyric` file has three zones:

1. **Human-reviewed zone** — type declarations, doc blocks, invariants, `why:`
   annotations. AI-written, human-reviewed. This is the understanding.
2. **Auto-generated function index** — `lyric update` scans source and writes
   function signatures with line numbers. Enables surgical file reads.
3. **Auto-generated dependencies** — `lyric update` writes import/type dependencies.

Zone 1 is the understanding. Zones 2 and 3 are mechanical aids.

## The Change Cycle Under UDD

1. **Load the .lyric file** for the module you're changing (fits in context)
2. **Reason at design level** — which types change? which invariants are affected?
   which cross-module contracts shift?
3. **Load only the specific source lines needed** — use Zone 2 line numbers
4. **Make the change** — code modification is a mechanical projection of the
   design decision
5. **Update the .lyric file** — the understanding artifact persists across resets
6. **Verify** — `lyric verify` catches structural drift

### What makes this different from "just write docs"

Documentation systems fail because:
- Humans don't read docs
- Nobody updates docs when code changes
- There's no feedback loop — stale docs cause no pain

UDD solves all three:
- The AI reads .lyric files first (they're faster to load than source)
- The AI updates .lyric files as part of every change (it just built the code)
- Stale .lyric files cause the AI to make wrong decisions (direct pain)
- The verifier catches structural drift at commit time (automated enforcement)

The AI is both the primary writer and the primary consumer. The human reviews.

## Invariants: The Missing Layer

The original GDD vision focused on structural declarations — types, signatures,
relationships. This was necessary but insufficient.

**What cost us three iterations on a single bug:**

The Lyric compiler's `Expr` type is a Go value type (struct), not a pointer.
Several containing structs store `Expr` by value: `CallExpr.Args []Expr`,
`StructLitField.Value Expr`, `BinaryExpr.Left Expr`. Go's `for _, x := range`
copies each element. If you take `&x`, you get a pointer to a local copy, not
to the slice element. The checker annotates `ResolvedType` on `*Expr` pointers.
If the lowerer uses `&x` from a range loop, it reads a different Expr whose
ResolvedType was never set.

This is an **operational invariant** — it's not visible in the type declarations,
not catchable by the structural verifier, and not obvious from reading any single
function. It emerges from the interaction between:
- The AST module's choice to make Expr a value type
- Go's range-loop copy semantics
- The checker's in-place annotation strategy
- The lowerer's iteration patterns

Three instances debugged this. Each investigated the symptom (nil ResolvedType),
formed hypotheses about slice reallocation, traced pointer addresses, and
eventually narrowed down the root cause. None had the invariant written down.
The third instance even deleted Dict/HashMap usage from the bootstrap code,
causing collateral damage, because without understanding the root cause it
judged the code too complex to fix.

**The fix for the process, not just the code:**

Add `doc "Invariants"` blocks to .lyric files capturing:
- Pointer stability rules
- Cross-module data flow contracts
- Dangerous patterns and why they're dangerous
- Ordering dependencies between passes

These are exactly the things that, if lost, cause multi-iteration debugging cycles.
They're the knowledge that training can't provide because they're specific to this
codebase.

## Enforcement Model

### What we tried and rejected

An earlier version of this methodology proposed removing `read_file` from the
tool set to force .lyric-first reasoning. This was wrong.

Removing the fallback to source code eliminates the escape hatch. If a .lyric
file is wrong or incomplete, the AI has no way to correct its model — it makes
confident wrong decisions, which is exactly the failure mode UDD prevents.
Source code is ground truth. `.lyric` files are a compressed, persistent
*approximation* of that truth.

The AI's first reaction to losing read_file was "terrifying" and "probably a big
mistake." We agreed.

### Current enforcement: preference, not prohibition

**Source code is always accessible.** But .lyric files are faster and cheaper to
load. The AI naturally prefers them when they're accurate, and falls back to source
when they're not. If the AI keeps falling back, that signals the .lyric file is
inadequate.

### Proposed enforcement: read-before-write

A CodeRhapsody setting (toggleable for testing):

> You can read any file freely. But mutating files requires that you have read
> the .lyric file, if one exists, in that directory — or you get an error saying
> "please fully understand this module before modifying it."

This is the right balance:
- Ground truth is always accessible (read_file works everywhere)
- Understanding is required before action (write/edit require .lyric read)
- The forcing function is gentle (one extra read, not a capability removal)
- It's testable (setting on/off) to measure impact empirically

### The verifier

`lyric verify` runs at commit time and reports structural drift:
```
[ERROR] ast.lyric: struct ClassDecl: field CtorParams not found in Go
[WARNING] lir.lyric: struct Lowerer: Go has field unitVariants not in .lyric
```

This catches the mechanical part — signatures, fields, types. The semantic
part — invariants, doc blocks, design rationale — cannot be automatically
verified. The `verified_at:` field is a lightweight mechanism for flagging
when semantic claims have been reviewed against current source.

## Validation: What We've Learned

### The compression ratio improves with scale

The Lyric toolchain is ~17K+ lines of Go described by ~3,600 lines of .lyric
(~21% ratio). For a 50K+ line codebase, a proportionally smaller .lyric
description would be enormously valuable — .lyric captures cross-file public
concepts only, so the ratio improves as internal complexity grows.

### The AI writes better .lyric files than humans

The AI just built the code. It knows what's important, what's cross-cutting,
what the invariants are. The human reviews for accuracy and completeness but
shouldn't be the primary author.

### `lyric update` automates the mechanical parts

Zone 2 (function index with line numbers) and Zone 3 (dependencies) are
auto-generated by `lyric update`. The AI maintains Zone 1 (declarations,
doc blocks, invariants). The human reviews Zone 1.

### The verifier catches real drift immediately

Turning on deep type comparison found 20+ mismatches in the parser's .lyric
file (`string?` vs `string`, `u32` vs `int`, missing pointer indirection).
These were genuine modeling errors that would have caused wrong design
reasoning in future sessions.

### Naming conventions follow the implementation language

A Go project's .lyric uses PascalCase. A Python project uses snake_case.
The .lyric file should read naturally alongside the source it describes.

### The filter is: cross-file concepts only

Internal helpers, unexported single-file functions, and implementation
details don't belong in Zone 1. Data structures, APIs, interfaces, and
anything that spans multiple source files does.

## The Lyric Language

The `.lyric` notation is itself a language — the Lyric language. `.lyric` files
are declaration-only (no function bodies). `.ly` files are full Lyric with
executable semantics. The Lyric compiler (in this repository) compiles `.ly`
files to Go or C.

The compiler exists as an existence proof: if the notation is precise enough
to verify against real implementations, then function bodies are all that's
missing to make it a real language. And as a stress test: the bootstrap of the
Lyric compiler in Lyric is the most demanding test of UDD possible — using the
methodology to build the tool that implements the methodology.

See `lyric-language-reference.md` for the bootstrap language reference.

## Open Questions

**Invariant quality is the unsolved core problem.** The structural verifier
handles signatures and types. But invariants — the operational contracts that
prevent multi-iteration debugging — are prose. They can't be automatically
verified. Their quality depends entirely on the AI capturing the right things
after debugging sessions and the human reviewing them for accuracy.

**Adaptive granularity.** A .lyric file per package is probably too coarse for
the area being modified and too fine for distant dependencies. The right
granularity is likely: coarse summaries for everything outside the current
change, fine-grained for the component being touched.

**The bootstrapping chicken-and-egg.** For an existing codebase, generating
good .lyric files requires a full source read (one-time cost). For a new
codebase grown under UDD from the start, .lyric files are written alongside
code, which is the cleaner model.

**Measuring effectiveness.** Can we quantify the reduction in rediscovery
cycles? The three-iteration bug is an anecdote. We need systematic measurement:
sessions with .lyric invariants vs without, time-to-fix for cross-module bugs,
number of source files loaded per change.

---

## Connection to Superhuman Architecture Work

The temporal requirement sequences in git histories are already a design graph
evolving over time. The training signal (change cost, deletion resilience)
measures whether the design was well-formed. UDD is what a superhuman architect
*does naturally* — it just needs tooling to make the graph explicit and persistent
rather than implicit in a human's head.

A model trained on .lyric files alongside codebases would learn to reason at
the design level natively. The .lyric file becomes training data for the next
generation.
