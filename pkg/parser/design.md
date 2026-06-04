# Parser Module Design

## Executive Summary

The `parser` module is the gateway to the Grok toolchain, responsible for transforming raw source text into a structured Abstract Syntax Tree (AST). It supports two distinct but related file formats: `.grok` files, which define high-level architectural specifications and invariants, and `.gk` files, which contain the concrete implementation logic. 

The module employs a sophisticated, hand-written lexical scanner and a hybrid parsing engine. It uses recursive descent for the broad structural elements of the language—such as classes, interfaces, and control flow statements—and a Pratt parser (precedence climbing) for expressions. This dual approach allows the parser to handle the complex operator hierarchies and concurrency primitives of Grok with high performance and precision.

## File Inventory

- [lexer.go](lexer.go): A hand-written scanner that tokenizes Grok source code. It manages significant newlines, multi-line triple-quoted strings, and the initial capture of interpolated f-strings.
- [parser.go](parser.go): Implements the core recursive descent parser. It handles top-level architectural declarations (`grok` blocks, `struct`, `class`, `interface`), type expressions, and architectural annotations.
- [expr_parser.go](expr_parser.go): Extends the parser with implementation-level logic. It contains the Pratt parser for expressions and recursive descent handlers for statements (`if`, `for`, `match`, `spawn`, `select`, `lock`).
- [parser.grok](parser.grok): The Grok specification for the parser module, defining its internal structures and relationships in its own language.

## Architecture and Data Flow

The `parser` module operates as a linear pipeline that converts a stream of characters into a tree of nodes. The process is driven by the `ParseFile` and `ParseString` entry points, which orchestrate the interaction between the `Lexer` and the `Parser`.

### Lexical Analysis
The `Lexer` is the first stage of the pipeline. It scans the input string and emits a sequence of `Token` objects. Each token includes its `TokenKind`, the literal `Text` from the source, and a `Span` (file, line, column) for error reporting and downstream analysis. 

The lexer is "newline-aware." In Grok, newlines are often significant delimiters. The lexer intelligently collapses multiple consecutive newlines and whitespace into a single `TNewline` token, ensuring the parser can rely on them for structural boundaries without being overwhelmed by formatting variations. It also handles complex literal types like triple-quoted strings (`"""..."""`) and f-strings (`f"..."`), where it captures the raw content for later processing by the parser.

### Structural Parsing
The `Parser` consumes the token stream produced by the lexer. It is organized as a set of mutually recursive methods that correspond to the Grok grammar. 

For high-level constructs, the parser uses **Recursive Descent**. When it enters a `grok` block, it dispatches to specific methods for parsing structs, classes, or interfaces based on the leading keyword. This phase is responsible for building the "skeleton" of the [pkg/ast](../ast/design.md), populating the `ast.File` and its constituent declaration nodes.

### Expression and Statement Parsing
When the parser encounters implementation logic (typically inside function bodies or variable initializers), it shifts its strategy:

1.  **Pratt Parsing**: For expressions, the parser uses a Pratt (precedence climbing) algorithm implemented in `expr_parser.go`. This allows it to handle a rich set of binary and unary operators—including mathematical, logical, and bitwise operations—by consulting a precedence table (`binaryPrec`). This approach elegantly handles operator associativity and prevents deep recursion issues.
2.  **Statement Dispatch**: Statements like `if`, `for`, and `match` are handled via recursive descent. The parser also includes specialized logic for Grok's concurrency primitives, such as `spawn` for asynchronous execution and `select` for channel communication.

### Data Flow Summary
1.  **Input**: Raw source string and filename.
2.  **Lexing**: `Lexer` produces a stream of `Token` objects.
3.  **Parsing**: `Parser` consumes tokens, using recursive descent for structure and Pratt parsing for expressions.
4.  **Output**: A pointer to a fully populated `ast.File` node, or a `ParseError` if the input violates the grammar.

## Interface Implementations

The `parser` module primarily produces data structures defined in `pkg/ast`. However, it implements the following standard interface:

- **`error`**: The `ParseError` struct implements the standard Go `error` interface. It provides a formatted string that includes the filename, line number, and column of the syntax error, followed by a descriptive message.

## Public API

The `parser` module provides a simple, functional API for external consumers:

- **`ParseFile(source, filename string) (*ast.File, error)`**: The primary entry point for parsing a file. The `filename` is used for populating the `Span` information in the AST.
- **`ParseString(source string) (*ast.File, error)`**: A convenience wrapper for parsing code from a string, often used in tests.
- **`Token` and `TokenKind`**: These types are exported to allow other modules (like syntax highlighters) to perform lexical analysis without a full parse.
- **`ParseError`**: Exported so that callers can perform type assertions and extract precise error locations.

The `Lexer` and `Parser` structs are kept internal to the package to hide implementation details and maintain a stable API surface.

## Implementation Details

### Significant Newlines and Whitespace
Grok uses newlines to separate declarations and statements. The `Lexer.scan()` method handles this by skipping horizontal whitespace (spaces, tabs) but preserving `\n`. It collapses sequences of newlines to simplify the parser's job. The `Parser.skipNewlines()` helper is used throughout the parsing logic to allow for flexible formatting between major constructs.

