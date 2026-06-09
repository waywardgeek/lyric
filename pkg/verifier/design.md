# Verifier Module Design

The `verifier` module is the architectural guardian of the Forge project. It serves as a structural integrity engine that detects and reports "architectural drift"—the divergence between the high-level design documented in `.forge` files and the actual implementation in Go source code. By performing a deep, recursive comparison between these two distinct type systems, the verifier ensures that the developer's mental model remains synchronized with the evolving codebase, transforming documentation from a passive artifact into an active, verifiable contract.

## Executive Summary

The `verifier` module provides a multi-stage analysis pipeline that validates Go source code against Forge specifications. It parses `.forge` files, discovers referenced Go source files through `source:` annotations, extracts their type information using the Go standard library's AST tools, and performs a structural comparison. The verifier identifies mismatches in fields, methods, function signatures, and naming conventions, reporting them with varying severity levels (Error, Warning, Info). 

A critical feature of the module is its "completeness check," which ensures that all exported Go symbols are documented in the corresponding Forge block. This prevents the specification from becoming an incomplete subset of the implementation. Beyond simple drift detection, the module also serves as a host for project-wide invariant testing, using its AST analysis capabilities to enforce critical architectural rules that cannot be captured by the Go type system alone, such as the value-type nature of AST nodes and the specific order of desugaring passes.

## File Inventory

- [verifier.go](verifier.go): The primary implementation file containing the verification pipeline, Go type extraction logic, and structural comparison algorithms. It defines the `Verify` entry point and the core reporting types.
- [invariants_test.go](invariants_test.go): A specialized test suite that enforces cross-cutting project invariants by analyzing the Go AST of other modules. It ensures that critical architectural rules are maintained across the codebase.
- [verifier_test.go](verifier_test.go): Comprehensive test suite that validates drift detection, naming convention mapping, and completeness checks against both synthetic examples and the project's own specifications.
- [verifier.forge](verifier.forge): The module's own architectural specification, providing a self-documented model of its internal logic and public API.

## Architecture and Data Flow

The verification process is designed as a stateless operation from the perspective of the caller, although it maintains significant internal state during a single run to aggregate information across multiple files. The workflow follows a linear pipeline:

1.  **Forge Parsing**: The verifier delegates to the [pkg/parser](../parser/design.md) module to produce an `ast.File` from the provided `.forge` source path.
2.  **Go Source Discovery**: It iterates through each `forge` block within the file, inspecting the `source:` annotations to locate the relevant Go files or directories. These paths are resolved relative to the directory containing the `.forge` file itself.
3.  **Go Type Extraction**: For each discovered source, the verifier utilizes the `go/parser` and `go/ast` packages from the Go standard library to extract detailed type information. This data is populated into a `goTypeInfo` registry, which serves as a comprehensive database of all structs, interfaces, functions, and type definitions found across all specified sources for a given block.
4.  **Structural Comparison**: Once the Go type registry is fully populated, the verifier recursively walks the Forge AST, comparing each declaration—including structs, classes, interfaces, enums, and functions—against the aggregated Go types. It normalizes naming conventions and type expressions on the fly to enable direct comparison.
5.  **Reporting**: Findings are collected into a `Result` object, which classifies each discrepancy by severity and provides detailed messages and file locations.

A key design choice is the aggregation of all source files before comparison. Because a single Forge block's implementation may span multiple Go files within the same package, the verifier must merge information from all referenced files to form a complete picture of the Go implementation before validating it against the Forge specification.

## Interface Implementations

The `verifier` module is a standalone utility and does not implement interfaces defined in other packages. It acts as a consumer of the AST produced by [pkg/ast](../ast/design.md) and [pkg/parser](../parser/design.md) and provides its own reporting structures for use by CLI tools and CI pipelines.

## Public API

The verifier's public API is focused on the orchestration of the verification pipeline and the reporting of its results.

- `Verify(forgePath string) (*Result, error)`: The primary entry point. It orchestrates the entire pipeline, from parsing the Forge file and discovering Go sources to extracting types and performing the final comparison.
- `Result`: A container for the findings of a verification run. It provides an `ErrorCount() int` method to determine if any critical discrepancies were found, which is typically used to fail CI builds.
- `Finding`: Represents a single discrepancy. It includes the `Severity` (Error, Warning, or Info), the locations in both the `ForgeFile` and `GoFile`, and a descriptive `Message`.
- `Severity`: An enumeration of `Error` (fundamental mismatch), `Warning` (minor drift or extra Go fields), and `Info` (contextual information).

## Implementation Details

### Deep Type Comparison

The core of the verifier's logic resides in the `forgeTypeToGoString` function, which recursively converts Forge `TypeExpr` nodes into a Go-compatible string format. This transformation allows the verifier to compare types across the two systems using string equality. Key mappings include:
- Forge optional types (`T?`) map to Go pointers (`*T`).
- Forge sequences (`[T]`) map to Go slices (`[]T`).
- Forge maps map directly to Go maps.
- Forge tuples are used to represent Go's multiple return values and are unpacked during comparison to be checked element-wise.
- Forge `unit` represents a void return or an empty parameter list.

