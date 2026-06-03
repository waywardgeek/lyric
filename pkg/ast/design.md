# AST Module Design

## Executive Summary

The `ast` module provides the foundational data model for the Grok language. It defines the Abstract Syntax Tree (AST), which serves as the common representation for both `.grok` (declaration-only) and `.gk` (full implementation) files. The AST is designed to be highly expressive, capturing complex type systems, semantic relations, and rich metadata while maintaining precise source location information. As a leaf package in the Grok hierarchy, it manages no internal state and provides the structural contracts consumed by the parser, checker, transpiler, and verifier.

## File Inventory

- [ast.go](ast.go): The primary source file containing the Go type definitions for core AST nodes, including declarations for classes, structs, enums, interfaces, and functions, as well as the type expression system.
- [expr.go](expr.go): Defines the expression and statement system used for function bodies and implementation logic, including literals, calls, control flow, and pattern matching.
- [ast.grok](ast.grok): A self-describing Grok specification of the AST, providing a high-level overview of the data structures and their relationships in the Grok language itself.

## Architecture and Data Flow

The AST is organized as a strict hierarchy, rooted in the `File` structure. A `File` contains one or more `GrokBlock` nodes, each representing a logical grouping of declarations within a `grok { ... }` block. This structure allows a single source file to contain multiple architectural modules or namespaces.

Data flows into the AST from the [parser](../parser/design.md), which transforms raw source text into this structured representation. Once constructed, the AST becomes the "source of truth" for the rest of the toolchain. The [checker](../checker/design.md) performs semantic analysis and type inference on the AST. The [verifier](../verifier/design.md) traverses this tree to compare the declarative model against actual Go source code. The [transpiler](../transpiler/design.md) consumes the AST to generate idiomatic Go code.

A central architectural pattern in this module is the use of a tagged union for recursive structures. This pattern uses a `Kind` field (an enum) to discriminate between different types of nodes and a `Data` field (`any`) to hold the specific payload. This approach avoids the complexity of a deep interface hierarchy while maintaining pragmatic type safety through discriminators.

- **TypeExpr**: Uses `TypeExprKind` to discriminate between types like `TypeNamed`, `TypeUnion`, and `TypeSequence`.
- **Expr**: Uses `ExprKind` to discriminate between expressions like `ExprCall`, `ExprBinary`, and `ExprLambda`.
- **Stmt**: Uses `StmtKind` to discriminate between statements like `StmtVarDecl`, `StmtIf`, and `StmtMatch`.
- **Pattern**: Uses `PatternKind` to discriminate between patterns like `PatIdent`, `PatVariant`, and `PatTuple`.

This recursive design allows the AST to represent deeply nested structures with a consistent and manageable interface. Every major node in the AST includes a `Span` (composed of `Pos` start and end points), ensuring that every element can be traced back to its exact location in the source file.

## Interface Implementations

The `ast` module is a pure data-modeling package and does not implement any external interfaces. It defines the concrete types that serve as the interface between the parser and the rest of the toolchain.

## Public API

The public API consists of exported structs and enums that define the grammar of the Grok language.

### Core Structures

The `File` struct is the entry point for any parsed Grok source. It aggregates `GrokBlock` nodes, which are the primary containers for declarations. Each `GrokBlock` can include imports, documentation blocks, invariants, and the full suite of Grok type declarations.

### Type Expressions

The `TypeExpr` struct is the most versatile component of the API. It uses `TypeExprKind` to identify the specific nature of a type:
- **TypeNamed**: Standard types, potentially with type arguments (e.g., `Stack<T>`).
- **TypeOptional**: Nullable types (e.g., `T?`).
- **TypeUnion**: Sum types (e.g., `T | U`).
- **TypeSequence**: List types (e.g., `[T]`).
- **TypeMap**: Key-value mappings (e.g., `map[K]V`).
- **TypeTuple**: Positional or named tuples (e.g., `(T, U)` or `(x: T, y: U)`).
- **TypeFunc**: Function signatures (e.g., `T -> U`).
- **TypeChannel**: Communication channels (e.g., `channel<T>`).
- **TypeLock** and **TypeUnit**: Special types for synchronization and empty values.

