# Grok Project Architecture

## Executive Summary

The Grok project is a structural integrity engine and language toolchain designed to bridge the gap between high-level architectural design and concrete implementation. It introduces the Grok language, which allows developers to declare the structural and semantic contracts of their systems—such as types, relations, and concurrency constraints—in `.grok` files, as well as full procedural logic in `.gk` files. The primary goal of the project is to detect "architectural drift" by comparing these declarative models against the actual Go source code implementation, acting as a continuous verification system for software architecture. Furthermore, the toolchain supports transpilation, enabling the generation of idiomatic Go code directly from Grok models, and semantic analysis to ensure the soundness of the architectural declarations. By ensuring that the developer's mental model remains synchronized with the reality of the codebase, Grok serves as a comprehensive ecosystem for architectural design, verification, and code generation.

## System Architecture

The architecture of the Grok project follows a classic compiler frontend pipeline integrated with a static analysis engine and a code generation backend. The system is composed of several primary modules: the Abstract Syntax Tree (`ast`), the `parser`, the `checker`, the `transpiler`, and the `verifier`. The philosophy of the architecture is centered around a single source of truth for the structural model, which is the AST. Data flows linearly from source text into the parser, which constructs the AST. This AST then serves as the foundational data structure for all subsequent operations. The checker performs deep semantic analysis and type inference on the AST to ensure correctness. The verifier acts as a bridge between two distinct type systems, extracting type information from the target Go source code and performing a deep, recursive comparison against the Grok AST to detect architectural drift. Alternatively, the transpiler consumes the AST to generate idiomatic Go source code, enabling a forward-engineering workflow. This decoupled design ensures that the parsing logic is entirely independent of the verification, checking, and transpilation logic, allowing the AST to serve as a versatile foundation for a growing suite of tools.

## Interface & Contract Map

While the Grok project currently relies on concrete data structures rather than abstract Go interfaces for its internal boundaries, the system is defined by several critical contracts that govern module interactions.

The primary structural contract for the entire system is defined by the AST file structure. The parser module acts as the sole producer of this contract, transforming raw source text into a valid syntax tree. The checker, transpiler, and verifier modules act as the primary consumers, traversing this file structure to drive their respective analyses and transformations.

Another critical contract is the verification result structure exposed by the verifier module. This structure contains a collection of finding objects, serving as the external contract for any build pipeline, continuous integration system, or integrated development environment that consumes the output of the Grok toolchain. Similarly, the checker produces a collection of semantic errors that serve as a contract for validating the correctness of the Grok source.

Finally, the verifier heavily consumes the Go standard library's abstract syntax tree and parser interfaces to extract type information from the target codebase. In this capacity, the verifier acts as a client to the Go compiler's frontend, relying on these external interfaces to build its internal representation of the implementation's reality.

## Module Map

- **Root Module**
  - [grok](design.md)
- **Core Data Structures**
  - [pkg/ast](pkg/ast/design.md)
- **Frontend Toolchain**
  - [pkg/parser](pkg/parser/design.md)
  - [pkg/checker](pkg/checker/design.md)
- **Code Generation & Analysis**
  - [pkg/transpiler](pkg/transpiler/design.md)
  - [pkg/verifier](pkg/verifier/design.md)

### Grok Root Module
The root module serves as the primary demonstration and testing ground for the Grok structural integrity engine. It contains sample Go implementations that provide a concrete target for the Grok verifier, illustrating how Go source code is structured and how it can be mapped to architectural declarations in `.grok` files. It manages no internal state and acts as a passive data source for the analysis engine, showcasing basic types, recursive functions, and structural patterns that the verifier is designed to analyze.

### AST Module
The `ast` module is the foundational data model for the Grok language. It defines the Abstract Syntax Tree, capturing the hierarchical structure of `.grok` and `.gk` files, including blocks, type declarations, functions, complex relations, and procedural logic. A critical aspect of its design is the use of a tagged union pattern for type expressions and statements, allowing for a flexible and recursive representation of Grok's rich type system and execution flow. It manages no internal state, serving purely as a collection of data structures. It implements no external interfaces but provides the core types consumed by the parser, checker, transpiler, and verifier.

