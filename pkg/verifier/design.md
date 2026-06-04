# Verifier Module Design

## Executive Summary

The `verifier` module is the structural integrity engine of the Grok system. Its primary purpose is to detect and report "architectural drift" — discrepancies between the high-level design declared in `.grok` files and the actual implementation in Go source code. By performing a deep, recursive comparison between the Grok AST and the Go source AST, it ensures that the developer's mental model (the design) remains synchronized with the reality of the codebase. It acts as a bridge between two distinct type systems, normalizing naming conventions and type expressions to provide a unified view of the system's architecture.

## File Inventory

- [verifier.go](verifier.go): The core implementation of the structural drift detector, containing the logic for Go type extraction, Grok-to-Go type mapping, and the recursive comparison engine.
- [verifier_test.go](verifier_test.go): Comprehensive test suite that verifies the detector's ability to catch various types of drift, including field mismatches, signature changes, and missing documentation.
- [verifier.grok](verifier.grok): The self-referential design specification for the verifier module, describing its own architecture, types, and invariants.

## Architecture and Data Flow

The verifier operates as a multi-stage analysis pipeline. While it is stateless between calls, it maintains significant internal state during a single verification run to aggregate information across multiple source files.

The data flow follows these primary stages:

1.  **Grok Parsing**: The pipeline begins by delegating to the [pkg/parser](../parser/design.md) module to produce a [pkg/ast](../ast/design.md) representation of the `.grok` file.
2.  **Go Source Discovery**: The verifier inspects the `source:` annotations within each `grok` block to locate the corresponding Go source files. These paths are resolved relative to the directory containing the `.grok` file.
3.  **Go Type Extraction**: Using the standard library's `go/parser` and `go/ast` packages, the verifier parses the identified Go files. It populates a `goTypeInfo` registry, which serves as a comprehensive database of all structs, interfaces, functions, and type definitions found across all specified source files. A critical design choice here is the aggregation of all source files before comparison, as a single Grok block's types may span multiple Go files within the same package.
4.  **Structural Comparison**: The verifier recursively walks the Grok AST, comparing each declaration (structs, classes, enums, interfaces, functions) against the aggregated Go types in the registry. During this walk, it normalizes naming conventions (e.g., snake_case to PascalCase) and converts Grok type expressions into a format comparable to Go's type strings.
5.  **Finding Accumulation**: Any discrepancies found during the comparison are recorded as `Finding` objects with associated severity levels (Error, Warning, Info) and collected into a `Result` object.
6.  **Completeness Check**: After verifying all symbols declared in the `.grok` file, the verifier performs a reverse check. It iterates through all exported Go symbols in the source files and reports an error for any that are not documented in the `.grok` block, ensuring the design file captures the full public API surface.

## Interface Implementations

The `verifier` module does not currently implement any external interfaces defined in other packages. It provides its own public API for use by the `grok-verify` CLI tool.

## Public API

The public API of the `verifier` module is intentionally minimal, centered around a single entry point:

- **`Verify(grokPath string) (*Result, error)`**: This function orchestrates the entire verification pipeline for a given `.grok` file. It returns a `Result` object containing all findings or an error if the verification process itself failed (e.g., file not found, syntax error in Grok or Go).

The **`Result`** type provides:
- **`Findings []Finding`**: A slice of all detected discrepancies.
- **`ErrorCount() int`**: A helper method to quickly determine the number of error-level findings, typically used to decide if a build or CI check should fail.

The **`Finding`** type contains:
- **`Severity`**: The seriousness of the drift (Error, Warning, or Info).
- **`GrokFile`**: The path to the `.grok` file where the discrepancy was found.
- **`GoFile`**: The path to the Go source file(s) involved in the mismatch.
- **`Message`**: A descriptive string explaining the nature of the drift.

## Implementation Details

### Deep Type Comparison

