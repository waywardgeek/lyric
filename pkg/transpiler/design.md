# Transpiler Module Design

## Executive Summary

The `transpiler` module serves as the final stage of the Grok compilation pipeline, responsible for transforming a type-checked Abstract Syntax Tree (AST) into idiomatic, executable Go source code. It bridges the gap between Grok's high-level architectural constructs—such as classes, enums with pattern matching, and the `?` error operator—and Go's structural and procedural primitives. By leveraging the semantic annotations provided by the `checker` module, the transpiler ensures that the generated Go code is not only syntactically correct but also semantically equivalent to the original Grok source, including proper type casting, visibility mapping, and automatic import management.

## File Inventory

- [transpiler.go](transpiler.go): The core implementation of the Go code generator. It contains the `Transpiler` struct, which maintains the state of the code generation process, and a comprehensive set of visitor methods that traverse the AST and emit Go code.
- [transpiler_test.go](transpiler_test.go): A robust test suite that verifies the transpilation of various Grok features, including structs, enums, classes, functions, control flow, concurrency primitives, and the `?` operator. It uses a helper to parse and type-check Grok source before transpilation to ensure end-to-end correctness.
- [transpiler.grok](transpiler.grok): The architectural specification for the transpiler module itself, written in Grok. It defines the module's purpose, internal data structures, and the mapping logic between Grok and Go constructs.

## Architecture and Data Flow

The transpiler is architected as a stateful, single-pass visitor over the Grok AST, but it employs a two-pass assembly strategy to handle the dynamic nature of Go imports. The process is orchestrated by the `Transpiler` struct, which tracks indentation levels, visibility rules, and auto-detected dependencies.

The data flow begins when the `Transpile` method is called with an `ast.File`. The transpilation proceeds in several distinct phases:

1.  **Metadata Registration**: Before emitting any code, the transpiler performs a pre-scan of all blocks in the file. It populates internal maps with metadata about classes, top-level functions, enums, and their visibility. This phase is crucial for resolving forward references and ensuring that constructor calls and method receivers are emitted correctly.
2.  **Body Generation**: The transpiler then walks the AST nodes, emitting the Go representation of each declaration and statement into a temporary buffer. During this phase, it identifies the need for standard library imports (e.g., `fmt` for string interpolation or `sync` for mutexes) and records them in an `autoImports` set.
3.  **Final Assembly**: Once the entire body is generated, the transpiler constructs the final Go source string. It writes the package declaration, followed by a consolidated list of imports—merging user-declared imports with those auto-detected during the body generation phase—and finally appends the contents of the temporary body buffer.

This architecture allows the transpiler to remain simple and linear while still supporting complex features like automatic import discovery and visibility-aware naming.

## Interface Implementations

The `transpiler` module does not explicitly implement any external interfaces defined in other packages. It is designed as a standalone service that consumes the `ast.File` data structure. Its primary contract is with the `grok-compile` CLI, which orchestrates the compilation pipeline.

## Public API

The public API of the transpiler module is intentionally minimal, providing a clean entry point for the compilation toolchain:

- `New(pkg string) *Transpiler`: This function initializes a new transpiler instance. It takes the target Go package name as an argument and sets up the internal state, including maps for tracking classes, functions, and imports.
- `Transpile(file *ast.File) string`: This is the main execution method. It takes a complete Grok AST file—ideally one that has been annotated by the `checker`—and returns the corresponding Go source code as a string.

The `Transpiler` struct itself is stateful and intended for single-threaded use per file. It resets its internal buffers and import tracking at the beginning of each `Transpile` call, ensuring that subsequent calls on the same instance do not leak state.

## Implementation Details

### Visibility and Naming Conventions
Grok uses the `pub` keyword to denote public visibility, while Go relies on the capitalization of the first letter of an identifier. The transpiler automatically manages this mapping using helper functions like `visName`, `exportName`, and `unexportName`. Symbols marked as `pub` in Grok are capitalized in the generated Go code, while others are lowercased. This logic is applied consistently across functions, structs, classes, enums, and their respective fields and methods.