### Parser Module
The `parser` module is responsible for lexical and syntactic analysis, transforming raw Grok source code into the structured AST. It employs a two-phase architecture with a hand-written, stateful lexer and a hybrid parser that combines recursive descent for declarations with Pratt parsing for expressions. The lexer manages state to handle complex tokens like triple-quoted strings and significant newlines, while the parser manages the state of the token stream, occasionally using lookahead and state restoration to resolve ambiguities. The parser consumes raw text and produces the `ast.File` contract.

### Checker Module
The `checker` module provides the semantic analysis engine for the Grok language, focusing on type checking and type inference. It traverses the AST produced by the parser, maintaining a stateful environment that tracks the types of all declared entities and the current lexical scope. It verifies that variables are used consistently, function calls provide correct arguments, and complex expressions resolve to valid types. The checker consumes the `ast.File` contract and produces a collection of semantic errors, acting as the final gatekeeper before architectural verification or code generation.

### Transpiler Module
The `transpiler` module is responsible for converting a Grok AST into idiomatic Go source code. It follows a visitor-like pattern, recursively traversing the AST and emitting Go source code that maps Grok's high-level constructs, such as enums with variants and cascade statements, to Go's language primitives. The transpiler maintains minimal state, primarily tracking the current indentation level and target package name within an internal buffer. It consumes the `ast.File` contract and produces a string containing the generated Go source, enabling a forward-engineering workflow from Grok models to Go implementations.

### Verifier Module
The `verifier` module is the structural integrity engine that detects architectural drift. It manages a complex internal state during execution, specifically aggregating all structs, interfaces, and functions extracted from the target Go source code. The verifier consumes the `ast.File` contract produced by the parser and the Go AST produced by the standard library. It bridges the gap between these two models by normalizing naming conventions and type expressions, ultimately producing a structured result containing any detected discrepancies.

## Integration Patterns & Workflows

The most critical cross-module workflow is the end-to-end verification process. When a user invokes the verifier on a `.grok` file, the workflow begins in the `verifier` module, which immediately delegates to the `parser` module. The `parser` instantiates its lexer, tokenizes the source, and constructs an `ast.File` tree, returning it to the verifier. The verifier then inspects the source annotations within the AST to locate the corresponding Go files. It invokes the Go standard library parser to build a Go AST and extracts the relevant type information into its internal state. Finally, the verifier recursively traverses the Grok AST, comparing each declaration against the extracted Go types, normalizing names and types on the fly, and accumulating findings into a final result.

Another complex workflow is the semantic checking and type inference pipeline. After the parser generates the AST, the `checker` module takes over to ensure semantic soundness. The checker performs a first pass over the AST to register all top-level declarations into its internal registry, allowing for forward references. It then performs a deep recursive traversal of all expressions and statements, managing a stack of lexical scopes. When it encounters a variable declaration with an implicit type, it evaluates the initializer expression, infers the type, and binds it to the variable in the current scope. This workflow ensures that the AST is not only syntactically correct but also semantically valid before it is passed to the transpiler or verifier.

The transpilation workflow represents the forward-engineering path of the Grok toolchain. Once the AST is parsed and optionally checked for semantic correctness, the `transpiler` module is invoked. The transpiler iterates through the blocks in the AST file, recursively visiting declarations, statements, and expressions. It performs a sophisticated mapping between Grok's rich type system and Go's simpler primitives, such as converting Grok enums into Go interface-based patterns and Grok cascade statements into Go defer blocks. The transpiler accumulates the generated Go source code in an internal buffer and returns the final string, providing a seamless transition from architectural design to concrete implementation.

## Dependency Overview

The dependency graph of the Grok project is strictly layered and unidirectional, preventing circular dependencies and ensuring a clean separation of concerns. At the base of the graph is the `ast` module, which is a leaf package depending only on the Go standard library. The `parser` module sits above the `ast`, depending on it to construct the syntax tree. The `checker`, `transpiler`, and `verifier` modules all sit at the top of the hierarchy, depending on the `ast` to traverse the data model. The `checker` and `transpiler` operate directly on the AST, while the `verifier` also introduces a heavy dependency on the Go standard library's `go/ast` and `go/parser` packages to analyze the target implementation. Furthermore, the testing suites for the checker and verifier rely on the parser to generate the necessary ASTs for their integration tests. This layered architecture constrains the flow of information: the AST has no knowledge of how it is parsed, and the parser has no knowledge of how the AST will be checked, transpiled, or verified.
