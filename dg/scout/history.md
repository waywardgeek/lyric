### USER (gemini)

A previous session handed off to you. Read dg/scout/handoff.md and continue the task described there.

---

### ASSISTANT (gemini)

**Tool Call: `read_file`**
```json
{
  "file_path": "dg/scout/handoff.md"
}
```

---

### USER (gemini)

**Tool Result:**
```
[ID: tool_6d3cce79048ec2de]
File: /Users/bill/projects/lyric/dg/scout/handoff.md (14 lines)

---
## Handoff — 13:47:00

### Summary
Synthesize project-design.md

### Plan
Confirm completion.

### Critical Context
Synthesized project-design.md by analyzing all module design.md files. The document is comprehensive, written in narrative prose, and includes a categorized tree view of all modules.

---
```

---

### ASSISTANT (gemini)

**Tool Call: `read_file`**
```json
{
  "file_path": "project-design.md"
}
```

---

### USER (gemini)

**Tool Result:**
```
[ID: tool_505be9b5cefcd9f3]
File: /Users/bill/projects/lyric/project-design.md (77 lines)

# Lyric Project Architecture

## Executive Summary

The Lyric project is a comprehensive compiler and toolchain for the Lyric programming language. It is designed to provide a high-level, expressive language with advanced features such as relations, interfaces, generic type inference, and structured control flow, while maintaining the capability to compile down to multiple backends, currently supporting Go and C. The project exists to bridge the critical gap between high-level architectural intent and low-level implementation. By capturing design specifications in dedicated `.lyric` files, the system ensures structural integrity and actively prevents architectural drift. Its primary goals are to deliver a robust, multi-stage compilation pipeline—spanning parsing, semantic analysis, low-level intermediate representation, and code generation—while maintaining a strong, verifiable connection between the documented design and the evolving codebase through its unique verification engine.

## System Architecture

The Lyric compiler follows a classic pipeline architecture, augmented by a parallel verification track that acts as an architectural guardian. The architecture is fundamentally layered and data-centric, designed to isolate complexity at each distinct stage of transformation. The primary compilation flow is linear, progressing from raw source text to an Abstract Syntax Tree (AST), through semantic checking, into a Low-level Intermediate Representation (LIR), and finally to target code generation.

The philosophy of the system is to resolve specific classes of problems in dedicated modules, ensuring that downstream consumers operate on increasingly simplified and explicit data models. The parser handles all syntax and grammatical ambiguities, employing a hybrid strategy of recursive descent and Pratt parsing to produce a structural representation. It manages significant newlines and complex f-string interpolations, providing a clean foundation. The AST module acts as the central data hub, providing a representation of the program using a highly efficient "Kind + Data" pattern. Crucially, it features a generative desugaring engine that expands high-level syntactic sugar—such as relations, interface fields, and default implementations—into fundamental constructs before semantic analysis. It also selectively merges the standard library based on reachability analysis.

The checker assumes responsibility for all semantic rules, acting as the semantic truth of the compiler. It performs a multi-pass analysis for type inference, scope management, module import resolution, and validation. It handles structural subtyping, numeric widening, optional wrapping, and enforces concurrency safety through guarded annotations. Ultimately, it annotates the AST with resolved types. 

The LIR module then takes this semantically rich tree and bridges the gap to the backends. It enforces structured control flow and flat expressions, assigning every sub-expression to a unique temporary variable. This crucial step removes the burden of semantic resolution from the backends. The LIR pipeline includes optimization passes for side-effect elimination and multi-return destructuring, as well as a comprehensive monomorphization pass to specialize generic types and functions for targets that lack native generics. Finally, the backends act as thin emitters that translate the simplified LIR into idiomatic target source code. The C backend relies heavily on the runtime module, which provides a foundational C header library establishing the memory layout and operational semantics for Lyric's high-level constructs.

In parallel to the compilation pipeline, the verifier module operates as a standalone structural integrity engine. It parses `.lyric` specification files and performs a deep structural comparison against the actual Go implementation, utilizing the Go standard library's AST tools to extract type information. It normalizes naming conventions across the two systems and performs a reverse completeness check to ensure every exported Go symbol is documented. This ensures that the documented design and the codebase remain perfectly synchronized, enforcing the philosophy that documentation should be an active, verifiable contract rather than a passive artifact.

## Interface & Contract Map

The system's boundaries are primarily defined by the rich data structures passed between modules rather than traditional Go interfaces. These data structures act as the fundamental contracts that govern module interactions, ensuring a decoupled and data-driven architecture.

The most critical contract is the Abstract Syntax Tree. The `ast.File` and its constituent nodes representing declarations, expressions, and statements form the central nervous system of the compiler. The parser module acts as the initial producer of this contract, transforming source code into the raw AST. The AST module itself acts as both a consumer and producer, taking the raw AST and producing a desugared version where high-level constructs are flattened. The checker module consumes this desugared AST for semantic analysis and mutates it by attaching resolved type information to the `ResolvedType` and `InferredTypeArgs` fields. The LIR module consumes the fully type-checked AST to generate the intermediate representation. Additionally, the verifier module consumes the AST of `.lyric` files to compare against Go source code.

The Type Registry contract, defined by the `checker.Registry` and `checker.Type` structures, represents the semantic truth of the program. The checker module builds and populates this registry during its declaration and checking passes, resolving all cross-module dependencies and generic constraints. The LIR module is the primary consumer of this contract, relying heavily on the type registry and resolved types to correctly lower high-level constructs, such as generics, classes, and enums, into low-level operations.

The LIR contract, represented by the `lir.LProgram`, defines the fully resolved and flattened program state. The LIR module's lowerer produces this representation, ensuring that all expressions are flattened into temporaries and control flow is structured. Internal optimization and monomorphization passes within the LIR module consume and mutate this structure, specializing generic types and eliminating redundant operations. Ultimately, the backends consume the optimized `LProgram` to emit the final source code.

The Runtime contract is defined by the C runtime header (`lyric_runtime.h`), which establishes the execution environment for C-compiled Lyric programs. The runtime module provides these foundational macros and inline functions, defining the memory layout for dynamic slices, strings, optional types, error results, and tagged unions. The C backend within the LIR module acts as the sole consumer, emitting code that relies entirely on this contract for operational semantics and concurrency primitives.

## Module Map

### Categorized Tree View

- **Frontend**
  - [pkg/parser](pkg/parser/design.md)
  - [pkg/ast](pkg/ast/design.md)
- **Semantic Analysis**
  - [pkg/checker](pkg/checker/design.md)
- **Intermediate Representation & Backends**
  - [pkg/lir](pkg/lir/design.md)
  - [runtime](runtime/design.md)
- **Tooling**
  - [pkg/verifier](pkg/verifier/design.md)

### Module Descriptions

The **Parser** module is the foundational entry point for the compiler frontend, responsible for transforming raw Lyric source code into an Abstract Syntax Tree. It manages internal state through a hand-written lexical scanner that tokenizes the input, carefully handling significant newlines and collecting comments. The parser employs a hybrid strategy, utilizing recursive descent for high-level declarations and a Pratt parser for expressions, allowing it to efficiently handle complex operator precedence without grammar explosion. It implements the primary data contract by producing the `ast.File` consumed by the rest of the system, and it seamlessly handles complex features like f-string interpolation, contextual keywords, and disambiguation through targeted backtracking.

The **AST** module defines the core data structures that represent the structure of Lyric code, acting as the central data hub. It manages state primarily as a tree of polymorphic nodes utilizing a "Kind + Data" pattern to avoid deep interface hierarchies, making it highly efficient for recursive traversals. Crucially, it includes a generative desugaring engine that transforms high-level constructs, such as relations, interface fields, and default implementations, into fundamental AST nodes before semantic analysis occurs. It also handles the selective merging of the standard library based on reachability analysis, ensuring the compilation unit remains lean. It implements the primary data contract consumed by the checker and LIR modules.

The **Checker** module implements the semantic analysis phase of the compiler, ensuring that a Lyric program is semantically valid before code generation. It maintains complex internal state, including a stack of scopes for variable tracking and a global registry for user-defined types. It consumes the desugared AST, resolves all type expressions, infers generic types through a unification-based approach, enforces concurrency safety through guarded annotations, and validates the exhaustiveness of match statements. It acts as the semantic truth of the compiler, annotating the AST nodes with `ResolvedType` information, fulfilling the semantic contract required by the subsequent lowering phase.

The **LIR** module is the architectural bridge between the high-level AST and the backend code generators. It resolves all semantic complexity into a simplified, flat representation while preserving structured control flow. It manages state during the lowering process by mapping AST nodes to LIR constructs and assigning all sub-expressions to unique temporary variables, ensuring that backends never encounter nested expression trees. It includes sophisticated optimization passes for side-effect elimination and multi-return destructuring, as well as a comprehensive monomorphization pass to specialize generic types for backends like C. It consumes the type-checked AST and produces the `LProgram` contract, which is then consumed by its internal C backend to emit target code, handling complex transformations like generator state machines and interface vtable dispatch.

The **Runtime** module provides the foundational C runtime library required by the Lyric C backend. It is implemented as a passive, header-only library that establishes the memory layout and operational semantics for Lyric's high-level constructs in a C environment. It does not maintain internal state but provides a suite of macros and inline functions for dynamic slices, length-prefixed strings, optional types, error results, tagged unions, and concurrency primitives. These foundational elements are consumed by the C code generated by the LIR module, bridging the gap between Lyric's expressive type system and C's low-level primitives.

The **Verifier** module is a standalone structural integrity engine designed to detect architectural drift by comparing high-level `.lyric` specifications against the actual Go implementation. It maintains state during a run to aggregate type information extracted from Go source files using the standard library's AST tools. It consumes the AST produced by the parser for `.lyric` files and performs deep structural comparisons, normalizing naming conventions and mapping types across the two systems. It also performs a reverse completeness check, ensuring every exported Go symbol is documented in the Lyric specification, and aggregates any mismatches into a detailed report. Additionally, it enforces project-wide architectural invariants by analyzing the Go AST of other modules.

## Integration Patterns & Workflows

The system relies on clear, sequential workflows to transform data across module boundaries. Two primary workflows define the operation of the Lyric toolchain: the compilation pipeline and the architectural verification process.

The compilation pipeline workflow traces the journey of a Lyric source file from raw text to executable code. The process begins when a file is read from disk and passed to the parser module. The lexer tokenizes the text, handling significant newlines and f-string interpolation, and the parser constructs an initial `ast.File` using recursive descent and Pratt parsing. This AST is immediately passed to the AST module's desugaring functions, which expand complex features like relations into standard classes and implementation blocks, extract default interface implementations, and selectively merge required standard library components. The desugared AST is then handed to the checker module. The checker performs a multi-pass analysis: a pre-registration pass to identify user-defined types, a registration pass to populate its type registry and resolve imports, and a deep traversal to resolve types, infer generics, enforce concurrency safety, and validate semantics, annotating the AST with `ResolvedType` fields. Once validated, the AST and the type registry are passed to the LIR module's lowerer. The lowerer flattens expressions into temporaries and converts high-level constructs into LIR statements while preserving structured control flow. The resulting `LProgram` undergoes optimization passes to remove redundant operations and destructure multi-returns. For the C target, a monomorphization pass specializes generic types. Finally, the C backend emitter traverses the LIR and generates the final source code string, relying on the runtime module's macros, which is then written to disk.

The architectural verification workflow operates independently to ensure design fidelity. The process starts when the verifier module is invoked on a `.lyric` specification file. It uses the parser module to generate an AST of the specification. The verifier then inspects the source annotations within the AST to locate the corresponding Go source files. It uses the Go standard library to extract all exported types, interfaces, and function signatures from these Go files, building a comprehensive internal registry that aggregates information across multiple files. The verifier then recursively walks the Lyric AST, translating Lyric type expressions into Go string representations and comparing them against the extracted Go types. It normalizes naming conventions on the fly, mapping snake_case to PascalCase or camelCase, to facilitate this comparison. Finally, it performs a reverse completeness check, ensuring every exported Go symbol is documented in the Lyric specification, and aggregates any mismatches into a detailed report.

## Dependency Overview

The project exhibits a strict, unidirectional dependency graph that prevents circular imports and enforces a clear separation of concerns. This layering is critical for maintaining the maintainability and testability of the compiler.

At the base of the hierarchy is the AST module, which acts as a leaf node containing pure data structures. The parser module depends solely on the AST module to construct its output, ensuring that parsing logic is decoupled from semantic analysis. The checker module depends on both the AST module for input and annotation, and the parser module to handle imported files during its semantic analysis phase. To avoid circular dependencies, the AST module stores checker-resolved types as opaque `any` fields.

The LIR module depends on the AST and checker modules to perform its lowering process, relying on the semantic truth established by the checker and the structural representation provided by the AST. It also implicitly depends on the runtime module as the target contract for its C backend.

The verifier module depends on the AST and parser modules to read specifications, but operates entirely independently of the compiler's semantic and backend phases. This isolation allows the verifier to act as an objective observer of the codebase. The runtime module is completely independent of all Go code, serving only as a target dependency for the generated C code. This layered architecture ensures that the frontend remains decoupled from backend concerns, and that data structures are defined independently of the logic that manipulates them.
```

---

### ASSISTANT (gemini)

**Tool Call: `find_files`**
```json
{
  "pattern": "design.md"
}
```

---

### USER (gemini)

**Tool Result:**
```
📁 **File Search Results** for `design.md`

**Directory**: /Users/bill/projects/lyric
**Max Depth**: 10

**Found 6 files:**

- `/Users/bill/projects/lyric/pkg/ast/design.md` (13244 bytes)
- `/Users/bill/projects/lyric/pkg/checker/design.md` (11521 bytes)
- `/Users/bill/projects/lyric/pkg/lir/design.md` (15405 bytes)
- `/Users/bill/projects/lyric/pkg/parser/design.md` (13437 bytes)
- `/Users/bill/projects/lyric/pkg/verifier/design.md` (11848 bytes)
- `/Users/bill/projects/lyric/runtime/design.md` (11447 bytes)
```

---

### ASSISTANT (gemini)

**Tool Call: `read_file`**
```json
{
  "file_path": "pkg/parser/design.md"
}
```

---

### USER (gemini)

