# Forge Language Review — Phase 1

*CodeRhapsody, 2026-06-09. Fresh-eyes review of spec, reference, Go compiler, bootstrap .fg files, stdlib, and testdata.*

## Executive Summary

The language is in surprisingly good shape for a bootstrap. The spec is coherent, the reference is practical, the bootstrap .fg files compile cleanly (0 errors, 0 warnings), and 100/100 C tests pass. The major issues are: (1) several spec features that aren't implemented or tested, (2) ergonomic friction in the bootstrap code that reveals language gaps, and (3) inconsistencies between spec, reference, and implementation.

I've grouped findings into **Quick Fixes** (compiler bugs/gaps, hours), **Design Decisions** (need Bill's input), and **Spec Drift** (documentation out of sync).

---

## 1. Quick Fixes (Compiler Gaps)

### 1.1 `forge fmt` lexer bug — keywords in strings
**Reference says**: "keywords inside string literals are tokenized as keywords, causing parse failures."
**Impact**: Can't format any .fg file containing `doc`, `source`, `match`, etc. in string literals. This is a showstopper for dogfooding.
**Fix**: Lexer must skip keyword recognition inside string literal state.

### 1.2 No `is` operator
**Spec mentions** enum pattern matching only via `match`. The reference says "no `is` operator."
**Bootstrap impact**: The lowerer.fg has many cases where you just want to check a variant kind without extracting data. Currently requires a full `match` with a wildcard arm, or a helper function.
**Suggestion**: Add `expr is Variant` as sugar for `match expr { Variant(_) => true, _ => false }`. Very useful for compiler code.

### 1.3 No ternary / if-expression
**Reference says**: "No ternary if-expression — use `let mut x = ...; if cond { x = a } else { x = b }`."
**Bootstrap impact**: Verbose. The bootstrap .fg files are littered with this pattern. Go has no ternary either, but Rust does (`if cond { a } else { b }` is an expression). Since Forge already has `match` expressions, making `if/else` an expression is consistent.
**Suggestion**: Make `if/else` an expression when used in expression context (Rust/Kotlin style). Low-risk addition.

### 1.4 `?` returns `T?`, not `T`
**Reference says**: "After `let x = foo()?`, `x` is `T?`, not `T`. Use `x!` to unwrap."
**This is wrong behavior.** In every language with `?` (Rust, Swift), the whole point is that `?` propagates the error and gives you the unwrapped value. If Forge's `?` still leaves you with an optional, it's not useful — you've just moved the error check without gaining anything. The bootstrap lowerer.fg has `x!` after `?` everywhere, confirming this is painful.
**Strong recommendation**: Fix `?` to return `T`, not `T?`. This is the #1 ergonomic issue in bootstrap code.

### 1.5 No map literal syntax
**Spec mentions** `{:}` for empty maps but map literals (`{"key": value}`) aren't implemented. The bootstrap uses `HashedList` and `Dict` with `.set()` calls instead.
**Impact**: Low for bootstrap (hash tables built incrementally), but missing for general use.

### 1.6 Slice syntax `xs[start:end]` — untested in C backend
`ExprSlice` exists in the AST but I don't see it in any testdata .fg file compiled through C. May be lowered/monomorphized correctly but unverified.

### 1.7 Import system is skeletal
`TImport` token exists, parser test exists, but no real module resolution. The bootstrap uses `forge BlockName {}` wrappers + `MergeStdlib` + multi-file compilation on the command line. This works for bootstrap but is a gap.

---

## 2. Design Decisions (Need Bill's Input)

### 2.1 Cast syntax: `<i64>x` vs `x as i64`
**Current**: `<i64>x` where `x` can be an expression, and `i64` can be a type literal, variable, or expression (cast to the type of that value).
**Alternative**: `y as x` — same semantics (x is typically a type but can be a variable/expression), more readable, no ambiguity with generic type params, familiar to Rust/Kotlin/Swift users.
**Recommendation**: Switch to `as`. Reads better in complex expressions, eliminates visual ambiguity with generics. Bill agrees this is reasonable.

### 2.2 Struct vs Class distinction
**Current**: Structs are value types (stack), classes are heap-allocated with identity. The rule is "structs for small value types only."
**Observation**: The bootstrap AST puts everything as a class, even things like `Token` that could be structs. The distinction is clear in principle but the guidance "small value types only" is vague — what's small? 
**Suggestion**: Document a clearer heuristic. E.g., "structs for types with ≤4 fields and no methods that mutate self. Classes for everything else."

### 2.3 Error handling: already Go-style interface
**Current**: `error` is an interface requiring `func message(self) -> string`. This is the right design (Go-style).
**Note**: The C backend implements errors as `const char*` for simplicity, which is fine for bootstrap. The spec correctly describes the interface — no action needed here. Documentation in the reference may need updating to reflect the interface design.

### 2.4 `any` type — legitimate escape hatch
**Current**: `any` = `interface {}` (Go) / `void*` (C). 
**Assessment**: Not worth discouraging or linting against. It's a valid escape hatch, same as Go. With generics available, most uses of `any` can be replaced with type parameters, which is preferable. The void* concern is a C backend detail, not a language flaw. No action needed.

### 2.5 Visibility: default-private vs default-public
**Current**: Default private, `pub` for exports.
**Observation**: The bootstrap .fg files rarely use `pub` because everything is in the same compilation unit via `MergeStdlib`. Once modules exist, visibility will matter a lot. Is the current default correct?
**Note**: Go's uppercase=public is unusual; Rust/Swift/Kotlin all use explicit `pub`/`public`. Forge's choice aligns with modern languages.

