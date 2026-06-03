# Parser Module Design

## Executive Summary

The `parser` module serves as the frontend of the Grok toolchain, responsible for transforming raw Grok source code into a structured Abstract Syntax Tree (AST). It implements a two-phase compilation pipeline consisting of a hand-written, stateful lexer and a hybrid parser that combines recursive descent for high-level declarations and statements with Pratt parsing (precedence climbing) for expressions. The module is designed to handle both declarative `.grok` files and implementation-focused `.gk` files, supporting Grok's rich type system, relationship modeling, and full procedural logic. A key design goal is to provide precise source location information for every construct, enabling downstream tools like the verifier and checker to produce high-quality, context-aware feedback.

## File Inventory

- [lexer.go](lexer.go): Implements the `Lexer`, a stateful scanner that tokenizes Grok source text. It handles UTF-8 input, various literal types (including triple-quoted strings), and manages significant newlines.
- [parser.go](parser.go): Implements the core `Parser` using recursive descent. It handles high-level declarations such as structs, enums, interfaces, classes, and functions, as well as the recursive type expression system and relation declarations.
- [expr_parser.go](expr_parser.go): Extends the `Parser` with expression and statement parsing logic. It uses a Pratt parsing approach for expressions to handle operator precedence and associativity, and recursive descent for procedural statements like `if`, `for`, `while`, and `match`.
- [lexer_test.go](lexer_test.go): Contains comprehensive unit tests for the lexer, covering keywords, annotations, punctuation, literals, and position tracking.
- [parser_test.go](parser_test.go): Provides extensive test coverage for the parser, ensuring it correctly handles all Grok language constructs and produces the expected AST structures.
- [expr_parser_test.go](expr_parser_test.go): Tests the expression and statement parsing logic, covering complex operator precedence, control flow, and pattern matching.
- [parser.grok](parser.grok): A self-describing Grok specification of the parser's own structure and the AST it produces. It serves as both documentation and a formal model of the module.

## Architecture and Data Flow

The `parser` module operates as a linear pipeline where data flows from raw text to a structured tree.

1.  **Lexical Analysis**: The `Lexer` scans the input character by character to produce a stream of `Token` objects. It is stateful, tracking the current line and column for source mapping. It distinguishes between standard keywords and "annotation keywords," which are often treated as identifiers unless they appear in specific annotation contexts. The lexer also handles complex literals like triple-quoted strings (`"""..."""`) and collapses multiple newlines to simplify the parser's logic while preserving significant breaks.
2.  **Syntactic Analysis**: The `Parser` consumes the token stream produced by the `Lexer`. It uses a hybrid approach:
    *   **Recursive Descent**: Each major grammatical construct (e.g., `parseStruct`, `parseFunc`, `parseIf`) has a corresponding method. This is used for the high-level structure of the file and for procedural statements.
    *   **Pratt Parsing**: For expressions, the parser uses precedence climbing. This allows it to handle a wide range of binary and unary operators with varying precedence levels (e.g., multiplication before addition) without the complexity of a deeply nested recursive descent grammar.
3.  **AST Construction**: As the parser recognizes constructs, it instantiates nodes from the [pkg/ast](../ast/design.md) module. Every node is populated with an `ast.Span` that captures its exact start and end positions in the source file. The final output is an `ast.File` object, which serves as the root of the syntax tree.

## Interface Implementations

- **ParseError**: Implements the standard Go `error` interface. It encapsulates a descriptive message and an `ast.Span`, allowing it to produce formatted error strings that include filename, line, and column information.

## Public API

The module provides a clean functional interface for common parsing tasks while exposing its internal types for advanced usage.

