# Forge Bootstrap Design

## Goal
Rewrite the Forge compiler in Forge, compile via C backend, produce a self-hosting compiler.

## Architecture

### Root Object Pattern
A single `Root` class owns all top-level allocations. Destroying Root cascades through all owned objects. No GC needed.

```
class Root() {}
class File() {}
class ForgeBlock() {}
class InterfaceDecl() {}
// ... etc

relation OwningList Root:root owns [File:file]
relation OwningList File:file owns [ForgeBlock:block]
relation OwningList ForgeBlock:block owns [InterfaceDecl:iface]
// ... etc
```

If we ever need to compile multiple programs concurrently, Root becomes Program — but not expected for bootstrap.

### Hash Tables via HashedList
No `map[K]V` primitive. Instead:

1. **HashedList<P, C>** — stdlib interface implementing a swiss hash table as a relation. Parent owns a hash table of children, keyed by a field on the child. Similar to ArrayList but with O(1) lookup by key.

2. **Dict<K, V>** — ref-counted convenience class wrapping HashedList for simple key→value storage. Used where the compiler currently uses `map[string]T`.

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

These can be provided as `.fg` wrappers around Go functions via `extern` or `forge gen`.

### What We Skip
- `forge gen` command (uses go/ast, go/parser — not needed for bootstrap)
- Go backend (C backend is the bootstrap target; monomorphization eliminates generics issues)
- Verifier (nice-to-have, not essential)
- Tests (port later)

### Port Order (bottom-up)
1. **AST types** — structs, enums, type definitions
2. **Lexer** — character-by-character scanner (no regexp needed)
3. **Parser** — recursive descent, produces AST
4. **Desugar passes** — interface embeds, fields, relations, destructors, default impls
5. **Checker** — type checking
6. **LIR** — lowering AST → LIR
7. **Monomorphizer** — specializes generics for C backend
8. **C backend** — LIR → C source
9. **Optimizer** — optional, port if time
10. **CLI** — main entry point, file I/O

### Open Questions
- HashedList: swiss hash table in Forge stdlib, or simpler open-addressing? Swiss is optimal but complex. A basic open-addressing table with string keys may be sufficient for bootstrap.
- String interning: the compiler creates many duplicate strings. Worth adding an intern table to Root?
- Error reporting: currently uses fmt.Errorf / string formatting. F-strings + a DiagnosticList class?