The lexer also treats `null` as a keyword alias for `nil`. Both map to the `TNil` token kind, which ensures that developers coming from different language backgrounds can use their preferred term while the internal representation remains unified.

### Expression Parsing via Pratt Precedence
To handle the rich set of operators in Grok—ranging from standard arithmetic to bitwise shifts and logical conjunctions—the module implements a Pratt parser in `expr_parser.go`. This algorithm uses a precedence table (`binaryPrec`) to drive the parsing loop with 12 distinct levels:

1.  **Logical OR** (`||`)
2.  **Logical AND** (`&&`)
3.  **Bitwise OR** (`|`)
4.  **Bitwise XOR** (`^`)
5.  **Bitwise AND** (`&`)
6.  **Equality** (`==`, `!=`)
7.  **Comparison** (`<`, `<=`, `>`, `>=`)
8.  **Shifts** (`<<`, `>>`)
9.  **Additive** (`+`, `-`)
10. **Multiplicative** (`*`, `/`, `%`)
11. **Unary** (`-`, `!`)
12. **Postfix** (`.`, `()`, `[]`, `!`, `?`)

Unary prefix operators are handled in `parseUnaryExpr` before entering the precedence loop. Postfix operations, including member access, function calls, indexing, and the unwrap (`!`) and try (`?`) operators, are handled in a tight loop within `parsePostfixExpr`.

### Statement Parsing and Desugaring
The parser handles a wide array of statements in `expr_parser.go`. Notably, it performs **desugaring** of compound assignments during the parse phase. For example, an expression like `x += y` is transformed directly into an `ast.AssignStmt` where the value is an `ast.BinaryExpr` representing `x + y`. This keeps the downstream transpiler and checker simpler by reducing the number of unique node types they must handle.

The `match` statement supports a variety of patterns, including:
- **Literal Patterns**: Matching against specific integers, strings, or booleans.
- **Identifier Patterns**: Binding a value to a new name.
- **Variant Patterns**: Destructuring enum variants (e.g., `Some(x)`).
- **Tuple Patterns**: Destructuring multiple values at once.
- **Wildcard Patterns**: The `_` catch-all.

### Recursive Type Parsing
The `parseTypeExpr` method handles Grok's recursive type system using a "base type" approach. It first parses a base type (like a named type, tuple, or sequence), then checks for an optional `?` suffix (optional type), then checks for a `|` infix (union type), and finally checks for a `->` infix (function type). This ordering correctly handles the precedence of type constructors. For function types, if the left-hand side is a tuple, the parser automatically unwraps its fields as individual function parameters.

### Disambiguation and Lookahead
The Grok grammar contains several ambiguities that require the parser to "peek" ahead:

- **Annotations vs. Fields**: The `isWhyAnnotation()` method peeks ahead to see if the `why` keyword is followed by a colon and a string (an annotation) or a type expression (a field named `why`).
- **Struct Literals vs. Blocks**: The `isStructLitAhead()` method distinguishes between a block of statements and a struct literal (e.g., `Point{x: 1}`) by looking for the `FieldName: Value` pattern.
- **Generic Calls vs. Comparisons**: The `tryParseTypeArgs()` method attempts to parse a list of type arguments. If it fails (e.g., it encounters a non-type token), it allows the parser to backtrack, treating the `<` as a comparison operator instead of the start of a generic argument list.

### F-String Interpolation
F-strings are handled in two stages. First, the `Lexer` captures the raw text between the quotes. Later, the `Parser.parseFString` method is called. It manually scans the raw text, splitting it into literal string parts and expressions enclosed in `{}`. For each expression, it creates a sub-parser to recursively parse the implementation logic, finally producing an `ast.ExprStringInterp` node.

### Concurrency Primitives
The parser provides first-class support for Grok's concurrency model:
- **`spawn { ... }`**: Parsed into a `StmtSpawn`, representing an asynchronous task.
- **`select { case ... }`**: Parsed into a `StmtSelect`, handling channel communication with optional default cases and variable bindings.
- **`lock(mutex) { ... }`**: Parsed into a `StmtLock`, ensuring mutual exclusion for a block of code.

## Dependencies

The `parser` module is a foundational component and has minimal dependencies:

- **[pkg/ast](../ast/design.md)**: The parser is the primary producer of the AST. It is tightly coupled with the node definitions in this package.
- **Go Standard Library**: Uses `fmt`, `strings`, `unicode`, and `unicode/utf8` for text processing and error formatting.

## Technical Debt and Future Work

- **Error Recovery**: The current implementation stops at the first error. Future versions should implement "panic mode" recovery (skipping to the next newline or brace) to report multiple errors in a single pass.
- **Performance**: For extremely large files, the recursive descent approach could be optimized with better token caching or a more efficient lookahead mechanism.
- **F-String Integration**: The manual scanning of f-strings in the parser could be moved into the lexer for more consistent error reporting and better handling of nested braces.
