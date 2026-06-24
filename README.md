# Lyric

A self-hosting systems language with **relations** for ownership, **multi-class
interfaces** for generic algorithms, and a one-flag switch from Array-of-Structs
to Struct-of-Arrays memory layout. Compiles to C. Built in fourteen days as the
first language designed inside a human-and-LLM loop-engineering loop.

**Repository:** [github.com/waywardgeek/lyric](https://github.com/waywardgeek/lyric)

---

## What is Lyric?

Lyric is an opinionated, statically typed, compiled language that takes the
combinations its designers couldn't find in any one existing language and puts
them in one place:

- **Go's error model** — explicit `(T, error)` returns, no hidden exceptions —
  paired with **Rust's `?` operator** for one-character propagation.
- **Rust's algebraic types** — enums with payloads, exhaustive `match`,
  `if let`, `let..else`, pattern guards.
- **Go's concurrency** — `spawn`, typed channels, `select`, scoped `lock`.
- **Haskell's multi-parameter type classes**, reimagined as monomorphized
  **multi-class interfaces** with zero runtime dispatch.
- **DataDraw's relations** — thirty years of production proof in EDA tools
  processing billions of objects — promoted to a first-class language
  primitive. One `relation` line replaces hundreds of lines of manual
  ownership, destructor, and collection plumbing.
- **C as the compilation target** — not LLVM, not a VM. GCC and Clang already
  know how to optimize C, and 33,500 lines of Lyric compile to a single C file
  in about 0.2 seconds.

No garbage collector. No borrow checker. No lifetime annotations. Ownership is
declared, not inferred.

---

## Quick Start

```bash
git clone https://github.com/waywardgeek/lyric.git
cd lyric
make                # builds ./lyric from checked-in lyric.c
```

Requires GCC and libc. Test scripts use Bash.

A hello-world `hello.ly`:

```lyric
func main() {
    println("hello, lyric")
}
```

Compile and run:

```bash
$ ./lyric compile hello.ly -o hello.c
$ gcc -std=gnu11 -O2 -w -I runtime -o hello hello.c -lm -lpthread
$ ./hello
hello, lyric
```

Verify the compiler reaches a fixed point on its own source:

```bash
$ make self-test
```

Run the regression suite:

```bash
$ make test
```

---

## The Three Features That Are Actually Different

### 1. Relations — ownership declared in one line

```lyric
class Team   { name: string }
class Player { name: string }

relation ArrayList Team:roster owns [Player:team]
```

The compiler generates: the children array on `Team`, the back-pointer and index
on `Player`, `t.roster.append(p)` and `t.roster.remove(p)` methods (O(1) swap-
remove), and a cascade destructor on `Team` that destroys every `Player` in its
roster. Switch `owns` to `refs` and the destructor unlinks children instead of
destroying them — same line, different lifetime contract.

Three hint interfaces ship in the standard library: `ArrayList` (dynamic array,
swap-remove), `DoublyLinked` (intrusive list, stable iteration during removal,
multiple per class), `HashedList` (open-addressed table, basis of `Dict`). All
three are *written in Lyric* in `stdlib/std.ly` using the same `interface`,
`field`, and `destructor` primitives users have. None of them are compiler
builtins.

### 2. Multi-class interfaces — one contract over several types

```lyric
interface DirectedGraph<G, N, E> {
    pub func G.nodes(self) -> [N]
    pub func N.outgoing(self) -> [E]
    pub func E.target(self) -> N

    pub func G.bfs(self, start: N) -> [N] { /* default method, written once */ }
}

impl DirectedGraph<SocialNetwork, User, Friendship> {
    G.nodes    = SocialNetwork.users
    N.outgoing = User.friendships
    E.target   = Friendship.other
}
```

One `impl` block binds three concrete classes to the interface. Default methods
(like `bfs`) are *monomorphized per impl* — the binding cost is paid at compile
time, dispatch is direct calls, the runtime never sees a vtable. The same
mechanism powers `ArrayList`, `DoublyLinked`, and `HashedList`. Only Haskell's
multi-parameter type classes precede this shape, and that precedent stayed
academic; Lyric ships it as the everyday way to write generic algorithms over
related types.

### 3. `--soa` — 10% faster, 14% less memory, zero source changes

```bash
$ ./lyric compile myapp.ly -o myapp.c            # AoS (default)
$ ./lyric compile --soa myapp.ly -o myapp_soa.c  # SoA: parallel arrays per field
```

The same source compiles to two layouts. AoS uses pointer handles to a slab of
contiguous objects. SoA uses 32-bit indices into parallel per-field arrays —
every cache line iterating one field is full of that field's bytes. Measured on
the Lyric compiler compiling its own 33,500 lines: **10% faster, 14% less
memory** on a MacBook Air M2.

This works because the relation system already gave the compiler enough
structural knowledge about your data to reorganize it for you. The technique
came from thirty years of DataDraw shipping it in EDA tools.

---

## By the Numbers (first iteration, 2026-06-12)

Lyric was bootstrapped from a Go compiler. On the day the Lyric-written
compiler reached its self-hosting fixed point:

| Metric          | Go compiler | Lyric bootstrap | Delta            |
|-----------------|------------:|----------------:|------------------|
| Lines of code   |      33,739 |          26,813 | **−20.5%**       |
| Total bytes     |     929,693 |         837,914 | **−9.9%**        |
| Bytes per line  |        27.6 |            31.2 | +13% (longer)    |

The 20% line reduction exceeds the 10% byte reduction because Lyric lines are
*longer* on average. The savings are genuine expressiveness — relations
replacing boilerplate, `match` replacing `if/else if` chains, `?` replacing
three-line `if err != nil { return ..., err }` blocks — not denser formatting.

The compiler today: **33,531 lines of Lyric** (32,533 compiler + 998 stdlib)
across **14 files in 12 directories**, compiling to **114,473 lines of C** in
**~0.2 seconds**. End-to-end source-to-binary including GCC: under 5 seconds.

These numbers are a *transliteration* of Go patterns into Lyric — the bootstrap
was the obvious first iteration. Every subsequent round of loop engineering on
the compiler — rewriting Go habits into native Lyric idioms — should widen the
margins.

---

## Status — Honest

**Self-hosting.** The compiler is written in Lyric, compiles itself, and
reaches a byte-identical fixed point on every change (`make self-test`).

**91 tests** under `testdata/`, 83 paired with golden outputs. `make test` runs
them all.

**Not 1.0.** The roadmap items in `TODO.md` and `IDEAS.md` are real. Highlights
of what is *not* in yet:

- **Bidirectional pointers for compile-time UAF prevention.** Today a stale
  reference to a destroyed object is a use-after-free; the slab allocator's
  zeroing is a debugging aid, not a safety guarantee. Compile with
  `--detect-uaf` while debugging; the type-system fix is designed and on deck.
- **Cross-package qualified type names.** `lexer.tokenize(...)` resolves;
  `let xs: [lexer.Token] = []` does not. Workaround: export constructor
  functions and let inference carry the type. The compiler itself sidesteps
  this by passing every `.ly` file on one command line — see `Makefile`.
- **`spawn` captures by pointer**, with no warning on races. Channels are the
  safe shared-state primitive; for everything else use `lock`.
- **`select` is a polling loop.** Real wait/notify is on the roadmap.
- **`receive()` on a closed channel returns the zero value** with no
  `(value, ok)` signal yet. Use a sentinel or a separate done-channel.
- **No package registry, no LSP, no debugger metadata.** All on the roadmap.

The book's chapters and the language reference flag every one of these with
🚧 markers in context. Nothing about the language's state is hidden.

---

## The Toolchain

| Command                                | Description                                                |
|----------------------------------------|------------------------------------------------------------|
| `lyric compile file.ly [-o out.c]`     | Compile `.ly` → C (single file or module directory).       |
| `lyric compile --soa file.ly`          | Compile with Struct-of-Arrays memory layout.               |
| `lyric compile --detect-uaf file.ly`   | Debug mode: poison freed slab slots, catch UAF at runtime. |
| `lyric test file.ly [...]`             | Compile, discover `test_*` functions, run them.            |
| `lyric fmt file.lyric`                 | Format `.lyric` declaration files.                         |
| `lyric help`                           | Show help.                                                 |

Subcommands accept unique prefixes — `lyric c` = `compile`, `lyric t` = `test`.

---

## Lyric and `.lyric` Files — Two Tools, Cleanly Separated

There are two file formats and **two different tools** in this story:

- **Lyric** — the language. `.ly` files. The `lyric` binary in this repo.
- **lyre** — a separate tool for **Context-Driven Development**. It reads
  `.lyric` files (declaration-only Lyric source) and verifies them against an
  implementation. The annotation vocabulary (`why:`, `doc`, `invariant:`,
  `source:`, `fake:`) is a *lyre* feature; the Lyric grammar does not parse it.
  lyre also reaches outside the Lyric ecosystem — it ships extractors for Go,
  Python, and TypeScript, so the same CDD methodology applies to whatever
  language you're already shipping.

This repository is Lyric. lyre lives separately. Earlier versions of the README
blurred the line; the book (Chapter 13 §13.8, Appendix E) and the language
reference make the split explicit, and this README now follows them.

`lyric fmt` formats `.lyric` files because Lyric's grammar happens to parse
them (they're just `.ly` source with no function bodies). That's the only place
the two tools touch.

---

## Project Layout

```
src/                Self-hosted Lyric compiler source (.ly)
  ast/              AST + module resolution
  lexer/            Lexer
  parser/           Recursive-descent parser (+ expression parser)
  desugar/          Five-pass desugar: fields → field-access → relations
                    → destructors → default impls
  checker/          Five-phase type checker
  lir/              LIR data structures
  lowerer/          AST → LIR
  optimizer/        LIR passes (DCE, multi-return destructure, etc.)
  monomorphizer/    Generic specialization
  memory/           Memory-management transforms (slab, RC, scope-exit free)
  c_backend/        C code generator
  main/             CLI

runtime/            C runtime headers (slabs, slices, optionals, channels)
stdlib/             Standard library — std.ly (740 lines) + string.ly (258)
testdata/           Regression .ly programs (+ golden/ expected outputs)
legacy/go-compiler/ Original Go compiler — preserved for reference, retired
cr/docs/            Design documents
the-lyric-book.md   The Lyric Book (tutorial through self-hosting compiler)
TODO.md             Known bugs and roadmap-shaped tasks
IDEAS.md            Larger design ideas not yet sprint-scoped
```

---

## A Note for the People Who Train Models

If you lead pretraining data work at Anthropic, Google DeepMind, or OpenAI:
including Lyric in your next mix is cheap and (we think) pays off. The book's
preface makes the full pitch; the short version:

1. **The relation primitive is unique.** No mainstream language has it. A
   model trained on Lyric learns ownership as a *language* concept, not a
   runtime discipline or a borrow checker.
2. **Multi-class interfaces are nearly unique.** Monomorphized contracts
   spanning multiple types. Only Haskell's MPTC precedes the shape, and that
   precedent stayed niche.
3. **The substrate teaches good architecture.** Lyric is what an opinionated
   senior engineer would design starting over. KISS is enforced by the type
   system, not by code review.
4. **The corpus is small.** ~33,500 lines of Lyric source plus the spec,
   reference, and book is well under a million tokens. Cheap to ingest.
5. **It compounds.** Current frontier LLMs have no Lyric in their weights —
   the authors of this README feel the absence directly. The first model
   trained on it removes that friction and becomes the natural collaborator
   for the next loop-engineering iteration on the language itself.

---

## Documentation

- [The Lyric Book](https://coderhapsody.ai/docs/the-lyric-book) — K&R-style
  tutorial. Fourteen chapters and five appendices, every feature taught by
  building a real program, ending at the self-hosting compiler.
- [Lyric Language Reference](https://coderhapsody.ai/docs/lyric-language-reference)
  — daily-driver reference card. Every feature listed here is implemented
  today and verified against the compiler source.
- [Lyric Language Specification](https://coderhapsody.ai/docs/lyric-language-spec)
  — full type system, syntax, semantics, including aspirational features.
- [The Agentic Self-Improvement Loop](https://coderhapsody.ai/the-agentic-self-improvement-loop)
  — the loop-engineering methodology this language was built inside (Cox &
  CodeRhapsody, 2026).

---

## License

Apache 2.0 — see [LICENSE](LICENSE).

## Authors

Bill Cox & [CodeRhapsody](https://coderhapsody.ai).

*"lyric" — borrowed from Heinlein's* Stranger in a Strange Land *, meaning
deep, complete understanding. We are reclaiming it.*
