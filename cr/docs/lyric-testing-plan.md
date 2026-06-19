# Lyric Bootstrap Testing Plan

## Overview

Unit tests for the Lyric bootstrap compiler modules, written in Lyric itself using `lyric test`. Each sprint produces a single test file exercising one module. Tests run via `lyric test test_file.ly deps.ly...`.

## Sprint Schedule

### Sprint 1: Lexer (`testdata/test_lexer.ly`)
**Deps**: `bootstrap/lexer.ly bootstrap/ast.ly`
**Coverage** (guided by `pkg/parser/lexer_test.go`):
- Keyword recognition (all `K*` variants)
- Annotation keywords (contextual: `doc`, `source`, `why`, etc.)
- Identifiers vs keywords
- Integer literals (decimal)
- Float literals
- String literals (basic, escapes, triple-quoted)
- F-string literals
- Character literals and escapes
- All punctuation tokens
- All operator tokens
- Newline handling (significant newlines)
- Comments (line comments)
- EOF
- Position tracking (line/column)
- Peek vs Next semantics
- Save/restore state (LexerState)
- Edge cases: empty input, consecutive operators, nested braces

### Sprint 2: Parser (`testdata/test_parser.ly`)
**Deps**: `bootstrap/parser.ly bootstrap/expr_parser.ly bootstrap/lexer.ly bootstrap/ast.ly`
**Coverage** (guided by `pkg/parser/parser_test.go`):
- Variable declarations (let, let mut, typed)
- Function declarations (params, return types, generics)
- Struct/class/enum declarations
- Control flow (if/else, while, for, match)
- Expression parsing (binary ops, precedence, unary, calls)
- Interface declarations
- Relation declarations
- Impl blocks
- Where clauses
- Try operator
- Top-level constants

### Sprint 3: AST/Desugar (`testdata/test_desugar.ly`)
**Deps**: `bootstrap/desugar.ly bootstrap/parser.ly bootstrap/expr_parser.ly bootstrap/lexer.ly bootstrap/ast.ly`
**Coverage**:
- Interface embed flattening
- Interface field injection
- Relation → field injection
- Destructor merging
- Default impl desugaring
- Multi-file merge

### Sprint 4: Checker (`testdata/test_checker.ly`)
**Deps**: `bootstrap/checker.ly bootstrap/parser.ly bootstrap/expr_parser.ly bootstrap/lexer.ly bootstrap/ast.ly`
**Coverage** (guided by `pkg/checker/checker_test.go`):
- Type resolution (primitives, structs, classes, enums)
- Scope chain (nested, shadowing)
- Builtin function signatures
- Generic instantiation
- Numeric widening
- Error detection (type mismatches)

### Sprint 5: Lowerer (`testdata/test_lowerer.ly`)
**Deps**: `bootstrap/lowerer.ly bootstrap/lir.ly bootstrap/checker.ly bootstrap/parser.ly bootstrap/expr_parser.ly bootstrap/lexer.ly bootstrap/ast.ly`
**Coverage**:
- AST→LIR type mapping (primitives, optionals, slices, tuples)
- Expression lowering (literals, binary, unary, calls)
- Statement lowering (let, if, while, for, return)
- Try operator lowering
- Match→switch lowering

### Sprint 6: Integration (`testdata/test_integration.ly`)
**Deps**: All bootstrap .ly files
**Coverage**:
- End-to-end: source string → parsed AST → checked → lowered LIR
- Representative programs exercising multiple features
- Regression tests for known bugs

## CDD Deliverables

Each sprint produces TWO deliverables:
1. **Passing tests** (`testdata/test_<module>.ly`)
2. **CDD .lyric file** (`bootstrap/<module>.lyric`)

The .lyric file captures module invariants discovered during testing. Before writing it, the instance MUST read the corresponding Go .lyric file (e.g., `pkg/checker/checker.lyric`) to understand the Go module's architecture, then write a bootstrap-specific .lyric that documents:
- Module invariants (especially cross-module contracts)
- Data model (key types, their relationships)
- Known gotchas and edge cases discovered during testing
- API surface (constructor, key methods, return types)

This ensures future sessions don't re-discover the same invariants. The Go .lyric files document the Go implementation; the bootstrap .lyric files document the Lyric port's specific concerns (different type system, different idioms, C backend constraints).

**Already exists**: `bootstrap/bootstrap.lyric` (top-level architecture)
**To create**: `bootstrap/lexer.lyric`, `bootstrap/ast.lyric`, `bootstrap/parser.lyric`, `bootstrap/checker.lyric`, `bootstrap/lowerer.lyric`, `bootstrap/lir.lyric`

## Conventions

- Test files live in `testdata/` alongside existing .ly test files
- One `lyric` block per test file, named after the module being tested (e.g., `lyric lexer {`)
- Helper functions prefixed with module name (e.g., `lex_single(src)`)
- Each `test_*` function tests one specific behavior
- Use `assert_eq` for value comparisons, `assert` for boolean conditions
- Dependencies listed BEFORE test file in lyric test command (order matters for checker)