The heart of the verifier is the `grokTypeToGoString` function, which recursively converts Grok `TypeExpr` nodes into Go-compatible type strings. This mapping handles several key transformations:
- Optional types (`T?`) map to Go pointers (`*T`).
- Sequence types (`[T]`) map to Go slices (`[]T`).
- Map types (`map[K]V`) map directly to Go maps.
- Tuple types (`(T, U)`) are treated as multi-return values in function signatures.
- The Grok `any` type is treated as equivalent to Go's `interface{}` (and vice versa for Go 1.18+).
- The Grok `lock` type maps to `sync.Mutex`.
- Generic types like `Stack<T>` are mapped to Go's `Stack[T]` syntax.

The `typesMatch` function performs the final comparison, including a recursive `stripPackagePrefix` helper that allows `.grok` files to use unqualified type names (e.g., `File`) while the Go source might use qualified names (e.g., `ast.File`). If the converter encounters a type it cannot handle, it returns a "?" placeholder, and `typesMatch` gracefully treats this as a match to avoid false positives.

### Name Mapping and Normalization

To bridge the gap between Grok's preferred naming (often snake_case) and Go's conventions (PascalCase for exported symbols, camelCase for unexported), the verifier employs three-way normalization. When looking for a Go equivalent of a Grok name, it tries:
1.  **`snakeToPascal`**: `foo_bar` → `FooBar` (the primary mapping for exported fields and methods).
2.  **`snakeToCamel`**: `foo_bar` → `fooBar` (for unexported identifiers).
3.  **Exact Match**: In case the names already align.

### Verification Rules

The verifier applies specific rules based on the type of declaration:
- **Structs**: Checks for field existence and type compatibility. Extra Go fields trigger warnings, while missing Go fields are errors.
- **Classes**: Similar to structs, but also verifies method signatures (parameter count, positional type matching, and return value matching). Constructor parameters are also checked against the struct's fields.
- **Interfaces**: Ensures every method declared in the Grok interface exists in the Go interface with a matching signature.
- **Enums**: Validates that a matching Go type exists, supporting three common patterns: a simple typedef (e.g., `type Color int`), a struct, or an interface. It also checks for the `XxxKind` naming pattern.
- **Functions**: Performs positional parameter and return type comparison. Multi-return values in Go are mapped to Grok tuples and compared element-wise.

### Completeness and Export Rules

The `verifyCompleteness` function ensures that the `.grok` file is a comprehensive record of the public API. It iterates through all exported symbols in the Go source (identified by their leading uppercase letter) and verifies that each one has a corresponding entry in the Grok AST. This prevents "undocumented" exports from creeping into the implementation.

## Dependencies

- **[pkg/ast](../ast/design.md)**: Provides the AST structures for Grok files.
- **[pkg/parser](../parser/design.md)**: Used to parse `.grok` files into the Grok AST.
- **`go/ast`, `go/parser`, `go/token`**: Standard library packages used to extract type information from Go source code.
- **`os`, `path/filepath`**: Used for file system operations and path resolution.

## Technical Debt and Future Work

- **Expression Verification**: Currently, `requires` and `ensures` clauses are treated as raw strings and are not structurally verified against the implementation.
- **Complex Pointer Indirection**: While basic optional-to-pointer mapping works, deeply nested or complex pointer scenarios may occasionally lead to false positives.
- **Import Resolution**: The `stripPackagePrefix` mechanism is heuristic-based and does not perform full symbol resolution to verify cross-package identity.
- **Performance**: The verifier currently re-parses Go source files for every Grok block. Implementing a caching layer for parsed Go ASTs would significantly improve performance in large projects.
- **Function Types in Containers**: Certain complex types, such as `map[string]func(...)`, cannot currently be expressed precisely in Grok and are often typed as `any`, which can lead to known false-positive errors during verification.
- **Generic Constraints**: While basic generics are supported, complex constraints on generic types are not yet fully verified.