The `typesMatch` function serves as the final arbiter of type equality. It handles the equivalence of `any` and `interface{}` and employs a `stripPackagePrefix` helper to heuristically remove package qualifiers (e.g., converting `ast.File` to `File`). This allows Forge files to use unqualified names while the Go implementation uses fully qualified types. To avoid false positives, the function returns true if either side contains a "?" character, representing an unconvertible or unknown type.

### Name Mapping and Normalization

Forge typically uses `snake_case` for identifiers, while Go uses `PascalCase` for exported symbols and `camelCase` for unexported ones. The verifier bridges this gap by attempting three-way matching in functions like `findGoName` and `findGoFieldType`. It checks for the `PascalCase` version (via `snake_to_pascal`) for exported fields and methods, the `camelCase` version (via `snake_to_camel`) for unexported identifiers, and finally an exact match. Forge primitive names are also mapped to their Go equivalents (e.g., `i32` to `int32`).

### Verification Rules

The verifier applies specific rules based on the type of declaration encountered:
- **Structs and Classes**: It checks for field existence and type matching. For classes, it additionally verifies method signatures, comparing parameter counts (excluding the `self` parameter) and ensuring that positional types and return types match.
- **Interfaces**: It ensures that every method declared in Forge exists in the Go interface with a matching signature.
- **Enums**: It validates enums against three common Go patterns: typedefs, structs, or interfaces, and also checks for the common `XxxKind` naming pattern.
- **Functions**: It performs positional parameter and return type comparison, with multi-return Go functions mapped to Forge tuple returns.

### Completeness Check

The `verifyCompleteness` function ensures that the Forge specification is a complete model of the public API. It identifies all exported Go symbols—types and functions—in the referenced source files and reports an error for any that are not documented in the Forge block. It uses the `isExported` helper (checking for an uppercase first letter) to distinguish between the public API and internal implementation details.

## Project Invariants

A unique responsibility of the `verifier` module, implemented in `invariants_test.go`, is the enforcement of project-wide architectural invariants. These are rules that cannot be easily enforced by the Go type system but are critical for the correct operation of the compiler. The module uses the Go standard library's AST tools to inspect the source code of other modules and verify these rules:

- **AST Value Types**: Ensures that `ast.Expr` slices are never iterated using range-copy (e.g., `for _, x := range slice`), which would copy the value type and lose checker annotations. Instead, index-based iteration (`&slice[i]`) must be used.
- **Monomorphization Safety**: Verifies that field and type lookups in the backend correctly check `ClassRenames` to account for monomorphized generic types.
- **Standard Library Integration**: Ensures that the standard library is merged into the correct AST block (Block 0).
- **Desugar Order**: Enforces the specific sequence of desugaring passes (Embeds → InterfaceFields → Relations → Destructors → DefaultImpls) in `cmd/forge/main.go`.
- **Checker Multi-Phase Structure**: Validating that the checker maintains its multi-phase resolution strategy (Phase 0 for type stubs, Phase 1 for registration, Phase 2 for bodies).
- **String Interpolation Safety**: Specifically checks that the string interpolation handling in the checker and lowerer uses index-based iteration to avoid regression of a known range-copy bug.

## Dependencies

- **[pkg/ast](../ast/design.md)**: Provides the `ForgeBlock` and `TypeExpr` structures used to represent the Forge specification.
- **[pkg/parser](../parser/design.md)**: Used to parse the `.forge` file into an AST.
- **Go Standard Library**:
    - `go/ast` and `go/parser`: Used to analyze the Go source code and extract type information.
    - `go/token`: Used for position tracking in Go files.
    - `os` and `path/filepath`: Used for file system operations and path resolution.

## Technical Debt and Future Work

- **Expression Verification**: Currently, `requires` and `ensures` clauses are treated as raw strings and are not parsed or verified against the Go implementation. Future versions could use a Go expression parser to validate these constraints.
- **Complex Pointer Indirection**: While basic optional-to-pointer mapping works, deeply nested or complex pointer structures may occasionally cause false positives.
- **Import Resolution**: The `stripPackagePrefix` mechanism is heuristic and does not perform full symbol resolution, meaning it cannot distinguish between types with the same name in different packages.
- **Performance**: The verifier re-parses Go files for every Forge block. For large projects, a caching mechanism for parsed Go ASTs and `goTypeInfo` would significantly improve performance.
- **Complex Function Types**: Complex function types, such as maps containing functions, cannot currently be fully expressed in Forge and may be typed as `any` or `?`, leading to known false-positive errors in some edge cases.
- **Generic Constraints**: While basic generic types are supported, complex constraints are not currently compared.
