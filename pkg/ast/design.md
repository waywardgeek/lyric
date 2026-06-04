# AST Module Design

## Executive Summary

The `ast` module defines the Abstract Syntax Tree (AST) for the Grok language. It serves as the foundational data model shared by both `.grok` (architectural declaration) and `.gk` (full implementation) files. As a leaf package, it provides pure data structures that are populated by the [parser](../parser/design.md) and consumed by the [checker](../checker/design.md), [transpiler](../transpiler/design.md), and [verifier](../verifier/design.md). The design prioritizes simplicity and precision, using a tagged union pattern to represent the recursive nature of language constructs while maintaining source-level fidelity through comprehensive span tracking.

## File Inventory

*   [ast.go](ast.go): Defines the core declaration nodes (classes, interfaces, structs, enums, type aliases), type expressions, and top-level structures like `GrokBlock` and `File`. It also contains the metadata structures for source positions, spans, and annotations.
*   [expr.go](expr.go): Defines expression, statement, and pattern nodes used primarily in `.gk` files. This includes literals, operators, control flow (if, for, while, match), and concurrency primitives (spawn, select, lock).
*   [ast.grok](ast.grok): The architectural declaration for the `ast` module itself, documenting its public interface and relationships in Grok syntax.

## Architecture and Data Flow

The AST is organized as a strict hierarchy rooted in the `File` node. A `File` represents a single source file and contains one or more `GrokBlock` nodes. Each `GrokBlock` corresponds to a `grok { ... }` block in the source, allowing a single file to define multiple architectural modules or namespaces.

Data flows through the AST in a multi-stage pipeline:
1.  **Construction**: The [parser](../parser/design.md) transforms raw source text into a raw AST.
2.  **Annotation**: The [checker](../checker/design.md) traverses the AST to perform type inference and validation. It annotates `Expr` and `Pattern` nodes by setting their `ResolvedType` fields.
3.  **Consumption**: The annotated AST is then used by the [transpiler](../transpiler/design.md) to generate Go code, or by the [verifier](../verifier/design.md) to detect structural drift between `.grok` declarations and Go implementations.

## Interface Implementations

The `ast` module is primarily a collection of data structures and does not implement external interfaces. Instead, it defines the "language" that other modules in the Grok toolchain speak. The `ResolvedType` fields in `Expr` and `Pattern` are of type `any` to avoid a circular dependency with the [checker](../checker/design.md) package, which defines the actual type system.

## Public API

The public API consists of the struct definitions for all AST nodes. The most significant types are:

### Top-Level Nodes
*   **`File`**: The root node, containing the filename and a list of `GrokBlock`s.
*   **`GrokBlock`**: The primary container for declarations, including imports, documentation, invariants, and type definitions.

### Declarations
*   **`ClassDecl`**, **`StructDecl`**, **`EnumDecl`**, **`InterfaceDecl`**: Represent the various ways to define types in Grok.
*   **`TypeAliasDecl`**: Represents type aliases (`type Name = OtherType`).
*   **`FuncDecl`**: Represents both top-level functions and methods. It includes a `Body` field (a `*Block`) which is `nil` in `.grok` files and populated in `.gk` files.
*   **`RelationDecl`**: Models explicit ownership (`owns`) and reference (`refs`) relationships between types.

### Type Expressions
*   **`TypeExpr`**: A universal container for any type reference (e.g., `[i32]`, `map[string]T`, `T?`, `T | U`). It uses a `TypeExprKind` to identify the specific type of expression and a `Data` field for the payload.

### Expressions and Statements
*   **`Expr`**: A tagged union for all expression types. This includes literals, calls, field access, binary/unary operations, and Grok-specific features like `ExprTry` (`expr?`), `ExprUnwrap` (`expr!`), `ExprCast` (`<T>expr`), and `ExprStringInterp` (interpolated strings).
*   **`Stmt`**: A tagged union for all statement types, including variable declarations (`let`), assignments, control flow, and concurrency primitives like `StmtSpawn`, `StmtSelect`, and `StmtLock`. It also includes `StmtCascade`, which functions similarly to Go's `defer`.
*   **`Block`**: A sequence of statements, used as the body for functions and control structures.

### Patterns
*   **`Pattern`**: Used in `match` expressions and statements. It supports identifier bindings, literals, enum variants (with nested patterns), and wildcards (`_`).

## Implementation Details

### Tagged Union Pattern
To represent the recursive and varied nature of types, expressions, and statements in Go (which lacks native sum types), the `ast` module employs a tagged union pattern. Each of these base types (`TypeExpr`, `Expr`, `Stmt`, `Pattern`) consists of:
1.  A `Kind` enum (e.g., `ExprBinary`, `TypeOptional`).
2.  A `Data` field of type `any` which holds the specific payload struct (e.g., `*BinaryExpr`, `OptionalType`).
3.  A `Span` for source tracking.

Consumers of the AST typically use a `switch` statement on the `Kind` and then type-assert the `Data` field to access the specific node properties.

### Source Mapping and Spans
Every major AST node carries a `Span`, which consists of a start and end `Pos` (File, Line, Column). While this increases the memory footprint of the AST, it is essential for:
*   Precise error reporting in the `checker`.
*   Detailed drift reports in the `verifier`.
*   Potential future support for IDE features like "Go to Definition" or refactoring.

### Annotations
The `Annotations` struct is a first-class member of `FuncDecl`. It captures Grok's safety and concurrency metadata, such as whether a function is `pure`, if it `spawns` goroutines, or if it requires specific locks (`RequiresLock`). These are not treated as comments but as structured data intended for formal verification and analysis.

### Error Propagation and Null Safety
The AST explicitly models Grok's approach to error handling and null safety through `ExprTry` (`?`) and `ExprUnwrap` (`!`). `ExprTry` represents the propagation of errors from functions returning `(T, error)`, while `ExprUnwrap` represents the explicit (and potentially panicking) conversion from an optional type `T?` to `T`.

## Dependencies

The `ast` module is a leaf package and has no dependencies on other internal `pkg` modules. It is consumed by:
*   [pkg/parser](../parser/design.md): To build the AST from source.
*   [pkg/checker](../checker/design.md): To validate and annotate the AST.
*   [pkg/transpiler](../transpiler/design.md): To generate Go code from the AST.
*   [pkg/verifier](../verifier/design.md): To compare the AST against Go source code.

## Technical Debt and Future Work

*   **Semantic Validation**: The AST nodes do not currently enforce semantic rules (e.g., preventing circular interface inheritance). This is currently handled by the `checker`.
*   **Serialization**: There is no built-in support for serializing the AST to JSON or other formats, which might be useful for external tools or caching.
*   **Visitor Pattern**: As the number of node types increases, the manual traversal logic in the `checker` and `transpiler` may become difficult to maintain. Implementing a Visitor or Walker pattern could simplify these consumers.
*   **Expression-based Match**: While `ExprMatch` exists in the AST, the transpiler currently has limited support for it because Go's `switch` is a statement, not an expression.
