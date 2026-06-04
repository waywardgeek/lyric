# Checker Module Design

## Executive Summary

The `checker` module is the semantic heart of the Grok toolchain. It performs a comprehensive analysis pass over the Abstract Syntax Tree (AST) produced by the [parser](../parser/design.md), transforming a raw tree of symbols into a fully validated and type-annotated model. Sitting between the parser and the [transpiler](../transpiler/design.md), the checker ensures that a Grok program is not only syntactically correct but also semantically sound.

The module implements a sophisticated type system that supports:
- **Type Inference**: Automatically determining variable types from initializers and function call sites.
- **Structural Subtyping**: Validating that classes and structs satisfy interface requirements based on their method signatures, even without explicit `implements` declarations.
- **Generics**: Handling polymorphic functions and classes with type parameters, constraints, and call-site type inference.
- **Advanced Language Features**: Validating union types, optional types, tuple destructuring, and the Go-style `?` (try) operator.
- **Exhaustiveness Analysis**: Ensuring that `match` statements on enums and unions cover all possible cases.

By annotating the AST with resolved types, the checker provides the necessary metadata for the transpiler to generate correct, type-safe Go code, including explicit casts and standard library method mappings.

## File Inventory

- [checker.go](checker.go): The primary implementation of the type checker. It defines the internal type representation (`Type`), the lexical environment (`Scope`), the type registry (`Registry`), and the core traversal logic for expressions and statements.
- [checker_test.go](checker_test.go): A comprehensive test suite covering type inference, interface satisfaction, generics, built-in methods, and error reporting across a wide variety of Grok language constructs.
- [checker.grok](checker.grok): A self-documenting Grok specification of the checker's own architecture and data structures, serving as both documentation and a formal model for verification.

## Architecture and Data Flow

The checker operates in a multi-phase process to handle forward references and complex dependencies between types:

1. **Registration Phase**: The checker first walks all top-level declarations in each `GrokBlock`. It populates a global `Registry` with information about interfaces, structs, classes, enums, and functions.
   - **Interfaces** are registered first to allow other types to reference them.
   - **Classes and Structs** are registered with their fields and methods. Class constructor parameters are automatically treated as fields.
   - **Enums** are registered with their variants. Unit variants are defined as values, while variants with fields are defined as constructor functions.
   - **Type Aliases** are resolved and stored in the registry.

2. **Validation Phase**: Once all types are registered, the checker performs structural validation:
   - **Interface Satisfaction**: It verifies that every class declaring an `implements` clause actually provides the required methods with matching signatures.
   - **Interface Composition**: It ensures that interfaces correctly inherit and compose methods from their parent interfaces.

3. **Checking Phase**: The checker then performs a deep traversal of function and method bodies.
   - **Lexical Scoping**: It manages a stack of `Scope` objects to handle variable visibility and shadowing.
   - **Expression Inference**: It recursively determines the type of every expression, annotating the AST nodes with the `ResolvedType`.
   - **Statement Validation**: It ensures that assignments, returns, and control flow conditions are type-consistent.
   - **Generics Handling**: It performs call-site type argument substitution and inference, ensuring that generic constraints are satisfied.

4. **Error Accumulation**: Rather than failing on the first error, the checker accumulates `CheckError` objects, allowing it to report multiple issues in a single pass.

## Interface Implementations

The `checker` module does not implement external interfaces but acts as the primary consumer of the [pkg/ast](../ast/design.md) structures. It populates the `ResolvedType` fields in `ast.Expr` and `ast.Pattern` nodes, which are defined as `any` to avoid circular dependencies. The `transpiler` then consumes these annotations.

The `CheckError` type implements the standard Go `error` interface, providing formatted error messages with source file and position information.

## Public API

The `checker` module exposes a clean interface for performing semantic analysis:

- **`New()`**: Initializes a fresh `Checker` instance with a pre-populated registry of built-in types (like `error`) and functions (like `println`, `len`, `append`, `make_channel`).
- **`Checker.CheckFile(file *ast.File)`**: The main entry point for analyzing a complete AST. It orchestrates the registration and checking phases across all blocks in the file.
- **`Checker.Errors() []error`**: Returns the list of all semantic errors encountered during the analysis.
- **`Type`**: A public struct representing a resolved Grok type. It includes fields for the type kind, bit width (for numerics), element types (for collections), and signature information (for functions).
- **`Registry`**: A container for all named types defined in the program, allowing for cross-block and cross-file type lookups.

## Implementation Details

### Type Representation
The internal type system uses a recursive `Type` structure. The `Kind` field (of type `TypeKind`) discriminates between primitive types (integers, floats, bool, string), composite types (lists, maps, tuples, optionals, channels), and named types (structs, classes, enums, interfaces). For numeric types, the `Bits` field specifies the precision (e.g., 32, 64, or -1 for platform-dependent `int`/`uint`).

### Type Equality and Assignability
The checker distinguishes between strict equality and assignability:
- **`Equal()`**: Performs structural equality checking. Two types are equal if they have the same kind and identical internal structures (e.g., the same element types for lists).
- **`assignableTo()`**: Implements the language's subtyping rules. It allows for:
    - **Numeric Widening**: Smaller integer/float types can be assigned to larger ones of the same sign.
    - **Optional Wrapping**: A type `T` is always assignable to `T?`.
    - **Interface Subtyping**: A struct or class is assignable to an interface if it implements all required methods (structural subtyping).
    - **Union Types**: A member type is assignable to its containing union, and a union is assignable to a target if all its variants are.
    - **Nil Assignability**: The `null` (or `nil`) value is assignable to any optional or interface type.

### Generics and Type Inference
Grok supports powerful generics with call-site type inference. When a generic function is called:
1. If explicit type arguments are provided (e.g., `f<i32>(...)`), the checker validates the count and constraints.
2. If type arguments are omitted, `inferTypeArgs` performs a parallel walk of the formal parameter types and actual argument types to bind type variables to concrete types.
3. A substitution map is created, and `substituteType` recursively replaces type variables in the function's signature to produce a concrete instance for validation.
4. Constraints like `Comparable` or `Equatable` are checked against the resolved types via `satisfiesConstraint`.

### Exhaustiveness Checking
For `match` statements on enums and unions, the checker performs an exhaustiveness check. It tracks which variants or types have been covered by the match arms. If a match is not exhaustive and lacks a wildcard (`_`) arm, the checker reports an error. This ensures safety when working with sum types.

### Error Handling and the Try Operator
The checker specifically recognizes the Go-style `(T, error)` return pattern. The `?` (try) operator is validated to ensure it is used only within functions that return an error type, and that the operand is a tuple whose last element is an error. It then "unwraps" the tuple to its non-error components.

## Dependencies

The `checker` module depends on:
- [pkg/ast](../ast/design.md): For the AST structures it analyzes and annotates.

It is a dependency for:
- [pkg/transpiler](../transpiler/design.md): Which relies on the `ResolvedType` annotations to generate Go code.
- [pkg/verifier](../verifier/design.md): Which may use the checker to validate the consistency of declarations.

## Technical Debt and Future Work

- **Constraint Enforcement**: Some complex generic constraints are currently deferred to the Go compiler. Bringing more of this logic into the checker would provide earlier feedback.
- **Error Recovery**: Improving the checker's ability to recover from errors would allow it to provide more comprehensive reports for highly broken code.
- **Circular Dependencies**: While the checker handles forward references within a file, complex circular dependencies between types across multiple files may require further refinement of the registration phases.
- **Performance**: For extremely large projects, the recursive nature of type substitution and structural matching could be optimized with memoization.