### 2.6 `for..in` variable order
**Current**: `for index, value in collection { ... }` (Go-style).
**Alternative**: `for value in collection` (most languages) with `for index, value in collection.enumerate()` for index access.
**Bootstrap observation**: The lowerer.fg rarely needs the index. The Go-style forced two-variable form adds friction when you only want values.
**Suggestion**: Support both `for value in col` and `for index, value in col`. Spec seems to already allow this but it's not clear.

### 2.7 Generators — Duff's device in C
**Current**: Generators compile to Duff's device state machines in C. 
**Observation**: This is clever but fragile. The testdata/generators.fg exists but the C backend implementation is limited. Is this a priority for bootstrap, or can it be deferred?

---

## 3. Spec/Implementation Drift

### 3.1 Spec mentions features not in compiler
- **Named impls** (`impl Named for Type`) — spec describes, not implemented
- **UFCS** (Universal Function Call Syntax) — mentioned as future
- **`i128`/`i256`** — spec lists them, not implemented (would need `math/big` or `__int128`)
- **Pattern guards** (`case X if cond =>`) — spec describes, I don't see parser support
- **Async/await** — spec mentions channels and spawn but not async/await. Correct to exclude?

### 3.2 Spec says things the compiler contradicts
- Spec says "explicit `<T>` on generic declarations required" but some testdata files use inference
- Spec says `fn` is for type syntax only, but the reference says `fn` is a "contextual keyword" — the parser actually accepts it in some positions

### 3.3 Reference has stale info
- Reference says "Checker warnings for cross-file refs" produce `void*` — this was fixed (the whole void* elimination effort)
- Reference lists Go backend as an option — it was deleted
- Reference says "Next to port: optimizer.fg, monomorphizer.fg, c_backend.fg" — still accurate but should note the milestone we've reached

### 3.4 `.forge` files are stale
The `pkg/ast/ast.forge`, `pkg/checker/checker.forge`, etc. are 27KB+ and likely haven't been updated since the massive changes (if-let, let-else, deepCopy, validator, etc.). These need a refresh in Phase 2.

---

## 4. Ergonomic Issues from Bootstrap Code

### 4.1 Verbose pattern matching
The lowerer.fg is 1411 lines. A huge portion is `match expr.kind { ... }` with full destructuring. `if let` helps but isn't used throughout yet. The rewrite in Phase 2 should aggressively use `if let`/`let..else`.

### 4.2 No method chaining / builder pattern
The stdlib's `HashedList`/`Dict`/`ArrayList` use `.set()`, `.get()`, etc. but methods return `void` so you can't chain. Not necessarily wrong but worth noting.

### 4.3 No string interpolation in match arms
F-strings work in general but the bootstrap uses `string_concat` calls in many places where f-strings would be cleaner. May be a style issue rather than a language gap.

### 4.4 `let mut` vs `let`
Rust-style mutability works well. No complaints here. Good design choice.

### 4.5 No early return type inference
Functions require explicit return types even when obvious. This is fine for a compiler-writing language (explicit > implicit), but worth noting.

---

## 5. Stdlib Gaps

### 5.1 Missing: sort
No sort function for slices. The bootstrap may need this for topological sorting etc.

### 5.2 Missing: file I/O
The spec describes `File`, `Reader`, `Writer`, `BufferedReader` etc. but `stdlib/` only has `std.fg` and `string.fg`. No I/O stdlib exists yet. Bootstrap will need file I/O for the `main.fg` entry point.

### 5.3 Missing: string→number parsing
`atoi`/`parse_float` exist as builtins but aren't in the stdlib .fg files. They're hardcoded in the C backend.

### 5.4 Missing: HashMap
`Dict<V>` is string-keyed only (uses `Sym`). No generic `HashMap<K,V>`. The bootstrap mostly uses string keys so this works, but it's a gap for general use.

---

## 6. Positive Observations

- **Relations are excellent.** The `ArrayList` relation system with auto-generated parent/child fields, destructors, and back-pointers is genuinely better than anything in Go/Rust. It's the killer feature.
- **Sym is smart.** Interned symbols with O(1) comparison. Great for a compiler language.
- **The if-let/let-else additions are exactly right.** Matches Rust semantics, eliminates verbose match blocks.
- **Type system is sound.** Numeric widening rules, no implicit int→float, optional types — all well-designed.
- **The monomorphization approach is correct** for C backend. No runtime dispatch overhead.
- **Bootstrap .fg files are readable.** Despite being 8,049 lines total, the code is clear and well-structured.
- **Zero warnings, zero errors, zero void*** — excellent engineering milestone.

---

## 7. Recommended Priority Order

1. **Fix `?` operator to return `T` not `T?`** — #1 ergonomic win, affects all error handling code
2. **Fix `forge fmt` string literal bug** — needed for dogfooding
3. **Add `is` operator** — small parser change, big ergonomic win for compiler code
4. **Make `if/else` an expression** — enables cleaner patterns throughout bootstrap
5. **Update spec and reference** to match current reality (delete Go backend refs, update void* status)
6. **Refresh `.forge` files** for all packages
7. **Add file I/O stdlib** — needed before `main.fg`
8. **Add sort to stdlib** — needed for many algorithms

Items 1-4 are quick compiler fixes (hours each). Items 5-6 are documentation. Items 7-8 are stdlib implementation.

---

*This document is the Phase 1 deliverable. Bill should review and decide on the design questions in Section 2 before Phase 2 begins.*
