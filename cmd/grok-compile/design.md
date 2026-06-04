# Grok-Compile Design

## Executive Summary
`grok-compile` is the primary command-line interface for the Grok compiler. It serves as the orchestrator for the entire compilation pipeline, transforming Grok source files (typically `.gk` files) into idiomatic, executable Go source code. The tool manages the lifecycle of a compilation unit, which may consist of multiple source files that share a common type context. By coordinating the interaction between the parser, semantic analyzer, and code generator, `grok-compile` provides a seamless transition from Grok's high-level architectural and implementation constructs to Go's robust execution environment.

## File Inventory
*   [main.go](main.go): The sole source file for the command. It implements the entry point, handles command-line argument parsing, manages file I/O, and coordinates the sequential execution of the parsing, type checking, and transpilation phases.

## Architecture and Data Flow
The `grok-compile` tool implements a classic multi-pass compiler architecture. The data flow is strictly linear, with each phase consuming the output of the previous one and enriching the internal representation of the program.

The process begins with **Argument Parsing**, where the `main` function scans the command-line arguments to identify input files and configuration flags. It supports specifying an output filename via `-o` and a target Go package name via `-pkg`.

Once the inputs are identified, the tool enters the **Parsing Phase**. It reads each input file from disk and passes the raw text to the `parser.ParseFile` function. This stage transforms the unstructured text into a collection of Abstract Syntax Tree (AST) nodes, rooted in an `ast.File` structure. These ASTs capture the structural essence of the Grok code but lack semantic meaning at this point.

The **Type Checking Phase** is the semantic heart of the orchestration. The tool initializes a single, shared `checker.New()` instance. This shared context is crucial because it allows the compiler to resolve type references across multiple files in the same compilation batch. The tool iterates through all parsed ASTs, calling `ch.CheckFile(pf.file)` for each. During this pass, the checker populates the `ResolvedType` fields within the AST nodes, effectively "coloring" the tree with semantic information. If any semantic errors are detected—such as type mismatches or missing declarations—they are collected and reported to the user.

Finally, the tool executes the **Transpilation Phase**. For each file in the batch, it creates a new `transpiler.New(pkg)` instance, initialized with the target Go package name. It then invokes `tr.Transpile(pf.file)`, which performs a single-pass traversal of the annotated AST to generate the corresponding Go source code. The resulting string is then written to the designated output file on disk.

## Interface Implementations
As a command-line application, `cmd/grok-compile` does not implement any internal library interfaces. Instead, it acts as the primary consumer of the interfaces and data structures defined in the core `pkg` modules. It relies on the `ast.File` structure as the universal data contract that binds the pipeline together.

## Public API
The "API" of `grok-compile` is its command-line interface. It is designed to be simple and composable, fitting into standard build pipelines.

`grok-compile <file.gk>... [-o output.go] [-pkg name]`

*   **`<file.gk>...`**: One or more Grok source files. If multiple files are provided, they are treated as a single compilation unit for type-checking purposes.
*   **`-o <output.go>`**: Specifies the output filename. If this flag is omitted, the tool generates a default filename by replacing the `.gk` extension of the input file with `.go`. Note that if multiple input files are provided with a single `-o` flag, the current implementation will overwrite the output file for each input.
*   **`-pkg <name>`**: Specifies the name of the Go package for the generated code. This defaults to `main` if not specified.

## Implementation Details
The implementation in `main.go` is structured for clarity and sequential execution. It uses a custom `parsedFile` struct to track the relationship between an input file, its parsed AST, and its intended output path.

The tool's handling of the type checker is a critical detail. By using a single `checker` instance for all files, it supports forward references and cross-file dependencies, which are essential for modular Grok development. The checker's `CheckFile` method is called sequentially for each file, allowing it to build a global registry of types before performing deep validation.

The transpilation step is performed independently for each file. This design choice reflects the mapping of one Grok file to one Go file. The `transpiler` is ephemeral, created fresh for each file to ensure that file-specific state (like local imports) does not leak between outputs.

## State Management
The application's state is primarily transient and held in memory during the execution of the `main` function.
*   **Command-line State**: Captured in local variables (`inputs`, `output`, `pkg`) during the initial argument scan.
*   **AST State**: Stored in a slice of `parsedFile` structs, which persist from the parsing phase through to transpilation.
*   **Type Registry**: Maintained internally by the `checker` instance. This is the only state that is shared across multiple files during the compilation process.
*   **Concurrency**: The current implementation is strictly single-threaded, processing files and phases sequentially. This simplifies error handling and ensures predictable output.

## Error Patterns
`grok-compile` follows a "fail-fast" philosophy for I/O and syntax errors, but an "accumulate-and-report" philosophy for semantic errors.
*   **I/O and Syntax Errors**: If a file cannot be read or fails to parse, the tool prints the error to `stderr` and exits immediately with a non-zero status.
*   **Semantic Errors**: Errors found during the type-checking phase are collected by the `checker`. The tool prints all discovered errors to `stderr`. Currently, it continues to the transpilation phase even if errors are found, which is a known limitation.

## Dependencies
*   [pkg/ast](../../pkg/ast/design.md): Provides the foundational data structures for the Abstract Syntax Tree.
*   [pkg/parser](../../pkg/parser/design.md): Handles the conversion of raw Grok source text into an AST.
*   [pkg/checker](../../pkg/checker/design.md): Performs semantic analysis, type resolution, and validation.
*   [pkg/transpiler](../../pkg/transpiler/design.md): Generates idiomatic Go source code from the annotated AST.

## Technical Debt and Future Work
*   **Output Collision**: The tool should be updated to handle the `-o` flag more gracefully when multiple input files are provided, perhaps by requiring the flag to be omitted or by supporting a directory output.
*   **Strict Error Handling**: The compiler should exit with a non-zero status and skip the transpilation phase if any type-checking errors are encountered. This would prevent the generation of invalid or broken Go code.
*   **Parallelism**: For large projects, the parsing and transpilation phases could be parallelized to improve performance, though the type-checking phase must remain coordinated.
*   **Package Mapping**: Future versions could support more complex mappings between Grok files and Go packages, perhaps through a configuration file or more advanced command-line flags.
