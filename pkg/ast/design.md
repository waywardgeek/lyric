# AST Module Design

The `ast` module defines the Abstract Syntax Tree (AST) for the Forge language, serving as the foundational data model for the entire compiler toolchain. It provides a structured representation of Forge source code that is shared between declaration-only `.forge` files and full implementation `.fg` files. Beyond simple data representation, the module includes a critical desugaring phase that simplifies high-level architectural constructs into fundamental language primitives before semantic analysis.

## Executive Summary

The `ast` module is the central data hub of the Forge compiler's frontend. It acts as the primary contract between the [parser](../parser/design.md), which produces the tree, and the [checker](../checker/design.md) and [lir](../lir/design.md) modules, which consume and transform it. The module is designed with a "Kind + Data" pattern to achieve polymorphism without the complexity of deep interface hierarchies, making it highly efficient for the recursive traversals common in compiler passes. 

A standout feature of this module is its generative desugaring engine, which automates the expansion of complex features like relations, interface fields, and default implementations into standard classes, methods, and implementation blocks. Additionally, the module handles the selective merging of the standard library, ensuring that only the necessary components are included in the final AST, thereby keeping the compilation process lean and focused. Finally, a post-desugar validation pass ensures that the tree is in a consistent state before being handed off to the type checker.

## File Inventory

- [ast.go](ast.go): Defines the core structural nodes for declarations, including functions, classes, structs, enums, interfaces, and relations. It also establishes the foundational types for source location tracking (`Pos` and `Span`) and type expressions.
- [expr.go](expr.go): Contains the definitions for all expression and statement nodes used in full implementation files. This includes literals, operators, control flow structures (if, for, while, match), and concurrency primitives like `spawn` and `select`.
- [desugar.go](desugar.go): Implements the transformation logic that simplifies the AST. It handles the expansion of relations into concrete fields and `impl` blocks, extracts default interface implementations into top-level functions, and generates destructor logic.
- [stdlib.go](stdlib.go): Manages the integration of the Forge standard library. It provides utilities for locating the stdlib on disk and selectively merging its interfaces, classes, and functions into the user's AST based on actual usage.
- [validate.go](validate.go): Provides post-desugar validation to ensure that all high-level constructs (like interface embeds) have been correctly flattened and that relations have been properly expanded into class fields.
- [ast.forge](ast.forge): A Forge source file that describes the AST module itself, serving as a self-documenting specification of the module's architecture and purpose.

## Architecture and Data Flow

The AST is the "central nervous system" of the compiler, with data flowing through it in a well-defined sequence of stages.

The process begins in the **Parsing** stage, where the [parser](../parser/design.md) transforms raw source text into a `File` node. This node contains one or more `ForgeBlock` structures, allowing a single file to house multiple architectural modules. Once the raw AST is constructed, it enters the **Desugaring** phase. This is a series of idempotent transformations that expand high-level "sugar"—such as `relation` declarations and `field` definitions within interfaces—into explicit, lower-level AST nodes. For instance, a single `relation` might result in new fields being injected into multiple classes and the creation of a corresponding `impl` block.

Following desugaring, the AST undergoes **Validation** to confirm that the transformations were successful and the tree is semantically ready for the next stage. The validated AST is then passed to the **Semantic Analysis** stage. The [checker](../checker/design.md) performs name resolution and type inference, decorating the AST nodes with semantic information. Crucially, the AST nodes include `ResolvedType` and `InferredTypeArgs` fields (stored as `any` to prevent circular dependencies) which the checker populates. Finally, the fully annotated and desugared AST is consumed by the **Lowering** stage in the [lir](../lir/design.md) module, which flattens the tree into a low-level intermediate representation for code generation.

## Interface Implementations

The `ast` module is intentionally data-centric and does not implement complex behavioral interfaces. The nodes are designed to be passive structures that are acted upon by external visitors or transformation functions. 

- The `File` type provides a `DocComment` utility method to retrieve contiguous comment blocks preceding a specific line, which is used by the [verifier](../verifier/design.md) and documentation generators to associate narrative text with code declarations.
- The `InvariantViolation` type in `validate.go` implements the `fmt.Stringer` interface to provide a standardized string representation of validation failures.

## Public API

### Core Data Structures

The API is dominated by the tree's root and its primary branches. The `File` struct is the entry point, holding a collection of `ForgeBlock` nodes and all source comments. Each `ForgeBlock` acts as a container for the various declarations supported by Forge: `StructDecl`, `EnumDecl`, `InterfaceDecl`, `ClassDecl`, `FuncDecl`, `RelationDecl`, `ImplBlock`, `TypeAliasDecl`, `ConstDecl`, `ImportDecl`, `DocBlock`, and `InvariantDecl`.

Type expressions are represented by the `TypeExpr` struct, which uses a `TypeExprKind` to distinguish between named types, optionals, unions, sequences, maps, tuples, function signatures, channels, generators, locks, and units. Similarly, the `Expr` and `Stmt` structs in `expr.go` use `ExprKind` and `StmtKind` respectively to represent the full breadth of the language's operational syntax, including complex constructs like `spawn`, `select`, `cascade`, and `lock`.

### Desugaring Engine

The module exports several high-level functions that perform the AST transformations, typically executed in a specific order:

