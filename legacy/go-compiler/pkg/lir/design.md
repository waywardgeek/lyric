# LIR Module Design

## Executive Summary

The `lir` (Low-level Intermediate Representation) module is the architectural bridge between the high-level, semantically rich Abstract Syntax Tree (AST) and the target-specific code generators of the Forge compiler. Its primary mission is to resolve the semantic complexity of the Forge language—such as generics, optional types, tagged unions, and method dispatch—into a simplified, flat representation. This transformation ensures that backends can remain "thin" emitters, focused primarily on syntax mapping rather than complex semantic resolution.

The LIR is built on four foundational design principles. First, it preserves **structured control flow** (if, while, for, switch) instead of decomposing logic into basic blocks and jumps, which is essential for generating idiomatic high-level target code. Second, it enforces **flat expressions**, where every sub-expression is assigned to a unique, named temporary variable, ensuring that backends never encounter nested expression trees. Third, it performs **semantic resolution**, lowering high-level Forge features into explicit LIR constructs. Finally, it is **backend agnostic**, providing a common foundation for multiple targets while offering optional passes like monomorphization for backends that require it.

## File Inventory

*   [lir.go](lir.go): Defines the core data structures for the LIR, including the type system (`LType`), operands (`LValue`), expressions (`LExpr`), and structured statements (`LStmt`).
*   [lower.go](lower.go): Implements the `Lowerer`, the primary engine that transforms a type-checked AST into an LIR program. This file contains the bulk of the semantic lowering logic, including expression flattening and control flow transformation.
*   [monomorphize.go](monomorphize.go): Implements a comprehensive monomorphization pass that specializes generic functions, classes, and structs into concrete versions for backends like C that do not support generics natively.
*   [optimize.go](optimize.go): Performs post-lowering LIR-to-LIR optimizations, such as side-effect temporary elimination, multi-return destructuring, and unused variable removal.
*   [validate.go](validate.go): Provides invariant checkers that verify the integrity of the LIR program after lowering and monomorphization passes.
*   [c_backend.go](c_backend.go): The C code generator, which transforms monomorphized LIR into C code, utilizing a dedicated runtime library and implementing complex features like generator state machines and interface vtables.
*   [lir.forge](lir.forge): A Forge specification file that formally defines the LIR module's structure and interfaces, used by the verifier to ensure implementation integrity.
*   [lower_test.go](lower_test.go): Unit tests for the lowering pass, verifying the transformation of various AST constructs.
*   [lower_extra_test.go](lower_extra_test.go): Additional tests for the lowering pass, covering edge cases and complex scenarios.
*   [c_backend_test.go](c_backend_test.go): Tests for the C code generator, verifying the emission of valid C code.

## Architecture and Data Flow

The `lir` module operates as a sophisticated transformation pipeline that converts high-level architectural intent into executable source code. The flow of data through the module follows a strictly defined sequence of stages.

The pipeline begins with the **Lowering** stage. The `Lowerer` consumes a type-checked `ast.File` from the [pkg/ast](../ast/design.md) module and a `checker.Registry` from the [pkg/checker](../checker/design.md) module. It performs a multi-pass walk of the AST, first registering all types and then flattening every nested expression into a sequence of single-operation assignments to named temporaries. During this process, high-level constructs are desugared into their LIR equivalents; for instance, a `match` statement on an enum is lowered into a `switch` on an integer variant tag, with explicit field extractions for any bound variables.

Once the initial LIR is generated, it enters the **Optimization** stage. The `Optimize` function runs several passes over the LIR to clean up the output of the lowerer. These passes are essential because the lowerer often produces redundant temporaries or patterns that are difficult for backends to emit efficiently. Key optimizations include fusing void-context calls into bare statements (side-effect elimination), destructuring multi-return values from single temporaries into separate variables, and inserting necessary type coercions.

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
*   `EmitC(prog *LProgram) string`: This function takes a monomorphized LIR program and generates a complete C source code string, relying on the `forge_runtime.h` contract.

