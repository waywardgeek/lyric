# Parser Module Design

The `parser` module is the foundational entry point for the Forge compiler frontend. Its primary responsibility is to transform raw Forge source code—whether in declarative `.forge` files or imperative `.fg` files—into a structured Abstract Syntax Tree (AST). It employs a hand-written lexical scanner and a hybrid parsing strategy that combines recursive descent for high-level structures with a Pratt (precedence climbing) parser for expressions. This architecture ensures that the parser is both highly performant and capable of providing precise error reporting, while handling the unique syntactic requirements of the Forge language.

## Executive Summary

The `parser` module serves as the "front door" of the compilation pipeline. It takes raw source text and produces a comprehensive representation of the program's structure as defined in the [pkg/ast](../ast/design.md) module. The module is designed to handle both the declarative specification subset used in `.forge` files and the full imperative implementation language used in `.fg` files. By producing a unified AST, it allows subsequent stages of the compiler, such as the checker and the lowerer, to operate on a consistent data model regardless of the original source file's purpose.

The parser is designed to be LL(1) where possible, using targeted backtracking and state restoration only for specific grammatical ambiguities. It features a stateful lexer that manages significant newlines and comment collection, and a hybrid parser that leverages the Pratt algorithm to handle Forge's 12 levels of operator precedence without the "grammar explosion" typical of pure recursive descent.

## File Inventory

- [lexer.go](lexer.go): Implements a stateful, hand-written lexical scanner. It tokenizes the source text into a stream of `Token` objects, handles significant newlines, and collects line comments for inclusion in the final AST. It also manages complex literal scanning for triple-quoted strings and f-strings.
- [parser.go](parser.go): Contains the core `Parser` implementation and the recursive descent logic for top-level declarations. It manages the parsing of `forge` blocks, classes, interfaces, enums, and complex type expressions. It handles the high-level structure of both `.forge` and `.fg` files and manages contextual keyword resolution.
- [expr_parser.go](expr_parser.go): Implements the expression engine using the Pratt algorithm and the statement parser using recursive descent. It also manages specialized logic for match expressions, lambdas, concurrency primitives (`spawn`, `select`, `lock`), and the recursive parsing of interpolated f-strings.
- [parser.forge](parser.forge): A Forge specification file that provides a high-level architectural overview and self-documentation of the parser module.
- [lexer_test.go](lexer_test.go): Unit tests for the lexical scanner, covering tokenization, newline handling, and literal scanning.
- [parser_test.go](parser_test.go): Unit tests for the declaration parser, ensuring correct AST construction for various Forge constructs.
- [expr_parser_test.go](expr_parser_test.go): Unit tests for the expression and statement parser, covering precedence, control flow, and complex expression trees.

## Architecture and Data Flow

The transformation from raw text to an AST follows a linear pipeline within the module. The process begins with the `Lexer`, which reads the source string character by character. The `Parser` then consumes these tokens to build the tree.

### Lexical Analysis

The `Lexer` is a stateful scanner that uses a `peek`/`advance` pattern. Unlike many traditional lexers, the Forge lexer treats newlines as significant tokens (`TNewline`) when they serve as statement or declaration separators. To simplify the parser's job, it automatically collapses multiple consecutive newlines. A critical feature for ergonomics is bracket-aware newline suppression: the lexer tracks the nesting depth of `()`, `[]`, and `{}` and suppresses newline tokens when the depth is greater than zero. This allows for multi-line expressions without explicit continuation characters. Comments are captured during scanning and stored in a dedicated slice, allowing them to be attached to the root `ast.File` node.

### Hybrid Parsing Strategy

The `Parser` employs a hybrid strategy to balance simplicity and power:

1.  **Recursive Descent for Declarations**: High-level structures like `class`, `interface`, and `func` are parsed using standard recursive descent. Each major construct has a corresponding method (e.g., `parseClass`, `parseFunc`). This approach is largely LL(1), using targeted lookahead only when necessary.
2.  **Pratt Parsing for Expressions**: For the complex expression grammar in `.fg` files, the parser switches to a Pratt (precedence climbing) algorithm. This allows for clean handling of 12 levels of operator precedence and associativity in a single recursive function (`parsePrecExpr`), avoiding the deep call stacks and redundant rules of pure recursive descent.

### AST Construction

As the parser matches grammatical rules, it instantiates nodes from the `pkg/ast` module. Every node is assigned an `ast.Span` capturing its exact location in the source file, which is critical for downstream error reporting. The output of the process is a single `ast.File` node containing all parsed blocks and declarations.

## Interface Implementations

The `parser` module does not implement external interfaces. It is designed as a pure producer of the `ast.File` structure. This structure serves as the primary data contract consumed by the [pkg/checker](../checker/design.md) and the [pkg/lir](../lir/design.md) modules.

## Public API

### Entry Points

The module provides two primary entry points for external callers:

- **`ParseFile(source, filename string) (*ast.File, error)`**: The standard entry point for parsing a Forge source file. It initializes the lexer and parser, executes the parsing logic, and returns the resulting AST.
- **`ParseString(source string) (*ast.File, error)`**: A convenience wrapper around `ParseFile` that uses `"<string>"` as the filename, typically used for testing or parsing small snippets of code.