- **`DesugarInterfaceEmbeds`**: Flattens embedded interfaces by copying fields and destructors from the embedded interface into the embedding interface, substituting type parameters as needed. Note that methods are not copied here to avoid duplication, as they are handled by the default implementation extraction.
- **`DesugarInterfaceFields`**: Converts abstract field declarations within interfaces into concrete getter and setter method signatures. This ensures that subsequent passes see them as standard interface requirements.
- **`DesugarRelations`**: Processes `relation` declarations by injecting labeled fields into the involved classes and generating or merging `impl` blocks that wire the interface methods to these fields. It uses label prefixing to avoid name collisions when a class participates in multiple relations of the same type.
- **`DesugarDefaultImpls`**: Extracts methods with bodies from interfaces and moves them to the top-level as standalone generic functions, adding the necessary relational `where` clauses to maintain their context.
- **`DesugarDestructors`**: Automatically generates `destroy` methods for classes involved in `owns` relations, incorporating cleanup logic defined in the associated interfaces. It performs deep copies of destructor bodies and renames method calls to match label-prefixed fields.

### Standard Library Integration

The `stdlib.go` file provides the mechanism for bringing the standard library into the compilation unit:

- **`MergeStdlib`**: This function takes the user's AST and a parsed version of the standard library. It performs a transitive usage analysis to identify which stdlib interfaces, classes, and functions are actually needed by the user's code (either directly or via relations) and merges only those into the first `ForgeBlock` of the user's `File`.
- **`MergeFiles`**: Combines multiple `File` nodes into a single one, which is essential for multi-file compilation.
- **`FindStdlibDir`**: A utility to locate the standard library directory based on environment variables, executable location, or the current working directory.

### AST Validation

The `validate.go` file provides the `ValidatePostDesugar` function, which performs a suite of invariant checks:
- **Relation Field Injection**: Verifies that every `owns` relation has resulted in the expected parent field being injected into the child class.
- **Embed Flattening**: Ensures that no `embed` declarations remain in interfaces, confirming that `DesugarInterfaceEmbeds` has completed its work.
- **Consistency**: Checks that the tree is in a state where the type checker can proceed without encountering unexpected high-level sugar.

## Implementation Details

### Tagged Union Pattern

To maintain a flat and performant structure in Go, the AST utilizes a tagged union pattern for its most polymorphic nodes: `TypeExpr`, `Expr`, `Stmt`, and `Pattern`. Each of these structs contains a `Kind` enum and a `Data` field of type `any`. 

```go
type Expr struct {
    Kind         ExprKind
    Data         any
    Span         Span
    ResolvedType any
}
```

This approach avoids the overhead of interface method calls during traversal and allows consumers to use efficient type switches. The `Data` field is type-asserted based on the `Kind`, ensuring that the logic remains clear and localized.

### Source Mapping and Diagnostics

Every major node in the AST carries a `Span`, consisting of a start and end `Pos` (file, line, column). While this increases the memory footprint of the tree, it is a deliberate trade-off to enable high-fidelity error reporting. This metadata allows the checker and verifier to point to the exact location of a violation in the source code, and it provides the necessary foundation for future IDE features like "go to definition" and refactoring tools.

### Generative Desugaring Logic

The desugaring logic in `desugar.go` is designed to be generative and order-dependent. It must be executed before the checker because the checker expects a fully explicit representation of the program. 

1.  **Interface Embed Flattening**: Embedded interfaces are resolved first to ensure all fields and destructors are visible in the embedding interface.
2.  **Interface Field Expansion**: Fields in interfaces are turned into method signatures so that subsequent passes see them as standard interface requirements.
3.  **Relation Processing**: The relation pass uses the `rewriteFieldType` helper to map interface-level type parameters to the concrete types involved in a relation. It handles label prefixing to avoid name collisions when a class participates in multiple relations of the same type.
4.  **Default Implementation Extraction**: By moving default methods to the top level, the compiler simplifies the task of the checker, which can then treat these methods as regular generic functions with relational constraints.
5.  **Destructor Generation**: Finally, `destroy` methods are synthesized for classes, aggregating cleanup logic from all relevant relations. This pass uses `deepCopyBlock` and `substituteTypeParamsRichInBlock` to ensure that each class gets a unique, correctly typed version of the destructor logic.

### Selective Stdlib Merging

The `MergeStdlib` function implements a sophisticated reachability analysis. It doesn't just pull in every interface; it looks at `RelationDecl` hints to see which stdlib interfaces are being used to define relationships. It then transitively follows `embed` links. Similarly, it scans the user's code for named types and function calls, pulling in the corresponding stdlib definitions. This ensures that the resulting AST is as small as possible while still being semantically complete.

## Dependencies

The `ast` module is a leaf package in the Forge internal hierarchy. It does not depend on any other `pkg/*` modules, which is essential as almost every other module in the system depends on `ast`. 

To avoid circular imports with the `checker` package, the `ResolvedType` and `InferredTypeArgs` fields are defined as `any`. This allows the checker to store its complex type structures within the AST without the AST needing to know about the checker's internal definitions.

## Technical Debt and Future Work

- **Visitor Pattern**: As the number of node types grows, the manual type-switching in consumers like the checker and LIR lowerer is becoming unwieldy. Implementing a standard Visitor or Walker pattern would centralize traversal logic.
- **Embed Removal**: Currently, `DesugarInterfaceEmbeds` copies content but does not explicitly clear the `Embeds` slice, which causes `ValidatePostDesugar` to report violations unless the slice is cleared elsewhere.
- **Serialization**: There is currently no support for serializing the AST to JSON or a binary format. This limits the ability to cache compilation results or provide the AST to external tools written in other languages.
- **In-place Mutation**: Desugaring modifies the AST in-place, which loses the original "sugared" representation. For tools that require high-fidelity source-to-source transformations, a non-destructive or "side-table" approach to desugaring might be necessary.
