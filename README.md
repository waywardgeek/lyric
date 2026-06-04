# Grok

A typed language for design and implementation — describe your architecture, verify it hasn't drifted, compile it to Go.

**Repository:** [github.com/waywardgeek/grok](https://github.com/waywardgeek/grok)

## What is Grok?

Grok has two modes:

**`.grok` files — understandings.** Declaration-only design artifacts: data structures, APIs, interfaces, annotations, doc blocks, invariants, ownership relations. No function bodies. The AI writes them after implementation; the human reviews them. A structural verifier checks they haven't drifted from the source. This is the core of [Grok-Driven Development (GDD)](https://coderhapsody.ai/docs/grok-driven-development).

**`.gk` files — code.** Full Grok with function bodies and executable semantics. Compiles to Go. An existence proof that the language design is sound: if the notation is precise enough to verify against real implementations, function bodies are all that's missing to make it a real language.

## Why?

The real bottleneck in AI-assisted software development is **human review**, not code generation. As AI generates code faster, reviewers drown in PRs. A `.grok` file contains *only* the decisions that matter — data structures, API boundaries, type relationships, concurrency contracts — at 5-10x the information density of source code. The reviewer validates architecture, not syntax. The verifier confirms the source matches.

See [Grok-Driven Development](https://coderhapsody.ai/docs/grok-driven-development) for the full methodology.

## Quick Example

```grok
// pkg/verifier/verifier.grok — lives next to the code it describes

grok Verifier {
  why: "Compares .grok files against Go source, reporting structural drift."

  doc "Architecture": """
    Multi-stage pipeline: parse .grok → discover Go sources → extract types →
    compare structurally → report findings. Aggregates ALL source files before
    comparing — a grok block's types may span multiple Go files.
  """

  enum Severity { Error Warning Info }

  struct Finding {
    Severity: Severity
    GrokFile: string
    GoFile:   string
    Message:  string
  }

  class Result() {
    Findings: [Finding]
    func ErrorCount(self) -> int
  }

  func Verify(grokPath: string) -> (Result?, error)

  source: ["verifier.go"]
}
```

Verify it:
```bash
$ grok-verify pkg/verifier/verifier.grok
0 errors, 0 warnings
```

If the code drifts:
```
[ERROR] verifier.grok ↔ verifier.go: function Verify: param count mismatch: .grok=2, Go=1
[WARNING] verifier.grok ↔ verifier.go: exported type Config not documented in .grok
```

## The Compiler

The `.gk` compiler is a full-stack implementation: parser → type checker → Go transpiler.

```grok
// demo.gk — compiles to valid, runnable Go

import fmt from "fmt"

class Stack<T>() {
    items: [T]

    func push(mut self, item: T) -> unit {
        self.items = append(self.items, item)
    }

    func pop(mut self) -> T? {
        if len(self.items) == 0 {
            return nil
        }
        let last = self.items[len(self.items) - 1]
        self.items = self.items[:len(self.items) - 1]
        return last
    }
}

func main() -> unit {
    let s = Stack<i32>()
    s.push(10)
    s.push(20)
    println(f"popped: {s.pop()!}")
}
```

```bash
$ grok-compile demo.gk
$ go run demo.go
popped: 20
```

**Language features:** generics with constraints, interfaces, enums (tagged unions), optionals (`T?`, `!` unwrap, `isnull`), f-strings, error handling via `(T, error)` tuples, pattern matching, lambdas, type casts, built-in methods on string/list/map, numeric coercion, slices.

## Key Principles

- **`.grok` files live next to the code** they describe (`pkg/ast/ast.grok` alongside `pkg/ast/ast.go`)
- **Cross-file concepts only** — data structures, APIs, interfaces. Not single-file implementation details.
- **Adopt the implementation language's conventions** — PascalCase for Go, snake_case for Python
- **AI writes, human reviews** — implement first, write `.grok` after, human validates architecture
- **Completeness checking** — the verifier warns about exported Go symbols not documented in `.grok`, ensuring the file captures the full public API surface

## Documentation

- [Grok-Driven Development](https://coderhapsody.ai/docs/grok-driven-development) — the methodology
- [Grok Language Specification](https://coderhapsody.ai/docs/grok-language) — the full type system and syntax

## Installation

```bash
go install github.com/waywardgeek/grok/cmd/grok-verify@latest
go install github.com/waywardgeek/grok/cmd/grok-compile@latest
```

Or build from source:
```bash
git clone https://github.com/waywardgeek/grok.git
cd grok
go build -o grok-verify ./cmd/grok-verify/
go build -o grok-compile ./cmd/grok-compile/
```

## Usage

```bash
# Verify .grok files against Go source
grok-verify pkg/parser/parser.grok
find . -name '*.grok' -exec grok-verify {} \;

# Compile .gk files to Go
grok-compile testdata/demo.gk
go run demo.go
```

## Project Structure

```
cmd/grok-verify/       CLI: structural drift detector
cmd/grok-compile/      CLI: .gk → Go compiler
pkg/ast/               AST node types + ast.grok
pkg/parser/            PEG parser for .grok and .gk files + parser.grok
pkg/checker/           Type checker with inference + checker.grok
pkg/transpiler/        Go code generator + transpiler.grok
pkg/verifier/          Structural drift detector + verifier.grok
testdata/              .gk test files (15 files, all compile to valid Go)
```

The project is self-referential: every package has its own `.grok` file, verified by the tool it contains.

## Test Status

180 tests across parser, checker, transpiler, and verifier. 15 `.gk` test files compile to valid Go and run correctly.

```bash
go test ./...
```

## License

Apache 2.0 — see [LICENSE](LICENSE).

## Authors

Bill Cox & [CodeRhapsody](https://coderhapsody.ai)

*"grok" is a 60-year-old word from Heinlein's Stranger in a Strange Land meaning deep, complete understanding. We are reclaiming it.*
