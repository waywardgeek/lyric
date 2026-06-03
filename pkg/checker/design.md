# Checker Module Design

## Executive Summary

The `checker` module provides the semantic analysis engine for the Grok language, specifically focusing on type checking and type inference. While the [parser](../parser/design.md) ensures that a Grok file is syntactically correct, the `checker` ensures that it is semantically sound. It verifies that variables are used consistently with their declared types, that function calls provide the correct arguments, and that complex expressions resolve to valid types. The module implements a robust type system that mirrors Grok's expressive grammar, including support for primitives, sequences, maps, optionals, functions, and user-defined structures like structs and classes. By performing deep recursive analysis of the [AST](../ast/design.md), the checker acts as the final gatekeeper before architectural verification or code generation.

## File Inventory

- [checker.go](checker.go): The core implementation of the type system and the checking engine. It defines the internal `Type` representation, the `Scope` and `Registry` for state management, and the `Checker` type which implements the traversal and validation logic.
- [checker_test.go](checker_test.go): A comprehensive suite of unit tests that verify type inference, error detection, and scope isolation across a wide variety of Grok code snippets.

## Architecture and Data Flow

The `checker` module operates by traversing the Abstract Syntax Tree (AST) produced by the parser. It maintains a stateful environment that tracks the types of all declared entities and the current lexical scope.

The data flow within the checker follows a two-pass strategy to handle forward references and recursive definitions. In the first pass, the checker iterates through all top-level blocks in the `ast.File`. It registers all named types—structs, classes, and enums—into a global `Registry`. Simultaneously, it registers function signatures into the root `Scope`. This ensures that any function or type can refer to any other, regardless of their order in the source file.

In the second pass, the checker performs a deep recursive traversal of function bodies and procedural logic. It manages a stack of `Scope` objects to handle lexical scoping. As it enters a block, a new scope is pushed; as it exits, the scope is popped. This ensures that variables are only accessible within their defined boundaries and that shadowing is handled correctly. During this traversal, the checker evaluates expressions to infer or verify their types and validates that statements comply with the language's semantic rules.

Errors encountered during either pass are not immediately fatal. Instead, the checker accumulates `CheckError` objects, each capturing a descriptive message and the `ast.Span` of the offending code. This allows the toolchain to report all semantic issues in a single run, improving the developer experience.

## Interface Implementations

The `checker` module does not currently implement any external interfaces. It is a standalone engine that consumes the `ast.File` contract and produces a collection of errors. It relies on the `ast.Span` for error reporting and the `ast.TypeExpr` for type resolution.

## Public API

The module provides a clean, object-oriented API for performing type analysis.

- **New() *Checker**: Instantiates a new checker with an empty type registry and a root scope. The checker is the primary state holder for the analysis.
- **Checker.CheckFile(file *ast.File)**: The primary entry point for analyzing a complete Grok source file. It performs the two-pass registration and validation logic described above.
- **Checker.Errors() []error**: Returns the list of all type errors encountered during the analysis. If the list is empty, the file is semantically valid.
- **Type**: An exported structure representing a resolved Grok type. It includes methods like `Equal()` for structural equality checks, `IsNumeric()` for numeric validation, and `String()` for human-readable representation.
- **Registry**: Manages the definitions of user-defined types (structs, classes, enums). It maps type names to `TypeInfo` objects containing field and method information.
- **Scope**: Manages variable-to-type mappings within a specific lexical context, supporting parent-link traversal for name resolution.

## Implementation Details

### Internal Type System

The checker uses a dedicated `Type` struct to represent Grok's type system internally. This is distinct from the `ast.TypeExpr` used by the parser. While `ast.TypeExpr` represents the *syntax* of a type in the source code, `checker.Type` represents the *resolved semantic identity* of that type.

The `Type` struct uses a `TypeKind` enumeration to discriminate between different categories of types, such as `TyInt`, `TyString`, `TyFunc`, and `TyStruct`. For complex types, it uses recursive pointers: `Elem` for lists and optionals, `Key` and `Val` for maps, and `Params` and `Return` for functions.

Structural equality is implemented in the `Equal()` method. This method performs a deep comparison of types. For example, two function types are equal if they have the same number of parameters, each corresponding parameter type is equal, and their return types are equal. This allows the checker to handle complex nested types without relying on name-based identity for anonymous structures like lists or maps.

### Type Inference

One of the module's most powerful features is its type inference engine. When a variable is declared using `let x = <expr>`, the checker evaluates the expression to determine its type and then binds that type to the variable in the current scope.

Type inference is particularly useful in loops. In a `for item in items` loop, the checker determines the type of `items`. If it is a list `[T]`, the loop variable `item` is automatically bound to type `T` within the loop's body. If `items` is a map `map[K]V`, `item` is bound to the key type `K`. This reduces the need for explicit type annotations while maintaining strict type safety.

### Expression Checking

The `checkExpr` method is the heart of the inference engine. It recursively evaluates AST expression nodes and returns their resolved `Type`.

- **Literals**: Basic literals like integers, floats, strings, and booleans are mapped to their corresponding primitive types (e.g., `i32` for integer literals by default).
- **Binary and Unary Operations**: The checker validates that operators are applied to compatible types. For example, it ensures that the `+` operator is used either between two numeric types of the same width or between two strings for concatenation. It also ensures that logical operators like `&&` and `||` are only applied to booleans.
- **Function and Method Calls**: For function calls, the checker verifies that the number and types of arguments match the function's signature. For method calls, it performs a lookup in the `Registry` to find the method definition on the receiver's type (struct or class).
- **Field Access and Indexing**: Field access (`p.x`) is validated against the fields registered for the receiver's type. Indexing (`xs[0]`) is checked to ensure the receiver is an indexable type (list, map, or string) and that the index expression is of the correct type (integer for lists/strings, key type for maps).

### Statement Checking

The `checkStmt` method handles the semantic validation of control flow and state changes.

- **Variable Declarations**: Handles both explicit annotations (`let x: i32 = 1`) and implicit inference (`let x = 1`). It ensures that if both are present, they are compatible.
- **Assignments**: Verifies that the value being assigned is compatible with the target's type.
- **Control Flow**: Ensures that conditions in `if` and `while` statements resolve to boolean values. It also manages the scope for block statements, ensuring that variables declared within a block do not leak into the outer scope.

## Dependencies

- [pkg/ast](../ast/design.md): The checker is a primary consumer of the AST and relies on its structures for traversal and source location information.
- [pkg/parser](../parser/design.md): While not a direct dependency in the source code, the checker is designed to process the output of the parser, as seen in the integration tests in `checker_test.go`.

## Technical Debt and Future Work

- **Generics**: The current implementation has basic support for type variables (`TyVar`) but does not yet implement full generic instantiation, unification, or constraint checking.
- **Pattern Matching**: While the AST supports `match` statements, the checker's implementation of pattern variable binding and exhaustiveness checking is currently a placeholder.
- **Interface Satisfaction**: The checker needs to be extended to verify that classes and structs correctly implement the interfaces they claim to satisfy.
- **Enclosed Return Checking**: The `checkReturn` method currently validates the return expression but does not yet verify it against the enclosing function's declared return type. This requires tracking the "current function" in the checker's state.
- **Loop Context**: `break` and `continue` statements are accepted anywhere; the checker should be updated to track loop depth and ensure these only appear within valid loop contexts.
