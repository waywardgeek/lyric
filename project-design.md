# Grok Toolchain Project Design

## Executive Summary

The Clean Grok project is a comprehensive toolchain designed to support Grok-Driven Development (GDD) and the compilation of the Grok programming language. The system serves a dual purpose: it provides a robust compiler that translates full Grok implementation files (`.gk`) into idiomatic, executable Go source code, and it offers a powerful verification engine that ensures high-level architectural declarations (`.grok` files) remain strictly synchronized with their underlying Go implementations. By bridging the gap between architectural intent and concrete code, the project enables developers to maintain complex systems with confidence, knowing that structural drift will be automatically detected and reported.

## System Architecture

The project is architected around a classic, multi-pass compiler pipeline, cleanly separated into reusable library packages and distinct command-line interfaces. The architecture is fundamentally data-driven, with the Abstract Syntax Tree (AST) serving as the central nervous system that connects all modules. 

The system is divided into two primary execution paths. The compilation path transforms raw Grok source text into a structured AST, performs rigorous semantic analysis and type checking to annotate that tree, and finally traverses the annotated tree to generate Go code. The verification path, conversely, parses architectural `.grok` files into an AST and performs a deep structural comparison against the actual Go source code parsed from the file system, acting as an automated architectural auditor.

This separation of concerns ensures that the core logic for parsing, type checking, and code generation remains decoupled from the CLI wrappers, allowing the internal packages to be tested independently and potentially reused in other contexts, such as language servers or IDE plugins.

## Interface & Contract Map

The boundaries between modules in the Grok toolchain are defined by strict data contracts rather than traditional Go interfaces. The primary contract that binds the system together is the `ast.File` structure and its constituent nodes. 

The `parser` module acts as the sole producer of the raw AST, establishing the initial contract. The `checker` module consumes this raw AST and fulfills its contract by mutating it—specifically, by populating the `ResolvedType` fields on expressions and patterns. To prevent circular dependencies between the pure data representation and the complex type system, these fields are typed as `any` within the `ast` package. 

The `transpiler` acts as the final consumer in the compilation pipeline, relying entirely on the type annotations provided by the `checker` to generate correct Go code, including necessary type casts and standard library mappings. 

In the verification pipeline, the `verifier` module consumes the AST produced by the `parser` and establishes a secondary contract with the Go standard library's `go/ast` package. It bridges these two distinct representations by normalizing names and mapping Grok type expressions to Go type strings, effectively creating a unified interface for structural comparison.

## Module Map

The project is organized into command-line applications and core library packages.

### Command-Line Tools
*   [cmd/grok-compile](cmd/grok-compile/design.md)
*   [cmd/grok-verify](cmd/grok-verify/design.md)

### Core Libraries
*   [pkg/ast](pkg/ast/design.md)
*   [pkg/parser](pkg/parser/design.md)
*   [pkg/checker](pkg/checker/design.md)
*   [pkg/transpiler](pkg/transpiler/design.md)
*   [pkg/verifier](pkg/verifier/design.md)

### Module Details

The **grok-compile** module is the command-line interface for the compiler. It orchestrates the entire compilation pipeline by parsing command-line arguments, reading input files, and sequentially invoking the parser, checker, and transpiler. It manages the batch processing of multiple files, ensuring that a single type-checking context is shared across all files before individual transpilation occurs.

The **grok-verify** module is a command-line utility dedicated to architectural auditing. It acts as a thin wrapper around the verifier package, taking a `.grok` file path as input, executing the verification engine, and formatting the resulting structural drift findings for console output. It is designed to fail with a non-zero exit status upon detecting errors, making it suitable for Continuous Integration environments.

The **ast** module is the foundational data model for the entire project. As a leaf package, it defines the Abstract Syntax Tree using a tagged union pattern to represent the recursive nature of language constructs. It captures everything from high-level architectural declarations like classes and interfaces to low-level implementation details like expressions and concurrency primitives, maintaining source-level fidelity through comprehensive span tracking.

