# Lyric Module System — Design

*Updated 2026-06-12 (originally 2026-06-04)*

## Model

- **Package** = directory of `.ly` files. Package name = directory name. `pub` controls exports.
- **Module** = project root with `lyric.mod`. Defines module path. Unit of compilation = one program or one `.so`.
- **Compilation** = whole-program. All packages resolved at compile time, merged into one C output, compiled to one binary.

## Syntax

```lyric
import ast
import parser

let file = parser.parse("hello.ly")
let node = ast.Node { name: "root" }
```

`import <name>` — name is both the identifier for qualified access and the directory name relative to module root. For nested packages: `import v2 from "parser/v2"`.

## Entry Point

```bash
lyric compile .                    # directory → find lyric.mod, find main(), compile
lyric compile myproject/           # same, explicit path
lyric compile main.ly -o program   # single-file, no module needed
lyric compile main.ly ast.ly       # multi-file, no module needed
```

When given a directory, the compiler looks for `lyric.mod` and finds `main()` in the root package. When given a `.ly` file, it checks parent directories for `lyric.mod` — if found, module mode; otherwise, single-file mode.

## Compilation Pipeline

```
1. Find lyric.mod, determine module root
2. Parse root package (all .ly files in module root dir)
3. Scan for import statements, resolve to directories
4. Recursively parse all imported packages (cycle detection)
5. For each package: merge all .ly files in directory → one AST
6. Merge stdlib into each package
7. Desugar all packages
8. Merge all packages into one AST with namespace prefixing
9. Check (three-phase: type names → signatures → bodies)
10. Lower → Optimize → Monomorphize → Emit C
11. gcc/clang → binary
```

## Namespace Prefixing

When merging packages, all declarations get prefixed with the package name in the C output to avoid collisions. The checker resolves qualified names (`ast.Node` → `ast_Node`) transparently.

Example: `ast/ast.ly` defines `pub struct Node` → C gets `ast_Node`.
`parser/parser.ly` defines `func helper()` → C gets `parser_helper`.

Within a package, no prefixing — all declarations are directly visible.

## Key Decisions

1. **`import ast`** — just the name, no alias, no quoted path. Simple as possible.
2. **Directory = package** (like Go). Filename is irrelevant to package identity.
3. **Whole-program compilation**. No separate compilation, no linking. The days of compiling one file at a time are over.
4. **Directory or file as entry point**. `lyric compile .` or `lyric compile main.ly` both work. Directory mode uses `lyric.mod`.
5. **Cycle detection** via tracking packages-in-progress during recursive resolution.
6. **Nested packages** use `import alias from "path"` (extended syntax, only when needed).
7. **Auto-detect module mode**. If a `.ly` file is given, walk up to find `lyric.mod`.

## lyric.mod Format

```
module github.com/user/mycompiler

# future:
# require github.com/other/lib v1.2.3
```

## Implementation Plan

1. Add `lyric.mod` parsing (one `module` line for now)
2. Update parser: `import <ident>` (no `from` required for simple case)
3. Update `cmdCompile` to accept directories, find `lyric.mod`, resolve packages
4. Recursive import resolution with cycle detection (checker has `CheckModuleFile` foundation)
5. Package-level AST merging (reuse `MergeFiles`)
6. Namespace prefixing during cross-package merge
7. Resolve qualified names in lowerer (`ast.Node` → prefixed C name)
8. Update testdata with multi-package test case
9. Update bootstrap to use imports instead of explicit file listing
