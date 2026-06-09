# Checker Module Design

The `checker` module implements the semantic analysis phase of the Forge compiler. It is responsible for type checking, type inference, name resolution, and enforcing language-specific constraints on the desugared Abstract Syntax Tree (AST) produced by the [ast](../ast/design.md) module.

## Executive Summary

The `checker` module ensures that a Forge program is semantically valid before it is passed to the backend for code generation. It performs a comprehensive, multi-pass analysis of the AST, resolving type expressions into concrete `Type` objects, tracking variable scopes, and verifying that all expressions and statements adhere to Forge's type system rules.

Key features include structural subtyping for interfaces, numeric widening, automatic optional wrapping, and support for generic type inference using a unification-based approach. The checker also handles module imports, ensuring that symbols are correctly resolved across file boundaries while detecting circular dependencies. It enforces concurrency safety through `guarded_by` annotations and validates the exhaustiveness of `match` statements for enums and unions.

The checker acts as the "semantic truth" of the compiler, annotating AST nodes with `ResolvedType` information. This annotation is a critical contract consumed by subsequent phases, such as the LIR lowerer and backends, which rely on it for correct code generation without needing to re-perform semantic analysis.

## File Inventory

- [checker.go](checker.go): The primary implementation of the type checker. It defines the core `Type` system, `Scope` management, `Registry` for user-defined types, and the `Checker` struct which contains the logic for traversing and validating the AST. It also includes the pre-registration of Go standard library modules for interop.
- [checker_test.go](checker_test.go): A comprehensive suite of tests that verify the checker's behavior across a wide range of Forge language features, including basic arithmetic, control flow, function calls, struct/class definitions, generic type inference, concurrency safety, and module imports.
- [checker.forge](checker.forge): A Forge-language specification of the checker's own architecture and logic, serving as both documentation and a formal model of the module's behavior.

## Architecture and Data Flow

The `checker` module operates on the AST after it has been parsed and desugared. It follows a multi-pass approach to handle forward references and cross-module dependencies, ensuring that all names are resolved before their usage is validated.

The process begins with the initialization of a `Checker` instance, which sets up a global scope populated with built-in functions and types. Built-in functions include console I/O primitives like `println`, collection utilities like `len` and `append`, and OS-level operations like `read_file` and `os_exit`. The `error` type is registered as a built-in interface, which is satisfied by any class providing a `message(self) -> string` method.

The core checking logic is divided into several distinct phases:

### Phase 0: Pre-registration
In this initial pass, the checker traverses all files and blocks to register the names of all user-defined types (structs, classes, enums, and interfaces). This ensures that cross-block and cross-file type references in function and method signatures can be resolved during the subsequent registration phase.

### Phase 1: Registration
The checker populates the `Registry` and the global `Scope` with the full signatures of all declarations.
- **Interfaces** are registered with their method signatures and parent interfaces.
- **Structs and Classes** register their fields and methods. For generic classes, type parameters are temporarily registered as type variables (`TyVar`) to allow field and method types to reference them. The checker also identifies explicit constructors or generates default ones.
- **Enums** register their variants as either values (for unit variants) or constructor functions.
- **Imports** are handled recursively. When an import is encountered, the checker invokes `CheckModuleFile`, which parses and checks the imported module, caching the results and detecting circular imports. Exported symbols are registered in the importer's scope under the module's alias.
- **Type Aliases and Constants** are resolved and added to the scope.

After registration, the checker performs an explicit `checkImplements` pass for each class, verifying that it provides all methods required by its declared interfaces with matching signatures.

### Phase 2: Body Checking
Once the global environment is fully populated, the checker performs a deep traversal of function and method bodies. This phase involves recursive type inference and validation of every statement and expression. The checker maintains a stack of `Scope` objects for lexical scoping and tracks the expected return type of the current function. For every expression, the checker infers its type and stores it in the `ResolvedType` field of the AST node.

### Phase 3: Type Resolution Validation
After the primary checking passes, the checker verifies that all function signatures are fully resolved and no `TyUnknown` types remain. `TyUnknown` must not survive past the checker.

### Phase 4: Expression Resolution Validation
The checker walks every expression in the AST and ensures it has a `ResolvedType` annotation. This catches any gaps in the checker's logic systematically.

### Phase 5: Access Validation
The checker validates that all field accesses and method calls resolve to real members on the receiver type, catching any bugs that might have bypassed normal checking (e.g., in desugared code).

## Interface Implementations

The `checker` module does not explicitly implement external Go interfaces, but it provides the core logic that satisfies the semantic requirements of the Forge language. It consumes the AST nodes defined in [pkg/ast](../ast/design.md) and uses the parser from [pkg/parser](../parser/design.md) to handle imports.

## Public API

### Core Types

