# Grok-Verify Design

## Executive Summary

The `grok-verify` utility is a critical component of the Grok-Driven Development (GDD) ecosystem, serving as the automated guardian of architectural integrity. Its primary mission is to eliminate the "documentation rot" that plague traditional software projects by ensuring that high-level architectural declarations—stored in `.grok` files—remain in perfect synchronization with their underlying Go implementations. 

By bridging the gap between abstract design and concrete code, `grok-verify` allows developers to treat their architectural diagrams and specifications as living, verified artifacts rather than passive, potentially misleading documentation. It acts as a structural auditor that detects discrepancies in types, naming conventions, and API surfaces, providing immediate feedback when the implementation drifts from the intended design.

## File Inventory

*   [main.go](main.go): The entry point and orchestration layer for the verification tool. It manages the lifecycle of a verification run, from command-line argument processing to the final reporting of structural drift findings.

## Architecture and Data Flow

The `grok-verify` tool is designed as a lightweight command-line interface that delegates its core analytical logic to the [pkg/verifier](../../pkg/verifier/design.md) module. The data flow through the application is a linear pipeline that transforms a file path into a set of actionable architectural insights.

The process begins with **Input Acquisition**, where the `main` function extracts the target `.grok` file path from the command-line arguments. This path is then passed to the **Verification Engine**, specifically the `verifier.Verify` function. Within this engine, the tool triggers a complex multi-stage process: it parses the Grok file into an AST, discovers associated Go source files via `source:` annotations, extracts type information from those Go files using the standard library's `go/ast` package, and performs a deep, recursive structural comparison.

The engine returns a **Result Object**, which encapsulates a collection of `Finding` structures. The CLI then enters the **Reporting Phase**, where it iterates through these findings, serializing them to standard output. It maintains internal counters for errors and warnings to provide a concise summary at the end of the run. Finally, the tool determines its **Exit Status** based on the severity of the findings; if any errors are detected, it exits with a non-zero status code, ensuring that architectural drift can automatically break a build or block a CI/CD pipeline.

## Interface Implementations

As a standalone command-line application, `grok-verify` does not implement any internal library interfaces. Instead, it acts as a consumer of the [pkg/verifier](../../pkg/verifier/design.md) and [pkg/ast](../../pkg/ast/design.md) packages. It fulfills the role of a "driver" that orchestrates these libraries to provide a user-facing tool.

## Public API

The primary interface for `grok-verify` is its command-line invocation. It follows a minimalist design, expecting exactly one positional argument:

```bash
grok-verify <file.grok>
```

*   **`<file.grok>`**: This required argument specifies the path to the Grok "understanding" file that serves as the architectural source of truth.

The tool's output is designed for both human readability and easy parsing by simple scripts. Each finding is printed on a new line in the following format:
`[SEVERITY] grok_file ↔ go_file: message`

The session concludes with a summary line:
`X errors, Y warnings`

## Implementation Details

The implementation of `main.go` is intentionally lean, focusing on error handling and output formatting. It uses the standard `os.Args` for argument retrieval rather than a complex flag parsing library, reflecting its focused purpose.

The core logic of the CLI resides in its handling of the `verifier.Result`. It performs a single pass over the `Findings` slice, using a `switch` statement to categorize each finding by its `Severity` (Error, Warning, or Info). This categorization is crucial because it directly influences the tool's exit code. The use of the `fmt.Println(f)` call relies on the `String()` method implementation of the `Finding` type (defined in the `verifier` package), ensuring consistent formatting between the library and the CLI.

The tool's error handling is robust; it distinguishes between "verification errors" (structural drift found in the code) and "system errors" (such as a missing file or a syntax error that prevents the verification from running). System errors are reported to `os.Stderr` and result in an immediate exit, while verification errors are collected and reported as part of the final summary.

## Dependencies

*   **[pkg/verifier](../../pkg/verifier/design.md)**: The primary dependency that provides the `Verify` function and the `Result`/`Finding` data structures.
*   **[pkg/ast](../../pkg/ast/design.md)**: Transitive dependency used for representing the Grok AST and severity levels.
*   **Standard Library**: Uses `fmt` for output, `os` for environment interaction and exit codes, and `path/filepath` (transitively) for file resolution.

## Technical Debt and Future Work

While highly effective in its current form, `grok-verify` has several planned enhancements to increase its utility in large-scale environments:

*   **Batch and Recursive Processing**: The current limitation to a single file per invocation can be cumbersome for large projects. Future versions should support directory recursion and multiple file arguments.
*   **Structured Output**: To better integrate with modern DevOps pipelines and IDEs, support for JSON, SARIF, or Checkstyle output formats is a high priority.
*   **Configuration Support**: Introducing a configuration file (e.g., `.grok-verify.yaml`) would allow teams to customize severity levels for specific types of drift or ignore certain files/patterns.
*   **Performance Optimization**: Implementing a caching mechanism for parsed Go ASTs would significantly speed up verification when multiple Grok files reference the same Go source files.
