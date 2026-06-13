# Lyric

A typed language for design and implementation — describe your architecture, verify it hasn't drifted, compile it to Go.

**Repository:** [github.com/waywardgeek/lyric](https://github.com/waywardgeek/lyric)

## What is Lyric?

Lyric has two modes:

**`.lyric` files — understandings.** Declaration-only design artifacts: data structures, APIs, interfaces, annotations, doc blocks, invariants, ownership relations. No function bodies. The AI writes them after implementation; the human reviews them. A structural verifier checks they haven't drifted from the source. This is the core of [Lyric-Driven Development (GDD)](https://coderhapsody.ai/docs/lyric-driven-development).

**`.ly` files — code.** Full Lyric with function bodies and executable semantics. Compiles to Go. An existence proof that the language design is sound: if the notation is precise enough to verify against real implementations, function bodies are all that's missing to make it a real language.

## Why?

The real bottleneck in AI-assisted software development is **human review**, not code generation. As AI generates code faster, reviewers drown in PRs. A `.lyric` file contains *only* the decisions that matter — data structures, API boundaries, type relationships, concurrency contracts — at 5-10x the information density of source code. The reviewer validates architecture, not syntax. The verifier confirms the source matches.

See [Lyric-Driven Development](https://coderhapsody.ai/docs/lyric-driven-development) for the full methodology.

---

## Quick Start

### Installation

Build the unified CLI from source:

```bash
git clone https://github.com/waywardgeek/lyric.git
cd lyric
go build -o lyric ./cmd/lyric/
# Optionally move to your PATH:
# mv lyric /usr/local/bin/
```

Or install individual tools:

```bash
go install github.com/waywardgeek/lyric/cmd/lyric-verify@latest
go install github.com/waywardgeek/lyric/cmd/lyric-compile@latest
```

### Verify a `.lyric` file

```bash
$ lyric verify pkg/parser/parser.lyric
0 errors, 0 warnings
```

If the code drifts:

```
[ERROR] parser.lyric ↔ parser.go: function ParseString: param count mismatch: .lyric=2, Go=1
[WARNING] parser.lyric ↔ parser.go: exported type Config not documented in .lyric
```

### Compile a `.ly` file

```bash
$ lyric compile testdata/demo.ly
wrote demo.go
$ go run demo.go
Task Manager Demo
Added: Buy groceries (priority 2)
...
```

### Generate a `.lyric` file from Go source

```bash
$ lyric gen pkg/ast/        # scaffolds ast.lyric from Go source
$ lyric update ast.lyric     # auto-adds missing exported symbols
$ lyric fmt ast.lyric        # formats to canonical style
```

---

## The Compiler

The `.ly` compiler is a full-stack transpiler: parser → type checker → Go code generator.

```
// demo.ly

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
      return <i32>len(self.tasks)
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

**Type system:** Generics with constraints and inference, interfaces (structural subtyping), enums (tagged unions with fields), optionals (`T?`, `!` unwrap, `isnull()`), union types (`T | U`), tuples `(T, U)`, type aliases.

**Error handling:** `(T, error)` tuples with Rust-style `?` operator for concise error propagation — works in any expression position:

```
func process(s: string) -> (i32, error) {
    let result = double(parse(s)?)      // nested ? in function args
    let sum = parse("a")? + parse("b")? // multiple ? in one expression
    return (result + sum, null)
}
```

**Concurrency:** `spawn` (goroutines), typed channels (`make_channel<T>`, send/receive/close), `select` with receive binding, `lock(mu) { ... }` (scoped mutex).

**Concurrency safety:** `guarded_by(mu)` annotations are enforced at compile time — accessing a guarded field outside its lock scope is a checker error:

```
class Counter() {
    count: i32 guarded_by(mu)  // must access within lock(mu)
    mu: lock

    pub fn increment(mut self) {
        lock(self.mu) {
            self.count = self.count + 1  // OK
        }
    }

    pub fn bad_read(self) -> i32 {
        return self.count  // ERROR: field "count" is guarded by "mu"
    }
}
```

**Pattern matching:** Enum/union type switches, nested patterns (`Some(Circle(r)) =>`), guard clauses (`x if x > 0 =>`), exhaustiveness warnings, tuple destructuring.

**Other:** Lambdas, f-strings, visibility (`pub`/private), built-in methods on string/list/map/channel, numeric types (i8–i256, u8–u256, f32–f128), type casts, modules (`import X from "file.ly"`), `cascade` (defer).

---

## The Toolchain

| Command | Description |
|---|---|
| `lyric compile file.ly` | Compile `.ly` to `.go` |
| `lyric verify file.lyric` | Check `.lyric` against Go source for structural drift |
| `lyric update file.lyric` | Auto-add missing exported symbols, regenerate function index |
| `lyric update --prune file.lyric` | Also remove stale declarations not in Go source |
| `lyric gen pkg/dir/` | Scaffold a new `.lyric` file from Go source |
| `lyric fmt file.lyric` | Format `.lyric` to canonical style |

---

## Key Principles

- **`.lyric` files live next to the code** they describe (`pkg/ast/ast.lyric` alongside `pkg/ast/ast.go`)
- **Cross-file concepts only** — data structures, APIs, interfaces. Not single-file implementation details.
- **Adopt the implementation language's conventions** — PascalCase for Go, snake_case for Python
- **AI writes, human reviews** — implement first, write `.lyric` after, human validates architecture
- **Completeness checking** — the verifier warns about exported Go symbols not documented in `.lyric`
- **Self-referential** — every compiler package has its own `.lyric` file, verified by the tool it contains

---

## Project Structure

```
cmd/lyric/              Unified CLI (compile, verify, update, gen, fmt)
cmd/lyric-compile/      Standalone compiler CLI
cmd/lyric-verify/       Standalone verifier CLI
cmd/lyric-gen/          Standalone scaffolding CLI
cmd/lyric-update/       Standalone update CLI
pkg/ast/               AST node types + ast.lyric
pkg/parser/            PEG parser for .lyric and .ly files + parser.lyric
pkg/checker/           Type checker with inference + checker.lyric
pkg/transpiler/        Go code generator + transpiler.lyric
pkg/verifier/          Structural drift detector + verifier.lyric
testdata/              36 .ly test files (all compile, 33 go-build clean)
testdata/modules/      Module system test files
```

---

## Test Status

233 tests across parser, checker, transpiler, and verifier. 36 `.ly` test files all compile; 33 produce Go that passes `go build` (1 known issue: `typealias.ly`). 6 self-referential `.lyric` files verify clean (0 errors, 0 warnings).

```bash
$ go test ./...
ok  github.com/waywardgeek/lyric/pkg/checker     0.018s
ok  github.com/waywardgeek/lyric/pkg/parser       0.006s
ok  github.com/waywardgeek/lyric/pkg/transpiler   0.005s
ok  github.com/waywardgeek/lyric/pkg/verifier     0.004s
```

### Known Issues

- `typealias.ly`: optional type alias wrapping generates Go that doesn't build in all cases
- `features.ly`: `go vet` warns about unreachable code after exhaustive match
- i128/i256 types silently downcast to int64/uint64 (math/big support planned)
- No LSP server yet (planned)

---

## Documentation

- [Lyric Language Specification](https://coderhapsody.ai/docs/lyric-language) — full type system, syntax, and examples
- [Lyric-Driven Development](https://coderhapsody.ai/docs/lyric-driven-development) — the methodology

---

## License

Apache 2.0 — see [LICENSE](LICENSE).

## Authors

Bill Cox & [CodeRhapsody](https://coderhapsody.ai)

*"lyric" is a 60-year-old word from Heinlein's Stranger in a Strange Land meaning deep, complete understanding. We are reclaiming it.*
