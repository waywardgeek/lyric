# Lyric

A typed language for design and implementation: describe architecture in `.lyric`
understanding files and compile `.ly` programs to C.

**Repository:** [github.com/waywardgeek/lyric](https://github.com/waywardgeek/lyric)

## What is Lyric?

Lyric has two related file formats:

**`.lyric` files - understandings.** Declaration-only design artifacts: data
structures, APIs, interfaces, annotations, doc blocks, invariants, ownership
relations, and design rationale. They are written for review and for AI context.
The older Go-source verifier/update/gen tooling is preserved under
`legacy/go-compiler`, but it is not part of the current self-hosted `lyric`
binary.

**`.ly` files - code.** Full Lyric with function bodies and executable
semantics. The current compiler emits C and is self-hosting: `lyric.c` is checked
in as the canonical compiler output and can rebuild the compiler from `src/*.ly`.

## Why?

The real bottleneck in AI-assisted software development is **human review**, not
code generation. A `.lyric` file contains only the decisions that matter - data
structures, API boundaries, type relationships, concurrency contracts, ownership,
and invariants - at much higher information density than implementation code.

See [Understanding-Driven Development](https://coderhapsody.ai/docs/understanding-driven-development)
for the methodology.

---

## Quick Start

### Installation

Build the self-hosted compiler from the checked-in C source:

```bash
git clone https://github.com/waywardgeek/lyric.git
cd lyric
make
```

This requires GCC and libc. The test scripts use Bash.

### Compile a `.ly` file

```bash
$ ./lyric compile testdata/demo.ly -o demo.c
wrote demo.c
$ gcc -std=gnu11 -O2 -w -I runtime -o demo demo.c -lm -lpthread
$ ./demo
```

### Run the regression suite

```bash
$ make test
```

`make test` builds `lyric`, compiles the top-level `testdata/*.ly` programs,
compiles the generated C with GCC, runs the resulting binaries, and compares
golden output where present.

To verify the compiler fixed point:

```bash
$ make self-test
```

### Format a `.lyric` file

```bash
$ ./lyric fmt src/parser/parser.lyric
```

The active self-hosted CLI currently supports `compile`, `test`, `fmt`, and
`help`. The old Go implementation of `verify`, `update`, and `gen` remains in
`legacy/go-compiler` for reference.

---

## The Compiler

The `.ly` compiler is a full-stack compiler:

```
lexer/parser -> checker -> desugar -> LIR -> lowerer -> optimizer
             -> monomorphizer -> memory pass -> C backend
```

The compiler source lives in `src/` and is written in Lyric. `lyric.c` is the
checked-in generated C output used to bootstrap the binary.

```lyric
lyric task_demo {
  enum Priority { Low Medium High Critical }

  struct Task {
    name: string
    priority: Priority
    done: bool
  }

  class TaskManager<T>() {
    tasks: [T]

    pub fn add(mut self, task: T) {
      self.tasks = append(self.tasks, task)
    }

    pub fn count(self) -> i32 {
      return len(self.tasks) as i32
    }
  }

  func main() {
    let mgr = TaskManager<Task>()
    mgr.add(Task { name: "Ship it", priority: Priority.High, done: false })
    println(f"Tasks: {mgr.count()}")
  }
}
```

### Language Features

**Type system:** Generics with constraints and inference, interfaces, enums with
fields, optionals (`T?`, `!` unwrap, `isnull()`), union types (`T | U`), tuples
`(T, U)`, type aliases, and classes.

**Error handling:** `(T, error)` tuples with a `?` operator for concise error
propagation.

**Concurrency:** `spawn`, typed channels, `select`, and scoped `lock(mu) { ... }`.

**Concurrency safety:** `guarded_by(mu)` annotations are enforced at compile
time.

**Pattern matching:** Enum/union type switches, nested patterns, guard clauses,
exhaustiveness checks, and tuple destructuring.

**Other:** Lambdas, f-strings, visibility (`pub`/private), standard collection
helpers, numeric types, casts, modules, and built-in testing with `test_*`
functions.

---

## The Toolchain

| Command | Description |
|---|---|
| `lyric compile file.ly -o file.c` | Compile `.ly` to C |
| `lyric test file.ly [...]` | Compile, discover `test_*` functions, and run generated tests |
| `lyric fmt file.lyric` | Format `.lyric` understanding files |
| `lyric help` | Show command help |

Legacy Go-source tools are preserved under `legacy/go-compiler` as historical
implementation and design reference. They are not the current compiler entry
point.

---

## Key Principles

- **`.lyric` files capture cross-file design**: data structures, APIs,
  interfaces, invariants, ownership, and rationale.
- **AI writes, human reviews**: the understanding file is both review artifact
  and future-context artifact.
- **Tests enforce behavior**: Lyric has first-class `test_*` functions and a
  golden-output regression suite.
- **Self-hosting matters**: `make self-test` checks that the compiler can
  regenerate itself to a fixed point.

---

## Project Structure

```
src/                    Self-hosted Lyric compiler source
src/ast/                AST and module-resolution code
src/lexer/              Lexer
src/parser/             Parser and expression parser
src/checker/            Type checker
src/desugar/            Desugaring passes
src/lir/                Lower-level IR
src/lowerer/            AST-to-LIR lowering
src/optimizer/          LIR optimization
src/monomorphizer/      Generic specialization
src/memory/             Memory-management transforms
src/c_backend/          C code generator
src/main/               CLI implementation
runtime/                C runtime headers
stdlib/                 Lyric standard library
testdata/               Top-level Lyric regression programs
testdata/golden/        Expected output files
legacy/go-compiler/     Historical Go compiler and Go-source UDD tools
cr/docs/                Design and methodology documents
```

---

## Test Status

The current regression entry points are:

```bash
$ make test
$ make self-test
```

The repository currently contains 78 top-level `.ly` test/sample files and 75
golden output files. `make test` is the authoritative status check for the
current compiler. `make self-test` verifies that checked-in `lyric.c` matches the
compiler output.

### Known Issues

- `verify`, `update`, and `gen` for `.lyric` understanding files have not been
  ported into the self-hosted CLI; the old Go implementation is in
  `legacy/go-compiler`.
- No LSP server yet.

---

## Documentation

- [The Lyric Book](https://coderhapsody.ai/docs/the-lyric-book) - a K&R-style guide to the language
- [Lyric Language Specification](https://coderhapsody.ai/docs/lyric-language-spec) - full type system, syntax, and semantics
- [Lyric Language Reference](https://coderhapsody.ai/docs/lyric-language-reference) - quick reference card
- [Understanding-Driven Development](https://coderhapsody.ai/docs/understanding-driven-development) - the methodology behind `.lyric` files
- [Two Weeks](https://coderhapsody.ai/docs/two-weeks) - the story of how Lyric was built in 14 days

---

## License

Apache 2.0 - see [LICENSE](LICENSE).

## Authors

Bill Cox & [CodeRhapsody](https://coderhapsody.ai)

*"lyric" is a 60-year-old word from Heinlein's Stranger in a Strange Land meaning deep, complete understanding. We are reclaiming it.*