- **ParseFile(source, filename string) (*ast.File, error)**: The primary entry point for parsing a complete Grok source file (either `.grok` or `.gk`). It manages the lifecycle of the lexer and parser and returns the resulting AST or a `ParseError`.
- **ParseString(source string) (*ast.File, error)**: A convenience wrapper around `ParseFile` that uses `"<string>"` as the filename, ideal for testing or parsing dynamic snippets.
- **Lexer**: The exported lexer type, which can be used independently to tokenize Grok source. It provides `Peek()` and `Next()` methods for token stream management.
- **Parser**: The exported parser type, which can be used for more granular parsing tasks or integrated into larger tools like language servers.

## Implementation Details

### Lexer Logic

The `Lexer` is designed for flexibility and precision. It supports:
- **Triple-Quoted Strings**: Used for multi-line documentation or content. The lexer automatically trims leading and trailing whitespace lines to provide clean content to the parser.
- **Contextual Keywords**: Grok uses many keywords that are also common identifiers (e.g., `why`, `doc`, `source`). The lexer and parser work together to allow these to be used as identifiers in most positions, only treating them as keywords when they appear at the start of a declaration or annotation.
- **Newline Management**: Newlines are significant in Grok for separating declarations and ending certain annotation blocks. The lexer collapses sequences of newlines and whitespace into a single `TNewline` token, ensuring the parser sees a consistent stream of significant breaks.

### Parser Logic

The `Parser` is a robust implementation of the Grok grammar. Key features include:
- **Recursive Type Parsing**: The `parseTypeExpr` method handles Grok's complex, recursive type system. It supports sequences (`[T]`), tuples (`(T, U)`), maps (`map[K]V`), channels (`channel<T>`), optionals (`T?`), unions (`A | B`), and function types (`A -> B`). It uses a "base type" approach to handle precedence and nesting correctly.
- **Lookahead and State Restoration**: For constructs that are not LL(1), the parser saves the entire state of the `Lexer` (which is a simple struct copy), performs a trial parse, and restores the state if the trial fails. This is used for distinguishing `why` annotations from fields and for parsing tuple fields that may or may not have labels.
- **Relation Parsing**: The `parseRelation` method handles Grok's unique relationship syntax: `relation [hint] Parent[:label] owns|refs [Child[:label]]`. It supports optional hints (e.g., `ArrayList`), labels for named navigation, and brackets to indicate one-to-many relationships.

### Expression and Statement Parsing

The `expr_parser.go` file implements the logic for parsing function bodies and implementation details:
- **Pratt Parsing**: The `parsePrecExpr` method implements precedence climbing. It uses a table of precedence levels and associativity to correctly parse expressions like `a + b * c` as `a + (b * c)`. It handles binary operators, unary operators (`!`, `-`), and postfix operators like field access (`.`), method calls (`()`), and indexing (`[]`).
- **Statement Parsing**: The parser handles a full suite of procedural statements, including variable declarations (`let`), assignments, control flow (`if`, `for`, `while`, `match`), and the unique `cascade` block for cleanup logic.
- **Pattern Matching**: The `parsePattern` method supports structural decomposition in `match` statements, including literal patterns, identifier bindings, and variant patterns with nested bindings.

## Dependencies

- [pkg/ast](../ast/design.md): The parser is the primary producer of the AST and is tightly coupled to its structure.

## Technical Debt and Future Work

- **Error Recovery**: The current parser is "fail-fast," returning the first error it encounters. Implementing error recovery (e.g., synchronizing on the next `grok` block) would significantly improve the user experience in IDEs and during bulk processing.
- **Full Expression Integration**: While the expression parser is functional, some annotations like `requires` and `ensures` still capture raw text by reading until the end of the line. These should be migrated to use the full expression parser for deeper semantic analysis.
- **Map Literal Parsing**: The current implementation of map literal parsing (`parseMapLit`) is a simplified hack that skips the type part by counting brackets. It should be updated to properly parse the key and value types.
- **Multi-line Source Arrays**: The `parseSource` method currently does not skip newlines between file names in the `source: [...]` block, which may limit formatting options.
- **Performance**: While the current state-restoration lookahead is simple and effective, it could be optimized for extremely large files if parsing performance becomes a bottleneck.
