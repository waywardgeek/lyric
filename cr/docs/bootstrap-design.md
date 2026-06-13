# Lyric Bootstrap Design

## Goals

1. **Prove Lyric is at least as good as Go for writing compilers.** The Go compiler
   is the gold standard for compiler ergonomics — fast builds, clean code, readable
   error messages. The bootstrap compiler must demonstrate that Lyric matches or
   exceeds that bar. Relations, Sym, match expressions, and the error model should
   make compiler code *cleaner* than the Go equivalent, not just equivalent.

2. **Jank-free language, jank-free compiler.** Every rough edge discovered while
   writing the bootstrap compiler is a language design bug that must be fixed.
   The bootstrap is the ultimate dogfooding exercise — if something is awkward to
   express in Lyric, it's awkward for every Lyric user. No workarounds, no "good
   enough for now." Fix the language.

3. **Self-hosting.** Compile the Lyric compiler with itself via C backend. This is
   the mechanical proof that goals 1 and 2 were achieved.

## Architecture

### Root Object Pattern
A single `Root` class owns all top-level allocations. Destroying Root cascades through all owned objects. No GC needed.

```
class Root() {}
class File() {}
class LyricBlock() {}
class InterfaceDecl() {}
// ... etc

relation OwningList Root:root owns [File:file]
relation OwningList File:file owns [LyricBlock:block]
relation OwningList LyricBlock:block owns [InterfaceDecl:iface]
// ... etc
```

If we ever need to compile multiple programs concurrently, Root becomes Program — but not expected for bootstrap.

### Hash Tables via HashedList
No `map[K]V` primitive. Instead:

1. **HashedList<P, C>** — stdlib interface implementing a swiss hash table as a relation. Parent owns a hash table of children, keyed by a field on the child. Similar to ArrayList but with O(1) lookup by key.

2. **Dict<V>** — generic string-keyed hash table wrapping HashedList. Used where the compiler currently uses `map[string]T`.

The compiler's maps are all `string → something`, so HashedList with string keys covers all cases.

### Stdlib Bindings Needed
The compiler uses these Go stdlib packages (non-test):
- `fmt` — f-strings cover most of this
- `os` — ReadFile, WriteFile, Stat, Args, Getwd, Exit
- `os/exec` — Command (for cc invocation in C backend)
- `path/filepath` — Join, Dir, Ext, Base
- `strings` — Contains, HasPrefix, HasSuffix, Join, Replace, Split, TrimSpace, Repeat
- `strconv` — Itoa, Atoi, FormatInt
- `unicode` — IsLetter, IsDigit, IsSpace
- `sort` — Strings (can use stdlib sort with comparator)
- `regexp` — only used in verifier/update, can defer or rewrite

These can be provided as `.ly` wrappers around Go functions via `extern` or `lyric gen`.

### What We Skip
- `lyric gen` command (uses go/ast, go/parser — not needed for bootstrap)
- Go backend (C backend is the bootstrap target; monomorphization eliminates generics issues)
- Verifier (nice-to-have, not essential)
- Tests (port later)

### Port Order (bottom-up)
1. ~~**AST types**~~ ✅ — structs, enums, type definitions (bootstrap/ast.ly)
2. ~~**Lexer**~~ ✅ — character-by-character scanner, Dict keyword tables (bootstrap/lexer.ly)
3. ~~**Parser**~~ ✅ — recursive descent + Pratt expr parser (bootstrap/parser.ly, expr_parser.ly)
4. **Desugar passes** — interface embeds, fields, relations, destructors, default impls
5. **Checker** — type checking, inference, constraint satisfaction
6. **LIR** — lowering AST → LIR
7. **Monomorphizer** — specializes generics for C backend
8. **C backend** — LIR → C source
9. **Optimizer** — optional, port if time
10. **CLI** — main entry point, file I/O

### Open Questions
- HashedList: swiss hash table in Lyric stdlib, or simpler open-addressing? Swiss is optimal but complex. A basic open-addressing table with string keys may be sufficient for bootstrap.
- String interning: the compiler creates many duplicate strings. Worth adding an intern table to Root?
- Error reporting: currently uses fmt.Errorf / string formatting. F-strings + a DiagnosticList class?