The **parser** module transforms raw Grok source code into the structured AST. It employs a hand-written lexer to handle significant newlines and complex literals, and a hybrid parsing strategy. It uses recursive descent for high-level architectural declarations and a Pratt parser (precedence climbing) for complex expressions. It is responsible for handling both `.grok` and `.gk` files, gracefully managing the transition between architectural definitions and implementation logic.

The **checker** module is the semantic heart of the toolchain. It performs a comprehensive, multi-phase analysis over the AST, handling forward references, interface satisfaction, generics, and exhaustiveness checking for pattern matching. It maintains a global registry of types and a lexical scope stack, ultimately annotating the AST with resolved types to ensure the program is semantically sound before code generation.

The **transpiler** module is the code generation engine. It consumes the type-checked AST and produces idiomatic Go source code. It manages the complex mapping between Grok's high-level features—such as classes, enums with pattern matching, and the try operator—and Go's structural primitives. It operates as a single-pass visitor over the AST, maintaining internal state to track visibility rules, required imports, and temporary variables for complex transformations.

The **verifier** module is the structural integrity engine. It performs a deep, recursive comparison between the Grok AST and the actual Go source code. It parses the associated Go files, extracts their type information, and normalizes naming conventions and type expressions to detect discrepancies. It aggregates findings of varying severities, ensuring that the documented architectural intent accurately reflects the concrete implementation.

## Integration Patterns & Workflows

The system relies on two primary workflows that dictate how modules interact to achieve the project's goals.

### The Compilation Workflow
The compilation process is driven by the `grok-compile` CLI and represents a linear transformation pipeline. When a user invokes the compiler with a set of `.gk` files, the CLI first reads the raw text of each file. It passes this text to the `parser`, which tokenizes the input and constructs an unannotated AST. Once all files are parsed, the CLI initializes a single instance of the `checker`. This shared checker instance processes all ASTs in a multi-phase approach: it first registers all top-level declarations across all files to resolve forward dependencies, and then it performs deep semantic validation and type inference, mutating the ASTs to include `ResolvedType` annotations. Finally, the CLI iterates through the annotated ASTs, creating a new `transpiler` instance for each file. The transpiler walks the AST, mapping Grok constructs to Go syntax, resolving necessary imports, and emitting the final Go source code to disk.

### The Verification Workflow
The verification process is driven by the `grok-verify` CLI and represents a complex comparative analysis. When invoked with a `.grok` file, the CLI delegates immediately to the `verifier` module. The verifier first utilizes the `parser` to generate an AST of the architectural declarations. It then inspects the AST for `source:` annotations to discover the corresponding Go implementation files. The verifier uses the Go standard library to parse these Go files and build a comprehensive registry of the implemented types. The core of the workflow is a recursive traversal of the Grok AST, where the verifier normalizes names and type expressions, comparing each declared Grok symbol against the aggregated Go registry. Any structural drift—such as missing fields, mismatched signatures, or undocumented exports—is collected into a result set and returned to the CLI for formatted output.

## Dependency Overview

The project's dependency graph is strictly layered to prevent circular imports and maintain clear boundaries of responsibility. 

At the very bottom of the dependency graph is the `ast` module. As a pure data representation, it depends on no other internal packages, making it universally consumable. 

The `parser` module sits directly above the AST, depending only on it to construct the tree. 

The `checker` module depends on the `ast` module to traverse and annotate the tree, but it remains entirely independent of the parsing logic. 

The `transpiler` module depends on both the `ast` and the `checker`. It requires the AST for its structural traversal and relies heavily on the type annotations provided by the checker to generate accurate Go code. 

The `verifier` module depends on the `ast` and the `parser` to read and understand the architectural declarations, but it operates independently of the checker and transpiler, as it performs its own specialized form of type resolution against Go source code.

Finally, the command-line applications sit at the top of the graph. `grok-compile` orchestrates the `ast`, `parser`, `checker`, and `transpiler` modules to execute the full compilation pipeline. `grok-verify` depends on the `verifier` module (and transitively the `ast` and `parser`) to execute the auditing pipeline. This strict layering ensures that the core logic remains highly modular, testable, and resilient to change.