### Type Mapping and Resolution
The transpiler maps Grok's primitive and composite types to their Go equivalents. Primitives like `i32` and `f64` map directly to `int32` and `float64`. Composite types are transformed as follows:
- **Optional Types (`T?`)**: Mapped to Go pointers (`*T`).
- **Sequence Types (`[T]`)**: Mapped to Go slices (`[]T`).
- **Map Types (`map[K]V`)**: Mapped to Go maps (`map[K]V`).
- **Tuple Types**: Mapped to multiple return values in function signatures or anonymous structs in other contexts.
- **Lock Types**: Mapped to `sync.Mutex`.

The transpiler relies heavily on the `ResolvedType` annotations provided by the `checker`. This allows it to insert explicit Go type casts when numeric types are mixed or when assigning values to union types (mapped to `any`), ensuring that the generated code satisfies Go's strict type system.

### Classes and Constructors
Grok classes are transpiled into Go structs. If a class defines constructor parameters, the transpiler generates a constructor function (e.g., `NewClassName`) that initializes the struct and returns a pointer to it. Methods are emitted as Go functions with pointer receivers. The transpiler tracks the receiver name (defaulting to `r`) and replaces Grok's `self` keyword with this name in the method body.

### Enums and Pattern Matching
Grok enums are implemented using a "tagged union" pattern in Go. The transpiler generates an interface with a private marker method and a set of structs—one for each variant—that implement this interface.
- **Pattern Matching**: Transpiled into Go type switches. The transpiler is sophisticated enough to handle nested pattern matching (e.g., `Some(Circle(r))`) by emitting nested type switches and managing temporary variables for the matched values.
- **Binding Extraction**: For variants with data, the transpiler emits code to extract fields into local variables within the `case` block of the type switch.

### Error Handling with the `?` Operator
The Grok `?` operator is expanded into idiomatic Go error checks. When the transpiler encounters a `?` operator in a variable declaration or expression statement, it generates code that calls the fallible function, checks the returned error, and performs an early return if the error is non-nil. It uses the `currentReturnAST` to determine the correct zero values for the function's return type, ensuring that the generated `return` statement is valid.

### Built-in Methods and Functions
The transpiler intercepts calls to built-in methods on strings, lists, and maps, mapping them to Go standard library functions or efficient IIFE (Immediately Invoked Function Expression) patterns. For example, `string.to_upper()` becomes `strings.ToUpper(s)`, and `list.push(x)` becomes an assignment using `append`. It also handles built-in functions like `println`, `len`, and `isnull`.

### Concurrency and Synchronization
Grok's concurrency primitives are mapped to Go's native features:
- `spawn` blocks become anonymous goroutines (`go func() { ... }()`).
- `cascade` blocks become deferred function calls (`defer func() { ... }()`).
- `lock(mu)` blocks are transpiled into a call to `mu.Lock()` followed by a `defer mu.Unlock()`, wrapped in a block to ensure the lock is released at the end of the Grok block.
- `select` statements map directly to Go `select`, with special handling for channel send and receive operations.

## Dependencies

- `[pkg/ast](../ast/design.md)`: The transpiler consumes the AST nodes defined in this package.
- `[pkg/checker](../checker/design.md)`: The transpiler depends on the type resolution and semantic analysis performed by the checker to generate correct Go code, especially for type casts and union types.

## Technical Debt and Future Work

- **Large Integer Support**: Currently, 128-bit and 256-bit integers are downcast to 64-bit types. Future versions should integrate `math/big` for full precision.
- **Tuple Optimization**: While basic tuple support exists, more complex usage could benefit from the generation of dedicated, named struct types to improve readability and performance.
- **Nested `?` Operators**: The current implementation only supports the `?` operator at the statement level. Extending this to arbitrary expression contexts would increase Grok's expressiveness.
- **For Loop Indexing**: The transpiler currently discards the index variable in `for` loops unless explicitly requested. Improving the default behavior or providing more control over loop transpilation is a potential area for improvement.