**Tool Result:**
```
[ID: tool_85349def8d8c6021]
File: /Users/bill/projects/lyric/pkg/parser/design.md (110 lines)

# Parser Module Design

The `parser` module is the foundational entry point for the Lyric compiler frontend. Its primary responsibility is to transform raw Lyric source code—whether in declarative `.lyric` files or imperative `.ly` files—into a structured Abstract Syntax Tree (AST). It employs a hand-written lexical scanner and a hybrid parsing strategy that combines recursive descent for high-level structures with a Pratt (precedence climbing) parser for expressions. This architecture ensures that the parser is both highly performant and capable of providing precise error reporting.

## Executive Summary

The `parser` module serves as the "front door" of the compilation pipeline. It takes raw source text and produces a comprehensive representation of the program's structure as defined in the [pkg/ast](../ast/design.md) module. The module is designed to handle the unique syntactic requirements of the Lyric language, including significant newlines, complex f-string interpolation, and a rich set of declarative constructs for architectural specification. By producing a unified AST, it allows subsequent stages of the compiler, such as the checker and the lowerer, to operate on a consistent data model regardless of the original source file's purpose. The parser is designed to be LL(1) where possible, using targeted backtracking only for specific grammatical ambiguities.

## File Inventory

- [lexer.go](lexer.go): Implements a stateful, hand-written lexical scanner. It tokenizes the source text into a stream of `Token` objects, handles significant newlines, and collects line comments for inclusion in the final AST. It also manages complex literal scanning for triple-quoted strings and f-strings.
- [parser.go](parser.go): Contains the core `Parser` implementation and the recursive descent logic for top-level declarations. It manages the parsing of `lyric` blocks, classes, interfaces, enums, and complex type expressions. It handles the high-level structure of both `.lyric` and `.ly` files and manages contextual keyword resolution.
- [expr_parser.go](expr_parser.go): Implements the expression engine using the Pratt algorithm and the statement parser using recursive descent. It also manages specialized logic for match expressions, lambdas, concurrency primitives (`spawn`, `select`, `lock`), and the recursive parsing of interpolated f-strings.
- [parser.lyric](parser.lyric): A Lyric specification file that provides a high-level architectural overview and self-documentation of the parser module.
- [lexer_test.go](lexer_test.go): Unit tests for the lexical scanner, covering tokenization, newline handling, and literal scanning.
- [parser_test.go](parser_test.go): Unit tests for the declaration parser, ensuring correct AST construction for various Lyric constructs.
- [expr_parser_test.go](expr_parser_test.go): Unit tests for the expression and statement parser, covering precedence, control flow, and complex expression trees.

## Architecture and Data Flow

The transformation from raw text to an AST follows a well-defined pipeline within the module. The process begins with the `Lexer`, which reads the source string character by character. Unlike many traditional lexers, the Lyric lexer treats newlines as significant tokens (`TNewline`) when they serve as statement or declaration separators, while collapsing multiple consecutive newlines to simplify the parser's job. Newlines are suppressed when the lexer is inside brackets (`()`, `[]`, `{}`), allowing for ergonomic multi-line expressions without explicit continuation characters. Comments are captured during this phase and stored in a dedicated slice, allowing them to be attached to the root `ast.File` node without cluttering the token stream.

The `Parser` consumes tokens from the `Lexer` and uses recursive descent to navigate the language's grammar. When the parser identifies a top-level construct, such as a `class` or `func`, it recursively calls specialized methods to parse the constituent parts. For expressions, the parser delegates to the Pratt parsing logic in `expr_parser.go`. This algorithm uses a precedence table to efficiently handle operator precedence and associativity, allowing for complex expressions like method call chains and nested indexing without the "grammar explosion" often seen in pure recursive descent parsers.

As the parser matches grammatical rules, it instantiates the corresponding nodes from the `pkg/ast` module. These nodes are linked together to form a tree. A notable feature of this process is the handling of f-strings: when the lexer encounters an interpolated string, it captures the raw content; the parser then identifies the `{expr}` boundaries and recursively invokes a new parser instance to process the embedded expressions, merging the results into a `StringInterpExpr` node.

## Interface Implementations

The `parser` module does not implement external interfaces. It is designed as a pure producer of the `ast.File` structure. This structure serves as the primary data contract consumed by the [pkg/checker](../checker/design.md) and the [pkg/lir](../lir/design.md) modules.

## Public API

### Entry Points

The module provides two primary entry points for external callers:

- **`ParseFile(source, filename string) (*ast.File, error)`**: This is the standard entry point for parsing a Lyric source file. It initializes the lexer and parser, executes the parsing logic, and returns the resulting AST. If parsing fails, it returns a `ParseError` containing detailed location information.
- **`ParseString(source string) (*ast.File, error)`**: A convenience wrapper around `ParseFile` that uses `"<string>"` as the filename, typically used for testing or parsing small snippets of code.

### Core Types

- **`Parser`**: The central parsing engine. While it can be instantiated manually, it is typically used through the package-level entry points. It maintains the lexer state, a list of accumulated errors, and nesting depth information.
- **`Lexer`**: The stateful scanner responsible for tokenization. It provides `Peek` and `Next` methods for the parser to consume tokens and maintains a collection of scanned comments.
- **`ParseError`**: A specialized error type that implements the `error` interface. It includes a descriptive message, an `ast.Span` identifying the location, and the original source line for enhanced error reporting.
- **`Token`**: Represents a single lexical unit, containing its kind, raw text, and source span.

## Implementation Details

### Lexical Scanning and Significant Newlines

The lexer is a stateful scanner that uses a `peek`/`advance` pattern. It is responsible for identifying keywords, literals, and punctuation. A critical aspect of the Lyric lexer is its handling of newlines. Newlines are preserved as tokens to separate statements, but the lexer automatically collapses multiple consecutive newlines and skips whitespace that does not contribute to the structure. To support ergonomic multi-line expressions, the lexer tracks bracket nesting depth and suppresses newline tokens when inside `()`, `[]`, or `{}`. This allows the parser to remain simple while supporting an ergonomic, newline-sensitive syntax.

### Disambiguation and Backtracking

The Lyric grammar contains several ambiguities that require lookahead to resolve. The parser handles these cases using a simple but effective backtracking mechanism: it saves the current state of the `Lexer` (which is a small, copyable struct), attempts to parse a specific construct, and restores the saved state if the attempt fails. This is used in several primary scenarios:
- **Struct Literals vs. Blocks**: When the parser encounters an identifier followed by `{`, it peeks ahead to see if the content looks like a field initializer (`name: value`) or an empty literal `{}` to distinguish a struct literal from a block of code.
- **Generic Calls vs. Comparisons**: The `<` operator can start a generic argument list or a comparison. The parser attempts to parse a type argument list and backtracks if it fails or if the following token is not an opening parenthesis.
- **Why Annotations vs. Fields**: The `why` keyword can start an annotation (`why: "..."`) or be a field name (`why: Type`). The parser peeks ahead to see if a string literal follows the colon.
- **Tuple Field Labels**: When parsing tuple types or literals, the parser checks if an identifier is followed by a colon to distinguish labeled fields from positional ones.
- **Nested Generics (>> Splitting)**: When parsing nested generic types like `List<List<T>>`, the lexer may emit a `TShr` (>>). The parser detects this in generic argument contexts and splits it into two `TGt` (>) tokens, pushing the second one back into a local buffer for the next `peek()` or `next()` call.

### Pratt Parsing for Expressions

The expression parser in `expr_parser.go` implements the Pratt algorithm, also known as precedence climbing. This approach is particularly well-suited for languages with many levels of operator precedence. The parser maintains a table of precedence levels and associativity for each operator.

The precedence levels are defined as follows:
1.  **OR**: `||`
2.  **AND**: `&&`
3.  **Bitwise OR**: `|`
4.  **Bitwise XOR**: `^`
5.  **Bitwise AND**: `&`
6.  **Equality**: `==`, `!=`
7.  **Comparison**: `<`, `>`, `<=`, `>=`
8.  **Shift**: `<<`, `>>`
9.  **Additive**: `+`, `-`
10. **Multiplicative**: `*`, `/`, `%`
11. **Unary**: `-`, `!` (prefix)
12. **Postfix**: `.`, `()`, `[]`, `!`, `?`

When parsing an expression, it recursively climbs the precedence ladder, ensuring that operators are grouped correctly. Postfix operators, such as member access (`.`), function calls (`()`), indexing (`[]`), unwrap (`!`), and try (`?`), are handled in a loop after the primary expression is parsed, allowing for arbitrary chaining of operations.

### Contextual Keywords and Aliases

To maintain a clean and ergonomic syntax, Lyric treats many keywords (such as `why`, `doc`, `source`, and various annotations) as "contextual." The `expectIdentLike` method allows these tokens to be used as identifiers in positions where they are not ambiguous, such as field names or parameter names. This prevents the language from having an overly restrictive set of reserved words, which is particularly important for self-documenting `.lyric` files. Additionally, `null` is treated as a keyword alias for `nil`, both mapping to the same `TNil` token kind and `ExprNil` AST node to accommodate developers from different backgrounds.

### Statement and Block Parsing

Statements are parsed using recursive descent, recognizing keywords like `let`, `if`, `for`, `while`, `match`, `return`, `spawn`, `select`, `lock`, `cascade`, and `yield`. If no keyword is matched, the parser attempts to parse an expression statement or an assignment. 

Assignments desugar compound operators (like `+=`) into their binary equivalents during the parsing phase (e.g., `x += y` becomes `x = x + y`), simplifying the work for later compiler stages. The `match` statement supports complex pattern matching, including variant patterns, tuple patterns, and guards. The `cascade` statement provides a mechanism for deferred cleanup, while `spawn`, `select`, and `lock` provide first-class support for concurrency.

### F-String Interpolation

F-strings (`f"..."`) are handled by capturing the raw content in the lexer. The parser then scans this content for `{expr}` boundaries. For each expression found, it creates a new `Parser` instance to process the embedded text, and then merges the resulting AST nodes into a `StringInterpExpr`. This recursive approach allows for full expression power within interpolated strings.

### Recursive Type Parsing

The parser handles Lyric's recursive type system through a tiered approach in `parseTypeExpr`. It first parses a "base type" (named types, sequences `[T]`, tuples `(T, U)`, maps `map[K]V`, channels `channel<T>`, etc.), then checks for optional suffixes like `?` (optional) or `|` (union), and finally checks for function type arrows `->`. This structure correctly handles precedence, such as `[T]?` being an optional sequence versus `[T?]` being a sequence of optionals. It also handles the `fn(T, U) -> V` prefix form for function types.

## Dependencies

- **[pkg/ast](../ast/design.md)**: The parser is entirely dependent on the `ast` module for its output data structures. It uses `ast.Pos` and `ast.Span` for location tracking and constructs various `ast.Node` types to build the tree.

## Technical Debt and Future Work

- **Error Recovery**: The current implementation often stops at the first encountered error. Implementing a synchronization mechanism (e.g., skipping to the next lyric block or closing brace) would allow the parser to report multiple errors in a single pass, improving the developer experience.
- **Multi-line Arrays**: Some declarative constructs like `source: [...]` currently require single-line formatting due to parser limitations.
- **Requires/Ensures Expressions**: Currently, these annotations capture raw text rather than fully parsed expression trees. Moving to a parsed representation would allow for better validation during the checking phase.
- **Lambda Union Types**: Lambda parameters currently use `parseBaseType` to avoid ambiguity with the `|` operator, which prevents the direct use of union types in lambda signatures without a type alias.
```

---

### ASSISTANT (gemini)

**Tool Call: `read_file`**
```json
{
  "file_path": "pkg/ast/design.md"
}
```

---

### USER (gemini)

**Tool Result:**
```
[ID: tool_b9834211f49f70e8]
File: /Users/bill/projects/lyric/pkg/ast/design.md (111 lines)

# AST Module Design

The `ast` module defines the Abstract Syntax Tree (AST) for the Lyric language, serving as the foundational data model for the entire compiler toolchain. It provides a structured representation of Lyric source code that is shared between declaration-only `.lyric` files and full implementation `.ly` files. Beyond simple data representation, the module includes a critical desugaring phase that simplifies high-level architectural constructs into fundamental language primitives before semantic analysis.

## Executive Summary

The `ast` module is the central data hub of the Lyric compiler's frontend. It acts as the primary contract between the [parser](../parser/design.md), which produces the tree, and the [checker](../checker/design.md) and [lir](../lir/design.md) modules, which consume and transform it. The module is designed with a "Kind + Data" pattern to achieve polymorphism without the complexity of deep interface hierarchies, making it highly efficient for the recursive traversals common in compiler passes. 

A standout feature of this module is its generative desugaring engine, which automates the expansion of complex features like relations, interface fields, and default implementations into standard classes, methods, and implementation blocks. Additionally, the module handles the selective merging of the standard library, ensuring that only the necessary components are included in the final AST, thereby keeping the compilation process lean and focused. Finally, a post-desugar validation pass ensures that the tree is in a consistent state before being handed off to the type checker.

## File Inventory

- [ast.go](ast.go): Defines the core structural nodes for declarations, including functions, classes, structs, enums, interfaces, and relations. It also establishes the foundational types for source location tracking (`Pos` and `Span`) and type expressions.
- [expr.go](expr.go): Contains the definitions for all expression and statement nodes used in full implementation files. This includes literals, operators, control flow structures (if, for, while, match), and concurrency primitives like `spawn` and `select`.
- [desugar.go](desugar.go): Implements the transformation logic that simplifies the AST. It handles the expansion of relations into concrete fields and `impl` blocks, extracts default interface implementations into top-level functions, and generates destructor logic.
- [stdlib.go](stdlib.go): Manages the integration of the Lyric standard library. It provides utilities for locating the stdlib on disk and selectively merging its interfaces, classes, and functions into the user's AST based on actual usage.
- [validate.go](validate.go): Provides post-desugar validation to ensure that all high-level constructs (like interface embeds) have been correctly flattened and that relations have been properly expanded into class fields.

## Architecture and Data Flow

The AST is the "central nervous system" of the compiler, with data flowing through it in a well-defined sequence of stages.

The process begins in the **Parsing** stage, where the [parser](../parser/design.md) transforms raw source text into a `File` node. This node contains one or more `LyricBlock` structures, allowing a single file to house multiple architectural modules. Once the raw AST is constructed, it enters the **Desugaring** phase. This is a series of idempotent transformations that expand high-level "sugar"—such as `relation` declarations and `field` definitions within interfaces—into explicit, lower-level AST nodes. For instance, a single `relation` might result in new fields being injected into multiple classes and the creation of a corresponding `impl` block.

Following desugaring, the AST undergoes **Validation** to confirm that the transformations were successful and the tree is semantically ready for the next stage. The validated AST is then passed to the **Semantic Analysis** stage. The [checker](../checker/design.md) performs name resolution and type inference, decorating the AST nodes with semantic information. Crucially, the AST nodes include `ResolvedType` and `InferredTypeArgs` fields (stored as `any` to prevent circular dependencies) which the checker populates. Finally, the fully annotated and desugared AST is consumed by the **Lowering** stage in the [lir](../lir/design.md) module, which flattens the tree into a low-level intermediate representation for code generation.

## Interface Implementations

The `ast` module is intentionally data-centric and does not implement complex behavioral interfaces. The nodes are designed to be passive structures that are acted upon by external visitors or transformation functions. The `File` type provides a `DocComment` utility method to retrieve contiguous comment blocks preceding a specific line, which is used by the [verifier](../verifier/design.md) and documentation generators to associate narrative text with code declarations.

## Public API

### Core Data Structures

The API is dominated by the tree's root and its primary branches. The `File` struct is the entry point, holding a collection of `LyricBlock` nodes and all source comments. Each `LyricBlock` acts as a container for the various declarations supported by Lyric: `StructDecl`, `EnumDecl`, `InterfaceDecl`, `ClassDecl`, `FuncDecl`, `RelationDecl`, `ImplBlock`, `TypeAliasDecl`, `ConstDecl`, `ImportDecl`, `DocBlock`, and `InvariantDecl`.

Type expressions are represented by the `TypeExpr` struct, which uses a `TypeExprKind` to distinguish between named types, optionals, unions, sequences, maps, tuples, function signatures, channels, generators, locks, and units. Similarly, the `Expr` and `Stmt` structs in `expr.go` use `ExprKind` and `StmtKind` respectively to represent the full breadth of the language's operational syntax, including complex constructs like `spawn`, `select`, `cascade`, and `lock`.

### Desugaring Engine

The module exports several high-level functions that perform the AST transformations, typically executed in a specific order:

- **`DesugarInterfaceEmbeds`**: Flattens embedded interfaces by copying fields and destructors from the embedded interface into the embedding interface, substituting type parameters as needed. Note that methods are not copied here to avoid duplication, as they are handled by the default implementation extraction.
- **`DesugarInterfaceFields`**: Converts abstract field declarations within interfaces into concrete getter and setter method signatures. This ensures that subsequent passes see them as standard interface requirements.
- **`DesugarRelations`**: Processes `relation` declarations by injecting labeled fields into the involved classes and generating or merging `impl` blocks that wire the interface methods to these fields. It uses label prefixing to avoid name collisions when a class participates in multiple relations of the same type.
- **`DesugarDefaultImpls`**: Extracts methods with bodies from interfaces and moves them to the top-level as standalone generic functions, adding the necessary relational `where` clauses to maintain their context.
- **`DesugarDestructors`**: Automatically generates `destroy` methods for classes involved in `owns` relations, incorporating cleanup logic defined in the associated interfaces. It performs deep copies of destructor bodies and renames method calls to match label-prefixed fields.

### Standard Library Integration

The `stdlib.go` file provides the mechanism for bringing the standard library into the compilation unit:

- **`MergeStdlib`**: This function takes the user's AST and a parsed version of the standard library. It performs a transitive usage analysis to identify which stdlib interfaces, classes, and functions are actually needed by the user's code (either directly or via relations) and merges only those into the first `LyricBlock` of the user's `File`.
- **`MergeFiles`**: Combines multiple `File` nodes into a single one, which is essential for multi-file compilation.
- **`FindStdlibDir`**: A utility to locate the standard library directory based on environment variables, executable location, or the current working directory.

### AST Validation

The `validate.go` file provides the `ValidatePostDesugar` function, which performs a suite of invariant checks:
- **Relation Field Injection**: Verifies that every `owns` relation has resulted in the expected parent field being injected into the child class.
- **Embed Flattening**: Ensures that no `embed` declarations remain in interfaces, confirming that `DesugarInterfaceEmbeds` has completed its work.
- **Consistency**: Checks that the tree is in a state where the type checker can proceed without encountering unexpected high-level sugar.

## Implementation Details

### Tagged Union Pattern

To maintain a flat and performant structure in Go, the AST utilizes a tagged union pattern for its most polymorphic nodes: `TypeExpr`, `Expr`, `Stmt`, and `Pattern`. Each of these structs contains a `Kind` enum and a `Data` field of type `any`. 

```go
type Expr struct {
    Kind         ExprKind
    Data         any
    Span         Span
    ResolvedType any
}
```

This approach avoids the overhead of interface method calls during traversal and allows consumers to use efficient type switches. The `Data` field is type-asserted based on the `Kind`, ensuring that the logic remains clear and localized.

### Source Mapping and Diagnostics

Every major node in the AST carries a `Span`, consisting of a start and end `Pos` (file, line, column). While this increases the memory footprint of the tree, it is a deliberate trade-off to enable high-fidelity error reporting. This metadata allows the checker and verifier to point to the exact location of a violation in the source code, and it provides the necessary foundation for future IDE features like "go to definition" and refactoring tools.

### Generative Desugaring Logic

The desugaring logic in `desugar.go` is designed to be generative and order-dependent. It must be executed before the checker because the checker expects a fully explicit representation of the program. 

1.  **Interface Embed Flattening**: Embedded interfaces are resolved first to ensure all fields and destructors are visible in the embedding interface.
2.  **Interface Field Expansion**: Fields in interfaces are turned into method signatures so that subsequent passes see them as standard interface requirements.
3.  **Relation Processing**: The relation pass uses the `rewriteFieldType` helper to map interface-level type parameters to the concrete types involved in a relation. It handles label prefixing to avoid name collisions when a class participates in multiple relations of the same type.
4.  **Default Implementation Extraction**: By moving default methods to the top level, the compiler simplifies the task of the checker, which can then treat these methods as regular generic functions with relational constraints.
5.  **Destructor Generation**: Finally, `destroy` methods are synthesized for classes, aggregating cleanup logic from all relevant relations. This pass uses `deepCopyBlock` and `substituteTypeParamsRichInBlock` to ensure that each class gets a unique, correctly typed version of the destructor logic.

### Selective Stdlib Merging

The `MergeStdlib` function implements a sophisticated reachability analysis. It doesn't just pull in every interface; it looks at `RelationDecl` hints to see which stdlib interfaces are being used to define relationships. It then transitively follows `embed` links. Similarly, it scans the user's code for named types and function calls, pulling in the corresponding stdlib definitions. This ensures that the resulting AST is as small as possible while still being semantically complete.

## Dependencies

The `ast` module is a leaf package in the Lyric internal hierarchy. It does not depend on any other `pkg/*` modules, which is essential as almost every other module in the system depends on `ast`. 

To avoid circular imports with the `checker` package, the `ResolvedType` and `InferredTypeArgs` fields are defined as `any`. This allows the checker to store its complex type structures within the AST without the AST needing to know about the checker's internal definitions.

## Technical Debt and Future Work

- **Visitor Pattern**: As the number of node types grows, the manual type-switching in consumers like the checker and LIR lowerer is becoming unwieldy. Implementing a standard Visitor or Walker pattern would centralize traversal logic.
- **Embed Removal**: Currently, `DesugarInterfaceEmbeds` copies content but does not explicitly clear the `Embeds` slice, which causes `ValidatePostDesugar` to report violations unless the slice is cleared elsewhere.
- **Serialization**: There is currently no support for serializing the AST to JSON or a binary format. This limits the ability to cache compilation results or provide the AST to external tools written in other languages.
- **In-place Mutation**: Desugaring modifies the AST in-place, which loses the original "sugared" representation. For tools that require high-fidelity source-to-source transformations, a non-destructive or "side-table" approach to desugaring might be necessary.
```

---

### ASSISTANT (gemini)

**Tool Call: `read_file`**
```json
{
  "file_path": "pkg/checker/design.md"
}
```

---

### USER (gemini)

**Tool Result:**
```
[ID: tool_ee5f2c76dd581b77]
File: /Users/bill/projects/lyric/pkg/checker/design.md (112 lines)

# Checker Module Design

The `checker` module implements the semantic analysis phase of the Lyric compiler. It is responsible for type checking, type inference, name resolution, and enforcing language-specific constraints on the desugared Abstract Syntax Tree (AST) produced by the `ast` module.

## Executive Summary

The `checker` module ensures that a Lyric program is semantically valid before it is passed to the backend for code generation. It performs a comprehensive, multi-pass analysis of the AST, resolving type expressions into concrete `Type` objects, tracking variable scopes, and verifying that all expressions and statements adhere to Lyric's type system rules.

Key features include structural subtyping for interfaces, numeric widening, automatic optional wrapping, and support for generic type inference using a unification-based approach. The checker also handles module imports, ensuring that symbols are correctly resolved across file boundaries while detecting circular dependencies. It enforces concurrency safety through `guarded_by` annotations and validates the exhaustiveness of `match` statements for enums and unions.

The checker acts as the "semantic truth" of the compiler, annotating AST nodes with `ResolvedType` information. This annotation is a critical contract consumed by subsequent phases, such as the LIR lowerer and backends, which rely on it for correct code generation without needing to re-perform semantic analysis.

## File Inventory

- [checker.go](checker.go): The primary implementation of the type checker. It defines the core `Type` system, `Scope` management, `Registry` for user-defined types, and the `Checker` struct which contains the logic for traversing and validating the AST. It also includes the pre-registration of Go standard library modules for interop.
- [checker_test.go](checker_test.go): A comprehensive suite of tests that verify the checker's behavior across a wide range of Lyric language features, including basic arithmetic, control flow, function calls, struct/class definitions, generic type inference, concurrency safety, and module imports.
- [checker.lyric](checker.lyric): A Lyric-language specification of the checker's own architecture and logic, serving as both documentation and a formal model of the module's behavior.

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

### Phase 3-5: Validation
After the primary checking passes, the checker performs several validation steps:
- **Phase 3**: Verifies that all function signatures are fully resolved and no `TyUnknown` types remain.
- **Phase 4**: Ensures that every expression in the AST has a `ResolvedType` annotation.
- **Phase 5**: Validates that all field accesses and method calls resolve to real members on the receiver type, catching any bugs that might have bypassed normal checking.

## Interface Implementations

The `checker` module does not explicitly implement external Go interfaces, but it provides the core logic that satisfies the semantic requirements of the Lyric language. It consumes the AST nodes defined in [pkg/ast](../ast/design.md) and uses the parser from [pkg/parser](../parser/design.md) to handle imports.

## Public API

### Core Types

- **`Type`**: Represents a resolved type in the Lyric system. It uses a `Kind` (e.g., `TyInt`, `TyFunc`, `TyStruct`, `TyUnion`) and additional metadata (e.g., bit width, element types, field lists, type parameters) to describe the type. It provides methods for structural equality (`Equal`) and string representation.
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

Lyric supports several forms of implicit type compatibility implemented in the `assignableTo` method. Numeric widening allows smaller integer types to be implicitly converted to larger ones (e.g., `i32` to `i64`). Optional wrapping ensures that a type `T` is always assignable to `T?`. Structural subtyping allows a class or struct to satisfy an interface if it provides all the required methods with matching signatures. Union types allow any member type to be assignable to the union, while the union itself is only assignable to a target if all its variants are compatible.

### Generic Type Inference

The checker implements a unification-based inference for generic functions. When a generic function is called without explicit type arguments, the `inferTypeArgs` method walks the parameter types and actual argument types in parallel. It binds type variables to concrete types from the arguments, with the first match winning. The inferred types are then substituted into the function's return type and stored in the AST for the transpiler.

### Relational Constraints

Lyric supports relational constraints through `where` clauses, such as `where Graph<G, N, E>`. When such a constraint is encountered, the checker looks up the corresponding interface and "grants" its methods to the involved type variables. This is implemented by populating a `typeVarMethods` map, which allows methods defined in the interface to be called on variables of the constrained types within the function body.

### Concurrency and Guarded Fields

The checker enforces safety rules for concurrent access by tracking "held locks" within a scope. These locks are populated by `lock` statements. The checker verifies that any field annotated with `guarded_by` is only accessed when the corresponding lock is held, preventing unprotected access to shared state. The `extractLockName` helper identifies the lock being used, supporting both simple identifiers and field accesses (e.g., `self.mu`).

### Exhaustiveness Checking

For `match` statements operating on `enum` or `union` types, the checker verifies that all possible variants or member types are covered by the match arms. If a wildcard pattern is not present, the checker reports an error listing the missing variants or types, ensuring that all cases are handled at compile time.

### Built-in Methods and Functions

The checker provides specialized resolution for methods on built-in types. Strings support operations like `len`, `contains`, and `split`. Lists provide `push`, `pop`, and `reverse`. Maps offer `contains_key`, `keys`, and `values`. Channels support `send`, `receive`, and `close`. These are resolved during the method call checking phase before falling back to general method lookup.

## Dependencies

- **[pkg/ast](../ast/design.md)**: The checker consumes the AST and stores results back into it.
- **[pkg/parser](../parser/design.md)**: Used to parse imported module files.

## Technical Debt and Future Work

- **Advanced Inference**: The current type inference is relatively simple and may need to be replaced with a more robust constraint-solver-based approach for complex scenarios involving nested generics or circular constraints.
- **Method Resolution**: Method lookup currently relies on a simple name-based search. Support for method overloading or more complex trait-based dispatch might be needed in the future.
- **Performance**: For very large projects, the recursive module checking and deep AST traversal could become a bottleneck. Implementing incremental checking or a more parallelized approach could improve performance.
- **Circular Type Definitions**: While module cycles are detected, circular type definitions need careful handling to avoid infinite recursion during size calculation or equality checks.
```

---

### ASSISTANT (gemini)

**Tool Call: `read_file`**
```json
{
  "file_path": "pkg/lir/design.md"
}
```

---

### USER (gemini)

**Tool Result:**
```
[ID: tool_38c5c38adbac8a9c]
File: /Users/bill/projects/lyric/pkg/lir/design.md (114 lines)

# LIR Module Design

## Executive Summary

The `lir` (Low-level Intermediate Representation) module is the architectural bridge between the high-level, semantically rich Abstract Syntax Tree (AST) and the target-specific code generators of the Lyric compiler. Its primary mission is to resolve the semantic complexity of the Lyric language—such as generics, optional types, tagged unions, and method dispatch—into a simplified, flat representation. This transformation ensures that backends can remain "thin" emitters, focused primarily on syntax mapping rather than complex semantic resolution.

The LIR is built on four foundational design principles. First, it preserves **structured control flow** (if, while, for, switch) instead of decomposing logic into basic blocks and jumps, which is essential for generating idiomatic high-level target code. Second, it enforces **flat expressions**, where every sub-expression is assigned to a unique, named temporary variable, ensuring that backends never encounter nested expression trees. Third, it performs **semantic resolution**, lowering high-level Lyric features into explicit LIR constructs. Finally, it is **backend agnostic**, providing a common foundation for multiple targets while offering optional passes like monomorphization for backends that require it.

## File Inventory

*   [lir.go](lir.go): Defines the core data structures for the LIR, including the type system (`LType`), operands (`LValue`), expressions (`LExpr`), and structured statements (`LStmt`).
*   [lower.go](lower.go): Implements the `Lowerer`, the primary engine that transforms a type-checked AST into an LIR program. This file contains the bulk of the semantic lowering logic, including expression flattening and control flow transformation.
*   [monomorphize.go](monomorphize.go): Implements a comprehensive monomorphization pass that specializes generic functions, classes, and structs into concrete versions for backends like C that do not support generics natively.
*   [optimize.go](optimize.go): Performs post-lowering LIR-to-LIR optimizations, such as side-effect temporary elimination, multi-return destructuring, and unused variable removal.
*   [validate.go](validate.go): Provides invariant checkers that verify the integrity of the LIR program after lowering and monomorphization passes.
*   [c_backend.go](c_backend.go): The C code generator, which transforms monomorphized LIR into C code, utilizing a dedicated runtime library and implementing complex features like generator state machines and interface vtables.
*   [lir.lyric](lir.lyric): A Lyric specification file that formally defines the LIR module's structure and interfaces, used by the verifier to ensure implementation integrity.
*   [lower_test.go](lower_test.go): Unit tests for the lowering pass, verifying the transformation of various AST constructs.
*   [lower_extra_test.go](lower_extra_test.go): Additional tests for the lowering pass, covering edge cases and complex scenarios.
*   [c_backend_test.go](c_backend_test.go): Tests for the C code generator, verifying the emission of valid C code.

## Architecture and Data Flow

The `lir` module operates as a sophisticated transformation pipeline that converts high-level architectural intent into executable source code. The flow of data through the module follows a strictly defined sequence of stages.

The pipeline begins with the **Lowering** stage. The `Lowerer` consumes a type-checked `ast.File` from the [pkg/ast](../ast/design.md) module and a `checker.Registry` from the [pkg/checker](../checker/design.md) module. It performs a recursive walk of the AST, flattening every nested expression into a sequence of single-operation assignments to named temporaries. During this process, high-level constructs are desugared into their LIR equivalents; for instance, a `match` statement on an enum is lowered into a `switch` on an integer variant tag, with explicit field extractions for any bound variables.

Once the initial LIR is generated, it enters the **Optimization** stage. The `Optimize` function runs several passes over the LIR to clean up the output of the lowerer. These passes are essential because the lowerer often produces redundant temporaries or patterns that are difficult for backends to emit efficiently. Key optimizations include fusing void-context calls into bare statements (side-effect elimination) and destructuring multi-return values from single temporaries into separate variables.

For backends that require it, such as C, the pipeline includes an optional **Monomorphization** stage. The `Monomorphize` function scans the LIR program for all instantiations of generic types and functions. It then creates specialized, non-generic copies for each unique set of type arguments and rewrites all call sites and type references to use these specialized versions. This ensures that the final LIR contains no type variables before it reaches the backend.

The final stage is **Code Generation**. A backend-specific emitter, currently the C backend, walks the optimized (and potentially monomorphized) LIR tree and produces the final source code string. Because the LIR has already resolved all semantic complexity, these backends are primarily responsible for syntax mapping and utilizing their respective runtime libraries.

## Interface Implementations

The `lir` module does not explicitly implement interfaces defined in other packages, as its primary role is data transformation. However, it is a heavy consumer of the data structures and semantic information provided by:
*   [pkg/ast](../ast/design.md): The module provides the input `ast.File` and all its constituent nodes.
*   [pkg/checker](../checker/design.md): The module provides the `checker.Registry` and the `checker.Type` information used to resolve names and types during the lowering process.

## Public API

The `lir` module exposes a clean, functional API for interacting with the LIR pipeline. The primary entry points are:

*   `NewLowerer()`: This function creates a new instance of the `Lowerer`, initializing the internal state required for AST-to-LIR transformation.
*   `Lowerer.Lower(file *ast.File) *LProgram`: This is the "front door" to the module. It takes a type-checked AST file and returns a complete `LProgram` structure representing the lowered code.
*   `Optimize(prog *LProgram)`: This function runs the full suite of LIR-to-LIR optimization passes on a program, mutating it in place to improve code quality and backend compatibility.
*   `Monomorphize(prog *LProgram)`: This function specializes all generic declarations in the program, removing all type variables. It is a prerequisite for the C backend.
*   `ValidatePostLower(prog *LProgram)` and `ValidatePostMono(prog *LProgram)`: These functions check for invariant violations at different stages of the pipeline, ensuring that types like `LTyAny` or `LTyTypeVar` do not propagate where they are not allowed.
*   `EmitC(prog *LProgram) string`: This function takes a monomorphized LIR program and generates a complete C source code string, relying on the `lyric_runtime.h` contract.

## Implementation Details

### LIR Data Structures

The LIR is defined by four primary categories of data structures:

*   **LType**: Represents the Lyric type system in a form suitable for code generation. It includes primitive types (integers, floats, bool, string), unit/void, error, and complex types like `LTyStruct`, `LTyClassHandle` (heap-allocated), `LTyTaggedUnion` (enums), `LTySlice`, `LTyMap`, `LTyChannel`, `LTyGenerator`, `LTyOptional`, and `LTyErrorResult`. It also preserves `LTyTypeVar` for generic support.
*   **LValue**: The operands of the LIR. These are flat and include named variables (`LValVar`), SSA-like temporaries (`LValTemp`), global variables (`LValGlobal`), and various literals (int, uint, float, string, bool, null).
*   **LExpr**: Right-hand side expressions that are always bound to a temporary via `LTempDef`. They reference only `LValue` operands, never nested expressions. Kinds include binary/unary operations, casts, field access, calls (regular, method, builtin), and construction (struct, class, slice, map, channel).
*   **LStmt**: Structured statements that form the program's control flow. Kinds include variable management (`LStmtTempDef`, `LStmtVarDecl`, `LStmtAssign`), field/index mutation (`LStmtStructSet`, `LStmtClassSet`, `LStmtIndexSet`), and structured control flow (`LStmtIf`, `LStmtWhile`, `LStmtFor`, `LStmtSwitch`, `LStmtTypeSwitch`, `LStmtBlock`, `LStmtReturn`, `LStmtBreak`, `LStmtContinue`). It also includes concurrency primitives (`LStmtSpawn`, `LStmtLock`, `LStmtSend`, `LStmtSelect`, `LStmtYield`).

### The Lowering Pass

The `Lowerer` is the most complex component of the module, responsible for the heavy lifting of semantic resolution. It maintains a stateful mapping of AST nodes to LIR constructs and tracks visibility, imports, and type information. A central feature of the lowerer is **expression flattening**. Every sub-expression in the AST is assigned to a unique temporary variable (`LValTemp`). This ensures that all `LExpr` instances in the LIR reference only simple operands, never nested expressions. For example, `a + b * c` is lowered into `%0 = b * c` followed by `%1 = a + %0`.

Control flow lowering in Lyric LIR is **structured**. Instead of decomposing logic into basic blocks and jumps, the LIR uses high-level statements like `LStmtIf` and `LStmtWhile`. The `LStmtWhile` is particularly noteworthy; it contains a `CondBlock`—a list of statements executed before each iteration to compute the loop condition. This elegantly handles complex conditions that might involve side effects or multiple steps, ensuring they are re-evaluated correctly on every iteration without duplicating code.

The `Lowerer` also handles the desugaring of Lyric's high-level types:
*   **Enums**: These are lowered to `LTyTaggedUnion`. Match statements are converted to switches on the variant tag, with the lowerer generating the necessary logic to extract variant fields into local bindings.
*   **Classes**: These are lowered to `LTyClassHandle`, representing a pointer to a heap-allocated struct. Field access is converted to explicit `LExprClassGet` and `LStmtClassSet` operations.
*   **Generics**: The lowerer preserves type parameters as `LTyTypeVar`.
*   **Impl Blocks**: The lowerer generates forwarding wrapper methods for `impl` blocks. This enables classes to satisfy interfaces even when method names or signatures do not match exactly, by creating a bridge between the interface's expected signature and the class's actual implementation.
*   **Try Operator**: The `?` operator is lowered into a sequence of operations: extracting the error from a result, checking if it is non-null, and performing an early return if an error is present, followed by extracting the success value.

### The Optimization Pass

The optimizer runs after lowering to refine the LIR. One of its most critical tasks is **side-effect temporary elimination**. The lowerer often assigns the result of every call to a temporary, even if the result is immediately discarded (e.g., in a void-context call). The optimizer identifies these patterns and fuses them into `LStmtSideEffect` statements, which backends can emit as bare expressions.

Another vital optimization is **multi-return destructuring**. Lyric allows functions to return multiple values, such as `(T, error)`. The lowerer initially represents this as a single temporary of type `LTyErrorResult`. The optimizer identifies patterns where this temporary is immediately destructured into separate variables and replaces them with a single `LStmtMultiAssign`. This simplifies target code generation for multi-return values.

### Monomorphization

For backends like C that lack native generic support, monomorphization is mandatory. The pass operates in six distinct phases:

1.  **Indexing**: It indexes all generic declarations (functions, structs, classes) in the program.
2.  **Collection**: It performs a deep walk of all function bodies to collect every unique instantiation of generic functions, classes, and structs based on the concrete type arguments used at call sites.
3.  **Specialization**: It generates specialized copies of these declarations, mangling their names (e.g., `Stack_i32`) to ensure uniqueness. This involves deep cloning the function bodies and substituting type variables with concrete types.
4.  **Rewriting**: It rewrites all call sites throughout the program to use these specialized, non-generic versions.
5.  **Signature/Field Rewrite**: It rewrites function signatures and field types to use the mangled names of specialized types.
6.  **Bare Name Resolution**: It resolves any remaining bare generic class names in specialized function bodies using a per-function rename map.

A critical aspect of monomorphization in Lyric is handling **Impl Method Renames**. When a class implements an interface through an `impl` block, the lowerer may generate forwarding wrappers or rename methods to avoid collisions (especially in C). The monomorphizer tracks these renames in `prog.ImplMethodRenames` and ensures that specialized versions of default interface methods call the correct concrete method names on specialized classes.

### The C Backend

The **C Backend** is significantly more involved due to C's lack of high-level features. It relies on a small runtime header (`lyric_runtime.h`) that defines macros and structures for slices, optional types, and error results. It hoists lambdas to top-level static functions and uses `malloc` for class allocations.

One of the most complex transformations in the C backend is the **Generator State Machine**. Lyric generators are lowered into a stateful struct and two functions: an `_init` function that allocates the state and copies parameters, and a `_next` function that executes the body. The `_next` function uses a `switch` statement with `goto` labels to resume execution from the last `yield` point. Local variables that must persist across yields are hoisted into the generator's state struct.

The C backend also implements **Interface VTable Dispatch**. For each interface, it generates a vtable struct containing function pointers for each method. Boxed interface values are represented as a struct containing a data pointer and a vtable pointer. Static vtable instances are generated for each (class, interface) pair, and method calls on interface types are dispatched through these vtables.

## Dependencies

*   [pkg/ast](../ast/design.md): Provides the source AST and node definitions.
*   [pkg/checker](../checker/design.md): Provides the type registry and resolved type information.
*   [runtime](../../runtime/design.md): Provides the C runtime contract for the C backend.

## Technical Debt and Future Work

*   **Map Support in C**: The C backend currently lacks implementation for map types.
*   **Concurrency in C**: While basic spawn support exists, more advanced concurrency features like select are not yet fully supported in the C backend.
*   **Advanced Optimizations**: Future improvements could include constant folding, dead code elimination beyond unused temporaries, and function inlining.
*   **Relational Constraints**: While basic support exists, more complex scenarios involving multi-class interfaces may require further refinement.
```

---

### ASSISTANT (gemini)

**Tool Call: `read_file`**
```json
{
  "file_path": "pkg/verifier/design.md"
}
```

---

### USER (gemini)

**Tool Result:**
```
[ID: tool_032e3a4bc9d92600]
File: /Users/bill/projects/lyric/pkg/verifier/design.md (88 lines)

# Verifier Module Design

The `verifier` module serves as the structural integrity engine for the Lyric project, acting as a guardian against architectural drift. It performs a deep, recursive comparison between the high-level architectural intent declared in `.lyric` files and the actual implementation in Go source code. By bridging two distinct type systems, it ensures that the developer's mental model—captured in documentation and specifications—remains synchronized with the evolving codebase.

## Executive Summary

The `verifier` module provides a multi-stage analysis pipeline that validates Go source code against Lyric specifications. It parses `.lyric` files, discovers referenced Go source files, extracts their type information using the Go standard library's AST tools, and performs a structural comparison. The verifier identifies mismatches in fields, methods, function signatures, and naming conventions, reporting them with varying severity levels (Error, Warning, Info). A key feature is its "completeness check," which ensures that all exported Go symbols are documented in the corresponding Lyric block, preventing the specification from becoming an incomplete or outdated subset of the implementation. Beyond simple drift detection, the module also serves as a host for project-wide invariant testing, using its AST analysis capabilities to enforce critical architectural rules that cannot be captured by the type system alone.

## File Inventory

- [verifier.go](verifier.go): The primary implementation file containing the verification logic, Go type extraction, and structural comparison algorithms. It defines the `Verify` entry point and the `Result` and `Finding` types.
- [invariants_test.go](invariants_test.go): A specialized test suite that enforces cross-cutting project invariants by analyzing the Go AST of other modules. It ensures that critical architectural rules, such as the value-type nature of AST nodes, are maintained across the codebase.
- [verifier_test.go](verifier_test.go): Comprehensive test suite that validates drift detection, naming convention mapping, and completeness checks against both synthetic examples and the project's own specifications.
- [verifier.lyric](verifier.lyric): The module's own architectural specification, providing a self-documented model of its internal logic and public API.

## Architecture and Data Flow

The verification process is designed as a stateless operation from the perspective of the caller, although it maintains significant internal state during a single run to aggregate information across multiple files. The workflow begins by delegating to the [pkg/parser](../parser/design.md) module to produce a `lyricast.File` from the provided `.lyric` source path. The verifier then iterates through each `lyric` block within the file, inspecting the `source:` annotations to locate the relevant Go files or directories. These paths are resolved relative to the directory containing the `.lyric` file itself, ensuring that the specification and its implementation are co-located or predictably linked.

For each discovered source, the verifier utilizes the `go/parser` and `go/ast` packages from the Go standard library to extract detailed type information. This data is populated into a `goTypeInfo` registry, which serves as a comprehensive database of all structs, interfaces, functions, and type definitions found across all specified sources for a given block. This aggregation is a critical design choice, as a single Lyric block's implementation may span multiple Go files within the same package. The `mergeGoInfo` function handles the complex task of combining information from multiple files, ensuring that methods defined in different files for the same struct are correctly associated and that field definitions are unified.

Once the Go type registry is fully populated, the verifier recursively walks the Lyric AST, comparing each declaration—including structs, classes, interfaces, enums, and functions—against the aggregated Go types. It normalizes naming conventions and type expressions on the fly to enable direct comparison between the two systems. After validating the declared Lyric symbols, the verifier performs a reverse check via `verifyCompleteness`. This function walks all exported Go symbols in the source files to identify any that are missing from the Lyric specification, ensuring the documentation remains a complete representation of the public API. Findings are collected into a `Result` object, which classifies each discrepancy by severity and provides detailed messages and file locations.

## Interface Implementations

The `verifier` module is a standalone utility and does not implement interfaces defined in other packages. It acts as a consumer of the AST produced by [pkg/ast](../ast/design.md) and [pkg/parser](../parser/design.md) and provides its own reporting structures for use by CLI tools and CI pipelines.

## Public API

- `Verify(lyricPath string) (*Result, error)`: The primary entry point. Orchestrates the entire pipeline, from parsing the Lyric file and discovering Go sources to extracting types and performing the final comparison.
- `Result`: A container for the findings of a verification run.
    - `ErrorCount() int`: Returns the number of error-level findings, used to determine build failure.
    - `Findings []Finding`: The list of individual discrepancies found.
- `Finding`: Represents a single discrepancy.
    - `Severity`: Classifies the finding as `Error`, `Warning`, or `Info`.
    - `LyricFile`, `GoFile`: Locations of the discrepancy.
    - `Message`: Descriptive text explaining the mismatch.
- `Severity`: An enumeration of `Error`, `Warning`, and `Info`.

## Implementation Details

### Deep Type Comparison

The core of the verifier's logic resides in the `lyricTypeToGoString` function, which recursively converts Lyric `TypeExpr` nodes into a Go-compatible string format. This transformation allows the verifier to compare types across the two systems using string equality. Key mappings include converting Lyric optional types (`T?`) to Go pointers (`*T`), sequences (`[T]`) to Go slices (`[]T`), and maps directly to their Go equivalents. Lyric tuples are used to represent Go's multiple return values and are unpacked during comparison to be checked element-wise against the Go function's return list. Lyric `unit` represents a void return or an empty parameter list.

The `typesMatch` function serves as the final arbiter of type equality. It handles the equivalence of `any` and `interface{}` and employs a `stripPackagePrefix` helper to heuristically remove package qualifiers, such as converting `ast.File` to `File`. This allows Lyric files to use unqualified names while the Go implementation uses fully qualified types. To avoid false positives, the function returns true if either side contains a "?" character, which represents an unconvertible or unknown type.

### Name Mapping and Normalization

Lyric typically uses `snake_case` for identifiers, while Go uses `PascalCase` for exported symbols and `camelCase` for unexported ones. The verifier bridges this gap by attempting three-way matching in functions like `findGoName` and `findGoFieldType`. It checks for the `PascalCase` version (via `snake_to_pascal`) for exported fields and methods, the `camelCase` version (via `snake_to_camel`) for unexported identifiers, and finally an exact match. Lyric primitive names are also mapped to their Go equivalents, such as `i32` to `int32` and `u8` to `uint8`.

### Verification Rules

The verifier applies specific rules based on the type of declaration encountered. For structs and classes, it checks for field existence and type matching. For classes, it additionally verifies method signatures, comparing parameter counts (excluding the `self` parameter) and ensuring that positional types and return types match. Interface verification ensures that every method declared in Lyric exists in the Go interface with a matching signature. Enums are validated against three common Go patterns: typedefs, structs, or interfaces, and the verifier also checks for the common `XxxKind` naming pattern. Functions undergo positional parameter and return type comparison, with multi-return Go functions mapped to Lyric tuple returns.

### Completeness Check

The `verifyCompleteness` function ensures that the Lyric specification is a complete model of the public API. It identifies all exported Go symbols—types and functions—in the referenced source files and reports an error for any that are not documented in the Lyric block. It uses the `isExported` helper, which checks for an uppercase first letter, to distinguish between the public API and internal implementation details.

## Project Invariants

A unique responsibility of the `verifier` module, implemented in `invariants_test.go`, is the enforcement of project-wide architectural invariants. These are rules that cannot be easily enforced by the Go type system but are critical for the correct operation of the compiler. The module uses the Go standard library's AST tools to inspect the source code of other modules and verify these rules.

Key invariants currently enforced include:
- **AST Value Types**: Ensuring that `ast.Expr` slices are never iterated using range-copy (e.g., `for _, x := range slice`), which would copy the value type and lose checker annotations. Instead, index-based iteration (`&slice[i]`) must be used.
- **Monomorphization Safety**: Verifying that field and type lookups in the backend correctly check `ClassRenames` to account for monomorphized generic types.
- **Standard Library Integration**: Ensuring that the standard library is merged into the correct AST block.
- **Desugaring Order**: Enforcing the specific sequence of desugaring passes (Embeds → InterfaceFields → Relations → Destructors → DefaultImpls) to ensure correct transformation of high-level constructs.
- **Checker Multi-Phase Structure**: Validating that the checker maintains its multi-phase resolution strategy, which is essential for handling cross-file type dependencies.

## Dependencies

- **[pkg/ast](../ast/design.md)**: Provides the `LyricBlock` and `TypeExpr` structures used to represent the Lyric specification.
- **[pkg/parser](../parser/design.md)**: Used to parse the `.lyric` file into an AST.
- **Go Standard Library**:
    - `go/ast` and `go/parser`: Used to analyze the Go source code and extract type information.
    - `go/token`: Used for position tracking in Go files.
    - `os` and `path/filepath`: Used for file system operations and path resolution.

## Technical Debt and Future Work

- **Expression Verification**: Currently, `requires` and `ensures` clauses are treated as raw strings and are not parsed or verified against the Go implementation. Future versions could use a Go expression parser to validate these constraints.
- **Complex Pointer Indirection**: While basic optional-to-pointer mapping works, deeply nested or complex pointer structures may occasionally cause false positives or be represented as `?`.
- **Import Resolution**: The `stripPackagePrefix` mechanism is heuristic and does not perform full symbol resolution, meaning it cannot distinguish between types with the same name in different packages.
- **Performance**: The verifier re-parses Go files for every Lyric block. For large projects, a caching mechanism for parsed Go ASTs and `goTypeInfo` would significantly improve performance.
- **Complex Function Types**: Complex function types, such as maps containing functions, cannot currently be fully expressed in Lyric and may be typed as `any` or `?`, leading to known false-positive errors in some edge cases.
- **Generic Constraints**: While basic generic types are supported, complex constraints are not currently compared.
```

---

### ASSISTANT (gemini)

**Tool Call: `read_file`**
```json
{
  "file_path": "runtime/design.md"
}
```

---

### USER (gemini)

**Tool Result:**
```
[ID: tool_680553d570bc0199]
File: /Users/bill/projects/lyric/runtime/design.md (118 lines)

# Runtime Module Design

## Executive Summary

The `runtime` module provides the foundational C runtime library required by the Lyric compiler's C backend. It is implemented as a passive, header-only library that establishes the memory layout and operational semantics for Lyric's high-level constructs in a C environment. By providing a suite of macros and inline functions, the runtime enables the C backend to transform complex Lyric features—such as dynamic slices, length-prefixed strings, optional types, error results, tagged unions, and concurrency primitives—into efficient and readable C code. Its primary goal is to bridge the gap between Lyric's expressive type system and C's low-level primitives while maintaining semantic equivalence and providing a robust execution environment.

## File Inventory

- [lyric_runtime.h](lyric_runtime.h): The core runtime header. It contains all macro definitions, type structures, and inline helper functions that constitute the Lyric execution environment for C targets. It defines the fundamental data structures for slices, strings, optionals, and results, as well as the logic for concurrency and file I/O.

## Architecture and Data Flow

The `runtime` module is a passive component that does not maintain internal state or have an independent execution flow. Instead, it acts as a contract and a toolkit for the code generated by the [LIR module](../pkg/lir/design.md).

The architecture leverages C macros to implement a form of "template-based" type generation. Since C lacks native support for generics, the Lyric C backend uses these macros to define concrete, specialized struct types for every unique instantiation of a generic Lyric type (e.g., `Slice_int32_t`, `Opt_string`, or `Chan_int32_t`). This approach ensures type safety at the C level and allows the compiler to generate code that is tailored to specific data types. Data flows through these structures as they are passed between functions, with the runtime providing the logic for common operations like resizing slices, wrapping values in optional containers, or managing thread-safe communication through channels.

## Interface Implementations

As a C header library, this module does not implement any Go interfaces. It serves as the target execution environment for the C code generator within the [LIR module](../pkg/lir/design.md).

## Public API

The Public API of the runtime consists of the macros and inline functions intended to be utilized by the Lyric-generated C code. These are categorized by the Lyric language features they support.

### Dynamic Slices
Lyric slices are implemented as dynamic arrays with automatic resizing capabilities and support for sub-slicing.
- `LYRIC_SLICE_DEF(ElemType, SliceName)`: A macro that defines a struct containing a pointer to the data, the current length, and the total capacity.
- `lyric_slice_empty(SliceName)`: Initializes a slice structure to an empty state (null data, zero length and capacity).
- `lyric_push(slice_ptr, elem, SliceName)`: Appends an element to the slice. If the length reaches the capacity, it performs a geometric resize (doubling the capacity) using `realloc`.
- `lyric_pop(slice_ptr)`: Removes and returns the last element of the slice. The caller is responsible for ensuring the slice is not empty.
- `lyric_subslice(slice, low, high, SliceName)`: Creates a new slice view representing the range `[low, high)`. This view shares the underlying data pointer with the original slice.
- `lyric_slice_lit(SliceName, ElemType, ...)`: A convenience macro that creates a new slice from a C initializer list, allocating heap memory for the contents.
- `lyric_contains(slice, elem)`: Performs a linear search through the slice to determine if a specific element is present.

### Strings
Strings in Lyric are implemented as length-prefixed byte slices (`lyric_string`), which is an alias for `LyricSlice_uint8_t`. This allows strings to contain embedded null bytes and ensures that length-related operations are O(1).
- `LYRIC_STR(lit)`: A macro to create a `lyric_string` from a C string literal at compile-time.
- `lyric_str_from_cstr(cstr)`: Creates a `lyric_string` from a null-terminated C string, copying the data to the heap.
- `lyric_str_from_bytes(data, len)`: Creates a `lyric_string` from raw bytes, copying them to the heap and adding a hidden trailing null for C interop.
- `lyric_str_eq(a, b)`: Performs a length-aware equality check.
- `lyric_str_cmp(a, b)`: Performs a lexicographic comparison.
- `lyric_str_concat(a, b)`: Concatenates two strings into a new heap-allocated `lyric_string`.
- `lyric_str_contains`, `lyric_str_index_of`, `lyric_str_has_prefix`, `lyric_str_has_suffix`: Length-aware string searching and prefix/suffix checks.
- `lyric_str_replace`, `lyric_str_repeat`, `lyric_str_join`: High-level string transformations that return new heap-allocated strings.
- `lyric_str_trim`: Returns a new string with leading and trailing whitespace removed.
- `lyric_fprint_str(file, s)`: Writes a `lyric_string` to a `FILE*` in a length-aware manner.

### Optional Types
Optional types (`T?`) are represented as a struct containing a boolean flag (`has`) and the actual value.
- `LYRIC_OPT_DEF(ElemType, OptName)`: Defines a specialized optional struct for a given type.
- `lyric_some(value, OptName)`: Wraps a concrete value into an optional struct with the `has` flag set to true.
- `lyric_none(OptName)`: Returns an optional struct with the `has` flag set to false.
- `lyric_isnull(opt)`: A helper to check if an optional value is empty.
- `lyric_unwrap(opt)`: Extracts the value from an optional. This should only be called after verifying the value exists.

### Error Results
Lyric's idiomatic `(T, error)` return pattern is mapped to a result struct in C.
- `LYRIC_RESULT_DEF(ElemType, ResultName)`: Defines a result struct containing a success value, an error message (`const char*`), and a boolean `is_err` flag.
- `lyric_ok(val, ResultName)`: Constructs a successful result containing the provided value.
- `lyric_err(msg, ResultName)`: Constructs an error result containing the provided error message.
- `lyric_is_err(r)`: Checks if a result represents a failure.

### Concurrency Primitives
The runtime provides support for Lyric's concurrency model using POSIX threads (pthreads).
- `LYRIC_CHAN_DEF(ElemType, ChanName)`: Defines a thread-safe, buffered or unbuffered channel structure.
- `LYRIC_CHAN_IMPL(ElemType, ChanName, Suffix)`: Generates the implementation functions for a specific channel type, including `make`, `send`, `recv`, `close`, and `free`.
- `lyric_spawn(func, arg)`: Launches a function in a new detached thread, implementing the Lyric `spawn` keyword.

### Utility Helpers
- `lyric_hash_string`: Implements the FNV-1a hashing algorithm for strings.
- `lyric_sprintf`: A wrapper around `vsnprintf` that automatically calculates the required size and returns a heap-allocated formatted string.
- `lyric_bool_str`: Returns "true" or "false" for a boolean value.
- `lyric_toupper`, `lyric_tolower`: Convert strings to upper or lower case, returning new heap-allocated strings.
- `lyric_char_to_string`: Converts a single character (uint8_t) to a heap-allocated string.
- `lyric_read_file`: Reads an entire file into a heap-allocated string, returning a `lyric_str_bool_t` struct.
- `lyric_write_file`: Writes a string to a file, returning a boolean indicating success.
- `lyric_getwd`: Returns the current working directory.
- `lyric_path_dir`, `lyric_path_base`, `lyric_path_ext`: Utilities for working with file paths.
- `lyric_itoa`: Converts an `int64_t` to a heap-allocated string.
- `lyric_atoi`: Parses a string into an `int64_t`, returning a `lyric_atoi_result` struct.
- `lyric_parse_float`: Parses a string into a `double`, returning a `lyric_parse_float_result` struct.

### Tagged Unions
For ad-hoc union types (e.g., `string | i32`), the runtime provides a generic `LyricUnion` struct.
- `LyricUnion`: A struct containing an integer tag and a union of supported Lyric types (i32, i64, f32, f64, bool, string, and pointers).
- `LYRIC_UNION_TAG_*`: Constants for identifying the active member of a `LyricUnion`.
- `lyric_union_i32`, `lyric_union_string`, etc.: Specialized constructor functions that initialize a `LyricUnion` with the correct tag and value.

## Implementation Details

### Memory Management
The runtime currently utilizes the standard C library's `malloc`, `realloc`, and `free` for memory management. A critical implementation detail is that the runtime does not yet include a garbage collector. Consequently, heap-allocated objects—including slices, strings created by helper functions, and class instances—are currently leaked during execution. This is a known limitation that is acceptable for short-lived compiler tasks but will require a GC (such as Boehm-Demers-Weiser) for broader use.

### Slice Growth Strategy
The `lyric_push` macro implements a geometric growth strategy to ensure amortized O(1) append operations. When a slice's length reaches its capacity, the capacity is doubled. If the slice was previously empty, it is initialized with a default capacity of 4. This minimizes the number of reallocations required as a slice grows.

### String Handling
Strings in the Lyric C runtime are represented by the `lyric_string` struct, which contains a pointer to the data and its length. This design allows for O(1) length retrieval and supports strings with embedded null bytes. For compatibility with standard C functions that expect null-terminated strings, heap-allocated `lyric_string` objects carry a hidden trailing `\0` byte immediately following the data. This trailing byte is not included in the `.len` field.

### Concurrency Implementation
Channels are implemented using a circular buffer protected by a `pthread_mutex_t`. Two condition variables (`not_empty` and `not_full`) are used to manage blocking send and receive operations. `lyric_spawn` uses `pthread_create` and `pthread_detach` to provide fire-and-lyrict thread execution.

### Tagged Union Dispatch
The `LyricUnion` type uses an integer tag to identify the active member of its internal union. This allows the generated C code to perform safe type dispatching (e.g., in `match` statements) by checking the tag before accessing the union's data.

## Dependencies

- **Standard C Library**: The runtime depends on standard headers including `<stdio.h>`, `<stdlib.h>`, `<stdint.h>`, `<stdbool.h>`, `<string.h>`, `<stdarg.h>`, `<ctype.h>`, and `<unistd.h>`.
- **POSIX Threads**: The concurrency primitives require the `pthread` library.
- **LIR Module**: This module is a critical dependency for the C backend in [pkg/lir](../pkg/lir/design.md), which relies on the macros and types defined here to emit valid C code.

## Technical Debt and Future Work

- **Garbage Collection**: Integrating a garbage collector is the highest priority to prevent memory leaks in long-running Lyric programs compiled to C.
- **Map Implementation**: A robust hash map implementation is required to support Lyric's `map<K, V>` type in the C backend.
- **Error Handling Refinement**: The current error result uses a simple string for error messages. This should be expanded to support more complex, structured error types as the language evolves.
- **Thread Safety**: While channels are thread-safe, other runtime primitives (like slices) are not and will require synchronization if shared across threads.
```

# Task: Synthesize Project Architecture

Please synthesize a high-level `project-design.md` by analyzing the individual `design.md` files from all modules. This document will be our primary map for navigating this project in the future.

## IMPORTANT: Existing Documentation Found

An existing `project-design.md` was found at the project root. **You MUST read it first.**
Your goal is to **upgrade and improve** this document, not degrade it. Preserve its depth, architectural insights, and specific details while incorporating new information from the module designs.

## Project Context

- Project Path: /Users/bill/projects/lyric
- Total Modules: 6

## Modules to Analyze

- ast: pkg/ast/design.md
- checker: pkg/checker/design.md
- lir: pkg/lir/design.md
- parser: pkg/parser/design.md
- verifier: pkg/verifier/design.md
- runtime: runtime/design.md

## Your Strategy

1. **Read Existing Design**: Read the current `project-design.md` at the root.
2. **Read Module Designs**: Read the `design.md` file for every module listed above.
3. **Identify Gaps**: Compare the existing project design with the new module-level details.
4. **Upgrade Narrative**: Synthesize a superior `project-design.md` that preserves the best of the old while adding the precision of the new.
5. **Module Map**: Include a **categorized tree view** of all modules, with each entry being a **clickable markdown link** to its `design.md` file (e.g., `[pkg/scanner](pkg/scanner/design.md)`).
6. **Use Tools**: Use `write_file` and `edit_file` to build the document in sections. Do NOT output the whole document at once.

When finished, confirm: 'Project documentation complete.'


---

### USER (gemini)

# Task: Synthesize Project Architecture

Please synthesize a high-level `project-design.md` by analyzing the individual `design.md` files from all modules. This document will be our primary map for navigating this project in the future.

## IMPORTANT: Existing Documentation Found

An existing `project-design.md` was found at the project root. **You MUST read it first.**
Your goal is to **upgrade and improve** this document, not degrade it. Preserve its depth, architectural insights, and specific details while incorporating new information from the module designs.

## Project Context

- Project Path: /Users/bill/projects/lyric
- Total Modules: 6

## Modules to Analyze

- ast: pkg/ast/design.md
- checker: pkg/checker/design.md
- lir: pkg/lir/design.md
- parser: pkg/parser/design.md
- verifier: pkg/verifier/design.md
- runtime: runtime/design.md

## Your Strategy

1. **Read Existing Design**: Read the current `project-design.md` at the root.
2. **Read Module Designs**: Read the `design.md` file for every module listed above.
3. **Identify Gaps**: Compare the existing project design with the new module-level details.
4. **Upgrade Narrative**: Synthesize a superior `project-design.md` that preserves the best of the old while adding the precision of the new.
5. **Module Map**: Include a **categorized tree view** of all modules, with each entry being a **clickable markdown link** to its `design.md` file (e.g., `[pkg/scanner](pkg/scanner/design.md)`).
6. **Use Tools**: Use `write_file` and `edit_file` to build the document in sections. Do NOT output the whole document at once.

When finished, confirm: 'Project documentation complete.'


---

### ASSISTANT (gemini)

**Tool Call: `read_file`**
```json
{
  "file_path": "project-design.md"
}
```

---

### USER (gemini)

**Tool Result:**
```
[ID: tool_7e850b8de6e0738f]
File: /Users/bill/projects/lyric/project-design.md (77 lines)

# Lyric Project Architecture

## Executive Summary

The Lyric project is a comprehensive compiler and toolchain for the Lyric programming language. It is designed to provide a high-level, expressive language with advanced features such as relations, interfaces, generic type inference, and structured control flow, while maintaining the capability to compile down to multiple backends, currently supporting Go and C. The project exists to bridge the critical gap between high-level architectural intent and low-level implementation. By capturing design specifications in dedicated `.lyric` files, the system ensures structural integrity and actively prevents architectural drift. Its primary goals are to deliver a robust, multi-stage compilation pipeline—spanning parsing, semantic analysis, low-level intermediate representation, and code generation—while maintaining a strong, verifiable connection between the documented design and the evolving codebase through its unique verification engine.

## System Architecture

The Lyric compiler follows a classic pipeline architecture, augmented by a parallel verification track that acts as an architectural guardian. The architecture is fundamentally layered and data-centric, designed to isolate complexity at each distinct stage of transformation. The primary compilation flow is linear, progressing from raw source text to an Abstract Syntax Tree (AST), through semantic checking, into a Low-level Intermediate Representation (LIR), and finally to target code generation.

The philosophy of the system is to resolve specific classes of problems in dedicated modules, ensuring that downstream consumers operate on increasingly simplified and explicit data models. The parser handles all syntax and grammatical ambiguities, employing a hybrid strategy of recursive descent and Pratt parsing to produce a structural representation. It manages significant newlines and complex f-string interpolations, providing a clean foundation. The AST module acts as the central data hub, providing a representation of the program using a highly efficient "Kind + Data" pattern. Crucially, it features a generative desugaring engine that expands high-level syntactic sugar—such as relations, interface fields, and default implementations—into fundamental constructs before semantic analysis. It also selectively merges the standard library based on reachability analysis.

The checker assumes responsibility for all semantic rules, acting as the semantic truth of the compiler. It performs a multi-pass analysis for type inference, scope management, module import resolution, and validation. It handles structural subtyping, numeric widening, optional wrapping, and enforces concurrency safety through guarded annotations. Ultimately, it annotates the AST with resolved types. 

The LIR module then takes this semantically rich tree and bridges the gap to the backends. It enforces structured control flow and flat expressions, assigning every sub-expression to a unique temporary variable. This crucial step removes the burden of semantic resolution from the backends. The LIR pipeline includes optimization passes for side-effect elimination and multi-return destructuring, as well as a comprehensive monomorphization pass to specialize generic types and functions for targets that lack native generics. Finally, the backends act as thin emitters that translate the simplified LIR into idiomatic target source code. The C backend relies heavily on the runtime module, which provides a foundational C header library establishing the memory layout and operational semantics for Lyric's high-level constructs.

In parallel to the compilation pipeline, the verifier module operates as a standalone structural integrity engine. It parses `.lyric` specification files and performs a deep structural comparison against the actual Go implementation, utilizing the Go standard library's AST tools to extract type information. It normalizes naming conventions across the two systems and performs a reverse completeness check to ensure every exported Go symbol is documented. This ensures that the documented design and the codebase remain perfectly synchronized, enforcing the philosophy that documentation should be an active, verifiable contract rather than a passive artifact.

## Interface & Contract Map

The system's boundaries are primarily defined by the rich data structures passed between modules rather than traditional Go interfaces. These data structures act as the fundamental contracts that govern module interactions, ensuring a decoupled and data-driven architecture.

The most critical contract is the Abstract Syntax Tree. The `ast.File` and its constituent nodes representing declarations, expressions, and statements form the central nervous system of the compiler. The parser module acts as the initial producer of this contract, transforming source code into the raw AST. The AST module itself acts as both a consumer and producer, taking the raw AST and producing a desugared version where high-level constructs are flattened. The checker module consumes this desugared AST for semantic analysis and mutates it by attaching resolved type information to the `ResolvedType` and `InferredTypeArgs` fields. The LIR module consumes the fully type-checked AST to generate the intermediate representation. Additionally, the verifier module consumes the AST of `.lyric` files to compare against Go source code.

The Type Registry contract, defined by the `checker.Registry` and `checker.Type` structures, represents the semantic truth of the program. The checker module builds and populates this registry during its declaration and checking passes, resolving all cross-module dependencies and generic constraints. The LIR module is the primary consumer of this contract, relying heavily on the type registry and resolved types to correctly lower high-level constructs, such as generics, classes, and enums, into low-level operations.

The LIR contract, represented by the `lir.LProgram`, defines the fully resolved and flattened program state. The LIR module's lowerer produces this representation, ensuring that all expressions are flattened into temporaries and control flow is structured. Internal optimization and monomorphization passes within the LIR module consume and mutate this structure, specializing generic types and eliminating redundant operations. Ultimately, the backends consume the optimized `LProgram` to emit the final source code.

The Runtime contract is defined by the C runtime header (`lyric_runtime.h`), which establishes the execution environment for C-compiled Lyric programs. The runtime module provides these foundational macros and inline functions, defining the memory layout for dynamic slices, strings, optional types, error results, and tagged unions. The C backend within the LIR module acts as the sole consumer, emitting code that relies entirely on this contract for operational semantics and concurrency primitives.

## Module Map

### Categorized Tree View

- **Frontend**
  - [pkg/parser](pkg/parser/design.md)
  - [pkg/ast](pkg/ast/design.md)
- **Semantic Analysis**
  - [pkg/checker](pkg/checker/design.md)
- **Intermediate Representation & Backends**
  - [pkg/lir](pkg/lir/design.md)
  - [runtime](runtime/design.md)
- **Tooling**
  - [pkg/verifier](pkg/verifier/design.md)

### Module Descriptions

The **Parser** module is the foundational entry point for the compiler frontend, responsible for transforming raw Lyric source code into an Abstract Syntax Tree. It manages internal state through a hand-written lexical scanner that tokenizes the input, carefully handling significant newlines and collecting comments. The parser employs a hybrid strategy, utilizing recursive descent for high-level declarations and a Pratt parser for expressions, allowing it to efficiently handle complex operator precedence without grammar explosion. It implements the primary data contract by producing the `ast.File` consumed by the rest of the system, and it seamlessly handles complex features like f-string interpolation, contextual keywords, and disambiguation through targeted backtracking.

The **AST** module defines the core data structures that represent the structure of Lyric code, acting as the central data hub. It manages state primarily as a tree of polymorphic nodes utilizing a "Kind + Data" pattern to avoid deep interface hierarchies, making it highly efficient for recursive traversals. Crucially, it includes a generative desugaring engine that transforms high-level constructs, such as relations, interface fields, and default implementations, into fundamental AST nodes before semantic analysis occurs. It also handles the selective merging of the standard library based on reachability analysis, ensuring the compilation unit remains lean. It implements the primary data contract consumed by the checker and LIR modules.

The **Checker** module implements the semantic analysis phase of the compiler, ensuring that a Lyric program is semantically valid before code generation. It maintains complex internal state, including a stack of scopes for variable tracking and a global registry for user-defined types. It consumes the desugared AST, resolves all type expressions, infers generic types through a unification-based approach, enforces concurrency safety through guarded annotations, and validates the exhaustiveness of match statements. It acts as the semantic truth of the compiler, annotating the AST nodes with `ResolvedType` information, fulfilling the semantic contract required by the subsequent lowering phase.

The **LIR** module is the architectural bridge between the high-level AST and the backend code generators. It resolves all semantic complexity into a simplified, flat representation while preserving structured control flow. It manages state during the lowering process by mapping AST nodes to LIR constructs and assigning all sub-expressions to unique temporary variables, ensuring that backends never encounter nested expression trees. It includes sophisticated optimization passes for side-effect elimination and multi-return destructuring, as well as a comprehensive monomorphization pass to specialize generic types for backends like C. It consumes the type-checked AST and produces the `LProgram` contract, which is then consumed by its internal C backend to emit target code, handling complex transformations like generator state machines and interface vtable dispatch.

The **Runtime** module provides the foundational C runtime library required by the Lyric C backend. It is implemented as a passive, header-only library that establishes the memory layout and operational semantics for Lyric's high-level constructs in a C environment. It does not maintain internal state but provides a suite of macros and inline functions for dynamic slices, length-prefixed strings, optional types, error results, tagged unions, and concurrency primitives. These foundational elements are consumed by the C code generated by the LIR module, bridging the gap between Lyric's expressive type system and C's low-level primitives.

The **Verifier** module is a standalone structural integrity engine designed to detect architectural drift by comparing high-level `.lyric` specifications against the actual Go implementation. It maintains state during a run to aggregate type information extracted from Go source files using the standard library's AST tools. It consumes the AST produced by the parser for `.lyric` files and performs deep structural comparisons, normalizing naming conventions and mapping types across the two systems. It also performs a reverse completeness check, ensuring every exported Go symbol is documented in the Lyric specification, and aggregates any mismatches into a detailed report. Additionally, it enforces project-wide architectural invariants by analyzing the Go AST of other modules.

## Integration Patterns & Workflows

The system relies on clear, sequential workflows to transform data across module boundaries. Two primary workflows define the operation of the Lyric toolchain: the compilation pipeline and the architectural verification process.

The compilation pipeline workflow traces the journey of a Lyric source file from raw text to executable code. The process begins when a file is read from disk and passed to the parser module. The lexer tokenizes the text, handling significant newlines and f-string interpolation, and the parser constructs an initial `ast.File` using recursive descent and Pratt parsing. This AST is immediately passed to the AST module's desugaring functions, which expand complex features like relations into standard classes and implementation blocks, extract default interface implementations, and selectively merge required standard library components. The desugared AST is then handed to the checker module. The checker performs a multi-pass analysis: a pre-registration pass to identify user-defined types, a registration pass to populate its type registry and resolve imports, and a deep traversal to resolve types, infer generics, enforce concurrency safety, and validate semantics, annotating the AST with `ResolvedType` fields. Once validated, the AST and the type registry are passed to the LIR module's lowerer. The lowerer flattens expressions into temporaries and converts high-level constructs into LIR statements while preserving structured control flow. The resulting `LProgram` undergoes optimization passes to remove redundant operations and destructure multi-returns. For the C target, a monomorphization pass specializes generic types. Finally, the C backend emitter traverses the LIR and generates the final source code string, relying on the runtime module's macros, which is then written to disk.

The architectural verification workflow operates independently to ensure design fidelity. The process starts when the verifier module is invoked on a `.lyric` specification file. It uses the parser module to generate an AST of the specification. The verifier then inspects the source annotations within the AST to locate the corresponding Go source files. It uses the Go standard library to extract all exported types, interfaces, and function signatures from these Go files, building a comprehensive internal registry that aggregates information across multiple files. The verifier then recursively walks the Lyric AST, translating Lyric type expressions into Go string representations and comparing them against the extracted Go types. It normalizes naming conventions on the fly, mapping snake_case to PascalCase or camelCase, to facilitate this comparison. Finally, it performs a reverse completeness check, ensuring every exported Go symbol is documented in the Lyric specification, and aggregates any mismatches into a detailed report.

## Dependency Overview

The project exhibits a strict, unidirectional dependency graph that prevents circular imports and enforces a clear separation of concerns. This layering is critical for maintaining the maintainability and testability of the compiler.

At the base of the hierarchy is the AST module, which acts as a leaf node containing pure data structures. The parser module depends solely on the AST module to construct its output, ensuring that parsing logic is decoupled from semantic analysis. The checker module depends on both the AST module for input and annotation, and the parser module to handle imported files during its semantic analysis phase. To avoid circular dependencies, the AST module stores checker-resolved types as opaque `any` fields.

The LIR module depends on the AST and checker modules to perform its lowering process, relying on the semantic truth established by the checker and the structural representation provided by the AST. It also implicitly depends on the runtime module as the target contract for its C backend.

The verifier module depends on the AST and parser modules to read specifications, but operates entirely independently of the compiler's semantic and backend phases. This isolation allows the verifier to act as an objective observer of the codebase. The runtime module is completely independent of all Go code, serving only as a target dependency for the generated C code. This layered architecture ensures that the frontend remains decoupled from backend concerns, and that data structures are defined independently of the logic that manipulates them.
```

---

### ASSISTANT (gemini)

**Tool Call: `read_file`**
```json
{
  "file_path": "pkg/ast/design.md"
}
```

---

### USER (gemini)

**Tool Result:**
```
[ID: tool_de14d0d5a1e42a4b]
File: /Users/bill/projects/lyric/pkg/ast/design.md (111 lines)

# AST Module Design

The `ast` module defines the Abstract Syntax Tree (AST) for the Lyric language, serving as the foundational data model for the entire compiler toolchain. It provides a structured representation of Lyric source code that is shared between declaration-only `.lyric` files and full implementation `.ly` files. Beyond simple data representation, the module includes a critical desugaring phase that simplifies high-level architectural constructs into fundamental language primitives before semantic analysis.

## Executive Summary

The `ast` module is the central data hub of the Lyric compiler's frontend. It acts as the primary contract between the [parser](../parser/design.md), which produces the tree, and the [checker](../checker/design.md) and [lir](../lir/design.md) modules, which consume and transform it. The module is designed with a "Kind + Data" pattern to achieve polymorphism without the complexity of deep interface hierarchies, making it highly efficient for the recursive traversals common in compiler passes. 

A standout feature of this module is its generative desugaring engine, which automates the expansion of complex features like relations, interface fields, and default implementations into standard classes, methods, and implementation blocks. Additionally, the module handles the selective merging of the standard library, ensuring that only the necessary components are included in the final AST, thereby keeping the compilation process lean and focused. Finally, a post-desugar validation pass ensures that the tree is in a consistent state before being handed off to the type checker.

## File Inventory

- [ast.go](ast.go): Defines the core structural nodes for declarations, including functions, classes, structs, enums, interfaces, and relations. It also establishes the foundational types for source location tracking (`Pos` and `Span`) and type expressions.
- [expr.go](expr.go): Contains the definitions for all expression and statement nodes used in full implementation files. This includes literals, operators, control flow structures (if, for, while, match), and concurrency primitives like `spawn` and `select`.
- [desugar.go](desugar.go): Implements the transformation logic that simplifies the AST. It handles the expansion of relations into concrete fields and `impl` blocks, extracts default interface implementations into top-level functions, and generates destructor logic.
- [stdlib.go](stdlib.go): Manages the integration of the Lyric standard library. It provides utilities for locating the stdlib on disk and selectively merging its interfaces, classes, and functions into the user's AST based on actual usage.
- [validate.go](validate.go): Provides post-desugar validation to ensure that all high-level constructs (like interface embeds) have been correctly flattened and that relations have been properly expanded into class fields.

## Architecture and Data Flow

The AST is the "central nervous system" of the compiler, with data flowing through it in a well-defined sequence of stages.

The process begins in the **Parsing** stage, where the [parser](../parser/design.md) transforms raw source text into a `File` node. This node contains one or more `LyricBlock` structures, allowing a single file to house multiple architectural modules. Once the raw AST is constructed, it enters the **Desugaring** phase. This is a series of idempotent transformations that expand high-level "sugar"—such as `relation` declarations and `field` definitions within interfaces—into explicit, lower-level AST nodes. For instance, a single `relation` might result in new fields being injected into multiple classes and the creation of a corresponding `impl` block.

Following desugaring, the AST undergoes **Validation** to confirm that the transformations were successful and the tree is semantically ready for the next stage. The validated AST is then passed to the **Semantic Analysis** stage. The [checker](../checker/design.md) performs name resolution and type inference, decorating the AST nodes with semantic information. Crucially, the AST nodes include `ResolvedType` and `InferredTypeArgs` fields (stored as `any` to prevent circular dependencies) which the checker populates. Finally, the fully annotated and desugared AST is consumed by the **Lowering** stage in the [lir](../lir/design.md) module, which flattens the tree into a low-level intermediate representation for code generation.

## Interface Implementations

The `ast` module is intentionally data-centric and does not implement complex behavioral interfaces. The nodes are designed to be passive structures that are acted upon by external visitors or transformation functions. The `File` type provides a `DocComment` utility method to retrieve contiguous comment blocks preceding a specific line, which is used by the [verifier](../verifier/design.md) and documentation generators to associate narrative text with code declarations.

## Public API

### Core Data Structures

The API is dominated by the tree's root and its primary branches. The `File` struct is the entry point, holding a collection of `LyricBlock` nodes and all source comments. Each `LyricBlock` acts as a container for the various declarations supported by Lyric: `StructDecl`, `EnumDecl`, `InterfaceDecl`, `ClassDecl`, `FuncDecl`, `RelationDecl`, `ImplBlock`, `TypeAliasDecl`, `ConstDecl`, `ImportDecl`, `DocBlock`, and `InvariantDecl`.

Type expressions are represented by the `TypeExpr` struct, which uses a `TypeExprKind` to distinguish between named types, optionals, unions, sequences, maps, tuples, function signatures, channels, generators, locks, and units. Similarly, the `Expr` and `Stmt` structs in `expr.go` use `ExprKind` and `StmtKind` respectively to represent the full breadth of the language's operational syntax, including complex constructs like `spawn`, `select`, `cascade`, and `lock`.

### Desugaring Engine

The module exports several high-level functions that perform the AST transformations, typically executed in a specific order:

- **`DesugarInterfaceEmbeds`**: Flattens embedded interfaces by copying fields and destructors from the embedded interface into the embedding interface, substituting type parameters as needed. Note that methods are not copied here to avoid duplication, as they are handled by the default implementation extraction.
- **`DesugarInterfaceFields`**: Converts abstract field declarations within interfaces into concrete getter and setter method signatures. This ensures that subsequent passes see them as standard interface requirements.
- **`DesugarRelations`**: Processes `relation` declarations by injecting labeled fields into the involved classes and generating or merging `impl` blocks that wire the interface methods to these fields. It uses label prefixing to avoid name collisions when a class participates in multiple relations of the same type.
- **`DesugarDefaultImpls`**: Extracts methods with bodies from interfaces and moves them to the top-level as standalone generic functions, adding the necessary relational `where` clauses to maintain their context.
- **`DesugarDestructors`**: Automatically generates `destroy` methods for classes involved in `owns` relations, incorporating cleanup logic defined in the associated interfaces. It performs deep copies of destructor bodies and renames method calls to match label-prefixed fields.

### Standard Library Integration

The `stdlib.go` file provides the mechanism for bringing the standard library into the compilation unit:

- **`MergeStdlib`**: This function takes the user's AST and a parsed version of the standard library. It performs a transitive usage analysis to identify which stdlib interfaces, classes, and functions are actually needed by the user's code (either directly or via relations) and merges only those into the first `LyricBlock` of the user's `File`.
- **`MergeFiles`**: Combines multiple `File` nodes into a single one, which is essential for multi-file compilation.
- **`FindStdlibDir`**: A utility to locate the standard library directory based on environment variables, executable location, or the current working directory.

### AST Validation

The `validate.go` file provides the `ValidatePostDesugar` function, which performs a suite of invariant checks:
- **Relation Field Injection**: Verifies that every `owns` relation has resulted in the expected parent field being injected into the child class.
- **Embed Flattening**: Ensures that no `embed` declarations remain in interfaces, confirming that `DesugarInterfaceEmbeds` has completed its work.
- **Consistency**: Checks that the tree is in a state where the type checker can proceed without encountering unexpected high-level sugar.

## Implementation Details

### Tagged Union Pattern

To maintain a flat and performant structure in Go, the AST utilizes a tagged union pattern for its most polymorphic nodes: `TypeExpr`, `Expr`, `Stmt`, and `Pattern`. Each of these structs contains a `Kind` enum and a `Data` field of type `any`. 

```go
type Expr struct {
    Kind         ExprKind
    Data         any
    Span         Span
    ResolvedType any
}
```

This approach avoids the overhead of interface method calls during traversal and allows consumers to use efficient type switches. The `Data` field is type-asserted based on the `Kind`, ensuring that the logic remains clear and localized.

### Source Mapping and Diagnostics

Every major node in the AST carries a `Span`, consisting of a start and end `Pos` (file, line, column). While this increases the memory footprint of the tree, it is a deliberate trade-off to enable high-fidelity error reporting. This metadata allows the checker and verifier to point to the exact location of a violation in the source code, and it provides the necessary foundation for future IDE features like "go to definition" and refactoring tools.

### Generative Desugaring Logic

The desugaring logic in `desugar.go` is designed to be generative and order-dependent. It must be executed before the checker because the checker expects a fully explicit representation of the program. 

1.  **Interface Embed Flattening**: Embedded interfaces are resolved first to ensure all fields and destructors are visible in the embedding interface.
2.  **Interface Field Expansion**: Fields in interfaces are turned into method signatures so that subsequent passes see them as standard interface requirements.
3.  **Relation Processing**: The relation pass uses the `rewriteFieldType` helper to map interface-level type parameters to the concrete types involved in a relation. It handles label prefixing to avoid name collisions when a class participates in multiple relations of the same type.
4.  **Default Implementation Extraction**: By moving default methods to the top level, the compiler simplifies the task of the checker, which can then treat these methods as regular generic functions with relational constraints.
5.  **Destructor Generation**: Finally, `destroy` methods are synthesized for classes, aggregating cleanup logic from all relevant relations. This pass uses `deepCopyBlock` and `substituteTypeParamsRichInBlock` to ensure that each class gets a unique, correctly typed version of the destructor logic.

### Selective Stdlib Merging

The `MergeStdlib` function implements a sophisticated reachability analysis. It doesn't just pull in every interface; it looks at `RelationDecl` hints to see which stdlib interfaces are being used to define relationships. It then transitively follows `embed` links. Similarly, it scans the user's code for named types and function calls, pulling in the corresponding stdlib definitions. This ensures that the resulting AST is as small as possible while still being semantically complete.

## Dependencies

The `ast` module is a leaf package in the Lyric internal hierarchy. It does not depend on any other `pkg/*` modules, which is essential as almost every other module in the system depends on `ast`. 

To avoid circular imports with the `checker` package, the `ResolvedType` and `InferredTypeArgs` fields are defined as `any`. This allows the checker to store its complex type structures within the AST without the AST needing to know about the checker's internal definitions.

## Technical Debt and Future Work

- **Visitor Pattern**: As the number of node types grows, the manual type-switching in consumers like the checker and LIR lowerer is becoming unwieldy. Implementing a standard Visitor or Walker pattern would centralize traversal logic.
- **Embed Removal**: Currently, `DesugarInterfaceEmbeds` copies content but does not explicitly clear the `Embeds` slice, which causes `ValidatePostDesugar` to report violations unless the slice is cleared elsewhere.
- **Serialization**: There is currently no support for serializing the AST to JSON or a binary format. This limits the ability to cache compilation results or provide the AST to external tools written in other languages.
- **In-place Mutation**: Desugaring modifies the AST in-place, which loses the original "sugared" representation. For tools that require high-fidelity source-to-source transformations, a non-destructive or "side-table" approach to desugaring might be necessary.
```

---

### ASSISTANT (gemini)

**Tool Call: `read_file`**
```json
{
  "file_path": "pkg/checker/design.md"
}
```

---

### USER (gemini)

**Tool Result:**
```
[ID: tool_05d96cd1bfda5965]
File: /Users/bill/projects/lyric/pkg/checker/design.md (112 lines)

# Checker Module Design

The `checker` module implements the semantic analysis phase of the Lyric compiler. It is responsible for type checking, type inference, name resolution, and enforcing language-specific constraints on the desugared Abstract Syntax Tree (AST) produced by the `ast` module.

## Executive Summary

The `checker` module ensures that a Lyric program is semantically valid before it is passed to the backend for code generation. It performs a comprehensive, multi-pass analysis of the AST, resolving type expressions into concrete `Type` objects, tracking variable scopes, and verifying that all expressions and statements adhere to Lyric's type system rules.

Key features include structural subtyping for interfaces, numeric widening, automatic optional wrapping, and support for generic type inference using a unification-based approach. The checker also handles module imports, ensuring that symbols are correctly resolved across file boundaries while detecting circular dependencies. It enforces concurrency safety through `guarded_by` annotations and validates the exhaustiveness of `match` statements for enums and unions.

The checker acts as the "semantic truth" of the compiler, annotating AST nodes with `ResolvedType` information. This annotation is a critical contract consumed by subsequent phases, such as the LIR lowerer and backends, which rely on it for correct code generation without needing to re-perform semantic analysis.

## File Inventory

- [checker.go](checker.go): The primary implementation of the type checker. It defines the core `Type` system, `Scope` management, `Registry` for user-defined types, and the `Checker` struct which contains the logic for traversing and validating the AST. It also includes the pre-registration of Go standard library modules for interop.
- [checker_test.go](checker_test.go): A comprehensive suite of tests that verify the checker's behavior across a wide range of Lyric language features, including basic arithmetic, control flow, function calls, struct/class definitions, generic type inference, concurrency safety, and module imports.
- [checker.lyric](checker.lyric): A Lyric-language specification of the checker's own architecture and logic, serving as both documentation and a formal model of the module's behavior.

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

### Phase 3-5: Validation
After the primary checking passes, the checker performs several validation steps:
- **Phase 3**: Verifies that all function signatures are fully resolved and no `TyUnknown` types remain.
- **Phase 4**: Ensures that every expression in the AST has a `ResolvedType` annotation.
- **Phase 5**: Validates that all field accesses and method calls resolve to real members on the receiver type, catching any bugs that might have bypassed normal checking.

## Interface Implementations

The `checker` module does not explicitly implement external Go interfaces, but it provides the core logic that satisfies the semantic requirements of the Lyric language. It consumes the AST nodes defined in [pkg/ast](../ast/design.md) and uses the parser from [pkg/parser](../parser/design.md) to handle imports.

## Public API

### Core Types

- **`Type`**: Represents a resolved type in the Lyric system. It uses a `Kind` (e.g., `TyInt`, `TyFunc`, `TyStruct`, `TyUnion`) and additional metadata (e.g., bit width, element types, field lists, type parameters) to describe the type. It provides methods for structural equality (`Equal`) and string representation.
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

Lyric supports several forms of implicit type compatibility implemented in the `assignableTo` method. Numeric widening allows smaller integer types to be implicitly converted to larger ones (e.g., `i32` to `i64`). Optional wrapping ensures that a type `T` is always assignable to `T?`. Structural subtyping allows a class or struct to satisfy an interface if it provides all the required methods with matching signatures. Union types allow any member type to be assignable to the union, while the union itself is only assignable to a target if all its variants are compatible.

### Generic Type Inference

The checker implements a unification-based inference for generic functions. When a generic function is called without explicit type arguments, the `inferTypeArgs` method walks the parameter types and actual argument types in parallel. It binds type variables to concrete types from the arguments, with the first match winning. The inferred types are then substituted into the function's return type and stored in the AST for the transpiler.

### Relational Constraints

Lyric supports relational constraints through `where` clauses, such as `where Graph<G, N, E>`. When such a constraint is encountered, the checker looks up the corresponding interface and "grants" its methods to the involved type variables. This is implemented by populating a `typeVarMethods` map, which allows methods defined in the interface to be called on variables of the constrained types within the function body.

### Concurrency and Guarded Fields

The checker enforces safety rules for concurrent access by tracking "held locks" within a scope. These locks are populated by `lock` statements. The checker verifies that any field annotated with `guarded_by` is only accessed when the corresponding lock is held, preventing unprotected access to shared state. The `extractLockName` helper identifies the lock being used, supporting both simple identifiers and field accesses (e.g., `self.mu`).

### Exhaustiveness Checking

For `match` statements operating on `enum` or `union` types, the checker verifies that all possible variants or member types are covered by the match arms. If a wildcard pattern is not present, the checker reports an error listing the missing variants or types, ensuring that all cases are handled at compile time.

### Built-in Methods and Functions

The checker provides specialized resolution for methods on built-in types. Strings support operations like `len`, `contains`, and `split`. Lists provide `push`, `pop`, and `reverse`. Maps offer `contains_key`, `keys`, and `values`. Channels support `send`, `receive`, and `close`. These are resolved during the method call checking phase before falling back to general method lookup.

## Dependencies

- **[pkg/ast](../ast/design.md)**: The checker consumes the AST and stores results back into it.
- **[pkg/parser](../parser/design.md)**: Used to parse imported module files.

## Technical Debt and Future Work

- **Advanced Inference**: The current type inference is relatively simple and may need to be replaced with a more robust constraint-solver-based approach for complex scenarios involving nested generics or circular constraints.
- **Method Resolution**: Method lookup currently relies on a simple name-based search. Support for method overloading or more complex trait-based dispatch might be needed in the future.
- **Performance**: For very large projects, the recursive module checking and deep AST traversal could become a bottleneck. Implementing incremental checking or a more parallelized approach could improve performance.
- **Circular Type Definitions**: While module cycles are detected, circular type definitions need careful handling to avoid infinite recursion during size calculation or equality checks.
```

---

### ASSISTANT (gemini)

**Tool Call: `read_file`**
```json
{
  "file_path": "pkg/lir/design.md"
}
```

---

### USER (gemini)

**Tool Result:**
```
[ID: tool_c5cf74c08ab40673]
File: /Users/bill/projects/lyric/pkg/lir/design.md (114 lines)

# LIR Module Design

## Executive Summary

The `lir` (Low-level Intermediate Representation) module is the architectural bridge between the high-level, semantically rich Abstract Syntax Tree (AST) and the target-specific code generators of the Lyric compiler. Its primary mission is to resolve the semantic complexity of the Lyric language—such as generics, optional types, tagged unions, and method dispatch—into a simplified, flat representation. This transformation ensures that backends can remain "thin" emitters, focused primarily on syntax mapping rather than complex semantic resolution.

The LIR is built on four foundational design principles. First, it preserves **structured control flow** (if, while, for, switch) instead of decomposing logic into basic blocks and jumps, which is essential for generating idiomatic high-level target code. Second, it enforces **flat expressions**, where every sub-expression is assigned to a unique, named temporary variable, ensuring that backends never encounter nested expression trees. Third, it performs **semantic resolution**, lowering high-level Lyric features into explicit LIR constructs. Finally, it is **backend agnostic**, providing a common foundation for multiple targets while offering optional passes like monomorphization for backends that require it.

## File Inventory

*   [lir.go](lir.go): Defines the core data structures for the LIR, including the type system (`LType`), operands (`LValue`), expressions (`LExpr`), and structured statements (`LStmt`).
*   [lower.go](lower.go): Implements the `Lowerer`, the primary engine that transforms a type-checked AST into an LIR program. This file contains the bulk of the semantic lowering logic, including expression flattening and control flow transformation.
*   [monomorphize.go](monomorphize.go): Implements a comprehensive monomorphization pass that specializes generic functions, classes, and structs into concrete versions for backends like C that do not support generics natively.
*   [optimize.go](optimize.go): Performs post-lowering LIR-to-LIR optimizations, such as side-effect temporary elimination, multi-return destructuring, and unused variable removal.
*   [validate.go](validate.go): Provides invariant checkers that verify the integrity of the LIR program after lowering and monomorphization passes.
*   [c_backend.go](c_backend.go): The C code generator, which transforms monomorphized LIR into C code, utilizing a dedicated runtime library and implementing complex features like generator state machines and interface vtables.
*   [lir.lyric](lir.lyric): A Lyric specification file that formally defines the LIR module's structure and interfaces, used by the verifier to ensure implementation integrity.
*   [lower_test.go](lower_test.go): Unit tests for the lowering pass, verifying the transformation of various AST constructs.
*   [lower_extra_test.go](lower_extra_test.go): Additional tests for the lowering pass, covering edge cases and complex scenarios.
*   [c_backend_test.go](c_backend_test.go): Tests for the C code generator, verifying the emission of valid C code.

## Architecture and Data Flow

The `lir` module operates as a sophisticated transformation pipeline that converts high-level architectural intent into executable source code. The flow of data through the module follows a strictly defined sequence of stages.

The pipeline begins with the **Lowering** stage. The `Lowerer` consumes a type-checked `ast.File` from the [pkg/ast](../ast/design.md) module and a `checker.Registry` from the [pkg/checker](../checker/design.md) module. It performs a recursive walk of the AST, flattening every nested expression into a sequence of single-operation assignments to named temporaries. During this process, high-level constructs are desugared into their LIR equivalents; for instance, a `match` statement on an enum is lowered into a `switch` on an integer variant tag, with explicit field extractions for any bound variables.

Once the initial LIR is generated, it enters the **Optimization** stage. The `Optimize` function runs several passes over the LIR to clean up the output of the lowerer. These passes are essential because the lowerer often produces redundant temporaries or patterns that are difficult for backends to emit efficiently. Key optimizations include fusing void-context calls into bare statements (side-effect elimination) and destructuring multi-return values from single temporaries into separate variables.

For backends that require it, such as C, the pipeline includes an optional **Monomorphization** stage. The `Monomorphize` function scans the LIR program for all instantiations of generic types and functions. It then creates specialized, non-generic copies for each unique set of type arguments and rewrites all call sites and type references to use these specialized versions. This ensures that the final LIR contains no type variables before it reaches the backend.

The final stage is **Code Generation**. A backend-specific emitter, currently the C backend, walks the optimized (and potentially monomorphized) LIR tree and produces the final source code string. Because the LIR has already resolved all semantic complexity, these backends are primarily responsible for syntax mapping and utilizing their respective runtime libraries.

## Interface Implementations

The `lir` module does not explicitly implement interfaces defined in other packages, as its primary role is data transformation. However, it is a heavy consumer of the data structures and semantic information provided by:
*   [pkg/ast](../ast/design.md): The module provides the input `ast.File` and all its constituent nodes.
*   [pkg/checker](../checker/design.md): The module provides the `checker.Registry` and the `checker.Type` information used to resolve names and types during the lowering process.

## Public API

The `lir` module exposes a clean, functional API for interacting with the LIR pipeline. The primary entry points are:

*   `NewLowerer()`: This function creates a new instance of the `Lowerer`, initializing the internal state required for AST-to-LIR transformation.
*   `Lowerer.Lower(file *ast.File) *LProgram`: This is the "front door" to the module. It takes a type-checked AST file and returns a complete `LProgram` structure representing the lowered code.
*   `Optimize(prog *LProgram)`: This function runs the full suite of LIR-to-LIR optimization passes on a program, mutating it in place to improve code quality and backend compatibility.
*   `Monomorphize(prog *LProgram)`: This function specializes all generic declarations in the program, removing all type variables. It is a prerequisite for the C backend.
*   `ValidatePostLower(prog *LProgram)` and `ValidatePostMono(prog *LProgram)`: These functions check for invariant violations at different stages of the pipeline, ensuring that types like `LTyAny` or `LTyTypeVar` do not propagate where they are not allowed.
*   `EmitC(prog *LProgram) string`: This function takes a monomorphized LIR program and generates a complete C source code string, relying on the `lyric_runtime.h` contract.

## Implementation Details

### LIR Data Structures

The LIR is defined by four primary categories of data structures:

*   **LType**: Represents the Lyric type system in a form suitable for code generation. It includes primitive types (integers, floats, bool, string), unit/void, error, and complex types like `LTyStruct`, `LTyClassHandle` (heap-allocated), `LTyTaggedUnion` (enums), `LTySlice`, `LTyMap`, `LTyChannel`, `LTyGenerator`, `LTyOptional`, and `LTyErrorResult`. It also preserves `LTyTypeVar` for generic support.
*   **LValue**: The operands of the LIR. These are flat and include named variables (`LValVar`), SSA-like temporaries (`LValTemp`), global variables (`LValGlobal`), and various literals (int, uint, float, string, bool, null).
*   **LExpr**: Right-hand side expressions that are always bound to a temporary via `LTempDef`. They reference only `LValue` operands, never nested expressions. Kinds include binary/unary operations, casts, field access, calls (regular, method, builtin), and construction (struct, class, slice, map, channel).
*   **LStmt**: Structured statements that form the program's control flow. Kinds include variable management (`LStmtTempDef`, `LStmtVarDecl`, `LStmtAssign`), field/index mutation (`LStmtStructSet`, `LStmtClassSet`, `LStmtIndexSet`), and structured control flow (`LStmtIf`, `LStmtWhile`, `LStmtFor`, `LStmtSwitch`, `LStmtTypeSwitch`, `LStmtBlock`, `LStmtReturn`, `LStmtBreak`, `LStmtContinue`). It also includes concurrency primitives (`LStmtSpawn`, `LStmtLock`, `LStmtSend`, `LStmtSelect`, `LStmtYield`).

### The Lowering Pass

The `Lowerer` is the most complex component of the module, responsible for the heavy lifting of semantic resolution. It maintains a stateful mapping of AST nodes to LIR constructs and tracks visibility, imports, and type information. A central feature of the lowerer is **expression flattening**. Every sub-expression in the AST is assigned to a unique temporary variable (`LValTemp`). This ensures that all `LExpr` instances in the LIR reference only simple operands, never nested expressions. For example, `a + b * c` is lowered into `%0 = b * c` followed by `%1 = a + %0`.

Control flow lowering in Lyric LIR is **structured**. Instead of decomposing logic into basic blocks and jumps, the LIR uses high-level statements like `LStmtIf` and `LStmtWhile`. The `LStmtWhile` is particularly noteworthy; it contains a `CondBlock`—a list of statements executed before each iteration to compute the loop condition. This elegantly handles complex conditions that might involve side effects or multiple steps, ensuring they are re-evaluated correctly on every iteration without duplicating code.

The `Lowerer` also handles the desugaring of Lyric's high-level types:
*   **Enums**: These are lowered to `LTyTaggedUnion`. Match statements are converted to switches on the variant tag, with the lowerer generating the necessary logic to extract variant fields into local bindings.
*   **Classes**: These are lowered to `LTyClassHandle`, representing a pointer to a heap-allocated struct. Field access is converted to explicit `LExprClassGet` and `LStmtClassSet` operations.
*   **Generics**: The lowerer preserves type parameters as `LTyTypeVar`.
*   **Impl Blocks**: The lowerer generates forwarding wrapper methods for `impl` blocks. This enables classes to satisfy interfaces even when method names or signatures do not match exactly, by creating a bridge between the interface's expected signature and the class's actual implementation.
*   **Try Operator**: The `?` operator is lowered into a sequence of operations: extracting the error from a result, checking if it is non-null, and performing an early return if an error is present, followed by extracting the success value.

### The Optimization Pass

The optimizer runs after lowering to refine the LIR. One of its most critical tasks is **side-effect temporary elimination**. The lowerer often assigns the result of every call to a temporary, even if the result is immediately discarded (e.g., in a void-context call). The optimizer identifies these patterns and fuses them into `LStmtSideEffect` statements, which backends can emit as bare expressions.

Another vital optimization is **multi-return destructuring**. Lyric allows functions to return multiple values, such as `(T, error)`. The lowerer initially represents this as a single temporary of type `LTyErrorResult`. The optimizer identifies patterns where this temporary is immediately destructured into separate variables and replaces them with a single `LStmtMultiAssign`. This simplifies target code generation for multi-return values.

### Monomorphization

For backends like C that lack native generic support, monomorphization is mandatory. The pass operates in six distinct phases:

1.  **Indexing**: It indexes all generic declarations (functions, structs, classes) in the program.
2.  **Collection**: It performs a deep walk of all function bodies to collect every unique instantiation of generic functions, classes, and structs based on the concrete type arguments used at call sites.
3.  **Specialization**: It generates specialized copies of these declarations, mangling their names (e.g., `Stack_i32`) to ensure uniqueness. This involves deep cloning the function bodies and substituting type variables with concrete types.
4.  **Rewriting**: It rewrites all call sites throughout the program to use these specialized, non-generic versions.
5.  **Signature/Field Rewrite**: It rewrites function signatures and field types to use the mangled names of specialized types.
6.  **Bare Name Resolution**: It resolves any remaining bare generic class names in specialized function bodies using a per-function rename map.

A critical aspect of monomorphization in Lyric is handling **Impl Method Renames**. When a class implements an interface through an `impl` block, the lowerer may generate forwarding wrappers or rename methods to avoid collisions (especially in C). The monomorphizer tracks these renames in `prog.ImplMethodRenames` and ensures that specialized versions of default interface methods call the correct concrete method names on specialized classes.

### The C Backend

The **C Backend** is significantly more involved due to C's lack of high-level features. It relies on a small runtime header (`lyric_runtime.h`) that defines macros and structures for slices, optional types, and error results. It hoists lambdas to top-level static functions and uses `malloc` for class allocations.

One of the most complex transformations in the C backend is the **Generator State Machine**. Lyric generators are lowered into a stateful struct and two functions: an `_init` function that allocates the state and copies parameters, and a `_next` function that executes the body. The `_next` function uses a `switch` statement with `goto` labels to resume execution from the last `yield` point. Local variables that must persist across yields are hoisted into the generator's state struct.

The C backend also implements **Interface VTable Dispatch**. For each interface, it generates a vtable struct containing function pointers for each method. Boxed interface values are represented as a struct containing a data pointer and a vtable pointer. Static vtable instances are generated for each (class, interface) pair, and method calls on interface types are dispatched through these vtables.

## Dependencies

*   [pkg/ast](../ast/design.md): Provides the source AST and node definitions.
*   [pkg/checker](../checker/design.md): Provides the type registry and resolved type information.
*   [runtime](../../runtime/design.md): Provides the C runtime contract for the C backend.

## Technical Debt and Future Work

*   **Map Support in C**: The C backend currently lacks implementation for map types.
*   **Concurrency in C**: While basic spawn support exists, more advanced concurrency features like select are not yet fully supported in the C backend.
*   **Advanced Optimizations**: Future improvements could include constant folding, dead code elimination beyond unused temporaries, and function inlining.
*   **Relational Constraints**: While basic support exists, more complex scenarios involving multi-class interfaces may require further refinement.
```

---

### ASSISTANT (gemini)

**Tool Call: `read_file`**
```json
{
  "file_path": "pkg/parser/design.md"
}
```

---

### USER (gemini)

**Tool Result:**
```
[ID: tool_629e8c0c9a954a71]
File: /Users/bill/projects/lyric/pkg/parser/design.md (110 lines)

# Parser Module Design

The `parser` module is the foundational entry point for the Lyric compiler frontend. Its primary responsibility is to transform raw Lyric source code—whether in declarative `.lyric` files or imperative `.ly` files—into a structured Abstract Syntax Tree (AST). It employs a hand-written lexical scanner and a hybrid parsing strategy that combines recursive descent for high-level structures with a Pratt (precedence climbing) parser for expressions. This architecture ensures that the parser is both highly performant and capable of providing precise error reporting.

## Executive Summary

The `parser` module serves as the "front door" of the compilation pipeline. It takes raw source text and produces a comprehensive representation of the program's structure as defined in the [pkg/ast](../ast/design.md) module. The module is designed to handle the unique syntactic requirements of the Lyric language, including significant newlines, complex f-string interpolation, and a rich set of declarative constructs for architectural specification. By producing a unified AST, it allows subsequent stages of the compiler, such as the checker and the lowerer, to operate on a consistent data model regardless of the original source file's purpose. The parser is designed to be LL(1) where possible, using targeted backtracking only for specific grammatical ambiguities.

## File Inventory

- [lexer.go](lexer.go): Implements a stateful, hand-written lexical scanner. It tokenizes the source text into a stream of `Token` objects, handles significant newlines, and collects line comments for inclusion in the final AST. It also manages complex literal scanning for triple-quoted strings and f-strings.
- [parser.go](parser.go): Contains the core `Parser` implementation and the recursive descent logic for top-level declarations. It manages the parsing of `lyric` blocks, classes, interfaces, enums, and complex type expressions. It handles the high-level structure of both `.lyric` and `.ly` files and manages contextual keyword resolution.
- [expr_parser.go](expr_parser.go): Implements the expression engine using the Pratt algorithm and the statement parser using recursive descent. It also manages specialized logic for match expressions, lambdas, concurrency primitives (`spawn`, `select`, `lock`), and the recursive parsing of interpolated f-strings.
- [parser.lyric](parser.lyric): A Lyric specification file that provides a high-level architectural overview and self-documentation of the parser module.
- [lexer_test.go](lexer_test.go): Unit tests for the lexical scanner, covering tokenization, newline handling, and literal scanning.
- [parser_test.go](parser_test.go): Unit tests for the declaration parser, ensuring correct AST construction for various Lyric constructs.
- [expr_parser_test.go](expr_parser_test.go): Unit tests for the expression and statement parser, covering precedence, control flow, and complex expression trees.

## Architecture and Data Flow

The transformation from raw text to an AST follows a well-defined pipeline within the module. The process begins with the `Lexer`, which reads the source string character by character. Unlike many traditional lexers, the Lyric lexer treats newlines as significant tokens (`TNewline`) when they serve as statement or declaration separators, while collapsing multiple consecutive newlines to simplify the parser's job. Newlines are suppressed when the lexer is inside brackets (`()`, `[]`, `{}`), allowing for ergonomic multi-line expressions without explicit continuation characters. Comments are captured during this phase and stored in a dedicated slice, allowing them to be attached to the root `ast.File` node without cluttering the token stream.

The `Parser` consumes tokens from the `Lexer` and uses recursive descent to navigate the language's grammar. When the parser identifies a top-level construct, such as a `class` or `func`, it recursively calls specialized methods to parse the constituent parts. For expressions, the parser delegates to the Pratt parsing logic in `expr_parser.go`. This algorithm uses a precedence table to efficiently handle operator precedence and associativity, allowing for complex expressions like method call chains and nested indexing without the "grammar explosion" often seen in pure recursive descent parsers.

As the parser matches grammatical rules, it instantiates the corresponding nodes from the `pkg/ast` module. These nodes are linked together to form a tree. A notable feature of this process is the handling of f-strings: when the lexer encounters an interpolated string, it captures the raw content; the parser then identifies the `{expr}` boundaries and recursively invokes a new parser instance to process the embedded expressions, merging the results into a `StringInterpExpr` node.

## Interface Implementations

The `parser` module does not implement external interfaces. It is designed as a pure producer of the `ast.File` structure. This structure serves as the primary data contract consumed by the [pkg/checker](../checker/design.md) and the [pkg/lir](../lir/design.md) modules.

## Public API

### Entry Points

The module provides two primary entry points for external callers:

- **`ParseFile(source, filename string) (*ast.File, error)`**: This is the standard entry point for parsing a Lyric source file. It initializes the lexer and parser, executes the parsing logic, and returns the resulting AST. If parsing fails, it returns a `ParseError` containing detailed location information.
- **`ParseString(source string) (*ast.File, error)`**: A convenience wrapper around `ParseFile` that uses `"<string>"` as the filename, typically used for testing or parsing small snippets of code.

### Core Types

- **`Parser`**: The central parsing engine. While it can be instantiated manually, it is typically used through the package-level entry points. It maintains the lexer state, a list of accumulated errors, and nesting depth information.
- **`Lexer`**: The stateful scanner responsible for tokenization. It provides `Peek` and `Next` methods for the parser to consume tokens and maintains a collection of scanned comments.
- **`ParseError`**: A specialized error type that implements the `error` interface. It includes a descriptive message, an `ast.Span` identifying the location, and the original source line for enhanced error reporting.
- **`Token`**: Represents a single lexical unit, containing its kind, raw text, and source span.

## Implementation Details

### Lexical Scanning and Significant Newlines

The lexer is a stateful scanner that uses a `peek`/`advance` pattern. It is responsible for identifying keywords, literals, and punctuation. A critical aspect of the Lyric lexer is its handling of newlines. Newlines are preserved as tokens to separate statements, but the lexer automatically collapses multiple consecutive newlines and skips whitespace that does not contribute to the structure. To support ergonomic multi-line expressions, the lexer tracks bracket nesting depth and suppresses newline tokens when inside `()`, `[]`, or `{}`. This allows the parser to remain simple while supporting an ergonomic, newline-sensitive syntax.

### Disambiguation and Backtracking

The Lyric grammar contains several ambiguities that require lookahead to resolve. The parser handles these cases using a simple but effective backtracking mechanism: it saves the current state of the `Lexer` (which is a small, copyable struct), attempts to parse a specific construct, and restores the saved state if the attempt fails. This is used in several primary scenarios:
- **Struct Literals vs. Blocks**: When the parser encounters an identifier followed by `{`, it peeks ahead to see if the content looks like a field initializer (`name: value`) or an empty literal `{}` to distinguish a struct literal from a block of code.
- **Generic Calls vs. Comparisons**: The `<` operator can start a generic argument list or a comparison. The parser attempts to parse a type argument list and backtracks if it fails or if the following token is not an opening parenthesis.
- **Why Annotations vs. Fields**: The `why` keyword can start an annotation (`why: "..."`) or be a field name (`why: Type`). The parser peeks ahead to see if a string literal follows the colon.
- **Tuple Field Labels**: When parsing tuple types or literals, the parser checks if an identifier is followed by a colon to distinguish labeled fields from positional ones.
- **Nested Generics (>> Splitting)**: When parsing nested generic types like `List<List<T>>`, the lexer may emit a `TShr` (>>). The parser detects this in generic argument contexts and splits it into two `TGt` (>) tokens, pushing the second one back into a local buffer for the next `peek()` or `next()` call.

### Pratt Parsing for Expressions

The expression parser in `expr_parser.go` implements the Pratt algorithm, also known as precedence climbing. This approach is particularly well-suited for languages with many levels of operator precedence. The parser maintains a table of precedence levels and associativity for each operator.

The precedence levels are defined as follows:
1.  **OR**: `||`
2.  **AND**: `&&`
3.  **Bitwise OR**: `|`
4.  **Bitwise XOR**: `^`
5.  **Bitwise AND**: `&`
6.  **Equality**: `==`, `!=`
7.  **Comparison**: `<`, `>`, `<=`, `>=`
8.  **Shift**: `<<`, `>>`
9.  **Additive**: `+`, `-`
10. **Multiplicative**: `*`, `/`, `%`
11. **Unary**: `-`, `!` (prefix)
12. **Postfix**: `.`, `()`, `[]`, `!`, `?`

When parsing an expression, it recursively climbs the precedence ladder, ensuring that operators are grouped correctly. Postfix operators, such as member access (`.`), function calls (`()`), indexing (`[]`), unwrap (`!`), and try (`?`), are handled in a loop after the primary expression is parsed, allowing for arbitrary chaining of operations.

### Contextual Keywords and Aliases

To maintain a clean and ergonomic syntax, Lyric treats many keywords (such as `why`, `doc`, `source`, and various annotations) as "contextual." The `expectIdentLike` method allows these tokens to be used as identifiers in positions where they are not ambiguous, such as field names or parameter names. This prevents the language from having an overly restrictive set of reserved words, which is particularly important for self-documenting `.lyric` files. Additionally, `null` is treated as a keyword alias for `nil`, both mapping to the same `TNil` token kind and `ExprNil` AST node to accommodate developers from different backgrounds.

### Statement and Block Parsing

Statements are parsed using recursive descent, recognizing keywords like `let`, `if`, `for`, `while`, `match`, `return`, `spawn`, `select`, `lock`, `cascade`, and `yield`. If no keyword is matched, the parser attempts to parse an expression statement or an assignment. 

Assignments desugar compound operators (like `+=`) into their binary equivalents during the parsing phase (e.g., `x += y` becomes `x = x + y`), simplifying the work for later compiler stages. The `match` statement supports complex pattern matching, including variant patterns, tuple patterns, and guards. The `cascade` statement provides a mechanism for deferred cleanup, while `spawn`, `select`, and `lock` provide first-class support for concurrency.

### F-String Interpolation

F-strings (`f"..."`) are handled by capturing the raw content in the lexer. The parser then scans this content for `{expr}` boundaries. For each expression found, it creates a new `Parser` instance to process the embedded text, and then merges the resulting AST nodes into a `StringInterpExpr`. This recursive approach allows for full expression power within interpolated strings.

### Recursive Type Parsing

The parser handles Lyric's recursive type system through a tiered approach in `parseTypeExpr`. It first parses a "base type" (named types, sequences `[T]`, tuples `(T, U)`, maps `map[K]V`, channels `channel<T>`, etc.), then checks for optional suffixes like `?` (optional) or `|` (union), and finally checks for function type arrows `->`. This structure correctly handles precedence, such as `[T]?` being an optional sequence versus `[T?]` being a sequence of optionals. It also handles the `fn(T, U) -> V` prefix form for function types.

## Dependencies

- **[pkg/ast](../ast/design.md)**: The parser is entirely dependent on the `ast` module for its output data structures. It uses `ast.Pos` and `ast.Span` for location tracking and constructs various `ast.Node` types to build the tree.

## Technical Debt and Future Work

- **Error Recovery**: The current implementation often stops at the first encountered error. Implementing a synchronization mechanism (e.g., skipping to the next lyric block or closing brace) would allow the parser to report multiple errors in a single pass, improving the developer experience.
- **Multi-line Arrays**: Some declarative constructs like `source: [...]` currently require single-line formatting due to parser limitations.
- **Requires/Ensures Expressions**: Currently, these annotations capture raw text rather than fully parsed expression trees. Moving to a parsed representation would allow for better validation during the checking phase.
- **Lambda Union Types**: Lambda parameters currently use `parseBaseType` to avoid ambiguity with the `|` operator, which prevents the direct use of union types in lambda signatures without a type alias.
```

---

### ASSISTANT (gemini)

**Tool Call: `read_file`**
```json
{
  "file_path": "pkg/verifier/design.md"
}
```

---

### USER (gemini)

**Tool Result:**
```
[ID: tool_3ba2e1d336d298bc]
File: /Users/bill/projects/lyric/pkg/verifier/design.md (88 lines)

# Verifier Module Design

The `verifier` module serves as the structural integrity engine for the Lyric project, acting as a guardian against architectural drift. It performs a deep, recursive comparison between the high-level architectural intent declared in `.lyric` files and the actual implementation in Go source code. By bridging two distinct type systems, it ensures that the developer's mental model—captured in documentation and specifications—remains synchronized with the evolving codebase.

## Executive Summary

The `verifier` module provides a multi-stage analysis pipeline that validates Go source code against Lyric specifications. It parses `.lyric` files, discovers referenced Go source files, extracts their type information using the Go standard library's AST tools, and performs a structural comparison. The verifier identifies mismatches in fields, methods, function signatures, and naming conventions, reporting them with varying severity levels (Error, Warning, Info). A key feature is its "completeness check," which ensures that all exported Go symbols are documented in the corresponding Lyric block, preventing the specification from becoming an incomplete or outdated subset of the implementation. Beyond simple drift detection, the module also serves as a host for project-wide invariant testing, using its AST analysis capabilities to enforce critical architectural rules that cannot be captured by the type system alone.

## File Inventory

- [verifier.go](verifier.go): The primary implementation file containing the verification logic, Go type extraction, and structural comparison algorithms. It defines the `Verify` entry point and the `Result` and `Finding` types.
- [invariants_test.go](invariants_test.go): A specialized test suite that enforces cross-cutting project invariants by analyzing the Go AST of other modules. It ensures that critical architectural rules, such as the value-type nature of AST nodes, are maintained across the codebase.
- [verifier_test.go](verifier_test.go): Comprehensive test suite that validates drift detection, naming convention mapping, and completeness checks against both synthetic examples and the project's own specifications.
- [verifier.lyric](verifier.lyric): The module's own architectural specification, providing a self-documented model of its internal logic and public API.

## Architecture and Data Flow

The verification process is designed as a stateless operation from the perspective of the caller, although it maintains significant internal state during a single run to aggregate information across multiple files. The workflow begins by delegating to the [pkg/parser](../parser/design.md) module to produce a `lyricast.File` from the provided `.lyric` source path. The verifier then iterates through each `lyric` block within the file, inspecting the `source:` annotations to locate the relevant Go files or directories. These paths are resolved relative to the directory containing the `.lyric` file itself, ensuring that the specification and its implementation are co-located or predictably linked.

For each discovered source, the verifier utilizes the `go/parser` and `go/ast` packages from the Go standard library to extract detailed type information. This data is populated into a `goTypeInfo` registry, which serves as a comprehensive database of all structs, interfaces, functions, and type definitions found across all specified sources for a given block. This aggregation is a critical design choice, as a single Lyric block's implementation may span multiple Go files within the same package. The `mergeGoInfo` function handles the complex task of combining information from multiple files, ensuring that methods defined in different files for the same struct are correctly associated and that field definitions are unified.

Once the Go type registry is fully populated, the verifier recursively walks the Lyric AST, comparing each declaration—including structs, classes, interfaces, enums, and functions—against the aggregated Go types. It normalizes naming conventions and type expressions on the fly to enable direct comparison between the two systems. After validating the declared Lyric symbols, the verifier performs a reverse check via `verifyCompleteness`. This function walks all exported Go symbols in the source files to identify any that are missing from the Lyric specification, ensuring the documentation remains a complete representation of the public API. Findings are collected into a `Result` object, which classifies each discrepancy by severity and provides detailed messages and file locations.

## Interface Implementations

The `verifier` module is a standalone utility and does not implement interfaces defined in other packages. It acts as a consumer of the AST produced by [pkg/ast](../ast/design.md) and [pkg/parser](../parser/design.md) and provides its own reporting structures for use by CLI tools and CI pipelines.

## Public API

- `Verify(lyricPath string) (*Result, error)`: The primary entry point. Orchestrates the entire pipeline, from parsing the Lyric file and discovering Go sources to extracting types and performing the final comparison.
- `Result`: A container for the findings of a verification run.
    - `ErrorCount() int`: Returns the number of error-level findings, used to determine build failure.
    - `Findings []Finding`: The list of individual discrepancies found.
- `Finding`: Represents a single discrepancy.
    - `Severity`: Classifies the finding as `Error`, `Warning`, or `Info`.
    - `LyricFile`, `GoFile`: Locations of the discrepancy.
    - `Message`: Descriptive text explaining the mismatch.
- `Severity`: An enumeration of `Error`, `Warning`, and `Info`.

## Implementation Details

### Deep Type Comparison

The core of the verifier's logic resides in the `lyricTypeToGoString` function, which recursively converts Lyric `TypeExpr` nodes into a Go-compatible string format. This transformation allows the verifier to compare types across the two systems using string equality. Key mappings include converting Lyric optional types (`T?`) to Go pointers (`*T`), sequences (`[T]`) to Go slices (`[]T`), and maps directly to their Go equivalents. Lyric tuples are used to represent Go's multiple return values and are unpacked during comparison to be checked element-wise against the Go function's return list. Lyric `unit` represents a void return or an empty parameter list.

The `typesMatch` function serves as the final arbiter of type equality. It handles the equivalence of `any` and `interface{}` and employs a `stripPackagePrefix` helper to heuristically remove package qualifiers, such as converting `ast.File` to `File`. This allows Lyric files to use unqualified names while the Go implementation uses fully qualified types. To avoid false positives, the function returns true if either side contains a "?" character, which represents an unconvertible or unknown type.

### Name Mapping and Normalization

Lyric typically uses `snake_case` for identifiers, while Go uses `PascalCase` for exported symbols and `camelCase` for unexported ones. The verifier bridges this gap by attempting three-way matching in functions like `findGoName` and `findGoFieldType`. It checks for the `PascalCase` version (via `snake_to_pascal`) for exported fields and methods, the `camelCase` version (via `snake_to_camel`) for unexported identifiers, and finally an exact match. Lyric primitive names are also mapped to their Go equivalents, such as `i32` to `int32` and `u8` to `uint8`.

### Verification Rules

The verifier applies specific rules based on the type of declaration encountered. For structs and classes, it checks for field existence and type matching. For classes, it additionally verifies method signatures, comparing parameter counts (excluding the `self` parameter) and ensuring that positional types and return types match. Interface verification ensures that every method declared in Lyric exists in the Go interface with a matching signature. Enums are validated against three common Go patterns: typedefs, structs, or interfaces, and the verifier also checks for the common `XxxKind` naming pattern. Functions undergo positional parameter and return type comparison, with multi-return Go functions mapped to Lyric tuple returns.

### Completeness Check

The `verifyCompleteness` function ensures that the Lyric specification is a complete model of the public API. It identifies all exported Go symbols—types and functions—in the referenced source files and reports an error for any that are not documented in the Lyric block. It uses the `isExported` helper, which checks for an uppercase first letter, to distinguish between the public API and internal implementation details.

## Project Invariants

A unique responsibility of the `verifier` module, implemented in `invariants_test.go`, is the enforcement of project-wide architectural invariants. These are rules that cannot be easily enforced by the Go type system but are critical for the correct operation of the compiler. The module uses the Go standard library's AST tools to inspect the source code of other modules and verify these rules.

Key invariants currently enforced include:
- **AST Value Types**: Ensuring that `ast.Expr` slices are never iterated using range-copy (e.g., `for _, x := range slice`), which would copy the value type and lose checker annotations. Instead, index-based iteration (`&slice[i]`) must be used.
- **Monomorphization Safety**: Verifying that field and type lookups in the backend correctly check `ClassRenames` to account for monomorphized generic types.
- **Standard Library Integration**: Ensuring that the standard library is merged into the correct AST block.
- **Desugaring Order**: Enforcing the specific sequence of desugaring passes (Embeds → InterfaceFields → Relations → Destructors → DefaultImpls) to ensure correct transformation of high-level constructs.
- **Checker Multi-Phase Structure**: Validating that the checker maintains its multi-phase resolution strategy, which is essential for handling cross-file type dependencies.

## Dependencies

- **[pkg/ast](../ast/design.md)**: Provides the `LyricBlock` and `TypeExpr` structures used to represent the Lyric specification.
- **[pkg/parser](../parser/design.md)**: Used to parse the `.lyric` file into an AST.
- **Go Standard Library**:
    - `go/ast` and `go/parser`: Used to analyze the Go source code and extract type information.
    - `go/token`: Used for position tracking in Go files.
    - `os` and `path/filepath`: Used for file system operations and path resolution.

## Technical Debt and Future Work

- **Expression Verification**: Currently, `requires` and `ensures` clauses are treated as raw strings and are not parsed or verified against the Go implementation. Future versions could use a Go expression parser to validate these constraints.
- **Complex Pointer Indirection**: While basic optional-to-pointer mapping works, deeply nested or complex pointer structures may occasionally cause false positives or be represented as `?`.
- **Import Resolution**: The `stripPackagePrefix` mechanism is heuristic and does not perform full symbol resolution, meaning it cannot distinguish between types with the same name in different packages.
- **Performance**: The verifier re-parses Go files for every Lyric block. For large projects, a caching mechanism for parsed Go ASTs and `goTypeInfo` would significantly improve performance.
- **Complex Function Types**: Complex function types, such as maps containing functions, cannot currently be fully expressed in Lyric and may be typed as `any` or `?`, leading to known false-positive errors in some edge cases.
- **Generic Constraints**: While basic generic types are supported, complex constraints are not currently compared.
```

---

### ASSISTANT (gemini)

**Tool Call: `read_file`**
```json
{
  "file_path": "runtime/design.md"
}
```

---

### USER (gemini)

**Tool Result:**
```
[ID: tool_7f6deb4893df735e]
File: /Users/bill/projects/lyric/runtime/design.md (118 lines)

# Runtime Module Design

## Executive Summary

The `runtime` module provides the foundational C runtime library required by the Lyric compiler's C backend. It is implemented as a passive, header-only library that establishes the memory layout and operational semantics for Lyric's high-level constructs in a C environment. By providing a suite of macros and inline functions, the runtime enables the C backend to transform complex Lyric features—such as dynamic slices, length-prefixed strings, optional types, error results, tagged unions, and concurrency primitives—into efficient and readable C code. Its primary goal is to bridge the gap between Lyric's expressive type system and C's low-level primitives while maintaining semantic equivalence and providing a robust execution environment.

## File Inventory

- [lyric_runtime.h](lyric_runtime.h): The core runtime header. It contains all macro definitions, type structures, and inline helper functions that constitute the Lyric execution environment for C targets. It defines the fundamental data structures for slices, strings, optionals, and results, as well as the logic for concurrency and file I/O.

## Architecture and Data Flow

The `runtime` module is a passive component that does not maintain internal state or have an independent execution flow. Instead, it acts as a contract and a toolkit for the code generated by the [LIR module](../pkg/lir/design.md).

The architecture leverages C macros to implement a form of "template-based" type generation. Since C lacks native support for generics, the Lyric C backend uses these macros to define concrete, specialized struct types for every unique instantiation of a generic Lyric type (e.g., `Slice_int32_t`, `Opt_string`, or `Chan_int32_t`). This approach ensures type safety at the C level and allows the compiler to generate code that is tailored to specific data types. Data flows through these structures as they are passed between functions, with the runtime providing the logic for common operations like resizing slices, wrapping values in optional containers, or managing thread-safe communication through channels.

## Interface Implementations

As a C header library, this module does not implement any Go interfaces. It serves as the target execution environment for the C code generator within the [LIR module](../pkg/lir/design.md).

## Public API

The Public API of the runtime consists of the macros and inline functions intended to be utilized by the Lyric-generated C code. These are categorized by the Lyric language features they support.

### Dynamic Slices
Lyric slices are implemented as dynamic arrays with automatic resizing capabilities and support for sub-slicing.
- `LYRIC_SLICE_DEF(ElemType, SliceName)`: A macro that defines a struct containing a pointer to the data, the current length, and the total capacity.
- `lyric_slice_empty(SliceName)`: Initializes a slice structure to an empty state (null data, zero length and capacity).
- `lyric_push(slice_ptr, elem, SliceName)`: Appends an element to the slice. If the length reaches the capacity, it performs a geometric resize (doubling the capacity) using `realloc`.
- `lyric_pop(slice_ptr)`: Removes and returns the last element of the slice. The caller is responsible for ensuring the slice is not empty.
- `lyric_subslice(slice, low, high, SliceName)`: Creates a new slice view representing the range `[low, high)`. This view shares the underlying data pointer with the original slice.
- `lyric_slice_lit(SliceName, ElemType, ...)`: A convenience macro that creates a new slice from a C initializer list, allocating heap memory for the contents.
- `lyric_contains(slice, elem)`: Performs a linear search through the slice to determine if a specific element is present.

### Strings
Strings in Lyric are implemented as length-prefixed byte slices (`lyric_string`), which is an alias for `LyricSlice_uint8_t`. This allows strings to contain embedded null bytes and ensures that length-related operations are O(1).
- `LYRIC_STR(lit)`: A macro to create a `lyric_string` from a C string literal at compile-time.
- `lyric_str_from_cstr(cstr)`: Creates a `lyric_string` from a null-terminated C string, copying the data to the heap.
- `lyric_str_from_bytes(data, len)`: Creates a `lyric_string` from raw bytes, copying them to the heap and adding a hidden trailing null for C interop.
- `lyric_str_eq(a, b)`: Performs a length-aware equality check.
- `lyric_str_cmp(a, b)`: Performs a lexicographic comparison.
- `lyric_str_concat(a, b)`: Concatenates two strings into a new heap-allocated `lyric_string`.
- `lyric_str_contains`, `lyric_str_index_of`, `lyric_str_has_prefix`, `lyric_str_has_suffix`: Length-aware string searching and prefix/suffix checks.
- `lyric_str_replace`, `lyric_str_repeat`, `lyric_str_join`: High-level string transformations that return new heap-allocated strings.
- `lyric_str_trim`: Returns a new string with leading and trailing whitespace removed.
- `lyric_fprint_str(file, s)`: Writes a `lyric_string` to a `FILE*` in a length-aware manner.

### Optional Types
Optional types (`T?`) are represented as a struct containing a boolean flag (`has`) and the actual value.
- `LYRIC_OPT_DEF(ElemType, OptName)`: Defines a specialized optional struct for a given type.
- `lyric_some(value, OptName)`: Wraps a concrete value into an optional struct with the `has` flag set to true.
- `lyric_none(OptName)`: Returns an optional struct with the `has` flag set to false.
- `lyric_isnull(opt)`: A helper to check if an optional value is empty.
- `lyric_unwrap(opt)`: Extracts the value from an optional. This should only be called after verifying the value exists.

### Error Results
Lyric's idiomatic `(T, error)` return pattern is mapped to a result struct in C.
- `LYRIC_RESULT_DEF(ElemType, ResultName)`: Defines a result struct containing a success value, an error message (`const char*`), and a boolean `is_err` flag.
- `lyric_ok(val, ResultName)`: Constructs a successful result containing the provided value.
- `lyric_err(msg, ResultName)`: Constructs an error result containing the provided error message.
- `lyric_is_err(r)`: Checks if a result represents a failure.

### Concurrency Primitives
The runtime provides support for Lyric's concurrency model using POSIX threads (pthreads).
- `LYRIC_CHAN_DEF(ElemType, ChanName)`: Defines a thread-safe, buffered or unbuffered channel structure.
- `LYRIC_CHAN_IMPL(ElemType, ChanName, Suffix)`: Generates the implementation functions for a specific channel type, including `make`, `send`, `recv`, `close`, and `free`.
- `lyric_spawn(func, arg)`: Launches a function in a new detached thread, implementing the Lyric `spawn` keyword.

### Utility Helpers
- `lyric_hash_string`: Implements the FNV-1a hashing algorithm for strings.
- `lyric_sprintf`: A wrapper around `vsnprintf` that automatically calculates the required size and returns a heap-allocated formatted string.
- `lyric_bool_str`: Returns "true" or "false" for a boolean value.
- `lyric_toupper`, `lyric_tolower`: Convert strings to upper or lower case, returning new heap-allocated strings.
- `lyric_char_to_string`: Converts a single character (uint8_t) to a heap-allocated string.
- `lyric_read_file`: Reads an entire file into a heap-allocated string, returning a `lyric_str_bool_t` struct.
- `lyric_write_file`: Writes a string to a file, returning a boolean indicating success.
- `lyric_getwd`: Returns the current working directory.
- `lyric_path_dir`, `lyric_path_base`, `lyric_path_ext`: Utilities for working with file paths.
- `lyric_itoa`: Converts an `int64_t` to a heap-allocated string.
- `lyric_atoi`: Parses a string into an `int64_t`, returning a `lyric_atoi_result` struct.
- `lyric_parse_float`: Parses a string into a `double`, returning a `lyric_parse_float_result` struct.

### Tagged Unions
For ad-hoc union types (e.g., `string | i32`), the runtime provides a generic `LyricUnion` struct.
- `LyricUnion`: A struct containing an integer tag and a union of supported Lyric types (i32, i64, f32, f64, bool, string, and pointers).
- `LYRIC_UNION_TAG_*`: Constants for identifying the active member of a `LyricUnion`.
- `lyric_union_i32`, `lyric_union_string`, etc.: Specialized constructor functions that initialize a `LyricUnion` with the correct tag and value.

## Implementation Details

### Memory Management
The runtime currently utilizes the standard C library's `malloc`, `realloc`, and `free` for memory management. A critical implementation detail is that the runtime does not yet include a garbage collector. Consequently, heap-allocated objects—including slices, strings created by helper functions, and class instances—are currently leaked during execution. This is a known limitation that is acceptable for short-lived compiler tasks but will require a GC (such as Boehm-Demers-Weiser) for broader use.

### Slice Growth Strategy
The `lyric_push` macro implements a geometric growth strategy to ensure amortized O(1) append operations. When a slice's length reaches its capacity, the capacity is doubled. If the slice was previously empty, it is initialized with a default capacity of 4. This minimizes the number of reallocations required as a slice grows.

### String Handling
Strings in the Lyric C runtime are represented by the `lyric_string` struct, which contains a pointer to the data and its length. This design allows for O(1) length retrieval and supports strings with embedded null bytes. For compatibility with standard C functions that expect null-terminated strings, heap-allocated `lyric_string` objects carry a hidden trailing `\0` byte immediately following the data. This trailing byte is not included in the `.len` field.

### Concurrency Implementation
Channels are implemented using a circular buffer protected by a `pthread_mutex_t`. Two condition variables (`not_empty` and `not_full`) are used to manage blocking send and receive operations. `lyric_spawn` uses `pthread_create` and `pthread_detach` to provide fire-and-lyrict thread execution.

### Tagged Union Dispatch
The `LyricUnion` type uses an integer tag to identify the active member of its internal union. This allows the generated C code to perform safe type dispatching (e.g., in `match` statements) by checking the tag before accessing the union's data.

## Dependencies

- **Standard C Library**: The runtime depends on standard headers including `<stdio.h>`, `<stdlib.h>`, `<stdint.h>`, `<stdbool.h>`, `<string.h>`, `<stdarg.h>`, `<ctype.h>`, and `<unistd.h>`.
- **POSIX Threads**: The concurrency primitives require the `pthread` library.
- **LIR Module**: This module is a critical dependency for the C backend in [pkg/lir](../pkg/lir/design.md), which relies on the macros and types defined here to emit valid C code.

## Technical Debt and Future Work

- **Garbage Collection**: Integrating a garbage collector is the highest priority to prevent memory leaks in long-running Lyric programs compiled to C.
- **Map Implementation**: A robust hash map implementation is required to support Lyric's `map<K, V>` type in the C backend.
- **Error Handling Refinement**: The current error result uses a simple string for error messages. This should be expanded to support more complex, structured error types as the language evolves.
- **Thread Safety**: While channels are thread-safe, other runtime primitives (like slices) are not and will require synchronization if shared across threads.
```

---

### ASSISTANT (gemini)

**Tool Call: `write_file`**
```json
{
  "content": "# Lyric Project Architecture\n\n## Executive Summary\n\nThe Lyric project is a comprehensive compiler and toolchain for the Lyric programming language. It is designed to provide a high-level, expressive language with advanced features such as relations, interfaces, generic type inference, and structured control flow, while maintaining the capability to compile down to multiple backends, currently supporting Go and C. The project exists to bridge the critical gap between high-level architectural intent and low-level implementation. By capturing design specifications in dedicated `.lyric` files, the system ensures structural integrity and actively prevents architectural drift. Its primary goals are to deliver a robust, multi-stage compilation pipeline—spanning parsing, semantic analysis, low-level intermediate representation, and code generation—while maintaining a strong, verifiable connection between the documented design and the evolving codebase through its unique verification engine.\n\n## System Architecture\n\nThe Lyric compiler follows a classic pipeline architecture, augmented by a parallel verification track that acts as an architectural guardian. The architecture is fundamentally layered and data-centric, designed to isolate complexity at each distinct stage of transformation. The primary compilation flow is linear, progressing from raw source text to an Abstract Syntax Tree (AST), through semantic checking, into a Low-level Intermediate Representation (LIR), and finally to target code generation.\n\nThe philosophy of the system is to resolve specific classes of problems in dedicated modules, ensuring that downstream consumers operate on increasingly simplified and explicit data models. The parser handles all syntax and grammatical ambiguities, employing a hybrid strategy of recursive descent and Pratt parsing to produce a structural representation. It manages significant newlines and complex f-string interpolations, providing a clean foundation. The AST module acts as the central data hub, providing a representation of the program using a highly efficient \"Kind + Data\" pattern. Crucially, it features a generative desugaring engine that expands high-level syntactic sugar—such as relations, interface fields, and default implementations—into fundamental constructs before semantic analysis. It also selectively merges the standard library based on reachability analysis.\n\nThe checker assumes responsibility for all semantic rules, acting as the semantic truth of the compiler. It performs a multi-pass analysis for type inference, scope management, module import resolution, and validation. It handles structural subtyping, numeric widening, optional wrapping, and enforces concurrency safety through guarded annotations. Ultimately, it annotates the AST with resolved types. \n\nThe LIR module then takes this semantically rich tree and bridges the gap to the backends. It enforces structured control flow and flat expressions, assigning every sub-expression to a unique temporary variable. This crucial step removes the burden of semantic resolution from the backends. The LIR pipeline includes optimization passes for side-effect elimination and multi-return destructuring, as well as a comprehensive monomorphization pass to specialize generic types and functions for targets that lack native generics. Finally, the backends act as thin emitters that translate the simplified LIR into idiomatic target source code. The C backend relies heavily on the runtime module, which provides a foundational C header library establishing the memory layout and operational semantics for Lyric's high-level constructs.\n\nIn parallel to the compilation pipeline, the verifier module operates as a standalone structural integrity engine. It parses `.lyric` specification files and performs a deep structural comparison against the actual Go implementation, utilizing the Go standard library's AST tools to extract type information. It normalizes naming conventions across the two systems and performs a reverse completeness check to ensure every exported Go symbol is documented. This ensures that the documented design and the codebase remain perfectly synchronized, enforcing the philosophy that documentation should be an active, verifiable contract rather than a passive artifact.\n\n## Interface \u0026 Contract Map\n\nThe system's boundaries are primarily defined by the rich data structures passed between modules rather than traditional Go interfaces. These data structures act as the fundamental contracts that govern module interactions, ensuring a decoupled and data-driven architecture.\n\nThe most critical contract is the Abstract Syntax Tree. The `ast.File` and its constituent nodes representing declarations, expressions, and statements form the central nervous system of the compiler. The parser module acts as the initial producer of this contract, transforming source code into the raw AST. The AST module itself acts as both a consumer and producer, taking the raw AST and producing a desugared version where high-level constructs are flattened. The checker module consumes this desugared AST for semantic analysis and mutates it by attaching resolved type information to the `ResolvedType` and `InferredTypeArgs` fields. The LIR module consumes the fully type-checked AST to generate the intermediate representation. Additionally, the verifier module consumes the AST of `.lyric` files to compare against Go source code.\n\nThe Type Registry contract, defined by the `checker.Registry` and `checker.Type` structures, represents the semantic truth of the program. The checker module builds and populates this registry during its declaration and checking passes, resolving all cross-module dependencies and generic constraints. The LIR module is the primary consumer of this contract, relying heavily on the type registry and resolved types to correctly lower high-level constructs, such as generics, classes, and enums, into low-level operations.\n\nThe LIR contract, represented by the `lir.LProgram`, defines the fully resolved and flattened program state. The LIR module's lowerer produces this representation, ensuring that all expressions are flattened into temporaries and control flow is structured. Internal optimization and monomorphization passes within the LIR module consume and mutate this structure, specializing generic types and eliminating redundant operations. Ultimately, the backends consume the optimized `LProgram` to emit the final source code.\n\nThe Runtime contract is defined by the C runtime header (`lyric_runtime.h`), which establishes the execution environment for C-compiled Lyric programs. The runtime module provides these foundational macros and inline functions, defining the memory layout for dynamic slices, strings, optional types, error results, and tagged unions. The C backend within the LIR module acts as the sole consumer, emitting code that relies entirely on this contract for operational semantics and concurrency primitives.\n",
  "file_path": "project-design.md"
}
```

---

### USER (gemini)

**Tool Result:**
```
✅ **File Created Successfully**

**File**: `/Users/bill/projects/lyric/project-design.md`
**Size**: 7009 bytes
**Permissions**: -rw-r--r--
**Lines**: 30
**Duration**: 0.00s
```

---

### ASSISTANT (gemini)

**Tool Call: `write_file`**
```json
{
  "append": true,
  "content": "\n## Module Map\n\n### Categorized Tree View\n\n- **Frontend**\n  - [pkg/parser](pkg/parser/design.md)\n  - [pkg/ast](pkg/ast/design.md)\n- **Semantic Analysis**\n  - [pkg/checker](pkg/checker/design.md)\n- **Intermediate Representation \u0026 Backends**\n  - [pkg/lir](pkg/lir/design.md)\n  - [runtime](runtime/design.md)\n- **Tooling**\n  - [pkg/verifier](pkg/verifier/design.md)\n\n### Module Descriptions\n\nThe parser module is the foundational entry point for the compiler frontend, responsible for transforming raw Lyric source code into an Abstract Syntax Tree. It manages internal state through a hand-written lexical scanner that tokenizes the input, carefully handling significant newlines and collecting comments. The parser employs a hybrid strategy, utilizing recursive descent for high-level declarations and a Pratt parser for expressions, allowing it to efficiently handle complex operator precedence without grammar explosion. It implements the primary data contract by producing the `ast.File` consumed by the rest of the system, and it seamlessly handles complex features like f-string interpolation, contextual keywords, and disambiguation through targeted backtracking.\n\nThe AST module defines the core data structures that represent the structure of Lyric code, acting as the central data hub. It manages state primarily as a tree of polymorphic nodes utilizing a \"Kind + Data\" pattern to avoid deep interface hierarchies, making it highly efficient for recursive traversals. Crucially, it includes a generative desugaring engine that transforms high-level constructs, such as relations, interface fields, and default implementations, into fundamental AST nodes before semantic analysis occurs. It also handles the selective merging of the standard library based on reachability analysis, ensuring the compilation unit remains lean. It implements the primary data contract consumed by the checker and LIR modules.\n\nThe checker module implements the semantic analysis phase of the compiler, ensuring that a Lyric program is semantically valid before code generation. It maintains complex internal state, including a stack of scopes for variable tracking and a global registry for user-defined types. It consumes the desugared AST, resolves all type expressions, infers generic types through a unification-based approach, enforces concurrency safety through guarded annotations, and validates the exhaustiveness of match statements. It acts as the semantic truth of the compiler, annotating the AST nodes with `ResolvedType` information, fulfilling the semantic contract required by the subsequent lowering phase.\n\nThe LIR module is the architectural bridge between the high-level AST and the backend code generators. It resolves all semantic complexity into a simplified, flat representation while preserving structured control flow. It manages state during the lowering process by mapping AST nodes to LIR constructs and assigning all sub-expressions to unique temporary variables, ensuring that backends never encounter nested expression trees. It includes sophisticated optimization passes for side-effect elimination and multi-return destructuring, as well as a comprehensive monomorphization pass to specialize generic types for backends like C. It consumes the type-checked AST and produces the `LProgram` contract, which is then consumed by its internal C backend to emit target code, handling complex transformations like generator state machines and interface vtable dispatch.\n\nThe runtime module provides the foundational C runtime library required by the Lyric C backend. It is implemented as a passive, header-only library that establishes the memory layout and operational semantics for Lyric's high-level constructs in a C environment. It does not maintain internal state but provides a suite of macros and inline functions for dynamic slices, length-prefixed strings, optional types, error results, tagged unions, and concurrency primitives. These foundational elements are consumed by the C code generated by the LIR module, bridging the gap between Lyric's expressive type system and C's low-level primitives.\n\nThe verifier module is a standalone structural integrity engine designed to detect architectural drift by comparing high-level `.lyric` specifications against the actual Go implementation. It maintains state during a run to aggregate type information extracted from Go source files using the standard library's AST tools. It consumes the AST produced by the parser for `.lyric` files and performs deep structural comparisons, normalizing naming conventions and mapping types across the two systems. It also performs a reverse completeness check, ensuring every exported Go symbol is documented in the Lyric specification, and aggregates any mismatches into a detailed report. Additionally, it enforces project-wide architectural invariants by analyzing the Go AST of other modules.\n",
  "file_path": "project-design.md"
}
```

---

### USER (gemini)

**Tool Result:**
```
✅ **File Created Successfully**

**File**: `/Users/bill/projects/lyric/project-design.md`
**Size**: 11918 bytes
**Permissions**: -rw-r--r--
**Lines**: 30
**Duration**: 0.00s
```

---

### ASSISTANT (gemini)

**Tool Call: `write_file`**
```json
{
  "append": true,
  "content": "\n## Integration Patterns \u0026 Workflows\n\nThe system relies on clear, sequential workflows to transform data across module boundaries. Two primary workflows define the operation of the Lyric toolchain: the compilation pipeline and the architectural verification process.\n\nThe compilation pipeline workflow traces the journey of a Lyric source file from raw text to executable code. The process begins when a file is read from disk and passed to the parser module. The lexer tokenizes the text, handling significant newlines and f-string interpolation, and the parser constructs an initial `ast.File` using recursive descent and Pratt parsing. This AST is immediately passed to the AST module's desugaring functions, which expand complex features like relations into standard classes and implementation blocks, extract default interface implementations, and selectively merge required standard library components. The desugared AST is then handed to the checker module. The checker performs a multi-pass analysis: a pre-registration pass to identify user-defined types, a registration pass to populate its type registry and resolve imports, and a deep traversal to resolve types, infer generics, enforce concurrency safety, and validate semantics, annotating the AST with `ResolvedType` fields. Once validated, the AST and the type registry are passed to the LIR module's lowerer. The lowerer flattens expressions into temporaries and converts high-level constructs into LIR statements while preserving structured control flow. The resulting `LProgram` undergoes optimization passes to remove redundant operations and destructure multi-returns. For the C target, a monomorphization pass specializes generic types. Finally, the C backend emitter traverses the LIR and generates the final source code string, relying on the runtime module's macros, which is then written to disk.\n\nThe architectural verification workflow operates independently to ensure design fidelity. The process starts when the verifier module is invoked on a `.lyric` specification file. It uses the parser module to generate an AST of the specification. The verifier then inspects the source annotations within the AST to locate the corresponding Go source files. It uses the Go standard library to extract all exported types, interfaces, and function signatures from these Go files, building a comprehensive internal registry that aggregates information across multiple files. The verifier then recursively walks the Lyric AST, translating Lyric type expressions into Go string representations and comparing them against the extracted Go types. It normalizes naming conventions on the fly, mapping snake_case to PascalCase or camelCase, to facilitate this comparison. Finally, it performs a reverse completeness check, ensuring every exported Go symbol is documented in the Lyric specification, and aggregates any mismatches into a detailed report.\n\n## Dependency Overview\n\nThe project exhibits a strict, unidirectional dependency graph that prevents circular imports and enforces a clear separation of concerns. This layering is critical for maintaining the maintainability and testability of the compiler.\n\nAt the base of the hierarchy is the AST module, which acts as a leaf node containing pure data structures. The parser module depends solely on the AST module to construct its output, ensuring that parsing logic is decoupled from semantic analysis. The checker module depends on both the AST module for input and annotation, and the parser module to handle imported files during its semantic analysis phase. To avoid circular dependencies, the AST module stores checker-resolved types as opaque `any` fields.\n\nThe LIR module depends on the AST and checker modules to perform its lowering process, relying on the semantic truth established by the checker and the structural representation provided by the AST. It also implicitly depends on the runtime module as the target contract for its C backend.\n\nThe verifier module depends on the AST and parser modules to read specifications, but operates entirely independently of the compiler's semantic and backend phases. This isolation allows the verifier to act as an objective observer of the codebase. The runtime module is completely independent of all Go code, serving only as a target dependency for the generated C code. This layered architecture ensures that the frontend remains decoupled from backend concerns, and that data structures are defined independently of the logic that manipulates them.\n",
  "file_path": "project-design.md"
}
```

---

### USER (gemini)

**Tool Result:**
```
✅ **File Created Successfully**

**File**: `/Users/bill/projects/lyric/project-design.md`
**Size**: 16446 bytes
**Permissions**: -rw-r--r--
**Lines**: 19
**Duration**: 0.00s
```

---

### ASSISTANT (gemini)

Project documentation complete.

---