### Core Types

- **`Parser`**: The central parsing engine. It maintains the lexer state, a list of accumulated errors, and contextual flags (like `exprDepth` and `noStructLit`) used to disambiguate grammar.
- **`Lexer`**: The stateful scanner responsible for tokenization. It provides `Peek` and `Next` methods and maintains the collection of scanned comments.
- **`ParseError`**: A specialized error type that implements the `error` interface. It includes a descriptive message, an `ast.Span`, and the original source line for enhanced error reporting.
- **`Token`**: Represents a single lexical unit, containing its kind, raw text, and source span.

## Implementation Details

### Disambiguation and Backtracking

The Forge grammar contains several ambiguities that require lookahead to resolve. The parser handles these cases by saving the current state of the `Lexer` (which is a small, copyable struct), attempting a trial parse, and restoring the saved state if it fails. Key scenarios include:

- **Struct Literals vs. Blocks**: In statement context, `Ident {` is ambiguous. The parser peeks ahead to see if the content looks like a field initializer (`name: value`) or an empty literal `{}` to distinguish a struct literal from a block. Inside parentheses or brackets (where `exprDepth > 0`), it always assumes a struct literal.
- **Generic Calls vs. Comparisons**: The `<` operator can start a generic argument list or a comparison. `tryParseTypeArgs` attempts to parse a type argument list and backtracks if it fails or if the following token is not an opening parenthesis.
- **Why Annotations vs. Fields**: The `why` keyword can start an annotation (`why: "..."`) or be a field name (`why: Type`). The parser peeks ahead to see if a string literal follows the colon.
- **Nested Generics (>> Splitting)**: When the lexer encounters `>>` (e.g., in `List<List<T>>`), it emits a `TShr` token. The parser detects this in generic contexts and splits it into two `TGt` (>) tokens, pushing the second one back into a local buffer for the next `peek()` or `next()` call.

### Pratt Parsing and Precedence

The expression parser in `expr_parser.go` implements the Pratt algorithm. It uses a precedence table to determine when to continue grouping expressions:

1.  **OR**: `||`
2.  **AND**: `&&`
3.  **Bitwise OR**: `|`
4.  **Bitwise XOR**: `^`
5.  **Bitwise AND**: `&`
6.  **Equality**: `==`, `!=`
7.  **Comparison**: `<`, `>`, `<=`, `>=`
8.  **Shift**: `<<`, `>>`
9.  **Additive**: `+`, `-`
10. **Multiplicative**: `*`, `/`, `%`
11. **Unary**: `-`, `!` (prefix)
12. **Postfix**: `.`, `()`, `[]`, `!`, `?`

Postfix operators, including member access, function calls, indexing, unwrap (`!`), and try (`?`), are handled in a loop after the primary expression is parsed, allowing for arbitrary chaining.

### Contextual Keywords

Forge treats many keywords (such as `why`, `doc`, `source`, `field`, `lock`, `implements`) as contextual. The lexer emits them as `TIdent`, and the parser rewrites their kind only when they appear in valid annotation or declaration positions. The `expectIdentLike` method allows these tokens to be used as identifiers in positions where they are not ambiguous, such as field or parameter names. Additionally, `null` is treated as a keyword alias for `nil`, both mapping to the same `TNil` token.

### F-String Interpolation

F-strings (`f"..."`) are handled by capturing the raw content in the lexer. The parser then scans this content for `{expr}` boundaries. For each expression found, it creates a new `Parser` instance to process the embedded text and merges the resulting nodes into a `StringInterpExpr`. This recursive approach allows for full expression power within interpolated strings.

### Recursive Type Parsing

The parser handles Forge's recursive type system through a tiered approach in `parseTypeExpr`. It first parses a "base type" (named types, sequences `[T]`, tuples `(T, U)`, maps `map[K]V`, channels `channel<T>`, etc.), then checks for optional suffixes like `?` (optional) or `|` (union), and finally checks for function type arrows `->`. This structure correctly handles precedence, such as `[T]?` being an optional sequence versus `[T?]` being a sequence of optionals.

### Statement Desugaring

Assignments desugar compound operators (like `+=`) into their binary equivalents during the parsing phase (e.g., `x += y` becomes `x = x + y`). This simplifies the work for later compiler stages by reducing the number of primitive operations they must handle.

## Dependencies

- **[pkg/ast](../ast/design.md)**: The parser is entirely dependent on the `ast` module for its output data structures. It uses `ast.Pos` and `ast.Span` for location tracking and constructs various `ast.Node` types to build the tree.

## Technical Debt and Future Work

- **Error Recovery**: The current implementation is largely fail-fast. Implementing a synchronization mechanism (e.g., skipping to the next forge block or closing brace) would allow the parser to report multiple errors in a single pass.
- **Multi-line Arrays**: Some declarative constructs like `source: [...]` currently require single-line formatting due to parser limitations.
- **Requires/Ensures Expressions**: Currently, these annotations capture raw text rather than fully parsed expression trees. Moving to a parsed representation would allow for better validation during the checking phase.
- **Lambda Union Types**: Lambda parameters currently use `parseBaseType` to avoid ambiguity with the `|` operator, which prevents the direct use of union types in lambda signatures without a type alias.
