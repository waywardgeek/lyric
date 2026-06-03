# Transpiler Module Design

## Executive Summary

The `transpiler` module is the forward-engineering engine of the Grok toolchain, responsible for converting a Grok Abstract Syntax Tree (AST) into idiomatic Go source code. While the `verifier` module ensures that existing Go code adheres to a Grok specification, the `transpiler` enables a model-driven workflow where developers can generate Go implementation skeletons or full procedural logic directly from Grok models. It bridges the semantic gap between Grok's high-level constructs—such as enums with variants, classes with constructors, and cascade statements—and Go's language primitives. The transpiler is designed to produce clean, readable, and compilable Go code that follows standard Go naming conventions, specifically ensuring that all generated types and fields are exported.

## File Inventory

- [transpiler.go](transpiler.go): The core implementation of the transpiler, containing the recursive logic for traversing the Grok AST and emitting Go source code into an internal buffer.
- [transpiler_test.go](transpiler_test.go): A comprehensive suite of unit tests that verify the correct translation of various Grok constructs, including structs, enums, classes, and complex control flow, into their Go equivalents.

## Architecture and Data Flow

The transpiler is architected as a stateful visitor that performs a single-pass traversal of the Grok AST. It is centered around the `Transpiler` struct, which manages an internal `strings.Builder` for code accumulation and an integer to track the current indentation level. The process begins when the `Transpile` method is called with an `ast.File`.

The data flow within the module is strictly linear and recursive:
1.  **Initialization**: A `Transpiler` instance is initialized with a target Go package name.
2.  **Package Declaration**: The transpiler emits the `package` header using the provided package name.
3.  **Top-Level Traversal**: The transpiler iterates through the blocks in the `ast.File`, delegating the processing of structs, enums, classes, and functions to specialized methods.
4.  **Recursive Emission**: Each declaration method recursively visits its constituent parts—such as fields, parameters, and bodies—emitting the corresponding Go syntax. Statements and expressions are handled by a deep recursive descent that mirrors the structure of the AST.
5.  **Indentation Management**: The transpiler manually manages indentation by incrementing and decrementing its internal counter when entering and exiting blocks, ensuring the generated code is properly formatted.
6.  **Output Generation**: Once the entire AST has been traversed, the accumulated content in the `strings.Builder` is returned as a single string.

## Interface Implementations

The `transpiler` module does not currently implement any external interfaces defined in other packages. It provides a concrete `Transpiler` type that serves as a standalone tool for AST-to-Go transformation.

## Public API

The public API of the `transpiler` module is designed for simplicity and ease of integration:

- **`New(pkg string) *Transpiler`**: This constructor creates a new transpiler instance. The `pkg` argument is critical as it defines the name of the Go package header for the generated file.
- **`Transpiler.Transpile(file *ast.File) string`**: This is the primary entry point for code generation. It resets the internal buffer, processes the provided AST file, and returns the resulting Go source code.

It is important to note that the `Transpiler` struct is not thread-safe for concurrent calls to `Transpile` on the same instance due to its shared internal buffer. However, the module is designed such that multiple `Transpiler` instances can be used concurrently across different goroutines.

## Implementation Details

The transpiler performs a sophisticated mapping between Grok's rich, expressive type system and Go's more minimalist primitives.

### Type Mapping and Primitives
Grok's primitive types are mapped to their closest Go equivalents. For instance, `i32` becomes `int32`, `f64` becomes `float64`, and `bool` remains `bool`. For types like `i128` or `f128` that lack direct Go equivalents, the transpiler currently uses `int64` or `float64` as placeholders, with the expectation that a more robust implementation might eventually use `math/big`.

Complex Grok types are handled through specific structural patterns:
- **Optional Types**: `Option[T]` is transpiled to a Go pointer `*T`.
- **Sequence Types**: `Seq[T]` is converted to a Go slice `[]T`.
- **Map Types**: `Map[K, V]` becomes a standard Go `map[K]V`.
- **Tuple Types**: These are transpiled as multiple return values in function signatures or as anonymous structs in other contexts.
- **Function and Channel Types**: These map directly to Go's `func` and `chan` types.
- **Lock Types**: These are mapped to `sync.Mutex`.

### Declarations and Structural Patterns
The transpiler ensures that all generated types and fields are exported by automatically capitalizing their names using the `exportName` helper.

- **Structs**: These are transpiled directly to Go structs. The transpiler also supports Go's generics by emitting type parameters and constraints (defaulting to `any` if no constraint is specified).
- **Enums**: Since Go does not have native sum types, the transpiler uses an interface-based pattern. It generates an interface with a private marker method (e.g., `isStatus()`) and then creates a struct for each variant that implements this interface.
- **Classes**: Grok classes are transpiled into a Go struct that combines constructor parameters and class fields. The transpiler generates a `New<ClassName>` function to serve as the constructor and converts class methods into Go methods with a pointer receiver. It intelligently handles `self` and `mut self` parameters to determine the receiver name.
- **Functions**: These are emitted as standard Go functions, with the transpiler handling the recursive generation of their parameter lists, return types, and statement bodies.

### Statements and Expressions
The transpiler supports a wide array of procedural logic. Variable declarations use `:=` for type inference when a value is provided without an explicit type, or `var` when a type is specified. Control flow constructs like `if`, `for` (range-based), and `while` (emitted as `for` in Go) are mapped to their Go equivalents.

A significant translation is the **Cascade Statement**, which is transpiled into a Go `defer` block. This preserves the semantic intent of ensuring a block of code executes at the end of the current scope. **Match Statements** are transpiled into Go `switch` statements, though the current implementation focuses on basic value switching rather than complex pattern destructuring.

## Dependencies

The `transpiler` module has a single internal dependency:

- **[pkg/ast](../ast/design.md)**: The transpiler is a heavy consumer of the `ast` module, relying on its data structures to represent the source model it is transforming.

The module also depends on the Go standard library, specifically `fmt` for formatted output and `strings` for buffer management and string manipulation.

## Technical Debt and Future Work

- **Import Management**: The transpiler does not currently track or emit necessary Go imports (e.g., `import "sync"` for `Lock` types). This requires a post-processing step like `goimports` or manual intervention.
- **Match Expression Support**: While `match` statements are supported, `match` used as an expression is not yet implemented and currently emits a placeholder comment.
- **Complex Pattern Matching**: The transpilation of patterns in `match` statements is currently limited to identifiers, literals, and basic variants; it does not yet support deep destructuring.
- **Large Number Support**: Robust handling for `i128`, `u128`, and `f128` using `math/big` is a planned improvement.
- **Error Handling**: The transpiler assumes it is operating on a valid, type-checked AST. It does not perform its own semantic validation or provide detailed error reporting for unsupported constructs.