## Implementation Details

### LIR Data Structures

The LIR is defined by four primary categories of data structures:

*   **LType**: Represents the Forge type system in a form suitable for code generation. It includes primitive types (integers, floats, bool, string), unit/void, error, and complex types like `LTyStruct`, `LTyClassHandle` (heap-allocated), `LTyTaggedUnion` (enums), `LTyUnion` (ad-hoc unions), `LTySlice`, `LTyMap`, `LTyChannel`, `LTyGenerator`, `LTyOptional`, and `LTyErrorResult`. It also preserves `LTyTypeVar` for generic support.
*   **LValue**: The operands of the LIR. These are flat and include named variables (`LValVar`), SSA-like temporaries (`LValTemp`), global variables (`LValGlobal`), and various literals (int, uint, float, string, bool, null).
*   **LExpr**: Right-hand side expressions that are always bound to a temporary via `LTempDef`. They reference only `LValue` operands, never nested expressions. Kinds include binary/unary operations, casts, field access, calls (regular, method, builtin), and construction (struct, class, slice, map, channel).
*   **LStmt**: Structured statements that form the program's control flow. Kinds include variable management (`LStmtTempDef`, `LStmtVarDecl`, `LStmtAssign`), field/index mutation (`LStmtStructSet`, `LStmtClassSet`, `LStmtIndexSet`), and structured control flow (`LStmtIf`, `LStmtWhile`, `LStmtFor`, `LStmtSwitch`, `LStmtTypeSwitch`, `LStmtBlock`, `LStmtReturn`, `LStmtBreak`, `LStmtContinue`). It also includes concurrency primitives (`LStmtSpawn`, `LStmtLock`, `LStmtSend`, `LStmtSelect`, `LStmtYield`).

### The Lowering Pass

The `Lowerer` is the most complex component of the module, responsible for the heavy lifting of semantic resolution. It maintains a stateful mapping of AST nodes to LIR constructs and tracks visibility, imports, and type information. A central feature of the lowerer is **expression flattening**. Every sub-expression in the AST is assigned to a unique temporary variable (`LValTemp`). This ensures that all `LExpr` instances in the LIR reference only simple operands, never nested expressions. For example, `a + b * c` is lowered into `%0 = b * c` followed by `%1 = a + %0`.

Control flow lowering in Forge LIR is **structured**. Instead of decomposing logic into basic blocks and jumps, the LIR uses high-level statements like `LStmtIf` and `LStmtWhile`. The `LStmtWhile` is particularly noteworthy; it contains a `CondBlock`—a list of statements executed before each iteration to compute the loop condition. This elegantly handles complex conditions that might involve side effects or multiple steps, ensuring they are re-evaluated correctly on every iteration without duplicating code.

The `Lowerer` also handles the desugaring of Forge's high-level types:
*   **Enums**: These are lowered to `LTyTaggedUnion`. Match statements are converted to switches on the variant tag, with the lowerer generating the necessary logic to extract variant fields into local bindings.
*   **Ad-hoc Unions**: These are lowered to `LTyUnion`. Type switches are used to match against the underlying types at runtime.
*   **Classes**: These are lowered to `LTyClassHandle`, representing a pointer to a heap-allocated struct. Field access is converted to explicit `LExprClassGet` and `LStmtClassSet` operations.
*   **Generics**: The lowerer preserves type parameters as `LTyTypeVar`.
*   **Impl Blocks**: The lowerer generates forwarding wrapper methods for `impl` blocks. This enables classes to satisfy interfaces even when method names or signatures do not match exactly, by creating a bridge between the interface's expected signature and the class's actual implementation.
*   **Try Operator**: The `?` operator is lowered into a sequence of operations: extracting the error from a result, checking if it is non-null, and performing an early return if an error is present, followed by extracting the success value.

### The Optimization Pass

