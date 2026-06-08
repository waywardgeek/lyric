# Forge Bootstrap Testing Plan

## Overview

Unit tests for the Forge bootstrap compiler modules, written in Forge itself using `forge test`. Each sprint produces a single test file exercising one module. Tests run via `forge test test_file.fg deps.fg...`.

## Sprint Schedule

### Sprint 1: Lexer (`testdata/test_lexer.fg`)
**Deps**: `bootstrap/lexer.fg bootstrap/ast.fg`
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

### Sprint 2: Parser (`testdata/test_parser.fg`)
**Deps**: `bootstrap/parser.fg bootstrap/expr_parser.fg bootstrap/lexer.fg bootstrap/ast.fg`
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

### Sprint 3: AST/Desugar (`testdata/test_desugar.fg`)
**Deps**: `bootstrap/desugar.fg bootstrap/parser.fg bootstrap/expr_parser.fg bootstrap/lexer.fg bootstrap/ast.fg`
**Coverage**:
- Interface embed flattening
- Interface field injection
- Relation → field injection
- Destructor merging
- Default impl desugaring
- Multi-file merge

### Sprint 4: Checker (`testdata/test_checker.fg`)
**Deps**: `bootstrap/checker.fg bootstrap/parser.fg bootstrap/expr_parser.fg bootstrap/lexer.fg bootstrap/ast.fg`
**Coverage** (guided by `pkg/checker/checker_test.go`):
- Type resolution (primitives, structs, classes, enums)
- Scope chain (nested, shadowing)
- Builtin function signatures
- Generic instantiation
- Numeric widening
- Error detection (type mismatches)

### Sprint 5: Lowerer (`testdata/test_lowerer.fg`)
**Deps**: `bootstrap/lowerer.fg bootstrap/lir.fg bootstrap/checker.fg bootstrap/parser.fg bootstrap/expr_parser.fg bootstrap/lexer.fg bootstrap/ast.fg`
**Coverage**:
- AST→LIR type mapping (primitives, optionals, slices, tuples)
- Expression lowering (literals, binary, unary, calls)
- Statement lowering (let, if, while, for, return)
- Try operator lowering
- Match→switch lowering

### Sprint 6: Integration (`testdata/test_integration.fg`)
**Deps**: All bootstrap .fg files
**Coverage**:
- End-to-end: source string → parsed AST → checked → lowered LIR
- Representative programs exercising multiple features
- Regression tests for known bugs

## UDD Deliverables

Each sprint produces TWO deliverables:
1. **Passing tests** (`testdata/test_<module>.fg`)
2. **UDD .forge file** (`bootstrap/<module>.forge`)

The .forge file captures module invariants discovered during testing. Before writing it, the instance MUST read the corresponding Go .forge file (e.g., `pkg/checker/checker.forge`) to understand the Go module's architecture, then write a bootstrap-specific .forge that documents:
- Module invariants (especially cross-module contracts)
- Data model (key types, their relationships)
- Known gotchas and edge cases discovered during testing
- API surface (constructor, key methods, return types)

This ensures future sessions don't re-discover the same invariants. The Go .forge files document the Go implementation; the bootstrap .forge files document the Forge port's specific concerns (different type system, different idioms, C backend constraints).

**Already exists**: `bootstrap/bootstrap.forge` (top-level architecture)
**To create**: `bootstrap/lexer.forge`, `bootstrap/ast.forge`, `bootstrap/parser.forge`, `bootstrap/checker.forge`, `bootstrap/lowerer.forge`, `bootstrap/lir.forge`

## Conventions

- Test files live in `testdata/` alongside existing .fg test files
- One `forge` block per test file, named after the module being tested (e.g., `forge lexer {`)
- Helper functions prefixed with module name (e.g., `lex_single(src)`)
- Each `test_*` function tests one specific behavior
- Use `assert_eq` for value comparisons, `assert` for boolean conditions
- Dependencies listed BEFORE test file in forge test command (order matters for checker)