### Declarations

The module defines several declaration types that map to Grok's high-level constructs:
- **StructDecl**: Represents named records or tuples with fields.
- **EnumDecl**: Represents sum types with multiple variants, each potentially having its own fields.
- **InterfaceDecl**: Defines sets of method signatures and supports interface composition through the `Implements` field.
- **ClassDecl**: Models complex objects with fields, methods, constructor parameters, and implemented interfaces.
- **FuncDecl**: Defines functions or methods, including parameters, return types, where clauses for constraints, and semantic annotations. It includes a `Body` field (a `*Block`) which is populated in `.gk` files.
- **RelationDecl**: Explicitly models relationships between types, such as ownership (`Owns`) or reference (`Refs`) associations. It supports cardinality (`IsMany`) and hints for implementation (e.g., `DoublyLinked`, `Hashed`).

### Expressions and Statements

The `expr.go` file extends the AST to support full implementation logic:
- **Expr**: Represents values and computations, from simple literals (`IntLitExpr`, `StringLitExpr`, `BoolLitExpr`) to complex operations (`CallExpr`, `BinaryExpr`, `LambdaExpr`, `MatchExpr`).
- **Stmt**: Represents actions and control flow, including variable declarations (`VarDeclStmt`), assignments, loops (`ForStmt`, `WhileStmt`), conditional logic (`IfStmt`), and pattern matching (`MatchStmt`).
- **Block**: A sequence of statements, typically used as a function body or a branch in a control flow statement.
- **Pattern**: Used in `MatchStmt` and `MatchExpr` for structural decomposition of values, supporting identifiers, literals, variants, and tuples.
- **CascadeStmt**: Represents cleanup logic, similar to Go's `defer`, ensuring a block of code runs at the end of the current scope.

## Implementation Details

The `TypeExpr`, `Expr`, `Stmt`, and `Pattern` implementations are critical pieces of the module's logic. The tagged union pattern is particularly effective for the recursive nature of type systems and expression trees. For example, a `TypeExpr` of kind `TypeUnion` will have a `Data` field containing a `UnionType` struct, which itself contains a slice of `TypeExpr` nodes, allowing for arbitrary nesting.

The `Annotations` struct is another vital component, reflecting Grok's focus on safety and concurrency. It captures metadata such as whether a function is `Pure`, whether it `Spawns` new tasks, and its requirements for locking (`RequiresLock`, `ExcludesLock`). It also supports preconditions (`Requires`) and postconditions (`Ensures`), as well as tracking which exceptions or errors a function `Raises`. These annotations are first-class members of the AST intended for formal verification.

Relations are handled via `RelationDecl`, which specifies the `Parent` and `Child` sides of a relationship. It supports different relationship kinds (`Owns` vs `Refs`) and cardinality (`IsMany`), allowing the architect to declare the structural constraints of the system's object graph. The `Hint` field allows the architect to suggest an implementation strategy, such as `DoublyLinked` or `Hashed`.

## Dependencies

The `ast` module is a leaf package and has no dependencies on other modules within the Grok project. It relies exclusively on the Go standard library.

## Technical Debt and Future Work

- **Semantic Validation**: The module currently only defines the structure of the AST. It does not provide any methods for semantic validation, such as checking for circular interface inheritance or duplicate field names. This is currently handled by the [checker](../checker/design.md).
- **Serialization**: There is currently no built-in support for serializing the AST to formats like JSON, which would be useful for external tooling or caching.
- **Visitor Pattern**: As the number of AST node types grows, implementing a Visitor pattern might simplify the traversal logic in the checker, verifier, and other consumers.
- **Expression Integration**: While the expression system is defined, its full integration into the parser and verifier for `.gk` files is an ongoing effort.