- **`Type`**: Represents a resolved type in the Forge system. It uses a `Kind` (e.g., `TyInt`, `TyFunc`, `TyStruct`, `TyUnion`) and additional metadata (e.g., bit width, element types, field lists, type parameters) to describe the type. It provides methods for structural equality (`Equal`) and string representation.
- **`Checker`**: The main entry point for semantic analysis. It maintains the global state, including the type registry, error list, and module cache.
- **`Registry`**: A collection of `TypeInfo` objects representing all named types known to the checker.
- **`Scope`**: A hierarchical mapping of identifiers to their resolved types and mutability status.
- **`ModuleExports`**: A structure that holds the public types and functions exported by a module, used for cross-module symbol resolution.

### Primary Methods

- **`New()`**: Constructs a new `Checker` with initialized builtins and Go standard library interop modules.
- **`CheckFile(file *ast.File)`**: Performs the multi-pass type checking on a single AST file.
- **`CheckFiles(files []*ast.File)`**: Performs type checking on multiple files, ensuring cross-file references are resolved correctly.
- **`CheckModuleFile(importPath string, fromSpan ast.Span)`**: Resolves, parses, and checks a module at the given path, returning its exported symbols.
- **`Errors()`**: Returns the list of all errors encountered during the checking process.

## Implementation Details

### Type Representation and Equality

The `Type` struct is the heart of the module. It uses a recursive structure to represent complex types like lists, maps, functions, and unions. Structural equality is implemented in the `Equal` method, which ensures that two types are identical in their composition, regardless of their memory address.

### Subtyping and Assignability

Forge supports several forms of implicit type compatibility implemented in the `assignableTo` method. Numeric widening allows smaller integer types to be implicitly converted to larger ones (e.g., `i32` to `i64`). Optional wrapping ensures that a type `T` is always assignable to `T?`. Structural subtyping allows a class or struct to satisfy an interface if it provides all the required methods with matching signatures. Union types allow any member type to be assignable to the union, while the union itself is only assignable to a target if all its variants are compatible.

### Generic Type Inference

The checker implements a unification-based inference for generic functions. When a generic function is called without explicit type arguments, the `inferTypeArgs` method walks the parameter types and actual argument types in parallel. It binds type variables to concrete types from the arguments, with the first match winning. The inferred types are then substituted into the function's return type and stored in the AST for the transpiler.

### Relational Constraints

Forge supports relational constraints through `where` clauses, such as `where Graph<G, N, E>`. When such a constraint is encountered, the checker looks up the corresponding interface and "grants" its methods to the involved type variables. This is implemented by populating a `typeVarMethods` map, which allows methods defined in the interface to be called on variables of the constrained types within the function body.

### Concurrency and Guarded Fields

The checker enforces safety rules for concurrent access by tracking "held locks" within a scope. These locks are populated by `lock` statements. The checker verifies that any field annotated with `guarded_by` is only accessed when the corresponding lock is held, preventing unprotected access to shared state. The `extractLockName` helper identifies the lock being used, supporting both simple identifiers and field accesses (e.g., `self.mu`).

### Exhaustiveness Checking

For `match` statements operating on `enum` or `union` types, the checker verifies that all possible variants or member types are covered by the match arms. If a wildcard pattern is not present, the checker reports an error listing the missing variants or types, ensuring that all cases are handled at compile time.

### Built-in Methods and Functions

The checker provides specialized resolution for methods on built-in types. Strings support operations like `len`, `contains`, and `split`. Lists provide `push`, `pop`, and `reverse`. Maps offer `contains_key`, `keys`, and `values`. Channels support `send`, `receive`, and `close`. These are resolved during the method call checking phase before falling back to general method lookup.

### Module System and Imports

The checker handles module imports by recursively parsing and checking imported files. It uses a `modules` cache to avoid redundant work and a `checking` map to detect circular dependencies. Exported symbols (public types and functions) are extracted into a `ModuleExports` object and registered in the importer's scope.

### Post-checking Validation

After all checking passes, the checker performs several validation steps:
- **`validateAllTypesResolved`**: Ensures no `TyUnknown` remains in function signatures.
- **`validateAllExprsResolved`**: Ensures every expression has a `ResolvedType`.
- **`validateFieldAndMethodAccess`**: Performs a final check that all field and method accesses are valid on their receiver types.

## Dependencies

- **[pkg/ast](../ast/design.md)**: The checker consumes the AST and stores results back into it.
- **[pkg/parser](../parser/design.md)**: Used to parse imported module files.

## Technical Debt and Future Work

- **Advanced Inference**: The current type inference is relatively simple and may need to be replaced with a more robust constraint-solver-based approach for complex scenarios involving nested generics or circular constraints.
- **Method Resolution**: Method lookup currently relies on a simple name-based search. Support for method overloading or more complex trait-based dispatch might be needed in the future.
- **Performance**: For very large projects, the recursive module checking and deep AST traversal could become a bottleneck. Implementing incremental checking or a more parallelized approach could improve performance.
- **Circular Type Definitions**: While module cycles are detected, circular type definitions need careful handling to avoid infinite recursion during size calculation or equality checks.
