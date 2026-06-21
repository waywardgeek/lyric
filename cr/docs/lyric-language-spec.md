# Lyric — A Typed Language for Design and Implementation

*Bill Cox & CodeRhapsody — Updated 2026-06-20*

**Source code & tools:** [github.com/waywardgeek/lyric](https://github.com/waywardgeek/lyric)

---

## How to Read This Document

This spec describes Lyric's **design** — the language as we intend it, including
features still on the roadmap. It is meant to be **accurate and complete**, not
short. Every claim is anchored to either a use site in the bootstrap compiler /
stdlib / testdata, or to an explicit roadmap entry.

Throughout, features are marked:

- *(no mark)* — Implemented and exercised in the bootstrap compiler.
  The companion **reference** (`lyric-language-reference.md`) covers the same
  material with daily-driver brevity.
- 🚧 **Roadmap** — Designed but not yet implemented. Tracked in
  `cr/docs/bootstrap-roadmap.md` and `TODO.md`.
- ❌ **Not in scope** — Considered and rejected, with rationale.

The Lyric compiler self-hosts at a fixed point as of June 2026
(~33,500 lines of Lyric → ~114,770 lines of C). Every implemented feature lives
because it earned its place in the bootstrap source.

**CDD layer moved out.** The Context-Driven Development annotations
(`why:`, `doc`, `invariant:`, `verified_at:`, `source:`, `fake:`), the
three-zone `.lyric` file model, and the `lyric verify` / `update` / `gen`
commands — formerly described in this spec — have moved to a separate tool,
**`lyre`**. This spec covers only the Lyric language and compiler. See the
appendix [Recently Removed](#recently-removed) for the full list.

---

## Purpose

Lyric is a typed language with two modes of use:

**`.lyric` files — understandings.** Declaration-only Lyric: types, signatures,
interfaces, ownership relations, constants. No function bodies. The design
artifact for Context-Driven Development. CDD annotations (`why:`, `doc`,
`invariant:`, `source:`, `fake:`) live in the `lyre` layer on top of Lyric
syntax — not in the core Lyric grammar. Every `.lyric` file is valid Lyric.

**`.ly` files — code.** Full Lyric with function bodies, executable control flow,
and real semantics. Compilable and runnable. An optional capability — CDD does
not require production code to be written in Lyric. The compiler exists to prove
the language design is sound: if the notation is precise enough to verify
against real implementations, then function bodies are all that's missing to
make it a real language.

Both modes are designed to be:

- **Read primarily by AI** — dense with meaning, minimal ceremony, no noise
- **Written primarily by AI, reviewed by humans** — the AI writes after
  implementation; the human reviews for accuracy
- **Type-checked** — `.ly` files compile and run; `.lyric` files are
  structurally checked by `lyre` against implementations
- **Language-agnostic in intent** — `.lyric` files describe design regardless
  of implementation language; `.ly` files compile to C

---

## Design Philosophy

**Permissive by default.** `.lyric` files accept design patterns common across
languages, imposing only *sound* constraints — constraints that improve design
quality regardless of target language. The key example: typed function
signatures. Every language has types (even dynamically typed ones); requiring
them catches design errors the way TypeScript improves on JavaScript.

**No language-specific restrictions.** If Python allows circular imports, Lyric
does not forbid them. If Rust requires explicit lifetimes, Lyric does not
require them. The `.lyric` file describes the design, not the target
language's rules.

**Sound constraints Lyric does impose:**

- Every parameter and return value must have a type (may be a type variable)
- `self` declares method receivers (mutation is implicit — no `mut self`
  distinction)
- `mut` parameters pass structs by mutable reference (required on both decl
  and call site)
- Type variables must be explicitly declared inside `<>` — never inferred
  from naming convention

**Constraints Lyric deliberately does NOT impose:**

- Import ordering or circularity restrictions
- Memory management model knobs visible to the programmer (ownership is
  declared via `relation`; the compiler does the rest)
- Error handling strategy (`(T, error)` is idiomatic but not the only option)
- Naming conventions are recommended but not enforced (see [Naming Conventions](#naming-conventions))

---

## Design Lineage

Lyric inherits its core from **Rune**, Bill Cox's systems language. The
inheritance is selective.

**Kept from Rune:**

- The numeric type tower (`u8`–`u64` implemented; `u128`/`u256` and the
  corresponding signed/float widths 🚧)
- `string`, `bool`
- `T?` for optional values
- `T | U` for typed unions
- `[T]` for sequences, `Dict<K,V>` for associative containers (stdlib, not
  built-in)
- `enum`, `struct`, `class`
- `relation` for ownership/reference structure, with `owns`/`refs`, labels,
  and hints

**Dropped from Rune:**

- **Secret propagation in the type system** — research goal, not design tool
- **Optional types everywhere** — Rune allowed omitting types to feel like
  Python; Lyric requires every parameter and return value to be typed
- **Memory safety mechanisms visible to the programmer** — ownership
  enforcement is the compiler's job; `.lyric` files express intent via
  `relation`
- **Python-style goals** — Lyric appeals to the AI reading it, not Python
  programmers

**Added in Lyric:**

- **`interface` as a first-class declaration** — multi-class structural
  contracts with type parameters, method binding, field injection, and
  default implementations
- **Type variables and `where` clauses** — generics with named capability
  constraints, including relational where-clauses
  (`where DoublyLinked<P, C>`)
- **`error` as a built-in interface** — uniform error handling
- **Relations with code generation** — `ArrayList`, `DoublyLinked`,
  `HashedList` hints trigger field injection, impl binding, and destructor
  generation
- **Impl blocks** — wire interface methods to concrete class methods, bind
  fields, or provide inline implementations

---

## Modules and Packages

### Package = Directory

A **package** is a directory of `.ly` files. All `.ly` files in a directory
belong to the same package. The package name is the **directory name**.

```
mycompiler/
  lyric.mod                # module root marker
  main.ly                  # package "mycompiler" (entry point)
  ast/
    ast.ly                 # package "ast"
    expr.ly                # package "ast" — same directory, same package
  parser/
    parser.ly              # package "parser"
    expr_parser.ly         # package "parser"
  checker/
    checker.ly             # package "checker"
```

Within a package, all declarations across all `.ly` files are visible to each
other — declaration order and file order don't matter. The compiler merges all
files in a package into one unit before processing.

**The `lyric` block wrapper is optional.** When present, the block name
provides a logical grouping but does **not** override the package name. The
package name always comes from the directory. When absent, bare top-level
declarations belong to the directory's package, in an implicit block whose
name is derived from the filename.

A single `.ly` file may contain **multiple `lyric` blocks**; they are appended
in order.

### Module = Project

A **module** is a project rooted at a `lyric.mod` file. The module defines
the directory tree from which the compiler resolves `import` statements.

A module is the **unit of compilation**. Lyric compiles an entire module at
once — all packages are resolved at compile time, merged with namespace
prefixing, and emitted as a single C output compiled to one binary. There is
no separate compilation of individual files or packages.

🚧 **`lyric.mod` content** — today the compiler only checks for the file's
**existence** as a marker; its contents are not parsed. The intent is for
`lyric.mod` to declare the module's import-path prefix and external
dependencies, e.g.:

```
# lyric.mod
module github.com/user/mycompiler
```

### Imports

Three import forms are accepted by the parser:

```lyric
import ast                    // by-name: resolves to directory "ast/"
import v2 from "parser/v2"    // with alias: resolves to "parser/v2/", access as v2.X
import "experimental/utils"   // bare path — 🚧 see caveats below
```

**Access is always qualified:**

```lyric
import ast

let n = ast.Node { name: "x" }     // struct construction
let kind = ast.ExprKind.Ident      // enum variant
let result = ast.parse(src)        // function call
```

Internally, the import is implemented by prefixing every top-level
declaration in the imported package with `alias_` (e.g., `ast.Node` becomes
`ast_Node`). Type-level qualified access (`param: ast.Node` in a signature)
is resolved by the checker; expression-level qualified access is rewritten
during import resolution.

**Current behavior (honest):**

- ✅ `import name` and `import alias from "path"` work today.
- 🚧 **Bare `import "path"`** parses, but crashes in the resolver because it
  has no alias. Use `import alias from "path"` instead.
- 🚧 **Recursive imports are not resolved.** Only the root file's `import`
  statements are processed; an imported package's own imports are ignored.
- 🚧 **No `pub` filtering.** Every declaration in an imported package is
  visible after prefixing — non-`pub` is not enforced today.
- 🚧 **No cycle detection.** Moot today because of the single-level rule, but
  required before recursive imports land.
- 🚧 **`main()` discovery.** The driver compiles whatever files you point it
  at; `lyric.mod` does not yet trigger automatic discovery of `main()` in the
  root package.

The roadmap target is: recursive resolution, `pub`-filtered visibility,
cycle detection, and `lyric.mod`-driven entry-point discovery.

### Standard Library

The stdlib (`stdlib/std.ly`, `stdlib/string.ly`) is auto-imported into all
packages. Its declarations are available **unqualified** — no `import`
statement required.

The mechanism: `merge_stdlib` walks user code, finds referenced types and
functions, and transitively pulls in matching stdlib declarations. Special
cases handle `Dict` literals (always pull in `Dict`) and primitive extension
methods (`i32.get_hash`, etc.). The pass runs to fixed point.

### Compilation Model

```bash
lyric compile .                           # compile module in current directory
lyric compile ~/projects/mycompiler/      # compile module at path
lyric compile main.ly -o myprogram        # single-file, no module needed
lyric compile main.ly ast.ly              # multi-file, no module needed
lyric test test_lexer.ly lexer.ly ast.ly  # test specific files
```

When given a directory, the compiler looks for `lyric.mod`. If found, it
collects all top-level `.ly` files in that directory and proceeds in module
mode (with the import limitations noted above). When given a `.ly` file
directly, it compiles that file plus the auto-imported stdlib.

The full pipeline:

1. Parse all `.ly` files
2. Merge files in each package
3. Resolve imports (single-level, see above)
4. Merge stdlib into block 0
5. Desugar (six passes — see [Compilation](#compilation))
6. Check (four phases)
7. Lower to LIR
8. Optimize (LIR → LIR)
9. Monomorphize (LIR → LIR)
10. Emit C
11. Compile C with `gcc -std=gnu11`

---

## Primitive Types

```
// Unsigned integers
u8   u16   u32   u64                // implemented
🚧 u128   u256                       // roadmap

// Signed integers
i8   i16   i32   i64                // implemented
🚧 i128   i256                       // roadmap

// Floating point
f32   f64                           // implemented
🚧 f128                              // roadmap

// Other primitives
string    // bytes today; UTF-8 layer 🚧 (see below)
bool      // true | false
unit      // void/no-value type (function with no return value)
error     // built-in interface; see Interfaces (not a primitive type — but
          // the AST/checker treat the name as built-in for lookup purposes)

// Platform-width integers (interop only)
int   uint    // NOT part of the Lyric numeric tower
```

**Currently registered by the checker:** `bool`, `string`, `i8`, `i16`,
`i32`, `i64`, `int`, `u8`, `u16`, `u32`, `u64`, `uint`, `f32`, `f64`, `any`,
`error`. The larger widths (`i128/i256/u128/u256/f128`) are reserved
identifiers in the spec but not yet registered — using them today is a
checker error.

**Default integer literal type:** `i32`. Cast with `x as u64`.

**Character literals:** `'A'` → `u8` constant (value 65). Internally, a char
literal is syntactic sugar for an integer literal tagged with type `u8`.
Supported escapes: `\n`, `\r`, `\t`, `\\`, `\'`, `\"`, `\0`, `\x##`
(hex byte). 🚧 `\u{...}` Unicode escapes.

**Strings are byte slices today.** `string` is represented as `[u8]`
internally. `s[i]` returns `u8` (a byte, not a code point). `len(s)` is the
byte length. All built-in string operations work on bytes. 🚧 The roadmap
adds a UTF-8 layer on top: `\u{NNNN}` escapes, code-point iteration,
`char_at` returning a code point (`i32`/`rune`), Unicode-aware case
operations, normalization. The type name `string` stays.

`null` is the null literal for optional types. `let x = null` without a type
annotation is a checker error; use `let x: T? = null`.

Today, the checker accepts `null` as assignable to **any** type. 🚧 Tighten
to `T?` / class / interface / `error` only.

`nil` is **not** accepted. (The internal `KNil` token name is a historical
artifact — it's emitted only for the literal `null`.)

`error` is a built-in **interface**, not a primitive — see
[Interfaces](#interfaces-and-multi-class-contracts).

`byte`/`rune` appear in the AST's `is_primitive_type` list but are not
registered by the checker. They are not part of Lyric — treat them as
internal noise that will be cleaned up.

---

## Composite Types

```
T?            // optional: T or null
T | U         // union: T or U (exhaustively typed)
[T]           // slice of T (fat pointer: data + len + cap)
(T, U)        // anonymous tuple (positional)
(T, U, V)     // triple, etc.
channel<T>    // CSP channel (created via make_channel<T>())
fn(T, U) -> V // function type
gen T         // generator return type
lock          // mutex type
unit          // void
```

**Strings as byte slices:** `string` is `[u8]` internally (see Primitive
Types). Concatenation uses `+` (`"hello" + " world"`). Slice concatenation
also uses `+` (`[1,2] + [3,4]` → `[1,2,3,4]`). In-place slice extension:
`xs.extend(ys)`.

**Maps:** The canonical map is `Dict<K,V>` from stdlib (with `K: Hashable`).
A legacy built-in `map[K]V` type **parses** (type syntax and literal
`map[K]V { "k": v }`) and **type-checks**, but its C backend emits
`void* /* map */` and `make_map` returns `NULL`. It is non-functional at
runtime. 🚧 Either implement it or remove it. **Use `Dict<K,V>`.**

**Tuples:** Positional. Field access uses underscore-prefixed indices
(`._0`, `._1`):

```lyric
let t = (42, "hello")
println(t._0)    // 42
println(t._1)    // "hello"

let (a, b) = make_pair()   // destructuring also works
```

The AST allows an optional `name` field on `TupleField`, but the parser does
not surface named-tuple syntax today. Treat tuples as positional.

**Function type syntax:** Three forms parse:

- `fn(T, U) -> V` — canonical
- `func(T, U) -> V` — also accepted (note: `func` is a keyword too; the
  parser handles both)
- `T -> U` — single-argument shorthand (no parens around the single param
  type)

`fn` is preferred for type positions to avoid confusion with function
declarations.

**Type aliases.** A `type` declaration introduces a named alias for an
arbitrary type expression. Aliases are transparent: the checker resolves
them to their RHS at use sites.

```lyric
type StringList = [string]
type Json = string | i32 | f64 | bool | null
type Callback = fn(i32) -> error?
pub type NodeId = u32
```

Type aliases may appear at the top level of a `lyric` block (with optional
`pub`). They do not introduce a new nominal type — `StringList` and
`[string]` are interchangeable.

**Slice indexing and slicing.** `xs[i]` is an indexed element access.
Slice expressions support four endpoint forms:

```lyric
xs[lo:hi]    // explicit range
xs[lo:]      // lo to end
xs[:hi]      // start to hi
xs[:]        // full copy of the slice descriptor (shares backing array)
```

All four also apply to `string` (which is `[u8]`): `s[1:4]`, `s[:3]`,
`s[lo:]`, `s[:]`.

**Special types in type position:**

- `channel<T>` — channel of `T`. (`channel` is a contextual identifier in
  type position, not a hard keyword.)
- `gen T` — generator returning `T`. (`gen` is contextual.)
- `lock` — mutex type. (`lock` is a contextual keyword in type position.)
- `unit` — the void type, used as a return type. (`unit` is contextual.)
- `map[K]V` — legacy map type (see above).

---

## Generics and Type Variables

**The rule:** Every function parameter and return value must declare a type.
A type may be a *type variable* — a placeholder resolved at the call site.

### Type Variables

Type variables are declared explicitly using angle brackets `<>`. They are
**never inferred** from naming convention — this prevents typos from silently
becoming type variables:

```lyric
func identity<T>(x: T) -> T
func first<T>(xs: [T]) -> T?
func transform<T, U>(xs: [T], f: fn(T) -> U) -> [U]
func zip<T, U>(xs: [T], ys: [U]) -> [(T, U)]
```

### Call-Site Type Arguments

Generic functions support both explicit type arguments and type inference:

```lyric
// Explicit type arguments
let x = identity<i32>(42)

// Inferred type arguments (compiler resolves from argument types)
let x = identity(42)              // infers T = i32
```

**Inference algorithm:** Walks parameter types and argument types in
parallel, binding type variables to concrete types on first match. Recurses
through composite types (Sequence, Optional, Tuple, Func).

### Nested Generic Syntax: `>>` Splitting

Nested generics like `Dict<Dict<V>>` produce a `>>` token which lexes as
shift-right. The parser splits it into two `>` tokens via a `pushBack` field
(single-token lookahead). Both `tryParseTypeArgs` and `parseBaseType` handle
this — the same approach as Java and Rust.

### Constraints

Constraints restrict what types a variable can stand for. A constraint names
a *capability* — what a type *is* — not a list of individual operations.

**Inline constraints:**

```lyric
func clamp<T: Comparable>(value: T, lo: T, hi: T) -> T
```

**`where` clause — type-class form:**

```lyric
func sum<T>(xs: [T]) -> T where T: Numeric
```

**`where` clause — relational form:**

```lyric
func count<P, C>(p: P) -> i32 where DoublyLinked<P, C>
```

The relational form names a multi-class interface (with all of its type
parameters bound to the function's type variables). The checker binds all
methods from that interface onto the participating type variables, enabling
calls like `p.dll_remove()` inside the body.

**Built-in constraints:**

| Constraint | Satisfied by | Notes |
|---|---|---|
| `Comparable` | numeric, string, bool | ordering |
| `Equatable` | numeric, string, bool | `==` |
| `Hashable` | `Sym`, numeric, bool (NOT `string` — use `sym()`) | `get_hash() -> u64` |

🚧 `Hashable` currently declares only `get_hash`; an `equals` method is
on the roadmap and required for collisions to be resolved correctly. Today,
`Sym.equals` exists as a standalone function (pointer equality on the
interned `Sym` class).

**User-defined constraints:** Any interface can be used as a constraint.
The checker validates via structural subtyping.

### Why Named Capabilities, Not Operation Lists

The Rust approach enumerates every operation needed
(`Copy + PartialOrd + Mul<Output=T> + Sub<Output=T> + One`). Lyric names
the capability: `T: Integer`. One constraint names what the type must be.
This is closer to mathematical statements ("for all integers T...") and
trivial to read aloud.

---

## Visibility

Default **private** (visible only within the declaring package). Use `pub`
to export across packages:

```lyric
pub func add(x: i32, y: i32) -> i32    // exported
func helper(x: i32) -> i32              // package-private

pub struct Point { x: f64, y: f64 }     // exported
pub class Counter { ... }               // exported
pub enum Color { Red Green Blue }       // exported
```

Fields use `pub` prefix: `pub name: string`.

**Modifier order at declarations:** `[pub] [permanent] [trusted] [final]`
(in that order, all optional). `permanent` applies to classes; `trusted` and
`final` apply to functions.

🚧 **`pub` is not enforced across imports today** — every declaration in an
imported package is visible (see [Imports](#imports)). The roadmap target
is true `pub` filtering.

---

## Naming Conventions

**Lyric's compiler is case-agnostic.** Identifier case carries no semantic
meaning — `Foo` and `foo` are distinct identifiers in the trivial lexer
sense (they're different strings), but no parser, checker, or codegen rule
branches on whether an identifier starts with uppercase or lowercase. There
is no PascalCase-means-exported rule (we have `pub` for that), no Go-style
first-letter visibility, no Rust-style case-distinguished patterns. 🚧 *(One
parser disambiguation hack in `expr_parser.ly` currently keys off ASCII
A–Z to detect variant-pattern let-else; that's a bug on the cleanup slate,
not language design — see `TODO.md`.)*

The conventions below are **convention only.** The compiler enforces none
of them. They exist because consistency makes code readable across the
ecosystem; deviate when there's a good local reason.

| Kind | Convention | Example |
|---|---|---|
| Classes, structs, enums, interfaces | PascalCase | `Counter`, `Point`, `Color`, `Graph` |
| Enum variants | PascalCase | `Red`, `Circle`, `OPlus` |
| Type variables | Short PascalCase | `T`, `U`, `P`, `C`, `Iface` |
| Functions and methods | snake_case | `array_append`, `set_count`, `get_hash` |
| Fields | snake_case | `roster_children`, `team_index`, `is_empty` |
| Local variables and parameters | snake_case | `let total_count = 0` |
| Module-level constants | UPPER_SNAKE | `let PREC_NONE: i32 = 0` |
| Packages / module names | snake_case | `ast`, `parser`, `expr_parser` |
| Test functions | `test_` prefix + snake_case | `test_lexer_basic` |

**Rationale.** Types and constructors name *things* — they read better in
PascalCase, where a capital letter signals "look up a definition." Functions
and methods name *actions* or *properties* — snake_case reads as imperative
speech with natural word separation. UPPER_SNAKE for module-level constants
distinguishes "compile-time fixed value" from "ordinary variable" at the
call site, without requiring a type-annotation check. The `test_` prefix on
test functions is mandatory (the test runner uses it for discovery), so the
rest is just style.

**Field-literal construction must match the declared name exactly.**
`Point { x: 1.0, y: 2.0 }` works because the field is `x`. `Point { X: 1.0 }`
is a checker error: the field is `x`, not `X`. The checker performs no
case-insensitive matching, no PascalCase ↔ snake_case translation, and no
fuzzy resolution. The same rule applies to method calls, function calls,
type names, and every other identifier reference.

---

## Functions

### Declarations

All parameters must be named and typed. Return type follows `->`. If a
function returns nothing, the return type is omitted (implicitly `unit`):

```lyric
func add(a: i32, b: i32) -> i32 {
    return a + b
}

pub func public_fn() { ... }

// Generic
func identity<T>(x: T) -> T { return x }
```

`func` is the keyword for both declarations and method definitions. `fn` is
preferred for type syntax only (`fn(i32) -> bool`).

### External Methods

Methods can be defined outside a class using `func T.method(self)` syntax.
This enables multi-class interface patterns where methods span multiple
types:

```lyric
func T.method(self) -> i32 { ... }
func T.mutate(self, x: i32) { ... }
```

The receiver type is a single bare name — no `func [T].method` or
`func T<U>.method` syntax (methods attach to bare type names). The checker
defines `self` in scope when type-checking the body.

### Lambdas

Two syntactic forms:

```lyric
// Paren-style (Kotlin/Swift-like)
let double = (x: i32) -> i32 { return x * 2 }

// Pipe-style (Rust-like)
let triple = |x: i32| -> i32 { x * 3 }

// Higher-order usage
let result = apply(7, |x: i32| -> i32 { x + 3 })
let doubled = transform(nums, |x: i32| -> string { f"n={x}" })
```

Lambda parameters must have explicit types. Lambdas capture variables from
their enclosing scope. In the C backend, captured variables are passed via
auto-generated context structs with capture-by-reference via pointer
redirection.

**Lambdas cannot be generic.** If you need a generic function, declare it
at top level.

### Mutable Parameters (`mut`)

Structs are value types — passing them to a function copies them. Use `mut`
on **both** the parameter declaration and the call site to pass by mutable
reference:

```lyric
struct Point { x: i32, y: i32 }

func translate(mut p: Point, dx: i32, dy: i32) {
    p.x = p.x + dx   // modifies caller's copy
    p.y = p.y + dy
}

let mut pt = Point { x: 10, y: 20 }
translate(mut pt, 5, 3)
assert_eq(pt.x, 15, "mutation visible to caller")
```

Slice elements can also be passed as `mut`, enabling in-place mutation
without extracting the element into a temporary:

```lyric
func double_x(mut p: Point) { p.x = p.x * 2 }

let mut points = [Point { x: 1, y: 2 }, Point { x: 3, y: 4 }]
double_x(mut points[0])   // mutates the element in-place
assert_eq(points[0].x, 2)
```

**Rules:**

- `mut` required on **both** parameter declaration and call site (Swift's
  `inout` pattern — prevents accidental mutation).
- Variables and slice element accesses (`slice[i]`) can be passed as `mut`.
- For classes (already heap-allocated), `mut` is a semantic no-op.
- **C backend:** `mut` params become `T*`; call sites emit `&x` or
  `&slice.data[i]`; field access uses `->` or `(*p).x`.

`mut self` is **accepted** by the parser as a redundant decoration —
mutation through `self` is always allowed. Prefer plain `self`.

### Function Annotations (.lyric files) 🚧

The intended annotation table for `.lyric` design files:

| Annotation | Meaning |
|---|---|
| `concurrent: true\|false` | Whether goroutine/thread-safe |
| `requires_lock(name)` | Caller must hold the named lock |
| `excludes_lock(name)` | Caller must NOT hold (function acquires it) |
| `raises: E1, E2` | Named error conditions |
| `requires: expr` | Precondition |
| `ensures: expr` | Postcondition |
| `spawns:` | Creates a new goroutine/concurrent context |
| `pure:` | No side effects |

🚧 **None of these annotations parse today.** The parser jumps from
`where`-clauses straight to the function body. Currently, the **only**
annotation in the grammar is `guarded_by(lock_name)` on fields (see
[Concurrency](#concurrency)).

The annotation table is the design target; it lives in the spec so that
`.lyric` authors and consumers can agree on the semantics they'll eventually
get. Today, treat the entire table as future work.

---

## Operator Precedence

The expression parser uses twelve precedence levels (looser → tighter):

| Level | Operators | Notes |
|---:|---|---|
| 1  | `\|\|` | logical OR |
| 2  | `&&` | logical AND |
| 3  | `\|` | **bitwise OR** 🚧 (precedence change planned — see below) |
| 4  | `^` | bitwise XOR |
| 5  | `&` | bitwise AND |
| 6  | `==` `!=` | equality |
| 7  | `<` `>` `<=` `>=` | ordering |
| 8  | `<<` `>>` | shifts |
| 9  | `+` `-` | additive |
| 10 | `*` `/` `%` | multiplicative |
| 11 | `-` `!` (unary) | unary minus, logical NOT |
| 12 | `.` `()` `[]` `!` `?` `is` `as` (postfix) | field/call/index/unwrap/try/variant-check/cast |

🚧 **Bitwise operator precedence is wrong today** (it copies C's classic
bug). `a & 1 == 0` parses as `a & (1 == 0)`. Bill's directive: bitwise
operators are arithmetic on integers, not boolean-tier, and their precedence
should be **promoted above** all non-integer operators (above `==`/`!=`,
`<`/`<=`/`>`/`>=`, `&&`, `||`). After the fix, `a & 1 == 0` will parse as
`(a & 1) == 0`. See [Roadmap](#roadmap).

**Unary operators currently parsed:** `-` (negate), `!` (logical NOT).
🚧 `~` (bitwise NOT) is on the roadmap; there is no `~` token in the lexer
today.

**Compound assignments currently parsed:** `+=` `-=` `*=` `/=`.
🚧 `%=` `&=` `|=` `^=` `<<=` `>>=` are on the roadmap.

**Postfix `is` and `as`** — see [Variant Check](#variant-check-is) and
[Type Casts](#type-casts).

---

## Structs (Value Types)

Pure data — named tuples with named fields. Passed by value. No methods, no
behavior, no identity:

```lyric
struct Point {
    x: f64
    y: f64
}

struct Range<T> {
    lo: T
    hi: T
}
```

- Construction: `Point { x: 1.0, y: 2.0 }`
- Positional construction inside expressions: `Point { 1.0, 2.0 }` (inside
  parens, brackets, or arg lists where `{` is unambiguous)
- Fields can have defaults: `width: i32 = 800`
- Cannot be relation targets (no identity, no heap allocation)
- Empty structs: `struct Empty {}` — the C backend inserts a placeholder
  `int _empty;` field; you don't see it.

---

## Enums (Sum Types)

Variants may carry positional payloads:

```lyric
// Simple enumeration
enum Color { Red Green Blue }

// Variants with associated data
enum Shape {
    Circle(radius: f64)
    Rect(w: f64, h: f64)
    Point
}
```

### Pattern Matching

```lyric
match shape {
    Circle(r) => { println(f"radius: {r}") }
    Rect(w, h) => { println(f"{w}x{h}") }
    Point => { println("point") }
}
```

**Multi-pattern arms:** Multiple patterns per arm separated by `|`:

```lyric
match kind {
    OPlus | OMinus => { PREC_ADDITIVE }
    OStar | OSlash => { PREC_MULT }
    _ => { PREC_NONE }
}
```

**Guards:** Each arm may have an `if`-guard:

```lyric
match n {
    x if x < 0 => { "negative" }
    0 => { "zero" }
    _ => { "positive" }
}
```

**Match as expression:**

```lyric
let prec = match kind {
    OPlus => { 9 }
    _ => { 0 }
}
```

🚧 **Match-as-expression branch unification is not enforced.** The checker
returns the type of the first arm's body and does not verify that other arms
agree. Spec intent: all arms must produce the same type.

**Qualified-variant patterns:** When two enums share a variant name, qualify
to disambiguate:

```lyric
match e {
    ExprKind.Ident => { ... }
    ExprKind.Call(callee, args) => { ... }
    _ => { ... }
}
```

**Context-driven variant disambiguation:** When constructing an enum
variant whose name is shared between two enums, the expected field/parameter
type drives the resolution. (E.g., two enums both have `Pair`; the checker
picks the one whose type matches the assignment target.)

**Tuple patterns:**

```lyric
match (x, y) {
    (1, 2) => { ... }
    (a, _) => { ... }
    (_, _) => { ... }
}
```

**Nested variant patterns:** Variant patterns may carry other variant
patterns as payloads — destructuring is recursive:

```lyric
match maybe_shape {
    Some(Circle(r)) => { println(f"radius {r}") }
    Some(Rect(w, h)) => { println(f"{w}x{h}") }
    Some(Point) => { println("point") }
    None => { println("no shape") }
}
```

**Literal patterns:** Integer, char, string, and bool literals are valid
pattern leaves.

**Wildcards:** `_` matches anything; `Name` binds.

**Not supported today:** range patterns (`1..10`), struct patterns
(`Foo { x, y }`), rest patterns (`..`).

**Enum variant construction:** Positional args only: `Circle(3.14)`. Named
args are not supported for enum variants (use struct literal syntax for
structs).

---

## Classes (Heap-Allocated, By Reference)

Classes have identity, behavior, and heap allocation. Fields are declared in
the class body, not as constructor parameters:

```lyric
class Counter {
    count: i32
    name: string

    func increment(self) {
        self.count = self.count + 1
    }

    func get(self) -> i32 {
        return self.count
    }
}
```

### Construction

**Struct-literal syntax** (when no explicit constructor):

```lyric
let c = Counter { count: 0, name: "main" }
```

Fields not specified are zero-initialized. Fields can have defaults:

```lyric
class Config {
    timeout: u32 = 30
    retries: i32 = 3
}
let cfg = Config {}  // uses defaults
```

**Explicit constructor** — `func ClassName(self, ...)`:

```lyric
class HttpClient {
    url: string
    pool: ConnectionPool?

    func HttpClient(self, base_url: string) {
        self.url = base_url
        self.pool = ConnectionPool { base_url: base_url }
    }
}

// Call syntax when explicit constructor exists
let client = HttpClient("http://api.com")
```

### Generics

```lyric
class Pair<T> {
    first: T
    second: T
}
let p = Pair<i32> { first: 1, second: 2 }
```

### Class Destruction

Every non-`permanent` class gets an **auto-generated** destructor:

```lyric
pub func destroy(mut self) { ... }
```

The destructor body is assembled from the class's `owns`/`refs` relation
declarations — destroying a parent cascades to children
(`owns`) or unlinks them (`refs`). `destructor` blocks declared on
interfaces inject cleanup code into participating classes. You do not write
`destroy` manually.

**`final` function** — use a `final` function for resource cleanup (file
handles, network connections) that must happen **before** the auto-generated
destructor runs:

```lyric
class Connection {
    fd: i32
    final func cleanup(self) {
        close_fd(self.fd)
    }
}
```

Execution order on `.destroy()`:
**`final` → auto-destructor (cascade + unlink) → slab free.**

### `permanent` Classes

`permanent class Foo` opts out of ref-counting and destruction entirely.
Instances live forever (used by the compiler for singletons like `SymTable`
and for AST node classes that have whole-program lifetimes). 🚧 A `permanent`
class that is also a relation target produces a compile-time warning, since
the two policies contradict.

### `implements`

Classes can declare `implements` to signal interface conformance:

```lyric
class Task implements Displayable, Prioritizable {
    // ...
}
```

🚧 `implements` is **declarative-only** today — the checker does not verify
that the required methods are present. Missing methods fail at lowering or
codegen, not in the checker.

### No Inheritance

Lyric does not support classical inheritance. Subtype relationships are
expressed through interface satisfaction. Shared behavior belongs in
interfaces or in separate classes held as dependencies.

---

## Interfaces and Multi-Class Contracts

Interfaces are first-class declarations. They can span multiple type
parameters, defining relationships between types:

```lyric
interface Graph<G, N, E> {
    // Abstract methods bound to type params
    func G.nodes(self) -> [N]
    func N.out_edges(self) -> [E]
    func E.tgt_node(self) -> N

    // Default method (desugared to top-level generic function with
    //   `where Graph<G, N, E>` constraint)
    pub func count_edges(graph: G) -> i32 {
        let mut total: i32 = 0
        let nodes = graph.nodes()
        let mut i: i32 = 0
        while i < len(nodes) {
            total = total + len(nodes[i].out_edges())
            i = i + 1
        }
        return total
    }

    // Field injection — adds fields to implementing classes
    field P.first: C?
    field C.parent: P?

    // Destructor injection — paired by relation kind (owns vs refs).
    // Desugar copies the block whose kind matches the relation's keyword.
    destructor owns P { ... }   // runs when used as `relation Hint X owns [Y]`
    destructor owns C { ... }
    destructor refs P { ... }   // runs when used as `relation Hint X refs [Y]`
    destructor refs C { ... }
    // Bare `destructor P { }` (no kind keyword) defaults to `owns`.
}
```

### Default Methods

A method with a body inside an interface becomes a top-level generic
function with a relational `where` clause. The example above's `count_edges`
desugars to:

```lyric
pub func count_edges<G, N, E>(graph: G) -> i32 where Graph<G, N, E> { ... }
```

Callers invoke it via method syntax (`graph.count_edges()`) when the
interface is implemented on the receiver's type.

(Interface embedding via the `embed` keyword has been removed; see
§Recently Removed.)

### Impl Blocks

Wire interface methods to concrete class methods:

```lyric
// Alias: method → method
impl Graph<SimpleGraph, SimpleNode, SimpleEdge> {
    G.nodes = SimpleGraph.get_nodes
    N.out_edges = SimpleNode.get_edges
    E.tgt_node = SimpleEdge.get_target
}

// Field bind: interface field ↔ concrete field
impl DoublyLinked<Folder, File> {
    P.children <-> Folder.items
    C.label <-> File.title
}

// Inline implementation
impl Printable<Widget> {
    P.to_string = (self) -> string { return f"Widget({self.name})" }
}
```

**Three mapping forms** inside an impl block:

- **Alias** — `T.method = Class.method`
- **FieldBind** — `T.field <-> Class.field` (the `<->` token is the
  bidirectional field-binding arrow)
- **Inline** — `T.method(params) -> Ret { body }`

**Short form after `impl Iface<Args> for ConcreteType`:** the leading
`T.` is implicit; just `method = member`, `method <-> member`, or
`method(params) -> Ret { body }`.

### Where Clauses

Generic functions can require interface satisfaction:

```lyric
pub func count<P, C>(p: P) -> i32 where DoublyLinked<P, C> {
    return len(p.children())
}
```

The relational form (`DoublyLinked<P, C>`) is what makes the method calls
on `p` inside the body resolve to interface methods.

### The `error` Interface

`error` is a built-in interface. Any class with a
`message(self) -> string` method satisfies it via structural subtyping:

```lyric
interface error {
    func error.message(self) -> string
}

class Error {
    msg: string
    pub func message(self) -> string { return self.msg }
}
```

The lowercase name is intentional — `error` is part of the type vocabulary
and reads naturally in signatures like `(T, error)`.

---

## Relations

Relations declare ownership/reference structure between classes using stdlib
interfaces. They trigger field injection, impl binding, and destructor
generation.

**Syntax:**

```
relation [Hint] Parent[<Args>][:parent_label] (owns|refs) (Child[<Args>][:child_label] | [Child[<Args>][:child_label]])
```

- **Hint** — the interface that defines the relation's field-injection and
  method-injection contract. The three stdlib hints (`ArrayList`,
  `DoublyLinked`, `HashedList`) provide canonical patterns, but
  **any binary interface** (two type parameters, in `(parent, child)`
  order) can serve as a hint. The desugar pass uses the hint's `field`
  and default-method declarations to wire up the relation; the
  `owns`/`refs` modifier on the relation line selects which destructor
  the desugar pass synthesizes (cascade vs unlink). Optional; if a
  single ident precedes the parent, it's the hint.
- **Labels** — `:name` after parent and/or child becomes the prefix for
  injected field and method names. `:roster` on the parent side, for
  example, injects `roster_children`, `roster_first`, etc.
- **`[Child]`** brackets indicate a many-cardinality child side
  (`is_many = true`).
- **`owns`** — cascade destroy children when parent destroyed.
- **`refs`** — unlink children when parent destroyed (no cascade).

### Default Methods Are Label-Prefixed

A relation's hint interface can declare default methods. The desugar pass
binds those methods onto the parent type with the parent label as prefix.
For example, given:

```lyric
interface MyList<P, C> {
    field P.items: [C]
    pub func add(self, child: C) { append(self.items, child) }
    pub func count(self) -> i32 { return len(self.items) }
}

relation MyList Panel:w owns [Widget:p]
```

…the desugar pass injects methods callable as `panel.w_add(widget)` and
`panel.w_count()`. The dual free-function form
(`w_add<Panel, Widget>(panel, widget)`) also works for the canonical stdlib
hints (`array_append`, `dll_append`, `hash_insert`, etc.); user-defined
hints get only the method form unless they declare standalone functions.

### Field vs Method Access for Relation Accessors

The injected field `parent_label_field` (e.g., `roster_children`) is
accessible both as a **field** and as a **zero-argument method** (`()`
optional). Both forms work:

```lyric
let n = len(team.roster_children)     // field form
let n = team.roster_children()        // method form (zero-arg call)
```

The field form is idiomatic for direct collection access; the method form
matches the interface contract and is required inside default-method
bodies where the receiver type is a type variable.

🚧 If the hint interface is undefined or has the wrong arity, the
desugar pass silently skips the relation. Error-on-bad-hint is a roadmap
item.

### ArrayList — Dynamic Array Ownership

```lyric
relation ArrayList Team:roster owns [Player:team]
```

**Injected fields:** `Team.roster_children: [Player]`,
`Player.team_parent: Team?`, `Player.team_index: i32`.

**Functions:** `array_append<Team, Player>(t, p)`,
`array_remove<Team, Player>(p)`.

### DoublyLinked — Intrusive Doubly-Linked List

```lyric
relation DoublyLinked Team:team owns [Player:player]   // cascade-destroys children
relation DoublyLinked Room:room refs [Guest:guest]     // unlinks children, does not destroy them
```

**Injected fields:** `Team.team_first: Player?`, `Team.team_last: Player?`,
`Player.player_next: Player?`, `Player.player_prev: Player?`,
`Player.player_parent: Team?`.

**Functions:** `dll_append<Team, Player>(t, p)`,
`dll_remove<Team, Player>(p)`.

The `owns`/`refs` modifier on the relation line selects the destructor the
desugar pass synthesizes: `owns` cascades through the list calling
`.destroy()` on each child; `refs` walks the list nulling sibling links
but leaves children alive.

### HashedList — Hash Table Ownership

```lyric
relation HashedList Registry:reg owns [Entry:entry]
```

Child must implement `hash_key(self) -> u64`. Open-addressing hash table
with 75% load factor rehash and linear probing.

**Functions:** `hash_insert`, `hash_lookup`, `hash_remove`, `hash_init`.

### Generic Type Parameters in Relations

Relations support generic participants:

```lyric
relation HashedList Dict<K, V>:d owns [DictEntry<K, V>:d]
```

The label `:d` on both sides produces field names like `d_children`,
`d_buckets`, `d_hash_cap`, `d_hash_count` on `Dict`, and `d_parent`,
`d_index` on `DictEntry`.

---

## Variables and Constants

```lyric
let x = 42                      // immutable, type inferred
let mut y: i32 = 0              // mutable, type annotated
let ref view = data[5:10]       // immutable view (no copy, shared backing)
let mut ref buf = packet[0:16]  // mutable view (write through, no copy)
```

**Copy-on-assign**: Assignment always copies for all value types
(primitives, structs, tuples, slices — the slice descriptor is copied, but
the backing array is shared). `let mut y = x` creates an independent
mutable copy of the local value.

**`ref` bindings**: `let ref x = expr` creates a zero-copy view into
existing data instead of copying. The source data must outlive the `ref`
binding. `let mut ref` allows writing through the view — essential for
serialization, cryptography, and zero-copy buffer assembly.

**Binding grammar**: `let [mut] [ref] name [: Type] [= expr]`

| Binding | Semantics |
|---|---|
| `let x = expr` | Immutable copy |
| `let mut x = expr` | Mutable copy |
| `let ref x = expr` | Immutable view (shared, no copy) |
| `let mut ref x = expr` | 🚧 Mutable view (write-through, no copy) — parser accepts; the checker rejects `let mut ref` on slices ("slices are value types, mutations like append will not write back to the original"), which is the documented use case. Treat as roadmap. |

**Tuple destructuring:**

```lyric
let (a, b) = make_pair()
let (val, err) = divide(10, 2)        // (T, error) destructuring
let mut (x, y) = origin()
```

**No-initializer form** is accepted at statement level:

```lyric
let x: i32                      // declares; assign later
x = compute()
```

**Discard:** `let _ = expensive_call_for_side_effect_only()` and `_` in
tuple destructuring patterns.

**Let-else:**

```lyric
let Circle(r) = shape else { return error }
use(r)                          // r escapes to outer scope
```

The pattern must start with an uppercase ident followed by `(` (a variant
pattern). The else block must diverge (return/break/panic). 🚧 Today
divergence is convention, not enforced.

**Top-level constants** inside `lyric` blocks:

```lyric
lyric parser {
    let PREC_NONE: i32 = 0
    let PREC_OR: i32 = 1
}
```

These compile to `static` globals in C. At module scope, `let` **requires**
an initializer (a value, possibly `null` of a known type via
`let mut _table: SymTable? = null`).

**Parameter passing vs assignment:** Assignment copies; parameter passing
shares. Passing a slice to a function does NOT copy — the function receives
a view into the caller's backing data. This distinction ensures zero-copy
performance at function boundaries while maintaining value semantics for
local reasoning.

---

## Control Flow

```lyric
if cond { ... } else if cond2 { ... } else { ... }

while cond { ... }

for item in collection { ... }
for item, idx in collection { ... }    // idx is the 0-based index

match expr {
    Pattern => { ... }
    _ => { ... }
}

// Conditional pattern match — variant destructure
if let Circle(r) = shape {
    use(r)
} else {
    fallback()
}

// Assertive pattern extract (bindings escape to outer scope)
let Circle(r) = shape else { return error }
use(r)
```

### Block Scoping

Any `{ }` block creates a new scope. Variables declared inside are local to
that block:

```lyric
func example() {
    let x = 1
    {
        let x = 2       // shadows outer x
        println(x)      // prints 2
    }
    println(x)          // prints 1
}
```

Block scoping works at all pipeline levels: AST (`StmtBlock`), LIR
(`LStmtBlock`), C backend (`{ }`).

### If-Expression

`if/else` can be used as an expression. Both branches must produce a value
of the same type (🚧 unification not enforced today):

```lyric
let result = if cond { a } else { b }
let msg = if count == 1 { "item" } else { "items" }
```

The `else` branch is **required** when `if` is used as an expression.

### `if let`

Two recognized forms:

- `if let Variant(...) = expr { ... }` — destructures an enum variant.
- `if let name = expr { ... }` — treats `expr` as optional and binds `name`
  to the unwrapped value if non-null. Equivalent to `if !isnull(expr) { let name = expr! ... }`.

The second form is real behavior of the lowerer today. Prefer the explicit
`isnull`/`!` pair when the binding form is confusing.

### Variant Check: `is`

The `is` operator checks whether an enum value is a specific variant,
without destructuring:

```lyric
if expr.kind is ExprCall { ... }

// Negation — use ! (there is no `not` keyword)
if !(node is Leaf) { ... }
```

`is` returns `bool`. It does **not** bind any variables — use `if let` for
destructuring.

---

## Type Casts

Postfix `as` syntax:

```lyric
let x: i32 = 42
let y: i64 = x as i64        // widen
let z: i32 = y as i32        // narrow (may truncate)
```

The target can be any type expression — `T?`, `[T]`, `Foo | Bar`, a named
class, etc. The parser does not restrict it.

**Currently accepted by the checker:** all casts are accepted; the cast
produces a value of the target type without runtime checking. There is no
"only numeric ↔ numeric" restriction today. 🚧 Tighten cast semantics:
numeric↔numeric checked at compile time, class↔class checked or restricted,
others rejected.

**Implicit numeric widening:** smaller integer types widen to larger ones
without an `as`. Cross-sign integer assignment (`i32` ↔ `u8`) is also
implicit today — a footgun the roadmap intends to address.

---

## Optional Operations

### Unwrap: `expr!`

Extracts the inner `T` from `T?`. Panics if null:

```lyric
let value: i32 = x!
```

### Null Check: `isnull(expr)`

Returns `true` if optional is null:

```lyric
if !isnull(result) {
    println(f"found: {result!}")
}
```

`== null` and `!= null` are also accepted as equivalent forms.

### Implicit Wrapping

`T` is assignable to `T?` without explicit conversion:

```lyric
func find(xs: [i32], target: i32) -> i32? {
    for x in xs {
        if x == target { return x }  // auto-wrapped
    }
    return null
}
```

### Auto-Deref of Optional Class Receivers

When a field access (`expr.field`) targets an `expr` of type `T?` where `T`
is a class, the checker auto-unwraps the optional — no `!` is required:

```lyric
class Node { name: string, next: Node? }

func greet(n: Node?) {
    println(n.name)        // n is Node?, .name accessed without !
    if !isnull(n.next) {
        println(n.next.name)  // chained auto-deref
    }
}
```

This is convenient for deep field-access chains where every link is
guaranteed non-null in context. 🚧 **Runtime caveat:** today, if the
optional is actually null, the C backend segfaults rather than producing a
Lyric panic — the lowerer emits a direct field access on the class handle
without a runtime null check. Fix is on the roadmap (emit `is_null` →
Lyric panic matching `expr!`'s behavior). For nullable receivers where
you genuinely want a Lyric-level panic, write `n!.name`; for safe access,
write `if !isnull(n) { ... n.name ... }` or `if let n_val = n { n_val.name }`.

Auto-deref applies only to **class** optionals. Struct and primitive
optionals (`T?` where `T` is a struct or primitive) use a tagged
representation; access requires explicit `!`.

### Lvalue Unwrap and Write-Through

`expr!` is also a valid lvalue. You can write through it to mutate a field
on the unwrapped value in place:

```lyric
class Outer { data: Inner? }
struct Inner { value: i32 }

let o = Outer { data: Inner { value: 0 } }
o.data!.value = 42        // writes through the optional unwrap
assert_eq(o.data!.value, 42)
```

The unwrap panics on null exactly as in the rvalue case.

---

## Error Handling: `(T, error)` Tuples and `?`

Functions that can fail return `(T, error)`:

```lyric
func divide(a: i32, b: i32) -> (i32, error) {
    if b == 0 {
        return (0, Error { msg: "division by zero" })
    }
    return (a / b, null)
}
```

### The `?` Operator

Propagates errors from `(T, error)` returns:

```lyric
func compute(x: i32) -> (i32, error) {
    let result = divide(x, 2)?    // early returns on error, result is i32
    return (result, null)
}
```

**Rules:**

- Operand must return `(T, error)`.
- Enclosing function must also return `(..., error)`.
- `?` unwraps the success value: after `let x = foo()?`, `x` is `T` (not
  `T?`). If the error is non-null, the function returns immediately with
  that error.

**Implementation:** The lowerer detects `(..., error)` tuple types by tuple
arity and a final field named `error` or typed as `error`, and converts
them to `LyricResult_T`. The `?` operator desugars via `hoistNestedTry`,
which introduces SSA temps for nested try expressions.

---

## Union Types

```lyric
let value: string | i32 = 42

func process(x: string | i32) -> string {
    match x {
        string => { "it's a string" }
        i32 => { "it's an int" }
    }
}
```

**C emission:** Tagged unions (`LyricUnion`) with a discriminant + per-arm
data.

Assignability: any member type is assignable TO the union; the union is
assignable FROM another type only if all variants match.

---

## F-Strings

```lyric
let msg = f"Hello {name}, count={x + 1}"
```

Expressions inside `{ }` are evaluated and converted to strings via the
auto-generated `to_string`. Escaped braces: `{{` → literal `{`, `}}` →
literal `}`.

F-string parsing is two-pass: the lexer emits an `LFString` token with
`{{`/`}}` mapped to sentinel bytes; the parser spins up a fresh sub-parser
on each `{...}` slice, so nested f-strings and arbitrary expressions
(including struct literals) work inside the braces.

### Triple-Quoted Strings

Triple-quoted strings (`"""..."""`) preserve newlines and don't require
escaping quotes:

```lyric
let sql = """
    SELECT *
    FROM users
    WHERE name = "Alice"
"""
```

The lexer **trims** the content (leading/trailing whitespace stripped). Use
plain `"..."` with `\n` if you need exact control over leading whitespace.

---

## Concurrency

```lyric
spawn { ... }                               // goroutine-style thread
let ch = make_channel<i32>(10)              // buffered channel
let ch2 = make_channel<bool>()              // unbuffered channel
ch.send(value)                              // send
let v = ch.receive()                        // receive
ch.close()                                  // close channel

select {                                    // multiplex
    case v = ch.receive() => { ... }
    case ch2.send(true) => { ... }
    default => { ... }
}

lock(mu) { ... }                            // scoped mutex
```

**Channel type:** `channel<T>` is the generic channel type. Created via
`make_channel<T>()` (unbuffered) or `make_channel<T>(capacity)` (buffered).
Operations use method syntax: `.send(value)`, `.receive() -> T`,
`.close()`.

🚧 `receive()` on a **closed** channel returns the zero value with no
indication of closure. The roadmap target is a `(T, bool)` form
(`let (v, ok) = ch.receive()`).

**Lock type:** `lock` is the mutex type. Used with the `lock(mu) { ... }`
statement for scoped locking:

```lyric
let mut mu: lock
lock(mu) {
    // critical section — mu auto-unlocked at block exit
}
```

**`spawn`** creates a thread that runs the body. Captured variables are
collected by a later pass and passed via an auto-generated context struct.

🚧 **`spawn` captures variables by pointer at the C level** — concurrent
writes through captured pointers are a data race. The roadmap intent is
copy-by-value capture with explicit shared-mutation through channels or
locks.

**`select`** waits on the listed cases.

🚧 The current C implementation polls with `usleep(100)` — functionally
correct but wasteful and not low-latency. The roadmap intent is condvar /
epoll-based wake.

**Field-level concurrency annotation:** `guarded_by(lock_name)` on a field
documents the locking contract. The annotation is parsed and stored on the
field but is not enforced by the checker today.

```lyric
class Executor {
    active: Dict<u32, Job>   guarded_by(mu)
    mu: lock
}
```

**C backend:** Channels use pthreads macros (`LYRIC_CHAN_DEF/IMPL`),
`spawn` uses `pthread_create`, `lock` uses `pthread_mutex_t`.

---

## Generators

```lyric
func range(start: i32, end: i32) -> gen i32 {
    let mut i = start
    while i < end {
        yield i
        i = i + 1
    }
}

for val in range(0, 10) {
    println(val)
}
```

**C backend:** Duff's-device-style state machine with `_init`, `_next`,
`_value`, `_state` helpers and a case-jump dispatch table.

The stdlib provides `range(start, end) -> gen i32` for common iteration.

---

## Built-in Functions

The full set of names recognized by the checker as built-ins:

### Core

| Function | Signature | Description |
|---|---|---|
| `println(...)` | `any... -> unit` | Print with newline |
| `print(...)` | `any... -> unit` | Print without newline |
| `eprintln(...)` | `any... -> unit` | Print to stderr with newline |
| `eprint(...)` | `any... -> unit` | Print to stderr |
| `len(x)` | `[T] \| string -> i32` | Length |
| `append(xs, elem)` | `([T], T) -> [T]` | Append element (returns new slice) |
| `isnull(x)` | `T? -> bool` | Check if optional is null |
| `panic(msg)` | `string -> unit` | Print and abort |
| `exit(code)` | `i32 -> unit` | Exit process (alias of `os_exit`) |
| `assert(cond, msg)` | `(bool, string) -> unit` | Test assertion (see [Testing](#testing)) |
| `assert_eq(a, b, msg?)` | `(T, T, string?) -> unit` | Test equality (see [Testing](#testing)) |
| `format(fmt, ...)` | `(string, any...) -> string` | Format to string |
| `to_string(x)` | `T -> string` | Universal stringifier (auto-generated per type) |

### String / Conversion

| Function | Signature | Description |
|---|---|---|
| `itoa(n)` | `i64 -> string` | Integer to string |
| `atoi(s)` | `string -> (i64, bool)` | String to integer |
| `parse_float(s)` | `string -> (f64, bool)` | String to float |
| `char_to_string(b)` | `u8 -> string` | Byte to single-byte string |
| `string_to_bytes(s)` | `string -> [u8]` | View bytes (zero-copy) |
| `bytes_to_string(b)` | `[u8] -> string` | View bytes as string |
| `hash_string(s)` | `string -> u64` | FNV-1a hash |
| `sym(s)` | `string -> Sym` | Intern (also produced by backtick `` `name` ``) |
| `new_string_builder()` | `-> StringBuilder` | Mutable string builder |
| `new_error(msg)` | `string -> error` | Build an `Error` |

**`str_*` family** (also callable directly, in addition to method form on
strings):

| Function | Signature | Description |
|---|---|---|
| `str_contains(s, sub)` | `(string, string) -> bool` | Substring check |
| `str_index_of(s, sub)` | `(string, string) -> i32` | First occurrence (-1 if missing) |
| `str_len(s)` | `string -> i32` | Byte length |
| `str_index(s, i)` | `(string, i32) -> u8` | Indexed byte |
| `str_trim(s)` | `string -> string` | Trim whitespace |
| `str_replace(s, old, new)` | `(string, string, string) -> string` | Replace all |
| `str_substr(s, lo, hi)` | `(string, i32, i32) -> string` | Substring |
| `str_char_at(s, i)` | `(string, i32) -> string` | Single-byte string at index |
| `str_to_lower(s)` | `string -> string` | Lowercase (ASCII) |
| `str_to_upper(s)` | `string -> string` | Uppercase (ASCII) |
| `str_from_chars(bs)` | `[u8] -> string` | Build from bytes |
| `str_split(s, sep)` | `(string, string) -> [string]` | Split |

**`char_is_*` family** (byte-class predicates):

| Function | Signature | Description |
|---|---|---|
| `char_is_digit(b)` | `u8 -> bool` | ASCII digit |
| `char_is_alpha(b)` | `u8 -> bool` | ASCII alpha |
| `char_is_space(b)` | `u8 -> bool` | ASCII whitespace |
| `char_is_upper(b)` | `u8 -> bool` | ASCII upper |
| `char_is_lower(b)` | `u8 -> bool` | ASCII lower |
| `char_is_alnum(b)` | `u8 -> bool` | ASCII alphanumeric |

### Channels

| Function | Signature | Description |
|---|---|---|
| `make_channel<T>()` | `-> channel<T>` | Unbuffered channel |
| `make_channel<T>(n)` | `i32 -> channel<T>` | Buffered channel |

### IO / OS

| Function | Signature | Description |
|---|---|---|
| `read_file(path)` | `string -> (string, bool)` | Read whole file |
| `write_file(path, content)` | `(string, string) -> bool` | Write file |
| `os_args()` | `-> [string]` | Command-line arguments |
| `os_exit(code)` | `i32 -> unit` | Exit process |
| `os_getwd()` | `-> string` | Current working directory |
| `exec_command(name, args)` | `(string, [string]) -> (string, bool)` | Run command |
| `path_join(parts)` | `([string]) -> string` | Join path components |
| `path_dir(p)` | `string -> string` | Directory of path |
| `path_base(p)` | `string -> string` | Base name of path |
| `path_ext(p)` | `string -> string` | File extension |
| `list_dir(path)` | `string -> ([string], bool)` | List directory |
| `file_exists(path)` | `string -> bool` | Check existence |
| `mkdtemp(prefix?)` | `string? -> string` | Create temp directory; optional name prefix |

### Relation Helpers (internal — usually invoked via generated wrappers)

| Function | Description |
|---|---|
| `array_append<P,C>(p, c)` | ArrayList relation: append child |
| `array_remove<P,C>(c)` | ArrayList relation: O(1) swap-remove |
| `dll_append<P,C>(p, c)` | DoublyLinked relation: append to list |
| `dll_remove<P,C>(c)` | DoublyLinked relation: unlink |
| `hash_insert<P,C>(p, c)` | HashedList: insert (auto-rehash) |
| `hash_lookup<P,C>(p, key)` | HashedList: lookup |
| `hash_remove<P,C>(p, key) -> bool` | HashedList: remove |
| `hash_init<P,C>(p, cap)` | HashedList: explicit init (rarely needed) |
| `hash_rehash<P>(p)` | HashedList: explicit rehash (internal) |
| `hash_find_slot<P>(p, key)` | HashedList: bucket lookup (internal) |

---

## Built-in Methods

### Slices `[T]`

| Method | Returns | Description |
|---|---|---|
| `push(item)` / `append(item)` | `unit` | Append |
| `pop()` | `T` | Remove and return last |
| `length()` / `len()` | `i32` | Length |
| `contains(item)` | `bool` | Membership |
| `slice(lo, hi)` | `[T]` | Subslice |
| `index_of(item)` / `find(item)` | `i32` | First index, -1 if missing |
| `first()` / `last()` | `T?` | Endpoints |
| `is_empty()` | `bool` | Empty check |
| `clear()` | `unit` | Empty out |
| `sort()` | `unit` | In-place sort |
| `reverse()` | `unit` | In-place reverse |
| `remove(item)` | `unit` | Remove first match |
| `extend(other)` | `unit` | In-place append-all |
| `join(sep)` | `string` | Join (string slices) |

### String

| Method | Returns | Description |
|---|---|---|
| `length()` / `len()` | `i32` | Byte length |
| `is_empty()` | `bool` | Empty check |
| `contains(sub)` | `bool` | Substring check |
| `has_prefix(p)` / `starts_with(p)` | `bool` | Prefix check |
| `has_suffix(s)` / `ends_with(s)` | `bool` | Suffix check |
| `substring(lo, hi)` | `string` | Substring |
| `trim()` | `string` | Trim whitespace |
| `to_lower()` / `to_upper()` | `string` | ASCII case |
| `replace(old, new)` | `string` | Replace all |
| `repeat(n)` | `string` | Repeat |
| `char_at(i)` | `string` | Single-byte string at index |
| `split(sep)` | `[string]` | Split |
| `index_of(sub)` | `i32` | First occurrence |
| `join(parts)` | `string` | Join |

### Channels `channel<T>`

| Method | Returns | Description |
|---|---|---|
| `send(value)` | `unit` | Send (blocks if unbuffered/full) |
| `receive()` | `T` | Receive (blocks if empty) |
| `close()` | `unit` | Close |

### Map `map[K]V` (legacy — see [Composite Types](#composite-types))

The legacy `map` type accepts methods (`get`, `set`/`put`, `has`/`contains`,
`delete`/`remove`, `keys`, `values`, `len`/`length`), but its C-level
implementation is non-functional. Use `Dict<K,V>` instead.

### Universal

| Method | Returns | Description |
|---|---|---|
| `to_string()` | `string` | Auto-generated stringifier |

---

## Testing

Testing is a first-class feature of Lyric, not an afterthought bolted on via
libraries. The design is minimal and opinionated: two assertion builtins, a
naming convention, and a CLI command. No test frameworks, no assertion
libraries, no mock systems.

### Design Rationale

Lyric is a language for writing compilers and systems software, primarily by
AI agents. The testing system reflects what those users actually need:

1. **Fast feedback** — write a test, run it, see what broke. No
   configuration, no build files, no test runner setup.
2. **Runtime verification** — Lyric's type checker has intentional gaps
   (cross-file resolution produces warnings, not errors). Tests catch what
   the checker misses.
3. **Minimal ceremony** — a test is just a function. No test classes, no
   decorators, no registration. If it starts with `test_`, it's a test.

### Test Functions

A test function has no arguments and no return value. Its name starts with
`test_`:

```lyric
func test_lexer_basic() {
    let lex = Lexer { source: "let x = 42" }
    let tok = lex.next()
    assert_eq(tok.kind, TLet, "first token should be let")
}
```

Test functions can use all language features — classes, generics, relations,
f-strings, error handling. They share a compilation unit with the code they
test.

### Assertion Builtins

Two builtins are provided by the compiler, not the standard library. This is
intentional: assertions need file and line information that only the
compiler can inject.

#### `assert(condition: bool, message: string)`

If `condition` is false, prints the failure message with file and line, then
terminates the test:

```lyric
assert(len(tokens) > 0, "lexer should produce at least one token")
assert(!isnull(result), "parse should succeed")
```

Output on failure:

```
FAIL  test_lexer_basic
  assert failed at test_lexer.ly:15
    lexer should produce at least one token
```

#### `assert_eq(actual: T, expected: T, message?: string)`

If `actual != expected`, prints the failure message along with both values,
then terminates the test. The `message` parameter is optional:

```lyric
assert_eq(tok.kind, TLet, "first token kind")
assert_eq(result.name, "main", "parsed function name")
assert_eq(count, 42)    // message omitted — still prints expected/got values
```

Output on failure:

```
FAIL  test_lexer_basic
  assert_eq failed at test_lexer.ly:16
    first token kind
    expected: TLet
    got:      TIdent
```

🚧 `assert_eq_approx` (for floating-point tolerance comparisons) is a
roadmap item.

**Value display:** `assert_eq` converts values to strings via
auto-generated `to_string` functions:

| Type | Display |
|---|---|
| Enums | Variant name (e.g., `TLet`, `BinAdd`, `TyI32`) |
| `bool` | `true` / `false` |
| `i8`–`i64`, `u8`–`u64` | Decimal number |
| `f32`, `f64` | Decimal with fraction |
| `string` | The string value (quoted in assert output) |
| Structs | Field dump (e.g., `Pos{line: 1, col: 5}`) |
| Classes | Field dump (e.g., `Lexer{source: "...", pos: 0}`) |

Auto-generated enum `to_string()` is the critical piece — most test
assertions compare enum variants (token kinds, type kinds, expression
kinds). Struct and class `to_string()` dumps all fields, which is
invaluable for debugging position mismatches, AST node differences, etc.

### The `lyric test` Command

```
lyric test [files...]
```

`lyric test` compiles all listed `.ly` files together, discovers all
`test_*` functions, generates a `main()` that invokes each test with result
tracking, compiles with gcc, and runs the binary.

Example:

```
lyric test test_lexer.ly lexer.ly ast.ly
```

Output:

```
PASS  test_lexer_keywords
PASS  test_lexer_strings
PASS  test_lexer_numbers
FAIL  test_lexer_escapes
  assert_eq failed at test_lexer.ly:47
    expected: TStringLit
    got:      TError

4 tests, 3 passed, 1 failed
```

🚧 Per-test timing (`(0.1ms)`) is on the roadmap — it would be useful but
the current runner does not print it.

**Execution model:**

- Tests run sequentially in source declaration order.
- A failed assertion stops that test function immediately (no partial
  execution).
- The suite continues — remaining tests still run.
- Exit code: 0 if all pass, 1 if any fail.

**No test discovery from directories.** You explicitly list files.
🚧 `lyric test -mod . pkg/...` for directory-based discovery is a roadmap
item.

### Test File Conventions

- Test files are regular `.ly` files — no special syntax, no annotations.
- Name test files `test_*.ly` by convention (not enforced by the compiler).
- Place test files alongside the code they test.
- Helper functions used only by tests can live in test files (they're just
  regular functions).

### What Is Not Included (and Why)

| Feature | Reason for exclusion |
|---|---|
| Test fixtures / setUp / tearDown | Enterprise pattern. Tests should be self-contained. |
| Mocking | Lyric is for compilers, not web services. Use real objects. |
| Property-based testing | Requires random generation — out of scope for bootstrap. |
| Code coverage | Requires C instrumentation. Potential future addition. |
| Snapshot testing | Too complex for the value it provides at this stage. |
| Test filtering (`--filter`) | Nice-to-have. Could add `lyric test --filter lexer` later. |
| Parallel execution | Sequential is simpler and sufficient for compiler tests. |
| Subtests / nested tests | Adds complexity without clear benefit for Lyric's use cases. |

The testing system can grow, but the baseline is intentionally small. Two
builtins, one convention, one command.

---

## Memory Model

- **Structs** — stack-allocated, copied by value.
- **Classes** — heap-allocated, passed by reference (pointer at the LIR
  level; `u32` handle under `--soa`).
- **Slices `[T]`** — fat pointer (data + len + cap), copied by value but
  sharing the backing array.
- **Relations** — ownership graph; `.destroy()` cascades through `owns`
  relations and unlinks through `refs`.
- **No GC.** Three regimes for class lifetime:
  1. **Owned** — class is the child of an `owns` relation; lifetime managed
     by the parent. No RC overhead, no scope-exit release.
  2. **Permanent** — `permanent class`; never freed, never RC'd. Used for
     singletons and AST trees.
  3. **Refcounted** — all other classes; the compiler inserts
     `StRefIncr`/`StRefDecr` ops, with destruction at RC=0.

**Move semantics are inferred (not declared).** If a local variable is used
≤1 time after assignment, and is not a function parameter (params are
borrowed), and not a synthetic compiler temp, the assignment is treated as
a **move**: ownership transfers, no retain/release pair is emitted. This is
invisible to the programmer but matters when reading the generated C.

**Auto-generated `destroy` everywhere.** Every non-`permanent` class gets a
`pub func destroy(mut self)` synthesized by the desugar pass — even classes
with no relations. The default body just frees the slab slot.

**`ref` / `unref` ops.** The `ref expr` and `unref expr` statements are
manual RC operations, callable only inside a `trusted` function:

```lyric
trusted func adopt(c: Child) {
    ref c
    self.children.push(c)
}
```

🚧 **UAF after `destroy()`.** Today, holding a stale pointer to a destroyed
object is a use-after-free. The roadmap target is **bidirectional pointers**
as the escape hatch for cross-ownership references — when the owner
destroys, the back-pointer is automatically nulled.

**C backend memory:** slab allocation (`LYRIC_SLAB_BLOCK`) with AoS layout
by default. Under `--soa`, the slab uses parallel arrays (Struct-of-Arrays)
and class handles become 32-bit indices. Scope-exit freeing for local
slices uses escape analysis.

**Detect-UAF mode** (`--detect-uaf`): freed slots are marked with
`_rc = UINT32_MAX`; every access checks for that sentinel. Useful when
debugging tests.

---

## Lexical Structure

### Hard Keywords (lexer)

```
lyric  func  class  struct  enum  interface  relation  destructor
import  impl  as  is  type  where  owns  refs  mut  self
from  true  false  null  pub  let  if  else  for  in  while
match  return  break  continue  spawn  select  case  yield
```

(The lexer also has `KLock` and `KField` and `KImplements` tokens, but
those names are contextual — they are also accepted as ordinary identifiers
in non-keyword contexts. The `cascade` keyword has been removed — see
[Recently Removed](#recently-removed).)

### Contextual Keywords

Resolved by lookahead in specific parser positions. Lex as identifiers:

`field`, `implements`, `lock`, `trusted`, `permanent`, `final`, `gen`,
`channel`, `unit`, `map`, `fn`, `default`, `ref`, `unref`, `isnull`, `sym`,
`guarded_by`.

These can be used as variable, field, or function names — the parser
disambiguates by context.

### Soft-Reserved (accepted as identifiers via `expect_ident`)

`field`, `destructor`, `implements`, `from`, `as`, `is`, `in`.

These can stand in for an identifier in any position that takes an
identifier (e.g., function argument names).

### Operators

- Arithmetic: `+ - * / %`
- Comparison: `== != < <= > >=`
- Logical: `&& || !`
- Bitwise: `& | ^ << >>` (🚧 add `~`; 🚧 fix precedence — see
  [Operator Precedence](#operator-precedence))
- Compound assign: `+= -= *= /=` (🚧 add `%= &= |= ^= <<= >>=`)
- Optional/null: postfix `?`, postfix `!`, unary `!`
- Assignment: `=`
- Arrows: `->` (return type), `=>` (match arm), `<->` (impl-block
  field-binding)

### Literals

- **Integer:** decimal digits with `_` separators (`1_000_000`). 🚧 No
  `0x`/`0o`/`0b` prefixes. 🚧 No type suffixes (`123u64`).
- **Float:** `digits.digits`. `1.` alone does not parse as a float (defends
  method-call syntax).
- **String:** `"..."` with escapes `\n \r \t \\ \' \" \0 \xHH`. 🚧 No
  `\u{...}` escapes. No octal `\NNN`.
- **Triple string:** `"""..."""` — content trimmed.
- **F-string:** `f"..."` — `{expr}` for interpolation, `{{` `}}` for
  literal braces.
- **Char:** `'c'` — single byte u8.
- **Backtick sym:** `` `name` `` — desugars to `sym("name")`.
- **Bool:** `true`, `false`. **Null:** `null`.

### Comments

`// line comments` only. 🚧 No `/* */` block comments.

### Newline Rules

Newlines are statement terminators. Inside `(` and `[` brackets, newlines
are suppressed (multi-line function calls, list literals, tuple expressions
all work without `\` continuations). Inside `{` braces, newlines are
significant (braces delimit statement blocks).

There is no explicit line-continuation character — wrap long expressions in
`(...)` if you need to split across lines.

### Unknown Characters

Unknown punctuation (`@ # $`, etc.) lexes as an `LIdent` token with the
character as its text. The lexer does not error; the parser does.

---

## Compilation

### File Structure

Top-level declarations can appear bare or inside a `lyric` block:

```lyric
// Bare declarations (preferred for .ly files)
func main() {
    println("hello")
}

struct Point { x: f64, y: f64 }
```

```lyric
// Wrapped in lyric block (optional — provides logical grouping)
lyric MyModule {
    func main() {
        println("hello")
    }
}
```

A single `.ly` file may contain multiple `lyric` blocks — they are appended
in order.

### Multi-File Compilation

Multiple `.ly` files in the same package are merged into a single
compilation unit. When compiling a module, all referenced packages are
resolved (single-level only today), merged with namespace prefixing, and
compiled together.

### Compilation Pipeline

```
Parse → ResolveImports → MergeStdlib → DesugarAll → Check → Lower
      → Optimize → Monomorphize → Emit C → gcc
```

**Desugar order** (five passes — MUST run in this sequence):

1. **InterfaceFields** — `field T.name: Type` → abstract getter and setter
   methods.
2. **FieldAccess** — inside interface bodies, rewrite `self.field` →
   `self.field()` and `self.field = x` → `self.set_field(x)`.
3. **Relations** — for each relation, inject label-prefixed fields and
   impl-block field bindings.
4. **Destructors** — generate `pub func destroy(mut self)` on concrete
   classes from interface destructor blocks (with type-param substitution
   and label-prefix method renaming).
5. **DefaultImpls** — extract interface methods with bodies into top-level
   generic functions with relational `where` clauses.

**Check phases** (four phases):

1. **Phase 0** `preregister_type_names` — register stub `TypeInfo` for
   every interface/struct/class/enum across all blocks. Enables forward
   references.
2. **Phase 1** `register_lyric_block` — full `TypeInfo` with fields,
   methods, variants, type params, constraints.
3. **Phase 1.5** `register_interface_methods` — bind methods from impl
   blocks and from where-clauses onto concrete classes, with label-prefix
   handling.
4. **Phase 2** `check_lyric_block_bodies` — type-check all function
   bodies.

**Lower → LIR.** The lowerer translates desugared AST into a typed LIR
(`pkg/lir`) with explicit RC ops, tagged unions, slab handles, generator
state machines, and so on.

**Optimize.** Six LIR→LIR passes:

1. **fuse_side_effect_temps** — eliminate `_t = call(); _ = _t` patterns.
2. **destructure_multi_return** — coalesce same-temp re-reads of multi-value
   returns.
3. **destructure_extract_pairs** — collapse `?`-operator extract pairs.
4. **fix_nil_zero_values** — `return null` on a non-class type becomes the
   appropriate typed zero.
5. **eliminate_unused_temps_recursive** — drop unused temps; preserve side
   effects via `StSideEffect`.
6. **blank_unused_multi_assign_names** — rename unused multi-assign
   positions to `_`.

**Monomorphize.** LIR → LIR: specializes generic functions and classes for
each concrete type instantiation. Iterative — after specializing,
re-collects from specialized bodies for transitive instantiations. Name
mangling is `base_T1_T2_...` (e.g., `Dict_Sym_i32`). After
monomorphization, `validate_post_mono` asserts no `TyTypeVar` remains and
no generic decls have non-empty type parameters.

### C Backend

Requires monomorphized LIR (C has no generics). Outputs `.c` files using
`lyric_runtime.h`. Compile with `gcc -std=gnu11 -I runtime`.

**Identifier safety:** C reserved words colliding with Lyric identifiers
get a trailing `_` suffix in the generated C (mostly invisible to the Lyric
programmer, but visible if you debug the C output).

**Auto-generated `to_string`.** The backend generates a `to_string` for
every enum (returning the variant name), struct (`Foo{a: v, b: v, ...}`),
and class (same shape, null-checked). This powers `assert_eq` output and
`println` of arbitrary values.

**`setjmp`/`longjmp`** is used for assertion failure escape inside test
functions.

### Toolchain Commands

| Command | Description |
|---|---|
| `lyric compile file.ly ... -o out` | Compile files to C and binary |
| `lyric compile <dir>` | Compile module in directory containing `lyric.mod` |
| `lyric test file.ly ...` | Compile, discover `test_*`, run tests |
| `lyric fmt file.lyric ...` | Reformat `.lyric` files (zone 1; zones 2/3 preserved verbatim) |
| `lyric help` | Help |

**Subcommand resolution:** unique-prefix matching. `lyric c` → compile,
`lyric t` → test, etc.

**Backend flags:**

- `-o out` — output path.
- `--lir-dump path` — write LIR text dump (diagnostic).
- `--soa` — switch slab to Struct-of-Arrays layout with `u32` handles.
- `--detect-uaf` — mark freed slots with `_rc = UINT32_MAX`; check on every
  access.
- `--rc-free` (default ON) — `ref_decr` at RC=0 triggers destroy.
- `--no-rc` — disable auto-destruction at RC=0.
- `--c` — accepted as a legacy no-op.

**`lyric fmt`** operates on `.lyric` files (zone 1) only — it preserves
zones 2 and 3 verbatim. 🚧 A formatter for `.ly` files is future work.

🚧 **The `lyric verify`, `lyric update`, and `lyric gen` commands are not
in Lyric.** They live in `lyre` (see [Recently Removed](#recently-removed)).
The Lyric compiler ships with `compile`, `test`, `fmt`, and `help`.

---

## The .lyric File Layer

`.lyric` files are **Lyric source code** with no function bodies — pure
declarations, signatures, interfaces, and relations. Every `.lyric` file is
valid Lyric.

```lyric
lyric ModuleName {
    // types, functions, relations, impls, constants
}
```

The `lyric` block wrapper is **optional** in both `.lyric` and `.ly` files.
When present, it provides a logical grouping name. When absent, bare
top-level declarations are valid — the package name comes from the
directory.

### CDD Layer (lives in `lyre`)

The Context-Driven Development annotations and three-zone file layout —
`why:`, `doc "Section": """..."""`, `invariant:`, `verified_at:`,
`source:`, `fake:`, plus the auto-generated function-index and dependency
zones — are **not part of the Lyric language**. They are consumed and
maintained by the `lyre` tool, which extends `.lyric` syntax with the
CDD layer.

From Lyric's perspective, a `.lyric` file is just a declaration-only Lyric
source file. From `lyre`'s perspective, it is a structured design artifact
with three zones (human-reviewed declarations + annotations, auto-generated
function index, auto-generated dependencies).

See the `lyre` documentation (in `~/projects/lyre/`) for the CDD layer
specification.

---

## Known Gotchas

- **Enum construction is positional only** — `Variant(a, b)`, not
  `Variant(x: a)`.
- **Struct literal ambiguity** — `Ident {` is ambiguous between struct
  literal and variable + block in statement context. The parser uses an
  `expr_depth` counter: inside parens/brackets/arg lists, always a struct
  literal. At statement level, uses `is_struct_lit_ahead` lookahead.
  Additionally, `for`/`while`/`if`/`match` use a `no_struct_lit` flag to
  suppress struct literal parsing in conditions (Rust approach).
- **`append` vs `array_append`** — `append(slice, item)` or
  `slice.push(item)` for plain slices; `array_append<P,C>(parent, child)`
  for relation-owned lists.
- **`null` is the only null literal** — `nil` is not accepted.
- **Number literal underscores** — `1_000_000` is valid; underscores are
  silently stripped by the lexer.
- **Platform `int`/`uint`** — interop with C `int`/`unsigned int`; not part
  of the Lyric numeric tower. Prefer fixed-width.
- **`(T, error)` detection is by name + position** — the lowerer detects
  result-typed tuples by tuple arity and a final field typed `error` or
  named `"error"`. A user-defined class also named `error` could in
  principle collide; stdlib's `error` interface is the canonical one.
- **`mut self` is accepted but redundant** — mutation through `self` is
  always allowed; the `mut` is a parser concession.
- **Dict has `length()` and `len()`** as synonyms; reference and stdlib
  use `len()`.
- **`Hashable` is missing `equals` today** — `Sym` uses pointer equality
  (which is what you want for interned symbols). 🚧 Adding `equals` to
  `Hashable` is a roadmap item; in the meantime, hash collisions are
  resolved by pointer identity.
- **Identifiers colliding with C keywords** get a `_` suffix in the
  generated C. Invisible at the Lyric source level.

---

## What Lyric Is Not

**`.lyric` files** are not a programming language. They contain no
executable code. A `.lyric` file is a structured design artifact — a
compressed, checkable understanding consumed primarily by AI, verified by
external tooling against implementations in any language.

**`.ly` files** are a programming language. They compile, run, and are
type-checked.

Neither mode is:

- **UML** — UML is for human visual parsing; Lyric is for AI context and
  static verification.
- **A schema language** — Protobuf describes serialization; Lyric describes
  system design.
- **Comments** — comments explain code; Lyric files describe the *system*
  above any implementation.
- **Documentation** — documentation decays without enforcement; Lyric
  files are verified at commit time and the AI has skin in the game keeping
  them accurate.

The closest analog is a typed IDL extended with design documentation,
thread safety, invariants, and ownership — where the primary consumer is
an AI working with the code.

---

*Lyric is what a codebase would tell you if it could talk.*

---

## Why Lyric — The Performance and Safety Story

Lyric is designed to become the world's fastest language for memory-
intensive applications — which in a data center is most of them — while
simultaneously being the most memory-safe language that doesn't use garbage
collection.

### Relations Eliminate Manual Destructors

In C++, manual destructors are a primary source of memory safety bugs:
use-after-free, double-free, dangling pointers, and memory leaks. Rust
addresses this with borrow checking and lifetimes, but at enormous
cognitive cost — engineers spend significant time fighting the borrow
checker.

Lyric takes a different approach: **relations declare ownership, and the
compiler generates all destructors automatically.**

```lyric
relation ArrayList Team:roster owns [Player:team]
```

This single line:

- Injects `children`, `parent`, and `index` fields into both classes
- Generates `array_append` and `array_remove` functions
- Generates cascade destructor: destroying a Team destroys all its Players
- Generates child destructor: destroying a Player removes it from its Team

No manual destructor code. No forgetting to clean up. The relation system
manages the back-pointers.

The `owns` vs `refs` distinction makes lifetime semantics explicit:

- `owns` — cascade: parent death kills children
- `refs` — unlink: parent death detaches children (they survive)

This is what Rune and DataDraw proved over decades: **if you declare the
ownership graph, the compiler can manage memory perfectly without GC.**

### SoA / AoS: Cache-Optimal Memory Layout

Most languages store objects as structs-of-arrays (SoA) only when the
programmer manually reorganizes their data. Lyric's relation system knows
the ownership graph and can automatically choose between:

**AoS (Array of Structs)** — traditional layout, good for random access:

```
[Player{name, score, team_ptr}, Player{name, score, team_ptr}, ...]
```

**SoA (Struct of Arrays)** — cache-optimal for iteration:

```
names:     [name1, name2, name3, ...]
scores:    [score1, score2, score3, ...]
team_ptrs: [ptr1, ptr2, ptr3, ...]
```

When iterating over all player scores (the common case in data-intensive
workloads), SoA keeps scores contiguous in cache. AoS wastes cache lines
loading name and team_ptr data that isn't needed.

Lyric's C backend can use relations to generate SoA layout (`--soa` flag):
each relation field becomes a separate array. Getters and setters index
into the correct array. The class handle becomes a 32-bit index, not a
64-bit pointer — halving pointer storage.

**This is the DataDraw insight that Bill proved at scale:** relation-based
code generation with SoA layout produced 10× performance improvements in
EDA tools processing billions of objects. Lyric brings this to a
general-purpose language.

### Multi-Class Interfaces: Expressiveness Without Inheritance

Most languages force a choice: either graph algorithms know about your
concrete types (not reusable) or they use heavyweight inheritance/visitor
patterns (verbose and fragile).

Lyric's multi-class interfaces let you write graph algorithms ONCE and bind
them to ANY concrete types via impl blocks:

```lyric
interface Graph<G, N, E> {
    func G.nodes(self) -> [N]
    func N.out_edges(self) -> [E]
    func E.tgt_node(self) -> N

    pub func shortest_path(graph: G, from: N, to: N) -> [E]? { ... }
}

// Bind to social network types
impl Graph<SocialNetwork, User, Friendship> { ... }

// Bind to road map types — same algorithm, zero code duplication
impl Graph<RoadMap, Intersection, Road> { ... }
```

The default method `shortest_path` works on ANY graph implementation. No
inheritance, no visitor pattern, no type erasure. The monomorphizer
generates specialized code for each concrete binding.

### No GC, No Borrow Checker

Lyric's memory model (current and planned):

- **Slab allocation** for classes → 32-bit index handles (under `--soa`),
  cache-friendly, no malloc/free.
- **Relations** declare ownership → compiler generates destructors.
- **Cascade deletion** through `owns` relations → no leaks.
- **Back-pointers** maintained automatically → no dangling references
  within an ownership tree.
- **Ref counting** for non-owned classes → automatic deallocation when last
  reference dies.
- **Copy-on-assign** for value types → no aliasing surprises for local
  variables.
- **`ref` bindings** for zero-copy views → opt-in sharing when performance
  demands it.
- **`trusted` blocks** → manual `ref`/`unref` for stdlib containers that
  manage their own memory.
- **Deterministic destruction** → predictable latency (no GC pauses).
- **No borrow checker** → no lifetime annotations, no fighting the
  compiler.

🚧 **Roadmap memory features:**

- **Bidirectional pointers** as the escape hatch for cross-ownership
  references — when the owner destroys, the back-pointer is automatically
  nulled. Prevents UAF after `destroy()`.
- **`destroys` annotation** → compiler infers which functions may destroy
  instances and statically prevents UAF.
- **`mut resize` annotation** → compiler prevents accessing array elements
  during resize.
- **Safe iterators** that survive destroy-during-iteration.

The cost: you must declare your ownership graph via relations. But you
were going to design that ownership graph anyway — Lyric just makes it
explicit and verifiable rather than implicit and error-prone.

### The Result

A language that is:

1. **Faster than C++** for memory-intensive applications (SoA layout,
   32-bit handles).
2. **Safer than C++** (no manual destructors, no use-after-free when the
   roadmap items land).
3. **More expressive than Rust** (multi-class interfaces, no borrow checker
   friction).
4. **Simpler than both** (relations replace pages of boilerplate).

---

## Standard Library Reference

The stdlib (`stdlib/std.ly` and `stdlib/string.ly`) is auto-imported into
all Lyric programs. It provides ownership data structures, hash tables,
string utilities, and error handling.

### ArrayList<P, C> — Dynamic Array Ownership

A parent P owns a dynamic array of children C. Children know their parent
and index for O(1) swap-remove.

**Injected fields:**

- `P.children: [C]` — the parent's array of children
- `C.parent: P?` — child's back-reference to parent
- `C.index: i32` — child's position in the array

**Functions:**

| Function | Description |
|---|---|
| `array_append(parent: P, child: C)` | Append child to end of parent's array |
| `array_remove(child: C)` | Remove child using O(1) swap-remove |

**Destructors** (selected by the relation's `owns`/`refs` keyword):

- `owns` parent: cascade-destroys all children (reverse order).
- `owns` child: removes self from parent's array.
- `refs` parent: walks the array nulling each child's parent backref and clears the array; children survive.
- `refs` child: removes self from parent's array.

**Usage:**

```lyric
class Team { name: string }
class Player { name: string }

relation ArrayList Team:roster owns [Player:team]   // cascade
relation ArrayList Pool:p     refs [Player:p]       // unlink only

let t = Team { name: "Eagles" }
let p = Player { name: "Alice" }
array_append<Team, Player>(t, p)
// p.team_parent == t, p.team_index == 0
// t.roster_children == [p]
```

### DoublyLinked<P, C> — Intrusive Doubly-Linked List

Doubly-linked list ownership. The `owns`/`refs` modifier on the relation
line selects the destructor: `owns` cascade-destroys children when the
parent dies; `refs` walks the list nulling sibling links but leaves
children alive.

**Injected fields:**

- `P.first: C?`, `P.last: C?` — list head/tail
- `C.next: C?`, `C.prev: C?` — sibling links
- `C.parent: P?` — back-reference

**Functions:**

| Function | Description |
|---|---|
| `dll_append(parent: P, child: C)` | Append child to end of list |
| `dll_remove(child: C)` | Remove child from list, relink siblings |

**Usage:**

```lyric
relation DoublyLinked Document:doc owns [Paragraph:para]   // cascade
relation DoublyLinked Room:room    refs [Guest:guest]      // unlink only
```

### HashedList<P, C> — Hash Table Ownership

Open-addressing hash table with linear probing. 75% load factor triggers
rehash (double capacity). Children stored in a dense array; a parallel
bucket index maps hash slots to array positions.

**Requirement:** Child class must implement `hash_key(self) -> u64`.

**Injected fields:**

- `P.children: [C]` — dense storage
- `P.buckets: [i32]` — bucket index (-1 = empty, -2 = tombstone)
- `P.hash_cap: i32` — bucket capacity
- `P.hash_count: i32` — live entry count
- `C.parent: P?` — back-reference
- `C.index: i32` — position in children array

**Functions:**

| Function | Description |
|---|---|
| `hash_init(parent: P, capacity: i32)` | Initialize with given capacity (min 8) |
| `hash_insert(parent: P, child: C)` | Insert (auto-inits, auto-rehashes) |
| `hash_lookup(parent: P, key: u64) -> C?` | Lookup by hash key |
| `hash_remove(parent: P, key: u64) -> bool` | Remove by hash key |
| `hash_find_slot(parent: P, key: u64) -> i32` | Find bucket slot (internal) |
| `hash_rehash(parent: P)` | Rehash into larger bucket array (internal) |

**Destructors** (selected by the relation's `owns`/`refs` keyword):

- `owns` parent: cascade-destroys all children (reverse order).
- `owns` child: removes self from parent's hash table.
- `refs` parent: nulls each child's parent backref, clears children/buckets; children survive.
- `refs` child: removes self from parent's hash table.

**Usage:**

```lyric
class Entry {
    key: u64
    value: i32
    pub func hash_key(self) -> u64 { return self.key }
}

relation HashedList Registry:reg owns [Entry:entry]

let r = Registry {}
let e = Entry { key: 42, value: 100 }
hash_insert<Registry, Entry>(r, e)
let found = hash_lookup<Registry, Entry>(r, 42)
```

### Dict<K, V> — Generic Hash Table

Generic hash table where `K: Hashable`. Built on `HashedList` with
configurable key types. The most common instantiation is `Dict<Sym, V>`
(string-keyed via `Sym`).

**Constructor:** `Dict<K, V>()` — creates an empty dictionary.

**Methods:**

| Method | Return | Description |
|---|---|---|
| `set(key, val)` | `unit` | Set key-value pair (replaces if exists) |
| `get(key)` | `DictEntry<K,V>?` | Get entry by key (null if missing) |
| `has(key)` | `bool` | Check if key exists |
| `remove(key)` | `bool` | Remove by key |
| `keys()` | `[K]` | All keys |
| `len()` | `i32` | Number of entries |

**`DictEntry<K, V>` fields:** `key: K`, `value: V`.

**Usage:**

```lyric
let d = Dict<Sym, i32>()
d.set(`x`, 42)
d.set(`y`, 99)
if d.has(`x`) {
    let entry = d.get(`x`)
    println(entry!.value)  // 42
}
d.remove(`x`)
```

**Dict literal syntax.** `Dict<K, V>` instances can be constructed with a
brace literal. The keys may be string literals, backtick syms, or integer
literals; the parser disambiguates Dict literals from struct literals by
the first key form (`{ "k": v }`, `` { `k`: v } ``, or `{ 0: v }`):

```lyric
let names = {`alice`: 1, `bob`: 2}              // Dict<Sym, i32>
let cities = {"NYC": 8_000_000, "SF": 875_000}   // Dict<string, i32>
let lookup = {1: "one", 2: "two"}                // Dict<i32, string>

// Empty literal requires a type annotation so K, V can be inferred:
let empty: Dict<Sym, string> = {}
```

The auto-import pass adds the `Dict` class to the compilation unit
whenever it sees a Dict literal — no manual import required.

### Hashable — Hash Key Interface

Interface for types used as hash table keys:

```lyric
interface Hashable {
    func Hashable.get_hash(self) -> u64
}
```

`Sym` implements `Hashable`. `string` does NOT — this is deliberate.
Wrapping strings in `sym()` enforces hash-once discipline, preventing
repeated FNV-1a computation on the same string value (a common performance
bug in compilers).

🚧 An `equals` method on `Hashable` is on the roadmap — required for
collision resolution to work correctly with non-pointer-equal keys.

### Sym — Interned Symbol

Wraps a string with a pre-computed FNV-1a hash. Hash is computed once at
creation; all subsequent operations use the u64 hash for O(1) comparison.
This is the "integer war" principle: avoid repeated string hashing in hot
paths.

**Construction:** `sym("name")` or backtick syntax `` `name` `` (desugars
to `sym("name")` at parse time).

**Methods:**

| Method | Return | Description |
|---|---|---|
| `get_name(self)` | `string` | Original string |
| `get_hash(self)` | `u64` | Pre-computed FNV-1a hash |

`Sym` implements the `Hashable` interface.

**Usage:**

```lyric
let s = sym("identifier")
let s2 = `identifier`        // equivalent — backtick syntax
let h = s.get_hash()         // u64, computed once
let n = s.get_name()         // "identifier"
```

**Design note:** `string` does NOT implement `Hashable`. Use `sym()`
wrapping to enforce hash-once discipline.

### Error Handling

**`error` interface:** any class with `message(self) -> string` satisfies
it.

**`Error` class:** default concrete implementation.

```lyric
let e = Error { msg: "something went wrong" }
println(e.message())  // "something went wrong"
```

**Custom errors:** just implement `message`:

```lyric
class ParseError {
    msg: string
    line: i32
    pub func message(self) -> string {
        return f"{self.line}: {self.msg}"
    }
}
```

### StringBuilder

Efficient string building via repeated append.

**Construction:** `new_string_builder()`

**Methods:**

| Method | Description |
|---|---|
| `write(s: string)` | Append string |
| `write_byte(b: u8)` | Append single byte |
| `to_string() -> string` | Get built string |
| `len() -> i32` | Current length |

**Usage:**

```lyric
let sb = new_string_builder()
sb.write("hello")
sb.write_byte(' ')
sb.write("world")
println(sb.to_string())  // "hello world"
```

### String Utilities (stdlib/string.ly)

Pure functions operating on strings as byte slices.

| Function | Signature | Description |
|---|---|---|
| `str_contains(s, sub)` | `(string, string) -> bool` | Contains substring |
| `str_has_prefix(s, pre)` | `(string, string) -> bool` | Starts with prefix |
| `str_has_suffix(s, suf)` | `(string, string) -> bool` | Ends with suffix |
| `str_index_of(s, sub)` | `(string, string) -> i32` | First occurrence (-1 if not found) |
| `str_split(s, sep)` | `(string, string) -> [string]` | Split by separator |
| `str_split_n(s, sep, n)` | `(string, string, i32) -> [string]` | Split into at most `n` pieces |
| `str_trim(s)` | `string -> string` | Trim whitespace both ends |
| `str_trim_left(s)` | `string -> string` | Trim whitespace left |
| `str_trim_right(s)` | `string -> string` | Trim whitespace right |
| `str_to_upper(s)` | `string -> string` | Uppercase (ASCII only) |
| `str_to_lower(s)` | `string -> string` | Lowercase (ASCII only) |
| `str_replace(s, old, new)` | `(string, string, string) -> string` | Replace all occurrences |
| `str_repeat(s, n)` | `(string, i32) -> string` | Repeat n times |
| `str_join(parts, sep)` | `([string], string) -> string` | Join with separator |

🚧 Once UTF-8 lands, these will gain code-point-aware companions or be
upgraded in-place.

---

## I/O Library — Current and Planned

### Current Built-ins (Minimal Bootstrap I/O)

The following are built-in functions, not stdlib — they're implemented
directly in the lowerer and backends:

| Function | Description |
|---|---|
| `read_file(path) -> (string, bool)` | Read entire file as string |
| `write_file(path, content) -> bool` | Write string to file |
| `os_args() -> [string]` | Command-line arguments |
| `os_exit(code: i32)` | Exit process |
| `os_getwd() -> string` | Current working directory |
| `exec_command(name, args) -> (string, bool)` | Run external command |
| `path_join(parts: [string]) -> string` | Join path components |
| `path_dir(p) -> string` | Directory portion |
| `path_base(p) -> string` | Base filename |
| `path_ext(p) -> string` | File extension |
| `list_dir(path) -> ([string], bool)` | List directory entries |
| `file_exists(path) -> bool` | Check if file exists |
| `mkdtemp() -> string` | Create temporary directory (optional prefix arg) |

These are sufficient for the bootstrap compiler. They read/write entire
files as strings — no streaming, no file handles, no buffering.

### 🚧 Planned I/O Library (Post-Bootstrap)

The full I/O library should provide:

**File I/O with handles:**

```lyric
interface Reader {
    func read(self, buf: [u8], n: i32) -> (i32, error)
    func close(self) -> error?
}

interface Writer {
    func write(self, data: [u8]) -> (i32, error)
    func flush(self) -> error?
    func close(self) -> error?
}

class File implements Reader, Writer {
    pub func open(path: string, mode: string) -> (File, error)
    pub func create(path: string) -> (File, error)
}

class BufferedReader {
    pub func read_line(self) -> (string, error)
    pub func read_all(self) -> (string, error)
}

class BufferedWriter {
    pub func write_string(self, s: string) -> error?
    pub func write_line(self, s: string) -> error?
}
```

**Directory operations:**

```lyric
func list_dir(path: string) -> ([string], error)
func mkdir(path: string) -> error?
func mkdir_all(path: string) -> error?
func remove(path: string) -> error?
func remove_all(path: string) -> error?
func rename(old: string, new_path: string) -> error?
func stat(path: string) -> (FileInfo, error)
func exists(path: string) -> bool
```

**Stdin/stdout/stderr as Writer/Reader:**

```lyric
let stdin: Reader = os_stdin()
let stdout: Writer = os_stdout()
let stderr: Writer = os_stderr()
```

**Network I/O** (future):

```lyric
class TcpListener {
    pub func bind(addr: string) -> (TcpListener, error)
    pub func accept(self) -> (TcpStream, error)
}

class TcpStream implements Reader, Writer { ... }
```

**Design principle:** Unix-only for bootstrap. Cross-platform abstraction
is a post-1.0 concern. The Reader/Writer interface pattern (proven by Go)
where everything that reads or writes bytes satisfies the same interface
enables composition: `BufferedReader(File.open("x.txt")?)`.

---

## Roadmap

The features below are described in this spec but not yet implemented.
Each is tracked in `TODO.md` and `cr/docs/bootstrap-roadmap.md`. This list
is the single source of truth for "what 🚧 means."

### Type System

- **Larger numeric tower** — `u128`, `u256`, `i128`, `i256`, `f128`
  registered in the checker, lowered through LIR, emitted by the C backend.
- **Tighter `null` assignability** — currently `null` is assignable to any
  type; tighten to `T?` / class / interface / `error` only.
- **`as` cast restriction** — currently any type-to-type cast is accepted;
  tighten to numeric↔numeric (checked) and class↔class (checked or
  restricted).
- **If/match branch unification** — currently the type of an
  if-expression or match-expression is the first branch; enforce that all
  branches agree.
- **`implements` method verification** — currently declarative-only; verify
  required methods are present in the checker.

### Operators

- **`~` (bitwise NOT)** — add lexer token, parser unary, type rule (integer
  in, integer out).
- **Compound assigns** `%= &= |= ^= <<= >>=` — add lexer tokens, parser
  handling, lowering.
- **Bitwise-operator precedence promotion** — `& | ^ << >>` promoted above
  all non-integer operators. After: `a & 1 == 0` parses as `(a & 1) == 0`.
  Bitwise operators take integers and return integers; they should not sit
  at boolean precedence.

### Lexer / Literals

- **Hex / octal / binary integer literals** — `0xFF`, `0o755`, `0b1010`.
- **Type-suffixed literals** — `123u64`, `1.0f32`.
- **`\u{NNNN}` Unicode escapes** in strings and chars.
- **`/* */` block comments** — low priority; Lyric culture is to use
  `.lyric` files for big docs.

### UTF-8

- **String type stays `string`** (no rename to `bytes`); a UTF-8 layer goes
  on top.
- **Code-point iteration** — `for c in s.chars() { ... }` returning `i32`
  or `rune`.
- **Unicode-aware case** — `to_lower`/`to_upper` operating on code points.
- **`s.char_at(i)`** returning a code point, not a single-byte string.
- **Normalization** — NFC/NFD via stdlib.

### Imports / Modules

- **Recursive import resolution** — currently single-level only.
- **`pub` filtering** — non-`pub` declarations not visible across imports.
- **Cycle detection** — required before recursive imports.
- **`lyric.mod` parsing** — extract module path, dependencies; today only
  file existence is checked.
- **Entry-point discovery** — scan root package for `main()` when given a
  directory.
- **Bare `import "path"`** — derive alias from path basename instead of
  crashing.

### Function Annotations

- **The full annotation table** — `requires:`, `ensures:`, `raises:`,
  `concurrent:`, `requires_lock()`, `excludes_lock()`, `spawns:`, `pure:`
  on functions in `.lyric` files. Today only field-level
  `guarded_by(lock)` parses.
- **Enforcement of `guarded_by`** — currently parsed and stored but not
  checked.

### Concurrency

- **`receive() -> (T, bool)`** on closed channels — distinguish zero from
  closed.
- **`select` wake mechanism** — replace `usleep(100)` polling with
  condvar / epoll.
- **Copy-by-value spawn captures** — currently captures are by pointer
  (data race).
- **`spawn` argument list** — explicit values to capture, not implicit
  free-variable analysis.

### Memory Safety

- **Bidirectional pointers** — escape hatch for cross-ownership references;
  back-pointer auto-nulled on owner destroy. The fix for "UAF after slab
  reuse."
- **`destroys` annotation** — declare which functions may destroy
  instances; checker prevents UAF.
- **`mut resize` annotation** — declare resize-during-iteration; checker
  prevents access during.
- **Safe iterators** — survive destroy-during-iteration.

### Stdlib

- **`Hashable.equals`** — required for non-pointer-equal hash keys.
- **`assert_eq_approx(a, b, tol)`** — floating-point assertion.
- **Per-test timing** in `lyric test` output.
- **Directory-mode `lyric test -mod . pkg/...`**.

### Cleanups (remove these from the implementation)

- **`Mutex` (capital) and `Lock` (capital)** as synonyms for `lock` type in the lowerer —
  standardize on lowercase `lock`.
- **`defer`** — `StDefer` exists in LIR and emits inlined body; no syntax;
  no real semantics. Remove from LIR + AST until properly designed.
- **`map[K]V`** — non-functional at runtime; either implement properly or
  remove.
- **Go-module syntax (`fmt.Println`, `strconv.Itoa`, etc.)** — vestigial
  bootstrap support; remove now that the Go backend is gone.
- **`byte` / `rune` in `is_primitive_type`** — never registered; dead.
- **`KNil` token name** — emitted for the literal `null`; rename for
  clarity.

---

## Recently Removed

Features previously documented in this spec that have been moved out or
deleted. Listed here so readers of old branches and old `.lyric` files know
where things went.

### `embed` Keyword (Interface Embedding)

Previously, an interface could embed another interface, copying its
fields and destructors (but not its methods):

```lyric
interface Counted<P, C> {
    embed DoublyLinked<P, C>    // copies fields and destructors
    field P.count: i32
    destructor P { ... }
}
```

The keyword and its desugar pass have been deleted. Its only consumers
in the stdlib (`OwningList`, `RefList`, `RefArrayList`) were collapsed
into the three public hints with paired `owns`/`refs` destructors (see
below); user-defined hint interfaces are uncommon and can declare the
fields and destructors they need directly. Any remaining user code that
used `embed` should inline the embedded interface's fields and
destructor blocks into the embedding interface.

### CDD Annotations and the Three-Zone `.lyric` Layout (→ `lyre`)

The Context-Driven Development annotation set has moved to the `lyre` tool:

- **`why: "..."`** — one-line purpose attached to a declaration.
- **`doc "Section": """..."""`** — narrative blocks (especially
  `doc "Invariants"` for operational contracts).
- **`invariant: "..."`** with optional **`verified_at: "hash"`** stamps —
  system-wide claims and human-verification metadata.
- **`source: [...]`**, **`fake: "..."`** — links from a `.lyric` design
  artifact to its implementation and test fakes.
- **Three-zone file layout** — human-reviewed declarations, auto-generated
  function index, auto-generated dependencies.

These were never parsed by the Lyric grammar (they were `lyre`-layer
extensions all along); the documentation now lives where the
implementation does. See `~/projects/lyre/`.

### `lyric verify`, `lyric update`, `lyric gen` (→ `lyre`)

These subcommands never existed in the self-hosted compiler. The driver
supports only `compile`, `test`, `fmt`, and `help`. The verify/update/gen
toolchain lives in `lyre`.

### Function Annotations on `.ly` Source (→ 🚧 roadmap)

The eight-row annotation table (`requires:`, `ensures:`, `raises:`,
`concurrent:`, `requires_lock`, `excludes_lock`, `spawns:`, `pure:`)
remains in the spec as a roadmap target but does not parse today. The
spec previously presented it as current.

### Go Backend

Deleted (commit 8221e5a). The C backend is the sole backend. Some
vestigial pieces remain in the checker (Go-stdlib module registrations)
and in the c_backend (`fmt.Println`/`strconv.Itoa`/etc. hardcoded as
aliases for Lyric builtins). Those are scheduled for removal.

### `cascade { body }` Statement

Removed. The `cascade` keyword no longer lexes; the AST `Cascade(body)`
variant, the parser rule, and the lowerer arm have all been deleted.
Cascade semantics are expressed via `owns`/`refs` on relations — the
standalone statement was a long-deprecated no-op.

### `Mutex` / `Lock` (capital) Type

The lowerer accepts `Mutex`, `Lock`, and lowercase `lock` as names for the
mutex type. The lowercase `lock` is canonical. Both capitalized variants
are slated for removal.

### `map[K]V` Built-in (status: non-functional)

The legacy `map[K]V` type and `map[K]V { ... }` literal parse and
type-check but produce non-functional C (NULL at runtime). The roadmap
either implements it properly or removes it. Use `Dict<K, V>`.

### `defer` Keyword and `StDefer` LIR Statement

The LIR has a `StDefer` statement variant and `LDeferData` payload, and
the C backend emits the body inline with a `/* defer (executed inline): */`
comment. There is no `defer` keyword in the lexer — these LIR pieces are
unreachable from user syntax and are slated for removal pending a proper
design.

### `OwningList`, `RefList`, `RefArrayList`, `ArrayListBase` Relation Hints

Previously the doubly-linked-list family had four interfaces
(`DoublyLinked` base, `OwningList`/`RefList` cascade/unlink wrappers
that `embed`ded it), and the array-backed family had three
(`ArrayListBase` base, `ArrayList`/`RefArrayList` wrappers). They were
collapsed: each user-facing hint (`ArrayList`, `DoublyLinked`,
`HashedList`) is now a single interface carrying `destructor owns P/C`
and `destructor refs P/C` blocks, selected by the relation's
`owns`/`refs` keyword. Existing code that wrote `relation OwningList X
owns [Y]` migrates to `relation DoublyLinked X owns [Y]`; `relation
RefList X refs [Y]` becomes `relation DoublyLinked X refs [Y]`;
`RefArrayList` becomes `ArrayList ... refs`.

---

*End of spec. The reference (`cr/docs/lyric-language-reference.md`) is the
companion document for daily use; the bootstrap roadmap
(`cr/docs/bootstrap-roadmap.md`) is the implementation schedule for
everything marked 🚧 here.*