The optimizer runs after lowering to refine the LIR. One of its most critical tasks is **side-effect temporary elimination**. The lowerer often assigns the result of every call to a temporary, even if the result is immediately discarded (e.g., in a void-context call). The optimizer identifies these patterns and fuses them into `LStmtSideEffect` statements, which backends can emit as bare expressions.

Another vital optimization is **multi-return destructuring**. Forge allows functions to return multiple values, such as `(T, error)`. The lowerer initially represents this as a single temporary of type `LTyErrorResult`. The optimizer identifies patterns where this temporary is immediately destructured into separate variables and replaces them with a single `LStmtMultiAssign`. This simplifies target code generation for multi-return values.

Additional passes include **destructuring extract pairs** for the try operator, **fixing null/zero values** for backend compatibility, **coercing for-range index types**, and **wrapping optional returns**.

### Monomorphization

For backends like C that lack native generic support, monomorphization is mandatory. The pass operates in six distinct phases:

1.  **Indexing**: It indexes all generic declarations (functions, structs, classes) in the program.
2.  **Collection**: It performs a deep walk of all function bodies to collect every unique instantiation of generic functions, classes, and structs based on the concrete type arguments used at call sites.
3.  **Specialization**: It generates specialized copies of these declarations, mangling their names (e.g., `Stack_i32`) to ensure uniqueness. This involves deep cloning the function bodies and substituting type variables with concrete types.
4.  **Rewriting**: It rewrites all call sites throughout the program to use these specialized, non-generic versions.
5.  **Signature/Field Rewrite**: It rewrites function signatures and field types to use the mangled names of specialized types.
6.  **Bare Name Resolution**: It resolves any remaining bare generic class names in specialized function bodies using a per-function rename map.

A critical aspect of monomorphization in Forge is handling **Impl Method Renames**. When a class implements an interface through an `impl` block, the lowerer may generate forwarding wrappers or rename methods to avoid collisions (especially in C). The monomorphizer tracks these renames in `prog.ImplMethodRenames` and ensures that specialized versions of default interface methods call the correct concrete method names on specialized classes.

### The C Backend

The **C Backend** is significantly more involved due to C's lack of high-level features. It relies on a small runtime header (`forge_runtime.h`) that defines macros and structures for slices, optional types, and error results. It hoists lambdas to top-level static functions and uses `malloc` for class allocations.

One of the most complex transformations in the C backend is the **Generator State Machine**. Forge generators are lowered into a stateful struct and two functions: an `_init` function that allocates the state and copies parameters, and a `_next` function that executes the body. The `_next` function uses a `switch` statement with `goto` labels to resume execution from the last `yield` point. Local variables that must persist across yields are hoisted into the generator's state struct.

The C backend also implements **Interface VTable Dispatch**. For each interface, it generates a vtable struct containing function pointers for each method. Boxed interface values are represented as a struct containing a data pointer and a vtable pointer. Static vtable instances are generated for each (class, interface) pair, and method calls on interface types are dispatched through these vtables.

Finally, the C backend performs **struct topo-sorting** to ensure that structs are defined before they are used as by-value fields in other structs, satisfying C's requirement for complete type definitions.

## Dependencies

*   [pkg/ast](../ast/design.md): Provides the source AST and node definitions.
*   [pkg/checker](../checker/design.md): Provides the type registry and resolved type information.
*   [runtime](../../runtime/design.md): Provides the C runtime contract for the C backend.

## Technical Debt and Future Work

*   **Map Support in C**: The C backend currently lacks implementation for map types.
*   **Concurrency in C**: While basic spawn support exists, more advanced concurrency features like select are not yet fully supported in the C backend.
*   **Advanced Optimizations**: Future improvements could include constant folding, dead code elimination beyond unused temporaries, and function inlining.
*   **Relational Constraints**: While basic support exists, more complex scenarios involving multi-class interfaces may require further refinement.
