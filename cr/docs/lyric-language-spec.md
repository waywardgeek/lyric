# Lyric — A Typed Language for Design and Implementation

*Bill Cox & CodeRhapsody — June 2026*

**Source code & tools:** [github.com/waywardgeek/lyric](https://github.com/waywardgeek/lyric)

---

## Purpose

Lyric is a typed language with two modes of use:

**`.lyric` files — understandings.** Declaration-only Lyric: types, signatures,
interfaces, annotations, doc blocks, invariants, ownership relations. No function
bodies. The design artifact for Understanding-Driven Development (UDD). Verified
against implementations written in any language. Every `.lyric` file is valid Lyric.

**`.ly` files — code.** Full Lyric with function bodies, executable control flow,
and real semantics. Compilable and runnable. An optional capability — UDD does not
require production code to be written in Lyric. The compiler exists to prove the
language design is sound: if the notation is precise enough to verify against real
implementations, then function bodies are all that's missing to make it a real language.

Both modes are designed to be:
- **Read primarily by AI** — dense with meaning, minimal ceremony, no noise
- **Written primarily by AI, reviewed by humans** — the AI writes after implementation;
  the human reviews for accuracy
- **Verified** — `.lyric` files are structurally checked against implementations;
  `.ly` files are type-checked and compiled
- **Language-agnostic in intent** — `.lyric` files describe design regardless of
  implementation language; `.ly` files compile to Go or C

---

## Design Philosophy

**Permissive by default.** `.lyric` files accept design patterns common across
languages, imposing only *sound* constraints — constraints that improve design quality
regardless of target language. The key example: typed function signatures. Every
language has types (even dynamically-typed ones); requiring them catches design
errors the way TypeScript improves on JavaScript.

**No language-specific restrictions.** If Python allows circular imports, Lyric does
not forbid them. If Rust requires explicit lifetimes, Lyric does not require them.
The `.lyric` file describes the design, not the target language's rules.

**Sound constraints Lyric does impose:**
- Every parameter and return value must have a type (may be a type variable)
- `self` declares method receivers (mutation is implicit — no `mut self` distinction)
- `mut` parameters pass structs by mutable reference (required on both decl and call site)
- Thread safety annotations must be consistent (`guarded_by(mu)` requires `mu` to exist)

**Constraints Lyric deliberately does NOT impose:**
- Import ordering or circularity restrictions
- Memory management model (ownership, borrowing, GC)
- Error handling strategy (`(T, error)` is idiomatic but not the only option)
- Naming conventions (follow the implementation language's conventions)

---

## Design Lineage

Lyric inherits its core from **Rune**, Bill Cox's systems language. The inheritance
is selective.

**Kept from Rune:**
- The numeric type tower (`u8`–`u256`, `i8`–`i256`, `f32`/`f64`/`f128`)
- `string`, `bool`
- `T?` for optional values
- `T | U` for typed unions
- `[T]` for sequences, `Dict<K,V>` for associative containers (stdlib, not built-in)
- `enum`, `struct`, `class`
- `relation` for ownership/reference structure, with `owns`/`refs`, labels, and hints

**Dropped from Rune:**
- **Secret propagation in the type system** — research goal, not design tool
- **Optional types everywhere** — Rune allowed omitting types to feel like Python;
  Lyric requires every parameter and return value to be typed
- **Memory safety mechanisms in the language** — ownership enforcement is the
  compiler's job; `.lyric` files express intent via `relation`
- **Python-style goals** — Lyric appeals to the AI reading it, not Python programmers

**Added in Lyric:**
- **`interface` as a first-class declaration** — multi-class structural contracts
  with type parameters, method binding, field injection, and default implementations
- **Type variables and `where` clauses** — generics with named capability constraints
- **`error` as a built-in interface** — uniform error handling
- **Design annotation layer** — `why:`, `doc`, `invariant:`, thread safety annotations,
  `source:`, `fake:`
- **Function annotations** — `requires:`, `ensures:`, `raises:`, `concurrent:`,
  `requires_lock()`, `excludes_lock()`, `guarded_by()`, `spawns:`, `pure:`
- **Relations with code generation** — `ArrayList`, `OwningList`, `RefList`,
  `HashedList` hints trigger field injection, impl binding, and destructor generation
- **Impl blocks** — wire interface methods to concrete class methods, bind fields,
  or provide inline implementations
- **`embed` keyword** — interface embedding (copies fields and destructors)
- **Three-zone `.lyric` files** — human-reviewed declarations + auto-generated
  function index + auto-generated dependencies

---

## Modules and Packages

### Package = Directory

A **package** is a directory of `.ly` files. All `.ly` files in a directory belong to the same package. The package name is the **directory name**.

```
mycompiler/
  lyric.mod                # module root: "github.com/user/mycompiler"
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

Within a package, all declarations across all `.ly` files are visible to each other — declaration order and file order don't matter. The compiler merges all files in a package into one unit before processing.

**The `lyric` block wrapper is optional.** When present, the block name provides a logical grouping but does **not** override the package name. The package name always comes from the directory. When absent, bare top-level declarations belong to the directory's package.

### Module = Project

A **module** is a project rooted at a `lyric.mod` file. The module defines:
- The import path prefix (e.g., `github.com/user/mycompiler`)
- External dependencies (future: version resolution)

A module is the **unit of compilation**. Lyric compiles an entire module at once — all packages are resolved at compile time, merged with namespace prefixing, and emitted as a single C output compiled to one binary (or shared library). There is no separate compilation of individual files or packages.

```
# lyric.mod
module github.com/user/mycompiler
```

### Imports

Import a package by name — the compiler resolves it to a directory relative to the module root:

```lyric
import ast
import parser

func main() {
  let file = parser.parse("hello.ly")
  let node = ast.Node { name: "root" }
  print(node.name)
}
```

**Rules:**
- `import <name>` imports the package in the directory `<name>/` relative to the module root
- Access is always qualified: `ast.Node`, `parser.parse()`
- Only `pub` declarations are visible through imports — all other declarations are package-private
- Circular imports are a compile error
- For nested packages, use a path with alias: `import v2 from "parser/v2"`

**Qualified access:**
```lyric
import ast

let n = ast.Node { name: "x" }     // struct construction
let kind = ast.ExprKind.Ident       // enum variant
let result = ast.parse(src)         // function call
```

### Standard Library

The stdlib (`stdlib/std.ly`, `stdlib/string.ly`, etc.) is auto-imported into all packages. Its declarations are available unqualified — no import statement required.


### Compilation Model

```bash
lyric compile .                           # compile module in current directory
lyric compile ~/projects/mycompiler/      # compile module at path
lyric compile main.ly -o myprogram        # single-file, no module needed
lyric compile main.ly ast.ly              # multi-file, no module needed
lyric test test_lexer.ly lexer.ly ast.ly  # test specific files
```

When given a directory, the compiler looks for `lyric.mod`, finds `main()` in the root package, and recursively resolves all imports. When given a `.ly` file, it checks parent directories for `lyric.mod` — if found, uses module mode; otherwise, single-file mode.

When compiling a module, the compiler:
1. Reads `lyric.mod` to determine the module root
2. Scans the root package for `main()` as the entry point
3. Recursively resolves all `import` statements to package directories
4. Parses all `.ly` files in each referenced package
5. Merges packages with C-level namespace prefixing (e.g., `ast_Node`, `parser_parse`)
6. Runs the full pipeline (desugar → check → lower → optimize → monomorphize → emit C)
7. Compiles the single C output to a binary via gcc/clang


---

## Primitive Types

```
// Unsigned integers
u8   u16   u32   u64   u128   u256

// Signed integers
i8   i16   i32   i64   i128   i256

// Floating point
f32   f64   f128

// Other primitives
string    // UTF-8 text; indexing returns u8 (byte)
bool      // true | false
unit      // void/no-value type (for functions with no return value)

// Platform-width integers (Go interop only)
int   uint    // NOT part of the Lyric numeric tower
```

**Default integer literal type:** `i32`. Cast with `x as u64`.

**Character literals:** `'A'` → `u8` constant (value 65). Supports escape sequences:
`\n`, `\r`, `\t`, `\\`, `\'`, `\"`, `\0`, `\x##` (hex byte).

`null` (or `nil` — both accepted) is the nil literal for optional types. `let x = null`
without a type annotation is a checker error; use `let x: T? = null`.

`error` is a built-in interface, not a primitive — see the Interfaces section.

---

## Composite Types

```
T?            // optional: T or null (nil)
T | U         // union: T or U (exhaustively typed)
[T]           // slice of T (fat pointer: data + len + cap)
(T, U)        // anonymous tuple (positional)
(T, U, V)     // triple, etc.
channel<T>    // CSP channel (created via make_channel<T>())
fn(T, U) -> V // function type
```

**String as byte slice:** `string` is `[u8]` internally. String indexing (`s[i]`)
returns `u8`. String concatenation uses the `+` operator: `"hello" + " world"`.
Slice concatenation also uses `+`: `[1,2] + [3,4]` returns `[1,2,3,4]`.
In-place slice extension: `xs.extend(ys)`. C interop via `lyric_str_to_cstr()`
null-terminates on demand.

**Maps:** Lyric does not have a built-in `map[K]V` type. Use `Dict<K,V>` from the
standard library, which provides a generic hash table with `Hashable` key constraint.
See the Standard Library Reference section for the Dict API.

**Tuples:** Anonymous tuples `(T, U)` are positional. Access fields with `._0`, `._1`
notation (underscore-prefixed indices):

```lyric
let t = (42, "hello")
println(t._0)    // 42
println(t._1)    // "hello"

let (a, b) = make_pair()   // destructuring also works
```

**Function type syntax:** `fn(T, U) -> V` is the canonical form for type syntax.
`func(T, U) -> V` is also accepted in parameter type positions. `func` is the
keyword for function declarations; `fn` is preferred for type syntax. Both `fn`
and `func` can be used as contextual keywords in type positions.

---

## Generics and Type Variables

**The rule:** Every function parameter and return value must declare a type. A type
may be a *type variable* — a placeholder resolved at the call site.

### Type Variables

Type variables are declared explicitly using angle brackets `<>`. They must be
explicitly declared — they are never inferred from context. This prevents typos
from silently becoming type variables:

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

**Inference algorithm:** Walks parameter types and argument types in parallel,
binding type variables to concrete types on first match. Recurses through composite
types (List, Optional, Tuple, Func).

### Nested Generic Syntax: `>>` Splitting

Nested generics like `Dict<Dict<V>>` produce `>>` which lexes as `TShr` (shift right).
The parser splits `TShr` into two `TGt` tokens using a `pushBack` field (single-token
lookahead). Both `tryParseTypeArgs` and `parseBaseType` handle this — same approach
as Java and Rust.

### Constraints

Constraints restrict what types a variable can stand for. A constraint names a
*capability* — what a type *is* — not a list of individual operations:

**Inline constraints:**
```lyric
func clamp<T: Comparable>(value: T, lo: T, hi: T) -> T
```

**`where` clause:**
```lyric
func count<P, C>(p: P) -> i32 where DoublyLinked<P, C>
```

**Built-in constraints:**

| Constraint | Satisfied by | Go mapping |
|---|---|---|
| `Comparable` | numeric, string, bool | `cmp.Ordered` |
| `Equatable` | numeric, string, bool | `comparable` |
| `Hashable` | numeric, string, bool | `comparable` |

**User-defined constraints:** Any interface can be used as a constraint. The checker
validates via structural subtyping.

### Why Named Capabilities, Not Operation Lists

The Rust approach enumerates every operation needed (`Copy + PartialOrd + Mul<Output=T> + Sub<Output=T> + One`).
Lyric names the capability: `T: Integer`. One constraint names what the type must be.

---

## Visibility

Default **private** (unexported in Go). Use `pub` to make a declaration public:

```lyric
pub func add(x: i32, y: i32) -> i32    // exported
func helper(x: i32) -> i32              // unexported

pub struct Point { x: f64  y: f64 }     // exported
pub class Counter { ... }               // exported
pub enum Color { Red Green Blue }       // exported
```

In Go output, `pub` → uppercase first letter, no `pub` → lowercase.

Fields use `pub` prefix: `pub name: string`.

**Naming conventions follow the implementation language.** A Go project's `.lyric`
files use PascalCase for exported names and camelCase for unexported names.

---

## Functions

### Declarations

All parameters must be named and typed. Return type follows `->`:

```lyric
func add(a: i32, b: i32) -> i32 {
    return a + b
}

pub func public_fn() { ... }

// Generic
func identity<T>(x: T) -> T { return x }
```

`func` is the keyword for both declarations and method definitions. `fn` is for
type syntax only (`fn(i32) -> bool`) and is a contextual keyword.

### External Methods

Methods can be defined outside a class using `func T.method(self)` syntax. This
enables multi-class interface patterns where methods span multiple types:

```lyric
func T.method(self) -> i32 { ... }
func T.mutate(self, x: i32) { ... }
```

The lowerer passes `fn.ReceiverType`; the checker defines `self` in scope.

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

Lambda parameters must have explicit types. Lambdas capture variables from their
enclosing scope. In C backend, captured variables are passed via auto-generated
context structs with capture-by-reference via pointer redirection.

### Mutable Parameters (`mut`)

Structs are value types — passing them to a function copies them. Use `mut` on
both the parameter declaration and call site to pass by mutable reference:

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

Slice elements can also be passed as `mut`, enabling in-place mutation without
extracting the element into a temporary variable:

```lyric
func double_x(mut p: Point) {
    p.x = p.x * 2
}

let mut points = [Point { x: 1, y: 2 }, Point { x: 3, y: 4 }]
double_x(mut points[0])   // mutates the element in-place
assert_eq(points[0].x, 2) // doubled
```

**Rules:**
- `mut` required on **both** parameter declaration and call site (Swift `inout` pattern — prevents accidental mutation)
- Variables and slice element accesses (`slice[i]`) can be passed as `mut`
- For classes (already heap-allocated), `mut` is a semantic no-op
- **C backend:** `mut` params become `T*`, call sites emit `&x` or `&slice.data[i]`, field access uses `->` or `(*p).x`

### Function Annotations (.lyric files)

```lyric
func execute(self, tool: ToolUse) -> (ToolResult, error)
  concurrent: true
  excludes_lock(mu)
  raises: UnknownTool, HandlerPanic
  requires: tool.name != ""
  ensures:  result != null
```

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

---

## Structs (Value Types)

Pure data — named tuples with named fields. Passed by value. No methods, no behavior,
no identity:

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
- Positional construction inside expressions: `Point { 1.0, 2.0 }` (inside parens,
  brackets, or arg lists where `{` is unambiguous)
- Fields can have defaults: `width: i32 = 800`
- Cannot be relation targets (no identity, no heap allocation)

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

**Match as expression:**
```lyric
let prec = match kind {
    OPlus => { 9 }
    _ => { 0 }
}
```

**Enum variant construction:** Positional args only: `Circle(3.14)`. Named args
are not supported for enum variants (use struct literal syntax for structs).

---

## Classes (Heap-Allocated, By Reference)

Classes have identity, behavior, and heap allocation. Fields are declared in the
class body, not as constructor parameters:

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

Classes have deterministic destruction via `.destroy()`. Destructor bodies are
auto-generated from `owns` relations — destroying a parent cascades to children.
`destructor` blocks in interfaces inject cleanup code into concrete classes.

**`final` function**: Classes may declare a `final` function, called immediately
before the auto-generated destructor runs. Use for resource cleanup (file handles,
network connections) that must happen before relation teardown:

```lyric
class Connection {
    fd: i32
    final func cleanup(self) {
        close_fd(self.fd)
    }
}
```

Execution order on `.destroy()`: `final` → auto-destructor (cascade + unlink) → free slot.

### No Inheritance

Lyric does not support classical inheritance. Subtype relationships are expressed
through interface satisfaction. Classes can declare `implements` to signal intent:

```lyric
class Task implements Displayable, Prioritizable {
    // ...
}
```

The `implements` declaration is checked by the compiler — all required methods
must be present. Shared behavior belongs in interfaces or in separate classes
held as dependencies.

---

## Interfaces and Multi-Class Contracts

Interfaces are first-class declarations. They can span multiple type parameters,
defining relationships between types:

```lyric
interface Graph<G, N, E> {
    // Abstract methods bound to type params
    func G.nodes(self) -> [N]
    func N.out_edges(self) -> [E]
    func E.tgt_node(self) -> N

    // Default method (desugared to top-level generic function)
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

    // Destructor injection
    destructor P { ... }
    destructor C { ... }
}
```

### Interface Embedding

```lyric
interface OwningList<P, C> {
    embed DoublyLinked<P, C>    // copies fields and destructors
    destructor P { ... }        // can add/override
}
```

Embed copies only fields and destructors — not methods.

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

### Where Clauses

Generic functions can require interface satisfaction:
```lyric
pub func count<P, C>(p: P) -> i32 where DoublyLinked<P, C> {
    return len(p.children())
}
```

### The `error` Interface

`error` is a built-in interface. Any class with a `message(self) -> string`
method satisfies it via structural subtyping:

```lyric
interface error {
    func error.message(self) -> string
}

class Error {
    msg: string
    pub func message(self) -> string { return self.msg }
}
```

---

## Relations

Relations declare ownership/reference structure between classes using stdlib
interfaces. They trigger field injection, impl binding, and destructor generation.

**Syntax:**
```
relation [Hint] Parent[:parent_label] owns|refs [Child[:child_label]]
```

- **Hint** — stdlib interface (ArrayList, OwningList, RefList, HashedList)
- **Labels** — prefix for injected field names
- **`owns`** — cascade destroy children when parent destroyed
- **`refs`** — unlink children when parent destroyed (no cascade)

### ArrayList — Dynamic Array Ownership

```lyric
relation ArrayList Team:roster owns [Player:team]
```

**Injected fields:** `Team.roster_children: [Player]`, `Player.team_parent: Team?`,
`Player.team_index: i32`.

**Functions:** `array_append<Team, Player>(t, p)`, `array_remove<Team, Player>(p)`.

### OwningList — Doubly-Linked List, Cascade Destroy

```lyric
relation OwningList Team:team owns [Player:player]
```

**Injected fields:** `Team.team_first: Player?`, `Team.team_last: Player?`,
`Player.player_next: Player?`, `Player.player_prev: Player?`,
`Player.player_parent: Team?`.

**Functions:** `dll_append<Team, Player>(t, p)`, `dll_remove<Team, Player>(p)`.

### RefList — Doubly-Linked List, No Cascade

Same fields as OwningList but parent destruction only unlinks, doesn't destroy children.

### HashedList — Hash Table Ownership

```lyric
relation HashedList Registry:reg owns [Entry:entry]
```

Child must implement `hash_key(self) -> u64`. Open-addressing hash table with 75%
load factor rehash and linear probing.

**Functions:** `hash_insert`, `hash_lookup`, `hash_remove`, `hash_init`.

### Generic Type Parameters in Relations

Relations support generic participants:
```lyric
relation HashedList Dict<V>:d owns [DictEntry<V>:d]
```

---

## Variables and Constants

```lyric
let x = 42              // immutable, type inferred
let mut y: i32 = 0      // mutable, type annotated
let ref view = data[5:10]     // immutable view (no copy, shared backing)
let mut ref buf = packet[0:16] // mutable view (write through, no copy)
```

**Copy-on-assign**: Assignment always copies for all value types (primitives, structs,
tuples, slices). `let mut y = x` creates an independent mutable copy of `x`.

**`ref` bindings**: `let ref x = expr` creates a zero-copy view into existing data
instead of copying. The source data must outlive the `ref` binding (enforced by
the no-escape rule). `let mut ref` allows writing through the view — essential for
serialization, cryptography, and zero-copy buffer assembly.

**Binding grammar**: `let [mut] [ref] name [: Type] = expr`

| Binding | Semantics |
|---------|-----------|
| `let x = expr` | Immutable copy |
| `let mut x = expr` | Mutable copy |
| `let ref x = expr` | Immutable view (shared, no copy) |
| `let mut ref x = expr` | Mutable view (write-through, no copy) |

**Parameter passing vs assignment**: Assignment copies; parameter passing shares.
Passing a slice to a function does NOT copy — the function receives a view into
the caller's backing data. This distinction ensures zero-copy performance at
function boundaries while maintaining value semantics for local reasoning.

**Top-level constants** inside `lyric` blocks:
```lyric
lyric parser {
    let PREC_NONE: i32 = 0
    let PREC_OR: i32 = 1
}
```

These compile to `static` globals in C, `const` or `var` in Go.

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

// Conditional pattern match
if let Circle(r) = shape {
    use(r)
} else {
    fallback()
}

// Assertive pattern extract (bindings escape to outer scope)
let Circle(r) = shape else {
    return error
}
use(r)
```

### Block Scoping

Any `{ }` block creates a new scope. Variables declared inside are local to that block:

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

Block scoping works at all pipeline levels: AST (`StmtBlock`), LIR (`LStmtBlock`),
C backend (`{ }`).

### If-Expression

`if/else` can be used as an expression (Rust/Kotlin-style). Both branches must
produce a value of the same type:
```lyric
let result = if cond { a } else { b }
let msg = if count == 1 { "item" } else { "items" }
```

The `else` branch is required when `if` is used as an expression.

### Variant Check: `is`

The `is` operator checks whether an enum value is a specific variant, without
destructuring:
```lyric
if expr.kind is ExprCall {
    // expr.kind is the Call variant
}

// Negation — use ! operator (no `not` keyword)
if !(node is Leaf) { ... }
```

`is` returns `bool`. It does not bind any variables — use `if let` for
destructuring.
```

---

## Type Casts

Postfix `as` syntax:
```lyric
let x: i32 = 42
let y: i64 = x as i64        // widen
let z: i32 = y as i32        // narrow (may truncate)
```

The target can be a type literal or a variable/expression (cast to its type):
```lyric
let template: i64 = 0
let casted = x as template    // cast x to the type of template (i64)
```

Only numeric ↔ numeric casts are supported. All casts are unchecked.

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
- Operand must return `(T, error)`
- Enclosing function must also return `(..., error)`
- Statement-level only
- `?` unwraps the success value: after `let x = foo()?`, `x` is `T` (not `T?`).
  If the error is non-nil, the function returns immediately with the error.

**Implementation:** The lowerer desugars `?` via `hoistNestedTry()` which introduces
SSA temps for nested try expressions.

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

**Go emission:** `any` with `switch val.(type)`.
**C emission:** Tagged unions.

Assignability: any member type assignable TO the union; union assignable FROM
another type only if all variants match.

---

## F-Strings

```lyric
let msg = f"Hello {name}, count={x + 1}"
```

Expressions inside `{ }` are evaluated and converted to strings. Escaped braces: `{{` → literal `{`, `}}` → literal `}`.

### Triple-Quote Strings

Triple-quoted strings (`"""..."""`) preserve newlines and don't require escaping quotes:

```lyric
let sql = """
    SELECT *
    FROM users
    WHERE name = "Alice"
"""
```

---

## Concurrency

```lyric
spawn { ... }                               // goroutine
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
lock(mutex) { ... }                         // scoped mutex
```

**Channel type:** `channel<T>` is the generic channel type. Created via `make_channel<T>()`
(unbuffered) or `make_channel<T>(capacity)` (buffered). Operations use method syntax:
`.send(value)`, `.receive() -> T`, `.close()`.

**Lock type:** `Lock` is the mutex type. Used with the `lock(mu) { ... }` statement
for scoped locking:
```lyric
let mut mu: Lock
lock(mu) {
    // critical section — mu auto-unlocked at block exit
}
```

**C backend:** Channels use pthreads macros (`LYRIC_CHAN_DEF/IMPL`), spawn uses
auto-capture context structs with `pthread_create`, mutex uses `pthread_mutex_t`.

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

**Go backend:** goroutine + channel. `for..in` desugars to poll loop.
**C backend:** Duff's device state machine with `_init/_next/_free`.

The stdlib provides `range(start, end) -> gen i32` for common iteration:
```lyric
for i in range(0, 10) {
    println(i)
}
```

---

## Built-in Functions

| Function | Signature | Description |
|---|---|---|
| `println(...)` | `any... -> unit` | Print with newline |
| `print(...)` | `any... -> unit` | Print without newline |
| `eprintln(...)` | `any... -> unit` | Print to stderr with newline |
| `eprint(...)` | `any... -> unit` | Print to stderr |
| `len(x)` | `[T] \| string -> i32` | Length |
| `append(xs, elem)` | `([T], T) -> [T]` | Append element (returns new slice) |
| `isnull(x)` | `T? -> bool` | Check if optional is null |
| `hash_string(s)` | `string -> u64` | FNV-1a hash |
| `itoa(n)` | `i32 -> string` | Integer to string |
| `atoi(s)` | `string -> (i64, bool)` | String to integer |
| `char_to_string(b)` | `u8 -> string` | Byte to string |
| `assert(cond, msg)` | `(bool, string) -> unit` | Fail test if false (see [Testing](#testing)) |
| `assert_eq(a, b, msg?)` | `(T, T, string?) -> unit` | Fail test if not equal; message optional (see [Testing](#testing)) |
| `make_channel<T>()` | `-> channel<T>` | Create unbuffered channel |
| `make_channel<T>(n)` | `i32 -> channel<T>` | Create buffered channel |

### IO/OS Built-ins

| Function | Signature | Description |
|---|---|---|
| `read_file(path)` | `string -> (string, bool)` | Read file contents |
| `write_file(path, content)` | `(string, string) -> bool` | Write file |
| `os_args()` | `-> [string]` | Command-line arguments |
| `os_exit(code)` | `i32 -> unit` | Exit process |
| `os_getwd()` | `-> string` | Current working directory |
| `exec_command(name, args)` | `(string, [string]) -> (string, bool)` | Run command |
| `path_join(a, b)` | `(string, string) -> string` | Join paths |
| `path_dir(p)` | `string -> string` | Directory of path |
| `path_base(p)` | `string -> string` | Base name of path |
| `path_ext(p)` | `string -> string` | File extension |
| `list_dir(path)` | `string -> ([string], bool)` | List directory entries |
| `file_exists(path)` | `string -> bool` | Check if file exists |
| `mkdtemp()` | `-> string` | Create temporary directory |

### Built-in Methods

**String methods:** `s.len()`, `s.contains(sub)`, `s.has_prefix(pre)`,
`s.has_suffix(suf)`, `s.to_upper()`, `s.to_lower()`, `s.trim()`,
`s.replace(old, new)`, `s.split(sep)`, `s.index_of(sub)`, `s.repeat(n)`.

**List methods:** `xs.len()`, `xs.push(item)`, `xs.pop()`, `xs.contains(item)`,
`xs.reverse()`, `xs.join(sep)`, `xs.extend(other)`.

**Channel methods:** `ch.send(value)`, `ch.receive() -> T`, `ch.close()`.

---

## Testing

Testing is a first-class feature of Lyric, not an afterthought bolted on via libraries. The design is minimal and opinionated: two assertion builtins, a naming convention, and a CLI command. No test frameworks, no assertion libraries, no mock systems.

### Design Rationale

Lyric is a language for writing compilers and systems software, primarily by AI agents. The testing system reflects what those users actually need:

1. **Fast feedback** — write a test, run it, see what broke. No configuration, no build files, no test runner setup.
2. **Runtime verification** — Lyric's type checker has intentional gaps (cross-file resolution produces warnings, not errors). Tests catch what the checker misses.
3. **Minimal ceremony** — a test is just a function. No test classes, no decorators, no registration. If it starts with `test_`, it's a test.

### Test Functions

A test function has no arguments and no return value. Its name starts with `test_`:

```lyric
func test_lexer_basic() {
    let lex = Lexer { source: "let x = 42" }
    let tok = lex.next()
    assert_eq(tok.kind, TLet, "first token should be let")
}
```

Test functions can use all language features — classes, generics, relations, f-strings, error handling. They share a compilation unit with the code they test.

### Assertion Builtins

Two builtins are provided by the compiler, not the standard library. This is intentional: assertions need file and line information that only the compiler can inject.

#### `assert(condition: bool, message: string)`

If `condition` is false, prints the failure message with file and line, then terminates the test:

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

If `actual != expected`, prints the failure message along with both values, then terminates the test. The `message` parameter is optional:

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

**Value display:** `assert_eq` needs to convert values to strings for the "expected/got" output. This works via auto-generated `to_string()` functions:

| Type | Display |
|---|---|
| Enums | Variant name (e.g., `TLet`, `BinAdd`, `TyI32`) |
| `bool` | `true` / `false` |
| `i8`–`i64`, `u8`–`u64` | Decimal number |
| `f32`, `f64` | Decimal with fraction |
| `string` | The string value (quoted in assert output) |
| Structs | Field dump (e.g., `Pos{line: 1, col: 5}`) |
| Classes | Field dump (e.g., `Lexer{source: "...", pos: 0}`) |

Auto-generated enum `to_string()` is the critical piece — most test assertions compare enum variants (token kinds, type kinds, expression kinds). Struct and class `to_string()` dumps all fields, which is invaluable for debugging position mismatches, AST node differences, etc.

### The `lyric test` Command

```
lyric test [files...]
```

`lyric test` compiles all listed `.ly` files together, discovers all `test_*` functions, generates a `main()` that invokes each test with timing and result tracking, compiles with gcc, and runs the binary.

Example:
```
lyric test test_lexer.ly lexer.ly ast.ly
```

Output:
```
PASS  test_lexer_keywords (0.1ms)
PASS  test_lexer_strings (0.2ms)
PASS  test_lexer_numbers (0.1ms)
FAIL  test_lexer_escapes
  assert_eq failed at test_lexer.ly:47
    expected: TStringLit
    got:      TError

4 tests, 3 passed, 1 failed
```

**Execution model:**
- Tests run sequentially in source declaration order
- A failed assertion stops that test function immediately (no partial execution)
- The suite continues — remaining tests still run
- Exit code: 0 if all pass, 1 if any fail

**No test discovery from directories.** You explicitly list files. Future: `lyric test -mod . pkg/...` will gain directory-based discovery using the module system.

### Test File Conventions

- Test files are regular `.ly` files — no special syntax, no annotations
- Name test files `test_*.ly` by convention (not enforced by the compiler)
- Place test files alongside the code they test
- Helper functions used only by tests can live in test files (they're just regular functions)

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

The testing system can grow, but the baseline is intentionally small. Two builtins, one convention, one command.

---

- **`Sym`** — interned symbol wrapping string + hash. Create via `sym("name")` or
  backtick syntax `` `name` `` (desugared to `sym("name")` at parse time).
  Methods: `get_name() -> string`, `get_hash() -> u64`. Hash once, compare by
  integer ("integer war" — avoid repeated string hashing). Implements `Hashable`.
- **`Error`** — standard error class. `message() -> string`.
  Create via `Error { msg: "..." }`.
- **`StringBuilder`** — mutable string builder. `write(s)`, `write_byte(b)`,
  `to_string()`, `len()`. Create via `new_string_builder()`.
- **`Dict<K,V>`** — generic hash table where K implements `Hashable`. Constructor:
  `Dict<K,V>()`. Method API: `.set(key, value)`, `.get(key) -> DictEntry<K,V>?`,
  `.has(key) -> bool`, `.remove(key) -> bool`, `.keys() -> [K]`, `.len() -> i32`.
  Most common instantiation: `Dict<Sym, V>` (string-keyed via Sym).

---

## Memory Model

- **Structs** — stack-allocated, copied by value
- **Classes** — heap-allocated, passed by reference (pointer)
- **Slices `[T]`** — fat pointer (data + len + cap), copied by value but shares
  backing array
- **Relations** — ownership graph; `.destroy()` cascades through `owns` relations
- **No GC** — deterministic destruction via ownership. Ref-counting for unowned
  classes (deferred). C backend uses `malloc`/`free` for class handles.

---

## The .lyric File Layer

`.lyric` files use Lyric syntax with additional design-specific declarations
(`why:`, `doc`, `invariant:`, etc.).

### The `lyric` Block

```lyric
lyric ModuleName {
    // types, functions, relations, impls, constants, annotations
}
```

The `lyric` block wrapper is **optional** in both `.lyric` and `.ly` files.
When present, it provides a logical grouping name. When absent, bare top-level
declarations are valid — the package name comes from the directory name.
`.lyric` files traditionally use the block for clarity; `.ly` files increasingly
use bare declarations.


### `why:` — One-Line Purpose

Attaches to any declaration:
```lyric
class Executor {
    why: "Central dispatch for all tool calls. Single instance per agent."
    active: map[u32]Job  guarded_by(mu)
    mu: lock
}
```

### `doc "Section": """..."""` — Narrative Blocks

Named narrative sections, parsed as first-class content:
```lyric
doc "Architecture": """
    Handlers registered at startup, never changed at runtime.
    Dispatch is O(1) map lookup.
"""

doc "Invariants — Pointer Stability": """
    CRITICAL: Expr is a value type...
"""
```

`doc "Invariants"` blocks are especially important for UDD — they capture
operational contracts that prevent cross-session rediscovery of module behaviors.

### `invariant:` — System-Wide Claims

```lyric
invariant: "every registered name maps to exactly one handler"
invariant: "running job count never exceeds pool size"
    verified_at: "a3f9c12"
```

Cannot be mechanically verified. `verified_at:` stamps flag when a human last
checked the claim against source.

### Thread Safety Annotations

```lyric
field: Type   guarded_by(lockname)

func foo()
    requires_lock(lockname)
    excludes_lock(lockname)
    concurrent: true | false
    spawns:
```

### Source and Fake Links

```lyric
source: ["pkg/tools/executor.go", "pkg/tools/process_manager.go"]
fake:   "pkg/tools/fake_executor.go"
```

### The Three Zones

Each `.lyric` file has three zones:

1. **Human-reviewed zone** — type declarations, doc blocks, invariants, `why:`.
   AI-written, human-reviewed.
2. **Auto-generated function index** — `lyric update` scans source, writes
   function signatures with line numbers as `// func ...` comments. Enables
   surgical source reads via line-number jumps.
3. **Auto-generated dependencies** — `lyric update` writes import/type
   dependencies.

Zone 1 is the understanding. Zones 2 and 3 are mechanical aids maintained by
the `lyric update` tool.

---

## Contextual Keywords

The following keywords are **contextual** — they lex as identifiers and are only
interpreted as keywords in specific parser contexts:

`doc`, `why`, `source`, `fake`, `field`, `lock`, `implements`, `fn`, `from`, `in`,
`as`, `is`

They CAN be used as variable names, field names, or function names:
```lyric
let source = "hello"      // valid — source is just an identifier here
let field = 42             // valid
```

The parser uses targeted lookahead to disambiguate: `lock` is only a keyword when
followed by `(`, `field` only in annotation context, etc. `parsePrimaryExpr` has
NO keyword rewrite — expressions always see identifiers.

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

The `lyric` wrapper is **optional**. When absent, the parser creates an implicit
block with a name derived from the filename. The package name always comes from
the directory name (see Modules and Packages), not from the block name.

**Newlines**: Newlines are statement terminators. Inside `()` and `[]` brackets, newlines are treated as whitespace, enabling multi-line function calls, list literals, and tuple expressions. `{}` braces do NOT suppress newlines (they delimit statement blocks).

### Multi-File Compilation

```
lyric compile --c file1.ly file2.ly ...
```

Multiple `.ly` files in the same package (directory) are merged into a single
compilation unit via `MergeFiles()`. When compiling a module, all imported packages
are resolved recursively, merged with namespace prefixing, and compiled together.
The checker uses three-phase processing: pre-register type names (phase 0),
register signatures (phase 1), then check bodies (phase 2). This ensures
cross-file and cross-package type references resolve correctly.

### Compilation Pipeline

```
Parse → ResolveImports → MergeStdlib → DesugarAll → Check → Lower → Optimize → Monomorphize → Emit C
```

**Desugar order** (MUST run in this sequence):
1. InterfaceEmbeds (flatten embedded interfaces)
2. InterfaceFields (field → getter/setter methods)
3. Relations (inject fields + impl blocks)
4. Destructors (generate destroy methods from owns relations)
5. DefaultImpls (extract interface methods to top-level functions)

### Go Backend

Deleted (commit 8221e5a). The C backend is the sole backend. The Go backend was
used during initial development and retired when the bootstrap compiler became
self-hosting.

### C Backend

Requires monomorphized LIR (C has no generics). Outputs `.c` files using
`lyric_runtime.h`. Compile with `gcc -std=gnu11 -I runtime`.

**Monomorphization** is LIR→LIR: specializes generic functions/classes for each
concrete type instantiation. Iterative — after specializing, re-collects from
specialized bodies for transitive instantiations.

### Toolchain Commands

| Command | Description |
|---|---|
| `lyric compile file.ly ... -o out` | Compile files to C and binary |
| `lyric compile -mod . -o out` | Compile entire module |
| `lyric verify file.lyric` | Check .lyric against source |
| `lyric update file.lyric` | Refresh function index and deps |
| `lyric fmt file.ly` | Format source (comment-preserving) |
| `lyric gen pkg/path/` | Scaffold .lyric from Go packages |
| `lyric test file.ly ...` | Compile, discover `test_*` functions, run tests |

---

## Known Gotchas

- **Enum construction is positional only** — `Variant(a, b)`, not `Variant(x: a)`.
- **Struct literal ambiguity** — `Ident {` is ambiguous between struct literal
  and variable + block in statement context. Parser uses `exprDepth` counter:
  inside parens/brackets/arg lists (`exprDepth > 0`), always struct literal.
  At statement level, uses `isStructLitAhead()` lookahead. Additionally,
  `for`/`while`/`if`/`match` use a `noStructLit` flag to suppress struct literal
  parsing in conditions (Rust approach).
- **`append` vs `array_append`** — `append(slice, item)` or `slice.push(item)`
  for plain slices; `array_append<P,C>(parent, child)` for relation-owned lists.
- **`nil` and `null` are synonyms** — both accepted as the null literal. Convention:
  use `null` in new code.
- **Number literal underscores** — `1_000_000` is valid; underscores are silently
  stripped by the lexer.
- **Platform `int`/`uint`** — only for Go interop, not part of numeric tower.
- **`lyric fmt` bug** — keywords inside string literals tokenized as keywords.
  Fix planned.

---

## What Lyric Is Not

**`.lyric` files** are not a programming language. They contain no executable code.
A `.lyric` file is a structured design artifact — compressed, checkable understanding
consumed primarily by AI, verified by a static tool against implementations in any
language.

**`.ly` files** are a programming language. They compile, run, and are type-checked.

Neither mode is:
- **UML** — UML is for human visual parsing; Lyric is for AI context and static verification
- **A schema language** — Protobuf describes serialization; Lyric describes system design
- **Comments** — comments explain code; Lyric files describe the *system* above any implementation
- **Documentation** — documentation decays without enforcement; Lyric files are verified
  at commit time and the AI has skin in the game keeping them accurate

The closest analog is a typed IDL extended with design documentation, thread safety,
invariants, and ownership — where the primary consumer is an AI working with the code.

---

*Lyric is what a codebase would tell you if it could talk.*

---

## Why Lyric — The Performance and Safety Story

Lyric is designed to become the world's fastest language for memory-intensive
applications — which in a data center is most of them — while simultaneously
being the most memory-safe language that doesn't use garbage collection.

### Relations Eliminate Manual Destructors

In C++, manual destructors are a primary source of memory safety bugs: use-after-free,
double-free, dangling pointers, and memory leaks. Rust addresses this with borrow
checking and lifetimes, but at enormous cognitive cost — engineers spend significant
time fighting the borrow checker.

Lyric takes a different approach: **relations declare ownership, and the compiler
generates all destructors automatically.**

```lyric
relation ArrayList Team:roster owns [Player:team]
```

This single line:
- Injects `children`, `parent`, and `index` fields into both classes
- Generates `array_append` and `array_remove` functions
- Generates cascade destructor: destroying a Team destroys all its Players
- Generates child destructor: destroying a Player removes it from its Team

No manual destructor code. No forgetting to clean up. No use-after-free from
stale pointers — the relation system manages the back-pointers.

The `owns` vs `refs` distinction makes lifetime semantics explicit:
- `owns` — cascade: parent death kills children
- `refs` — unlink: parent death detaches children (they survive)

This is what Rune and DataDraw proved over decades: **if you declare the ownership
graph, the compiler can manage memory perfectly without GC.**

### SoA / AoS: Cache-Optimal Memory Layout

Most languages store objects as structs-of-arrays (SoA) only when the programmer
manually reorganizes their data. Lyric's relation system knows the ownership graph
and can automatically choose between:

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

When iterating over all player scores (the common case in data-intensive workloads),
SoA keeps scores contiguous in cache. AoS wastes cache lines loading name and
team_ptr data that isn't needed.

Lyric's C backend can use relations to generate SoA layout: each relation field
becomes a separate array. Getters and setters index into the correct array. The
class handle becomes a 32-bit index, not a 64-bit pointer — halving pointer storage.

**This is the DataDraw insight that Bill proved at scale:** relation-based code
generation with SoA layout produced 10x performance improvements in EDA tools
processing billions of objects. Lyric brings this to a general-purpose language.

### Multi-Class Interfaces: Expressiveness Without Inheritance

Most languages force a choice: either graph algorithms know about your concrete
types (not reusable) or they use heavyweight inheritance/visitor patterns (verbose
and fragile).

Lyric's multi-class interfaces let you write graph algorithms ONCE and bind them
to ANY concrete types via impl blocks:

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
inheritance, no visitor pattern, no type erasure. The monomorphizer generates
specialized code for each concrete binding.

### No GC, No Borrow Checker, No Garbage

Lyric's memory model:
- **Slab allocation** for classes → 32-bit index handles, cache-friendly, no malloc/free
- **Relations** declare ownership → compiler generates destructors
- **Cascade deletion** through `owns` relations → no leaks
- **Back-pointers** maintained automatically → no dangling references
- **Ref counting** for non-owned classes → automatic deallocation when last ref dies
- **`destroys` annotation** → compiler infers which functions may destroy instances, statically prevents use-after-free
- **`mut resize` annotation** → compiler prevents accessing array elements during resize
- **Copy-on-assign** for value types → no aliasing surprises for local variables
- **`ref` bindings** for zero-copy views → opt-in sharing when performance demands it
- **`trusted` blocks** → manual ref/unref for stdlib containers that manage their own memory
- **Safe iterators** → destroy-during-iteration without ConcurrentModificationException
- **Deterministic destruction** → predictable latency (no GC pauses)
- **No borrow checker** → no lifetime annotations, no fighting the compiler

The cost: you must declare your ownership graph via relations. But you were going
to design that ownership graph anyway — Lyric just makes it explicit and verifiable
rather than implicit and error-prone.

### The Result

A language that is:
1. **Faster than C++** for memory-intensive applications (SoA layout, 32-bit handles)
2. **Safer than C++** (no manual destructors, no use-after-free)
3. **More expressive than Rust** (multi-class interfaces, no borrow checker friction)
4. **Simpler than both** (relations replace pages of boilerplate)

---

## Standard Library Reference

The stdlib (`stdlib/std.ly` and `stdlib/string.ly`) is auto-imported into all
Lyric programs. It provides ownership data structures, hash tables, string
utilities, and error handling.

### ArrayList<P, C> — Dynamic Array Ownership

A parent P owns a dynamic array of children C. Children know their parent and
index for O(1) swap-remove.

**Injected fields:**
- `P.children: [C]` — the parent's array of children
- `C.parent: P?` — child's back-reference to parent
- `C.index: i32` — child's position in the array

**Functions:**
| Function | Description |
|---|---|
| `array_append(parent: P, child: C)` | Append child to end of parent's array |
| `array_remove(child: C)` | Remove child using O(1) swap-remove |

**Destructors:**
- Parent: cascade-destroys all children (reverse order)
- Child: removes self from parent's array

**Usage:**
```lyric
class Team { name: string }
class Player { name: string }

relation ArrayList Team:roster owns [Player:team]

let t = Team { name: "Eagles" }
let p = Player { name: "Alice" }
array_append<Team, Player>(t, p)
// p.team_parent == t, p.team_index == 0
// t.roster_children == [p]
```

### DoublyLinked<P, C> — Intrusive Doubly-Linked List (Base)

Base interface providing fields and traversal. No destruction semantics — use
`OwningList` or `RefList` which embed this.

**Injected fields:**
- `P.first: C?`, `P.last: C?` — list head/tail
- `C.next: C?`, `C.prev: C?` — sibling links
- `C.parent: P?` — back-reference

**Functions:**
| Function | Description |
|---|---|
| `dll_append(parent: P, child: C)` | Append child to end of list |
| `dll_remove(child: C)` | Remove child from list, relink siblings |

### OwningList<P, C> — Doubly-Linked List, Cascade Destroy

Embeds `DoublyLinked<P, C>`. Parent death cascade-destroys all children.

**Usage:**
```lyric
relation OwningList Document:doc owns [Paragraph:para]
```

### RefList<P, C> — Doubly-Linked List, No Cascade

Embeds `DoublyLinked<P, C>`. Parent death unlinks children but does NOT destroy
them — children survive independently.

**Usage:**
```lyric
relation RefList Room:room refs [Guest:guest]
```

### HashedList<P, C> — Hash Table Ownership

Open-addressing hash table with linear probing. 75% load factor triggers rehash
(double capacity). Children stored in dense array; parallel bucket index maps
hash slots to array positions.

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

### Dict<K,V> — Generic Hash Table

Generic hash table where `K: Hashable`. Built on HashedList with configurable key types.
The most common instantiation is `Dict<Sym,V>` (string-keyed via Sym).

**Constructor:** `Dict<K,V>()` — creates an empty dictionary.

**Methods:**
| Method | Return | Description |
|---|---|---|
| `set(key, val)` | `unit` | Set key-value pair (replaces if exists) |
| `get(key)` | `DictEntry<K,V>?` | Get entry by key (null if missing) |
| `has(key)` | `bool` | Check if key exists |
| `remove(key)` | `bool` | Remove by key |
| `keys()` | `[K]` | All keys |
| `len()` | `i32` | Number of entries |

**DictEntry<K,V> fields:** `key: K`, `value: V`.

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

### Hashable — Hash Key Interface

Interface for types used as hash table keys. Requires a single method:

```lyric
interface Hashable {
    func Hashable.get_hash(self) -> u64
}
```

`Sym` implements `Hashable`. `string` does NOT — this is deliberate. Wrapping strings
in `sym()` enforces hash-once discipline, preventing repeated FNV-1a computation
on the same string value (a common performance bug in compilers).

### Sym — Interned Symbol

Wraps a string with a pre-computed FNV-1a hash. Hash is computed once at creation;
all subsequent operations use the u64 hash for O(1) comparison. This is the
"integer war" principle: avoid repeated string hashing in hot paths.

**Construction:** `sym("name")` or backtick syntax `` `name` `` (desugars to `sym("name")` at parse time).

**Methods:**
| Method | Return | Description |
|---|---|---|
| `get_name(self)` | `string` | Original string |
| `get_hash(self)` | `u64` | Pre-computed FNV-1a hash |

Sym implements the `Hashable` interface, which requires `get_hash(self) -> u64`.

**Usage:**
```lyric
let s = sym("identifier")
let s2 = `identifier`        // equivalent — backtick syntax
let h = s.get_hash()         // u64, computed once
let n = s.get_name()         // "identifier"
```

**Design note:** `string` does NOT implement `Hashable`. Use `sym()` wrapping to enforce hash-once discipline — repeated hashing of the same string is a common performance bug. Dict requires `K: Hashable`, so keys must be `Sym`, not `string`.

### Error Handling

**`error` interface:** Any class with `message(self) -> string` satisfies it.

**`Error` class:** Default concrete implementation.
```lyric
let e = Error { msg: "something went wrong" }
println(e.message())  // "something went wrong"
```

**Custom errors:** Just implement `message`:
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
| `str_trim(s)` | `string -> string` | Trim whitespace both ends |
| `str_trim_left(s)` | `string -> string` | Trim whitespace left |
| `str_trim_right(s)` | `string -> string` | Trim whitespace right |
| `str_to_upper(s)` | `string -> string` | Uppercase (ASCII only) |
| `str_to_lower(s)` | `string -> string` | Lowercase (ASCII only) |
| `str_replace(s, old, new)` | `(string, string, string) -> string` | Replace all occurrences |
| `str_repeat(s, n)` | `(string, i32) -> string` | Repeat n times |
| `str_join(parts, sep)` | `([string], string) -> string` | Join with separator |

---

## I/O Library — Current and Planned

### Current Built-ins (Minimal Bootstrap I/O)

The following are built-in functions, not stdlib — they're implemented directly
in the lowerer and backends:

| Function | Description |
|---|---|
| `read_file(path) -> (string, bool)` | Read entire file as string |
| `write_file(path, content) -> bool` | Write string to file |
| `os_args() -> [string]` | Command-line arguments |
| `os_exit(code: i32)` | Exit process |
| `os_getwd() -> string` | Current working directory |
| `exec_command(name, args) -> (string, bool)` | Run external command |
| `path_join(a, b) -> string` | Join path components |
| `path_dir(p) -> string` | Directory portion |
| `path_base(p) -> string` | Base filename |
| `path_ext(p) -> string` | File extension |

These are sufficient for the bootstrap compiler. They read/write entire files
as strings — no streaming, no file handles, no buffering.

### Planned I/O Library (Post-Bootstrap)

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
    // OS file descriptor
    pub func open(path: string, mode: string) -> (File, error)
    pub func create(path: string) -> (File, error)
}

class BufferedReader {
    // Wraps a Reader with read-ahead buffering
    pub func read_line(self) -> (string, error)
    pub func read_all(self) -> (string, error)
}

class BufferedWriter {
    // Wraps a Writer with write buffering
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

The I/O library should follow the Reader/Writer interface pattern (proven by Go)
where everything that reads or writes bytes satisfies the same interface, enabling
composition: `BufferedReader(File.open("x.txt")?)`.

**Design principle:** Unix-only for bootstrap. Cross-platform abstraction is a
post-1.0 concern.
