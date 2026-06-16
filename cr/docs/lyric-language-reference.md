# Lyric Language Reference

Concise reference for writing Lyric code. Based on parser, checker, stdlib, and test files as of 2026-06-16.

## Bootstrap Philosophy

**The #1 goal of the bootstrap is to make Lyric as awesome as Go — maybe even better — for writing compilers.** The bootstrap compiler is being ported from Go to Lyric. Every time we hit jank, friction, or missing features, we fix the compiler rather than work around the issue. The bootstrap process is the design feedback loop that makes Lyric great.

**Key principles:**
- Don't work around language issues — fix them in the compiler
- If something feels janky, that's a signal to improve the language
- The bootstrap .ly files (ast.ly, lexer.ly, parser.ly, etc.) are the primary test of language ergonomics
- Target: C backend via monomorphization (Go backend deleted)

## Modules and Packages

### Package = Directory

A **package** is a directory of `.ly` files. All `.ly` files in a directory belong to the same package. The package name is the **directory name** (not the filename).

```
myproject/
  lyric.mod              # module root
  main.ly                # package "myproject" (or "main" if entry point)
  ast/
    ast.ly               # package "ast"
    expr.ly              # package "ast" (same directory)
  parser/
    parser.ly            # package "parser"
    expr_parser.ly       # package "parser"
```

All `.ly` files within a package are merged into a single compilation unit — declaration order across files doesn't matter.

### Module = Project

A **module** is a project rooted at a `lyric.mod` file. A module defines the import path prefix and is the unit of compilation — it produces either a program binary or a shared library.

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
  let node = ast.new_node("expr")
  print(node.name)
}
```

The package name is both the import identifier and the directory name. Access is always qualified: `ast.Node`, `parser.parse()`.

**Visibility**: Only `pub` declarations are accessible through an import. Non-pub declarations are package-private.

```lyric
// ast/ast.ly
pub struct Node { name: string }     // visible to importers
func helper() -> i32 { return 42 }  // package-private
```

**Cycle detection**: Circular imports are a compile error.

**Nested packages**: For packages in subdirectories, use a path with an alias:
```lyric
import v2 from "parser/v2"
```

### The `lyric` Block (Optional)

```lyric
lyric BlockName {
  // types, functions, relations, impls, constants
}
```

The `lyric` wrapper is optional. Bare `.ly` files with top-level declarations are valid — the package name comes from the directory, not the block name or filename:

```lyric
// ast/expr.ly — no wrapper needed, package is "ast"
enum Color { Red, Green, Blue }
func greet(name: string) -> string { return f"Hello {name}" }
```

When a `lyric` block is present, its name is used for C symbol prefixing but does not affect the package name.

### Compilation Model

Lyric compiles an entire module at once — all packages are resolved, merged with namespace prefixing, and emitted as a single C file compiled to one binary. There is no separate compilation of individual files or packages.

```bash
lyric compile .                       # compile module in current directory
lyric compile ~/projects/mycompiler/  # compile module at path
lyric compile main.ly -o myprogram    # single-file, no module needed
lyric compile main.ly ast.ly          # multi-file, no module needed
```

When given a directory, the compiler looks for `lyric.mod`, finds `main()` in the root package, and recursively resolves all imports. When given a `.ly` file, it checks parent directories for `lyric.mod` — if found, uses module mode; otherwise, single-file mode.

The stdlib (`stdlib/std.ly`, `stdlib/string.ly`, etc.) is auto-imported into all packages.


## Newlines and Multi-line Expressions

Newlines are statement terminators. However, **inside `()` and `[]` brackets, newlines are treated as whitespace**, enabling multi-line expressions:

```lyric
let result = add(
    first_arg,
    second_arg,
    third_arg
)

let xs: [i32] = [
    10, 20, 30,
    40, 50, 60
]
```

Note: `{}` braces do NOT suppress newlines (they delimit blocks with statements).

## Primitives

`bool`, `u8`, `u16`, `u32`, `u64`, `u128`, `u256`, `i8`, `i16`, `i32`, `i64`, `i128`, `i256`, `f32`, `f64`, `f128`, `string`, `unit`, `any`

- Default integer literal: `i32`. Cast with `x as u64`.
- Implicit numeric widening: smaller types widen to larger in binary ops (e.g. `i32 + i64` → `i64`).
- `unit` — void/no-value type (for functions with no return value).
- `int`/`uint` — platform-width, Go interop only. NOT part of the Lyric numeric tower.
- `any` — stdlib union alias / `void*`.
- Character literals: `'A'` → `u8` (65). Escapes: `\n \r \t \\ \' \" \0 \x##`.
- Both `null` and `null` accepted by the lexer (mapped to the same token).

## Type Expressions

```
T?                  // optional (nullable)
T | U               // union type (exhaustively typed)
[T]                 // slice (fat pointer: data + len + cap)
(T1, T2)            // tuple (access via ._0, ._1)
fn(T1, T2) -> R     // function type
channel<T>          // channel
```

**No built-in `map[K]V`** — use `Dict<K,V>` from stdlib (see [Stdlib Classes](#stdlib-classes)).

## Slices

Slices are dynamic arrays: a fat pointer with `data`, `len`, and `cap`. Type syntax: `[T]`.

```lyric
let xs: [i32] = [1, 2, 3]
let empty: [string] = []
```

**Indexing and slicing:**
```lyric
xs[0]               // element access (0-indexed)
xs[1:3]             // sub-slice [low:high) — shares underlying data
```

**Concatenation:**
```lyric
let combined = xs + [4, 5, 6]   // returns new slice
xs.extend(other)                 // append other's elements in-place
```

**Builtin functions:**
```lyric
len(xs)             // length
append(xs, 42)      // returns new slice with element appended
```

**Methods:**
```lyric
xs.push(elem)       // append in-place (mutates xs)
xs.pop()             // remove and return last element
xs.len()             // same as len(xs)
xs.contains(elem)    // linear search, returns bool
xs.index_of(elem)    // returns i32 index, -1 if not found
xs.first()           // returns T? (null if empty)
xs.last()            // returns T? (null if empty)
xs.is_empty()        // returns bool
xs.sort()            // in-place sort
xs.reverse()         // in-place reverse
xs.remove(elem)      // remove first occurrence
xs.join(sep)         // join string slice with separator → string
xs.extend(other)     // append all elements of other in-place
```

**Mutating methods** (`.push`, `.pop`, `.sort`, `.reverse`, `.remove`, `.clear`, `.extend`) modify the receiver in-place. When called on a class field (`obj.items.push(x)`), the compiler automatically uses a reference to avoid the copy-then-discard problem.

## Strings

Strings are `[u8]` byte slices. String literals create `lyric_string` values (length-prefixed, with hidden trailing `\0` for C interop).

```lyric
let s = "hello"
let ch = s[0]          // u8 (104)
let sub = s[1:4]       // "ell" — sub-slice, shares data
```

**Concatenation:**
```lyric
let s = "hello" + " world"   // returns new string
push_bytes(dst, src)          // append src bytes to dst in-place
```

**Methods:**
```lyric
s.len()              // byte length (i32)
s.contains(needle)   // bool
s.has_prefix(p)      // bool
s.has_suffix(p)      // bool
s.index_of(needle)   // i32 (-1 if not found)
s.is_empty()         // bool
s.trim()             // strip whitespace from both ends → new string
s.to_lower()         // → new string
s.to_upper()         // → new string
s.replace(old, new)  // replace all occurrences → new string
s.repeat(n)          // repeat n times → new string
s.split(sep)         // → [string]
s.char_at(i)         // → string (single-byte string)
```

**Stdlib** (`string.ly`): `str_concat(a, b)`, `str_split_n(s, sep, n)`, `str_trim_left(s)`, `str_trim_right(s)`, `str_replace(s, old, new)`, `str_repeat(s, n)`, `str_join(parts, sep)`, `str_has_prefix(s, p)`, `str_has_suffix(s, p)`, `str_index_of(s, needle)`, `str_to_upper(s)`, `str_to_lower(s)`.

## Structs (Value Types)

```lyric
struct Point {
  x: f64
  y: f64
}
```

- Copied by value. No methods (use classes for methods).
- Direct field access: `p.x`.
- Fields can have defaults: `width: i32 = 800`.
- **Generic structs are supported**: `struct Pair<T> { first: T, second: T }`.
- Positional construction inside expressions: `Point { 1.0, 2.0 }` (inside parens, brackets, or arg lists where `{` is unambiguous).
- Cannot be relation targets (no identity, no heap allocation).

## Classes (Heap-Allocated, By Reference)

```lyric
class Counter {
    count: i32
    items: [string]

    func increment(self) {
        self.count = self.count + 1
    }

    func get(self) -> i32 {
        return self.count
    }
}

// Struct-literal construction
let c = Counter { count: 10 }
c.increment()
```

- Fields declared in body. Unset fields zero-initialized.
- Fields can have defaults: `count: i32 = 0`.
- `pub` prefix for public fields: `pub name: string`.
- Construction uses struct-literal syntax: `ClassName { field: value }`.
- `self` for receiver. Direct field access: `self.count`.
- Generic: `class Pair<T> { first: T  second: T }`
- `.destroy()` — deterministic destruction (from relation destructors).

### Explicit Constructors

```lyric
class HttpClient {
    url: string
    timeout: u32 = 30
    pool: ConnectionPool?

    func HttpClient(self, base_url: string, timeout: u32) {
        self.url = base_url
        self.timeout = timeout
        self.pool = ConnectionPool { base_url: base_url }
    }
}

// Call syntax when explicit constructor exists
let client = HttpClient("http://api.com", 60)
```

Without an explicit constructor, only struct-literal syntax is allowed.

### `final` Function

Classes may declare a `final` function, called immediately before the auto-generated destructor:

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

Lyric does not support classical inheritance. Classes can declare `implements` to signal interface satisfaction (compiler-checked):

```lyric
class Task implements Displayable, Prioritizable { ... }
```

Shared behavior belongs in interfaces or in separate classes held as dependencies.

## Enums

```lyric
// Simple (no associated data — emits C typedef enum)
enum Color { Red Green Blue }

// With associated data (tagged union)
enum Shape {
  Circle(radius: f64)
  Rect(w: f64, h: f64)
  Point
}

// Qualified access
let c = Color.Red
```

Match on enums:
```lyric
match shape {
  Circle(r) => { println(f"radius: {r}") }
  Rect(w, h) => { println(f"{w}x{h}") }
  Point => { println("point") }
}
```

### Multi-pattern match arms

Multiple patterns per arm separated by `|`:
```lyric
match kind {
  OPlus | OMinus => { PREC_ADDITIVE }
  OStar | OSlash | OPercent => { PREC_MULT }
  OEqEq | OBangEq => { PREC_EQUALITY }
  _ => { PREC_NONE }
}
```

This works for simple variants, variants with bindings (if all alternatives bind the same names), and wildcard `_`.

### Match guards

```lyric
match token {
  Ident(name) if name == "self" => { handle_self() }
  Ident(name) => { handle_ident(name) }
  _ => { handle_other() }
}
```

### Match as expression

`match` can be used as an expression (returns a value):
```lyric
let prec = match kind {
  OPlus => { 9 }
  OStar => { 10 }
  _ => { 0 }
}
```

### Match on non-enum values

Match works on enum types. For non-enum dispatch, use `if`/`else if` chains.

### `if let` — conditional pattern match

Extract a single variant without a full `match`:

```lyric
if let Circle(r) = shape {
    println(f"radius: {r}")
} else {
    println("not a circle")
}
```

Bindings (`r`) are scoped to the then-block. The `else` branch is optional.

### `let..else` — assertive pattern extract

Extract variant data into the surrounding scope, with a mandatory diverging `else`:

```lyric
let Circle(r) = shape else {
    return -1.0
}
// r is now in scope
println(f"radius: {r}")
```

The `else` block must diverge (`return`, `break`, `continue`). Bindings escape into the outer scope — this avoids rightward drift for the common "extract or bail" pattern.

## Type Aliases

```lyric
type Name = string
type Callback = fn(i32) -> bool
```

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

Any member type is assignable to the union. C backend emits tagged unions; Go backend emits `any` with `switch val.(type)`.

## Variables & Constants

```lyric
let x = 42              // immutable
let mut y: i32 = 0      // mutable, typed
let ref view = data[5:10]     // immutable view (no copy, shared backing)
let mut ref buf = packet[0:16] // mutable view (write through, no copy)
```

**Copy-on-assign**: Assignment always copies for value types (primitives, structs, tuples, slices). `let mut y = x` creates an independent mutable copy.

**`ref` bindings**: `let ref` creates a zero-copy view into existing data instead of copying. Useful for parsing, serialization, and crypto. The source must outlive the `ref` binding. `let mut ref` allows writing through the view.

**Binding grammar**: `let [mut] [ref] name [: Type] = expr`

**Top-level constants** can be declared directly inside `lyric` blocks:

```lyric
lyric parser {
  let PREC_NONE: i32 = 0
  let PREC_OR: i32 = 1
  // ...
}
```

These compile to `static` globals in C.

## Control Flow

```lyric
if cond { ... } else if cond2 { ... } else { ... }

// if-expression (both branches required, same type)
let result = if cond { a } else { b }

while cond { ... }

for item in collection { ... }
for item, idx in collection { ... }

match expr {
  Pattern => ...
}
```

### Variant Check: `is`

```lyric
if expr.kind is ExprCall { ... }
if !(node is Leaf) { ... }
```

Returns `bool`. Does not bind variables — use `if let` for destructuring.

### Type Casts: `as`

Postfix `as` for numeric conversions:
```lyric
let x: i32 = 42
let y: i64 = x as i64        // widen
let z: i32 = y as i32        // narrow (may truncate)
```

Only numeric ↔ numeric casts are supported. All casts are unchecked.

### Block Scoping

Lyric has block-level scoping. Any `{ }` block creates a new scope — variables declared inside are local to that block:

```lyric
func example() {
    let x = 1
    {
        let x = 2       // shadows outer x, legal
        println(x)      // prints 2
    }
    println(x)          // prints 1
}
```

## Functions

```lyric
func add(a: i32, b: i32) -> i32 { return a + b }

pub func public_fn() { ... }

// Generic
func identity<T>(x: T) -> T { return x }

// External method (multi-class interface pattern)
func T.method(self) -> i32 { ... }

// Lambdas (two syntaxes)
let f = (x: i32) -> i32 { return x * 2 }       // paren-lambda
let g = |x: i32| -> i32 { return x * 2 }       // pipe-lambda
```

`func` is the function declaration keyword. `fn` is for type syntax only (e.g., `fn(i32) -> bool`) and is a contextual keyword — it can be used as a variable name. `func` and `fn` are interchangeable in declarations.

### Mutable Parameters (`mut`)

Structs are value types — passing them to a function copies them.
Use `mut` on both the parameter declaration and call site to pass by mutable reference:

```lyric
struct Point { x: i32, y: i32 }

func translate(mut p: Point, dx: i32, dy: i32) {
    p.x = p.x + dx   // modifies caller's copy
    p.y = p.y + dy
}

let mut pt = Point { x: 10, y: 20 }
translate(mut pt, 5, 3)
assert_eq(pt.x, 15)   // mutation visible to caller
```

Slice elements can also be passed as `mut`, enabling in-place mutation:

```lyric
func double_x(mut p: Point) {
    p.x = p.x * 2
}

let mut points = [Point { x: 1, y: 2 }, Point { x: 3, y: 4 }]
double_x(mut points[0])   // mutates the element in-place
assert_eq(points[0].x, 2) // doubled
assert_eq(points[1].x, 3) // unchanged
```

Rules:
- `mut` required on both parameter and call site (prevents accidental mutation)
- Variables and slice element accesses can be passed as `mut`
- For classes (already heap-allocated), `mut` is unnecessary — they're already pointers
- Don't use `mut` on class params — creates double-pointer segfaults
- In C backend: `mut` params become `T*`, call sites emit `&x` or `&slice.data[i]`

## Try Operator and Error Handling

Lyric uses `(T, error)` tuples for error handling, similar to Go.

### The `error` interface

`error` is a built-in interface declared in stdlib:

```lyric
interface error {
  func error.message(self) -> string
}
```

Any class with a `message(self) -> string` method satisfies `error` via structural subtyping. The stdlib provides a default concrete implementation:

```lyric
class Error {
    msg: string
    pub func message(self) -> string { return self.msg }
}
```

### Custom error types

```lyric
class ParseError {
    msg: string
    span: Span
    source_line: string

    pub func message(self) -> string {
        return f"{self.span.start.line}: {self.msg}"
    }
}
```

Any class with `message(self) -> string` can be used where `error` is expected.

### The `?` operator

The `?` operator propagates errors from `(T, error)` returns:

```lyric
func load() -> (string, error) {
  let data = read_file("x.txt")?    // propagates error on failure, data is string
  return (data, null)
}
```

`?` unwraps the success value: after `let x = foo()?`, `x` is `T` (not `T?`). If the error is non-null, the function returns immediately with the error.

Statement-level only. Containing function must return `(T, error)`.

## F-strings

```lyric
let msg = f"Hello {name}, count={x + 1}"
```

Escaped braces: `{{` → literal `{`, `}}` → literal `}`.

### Triple-quote Strings

Triple-quoted strings preserve newlines and don't require escaping quotes:

```lyric
let sql = """
    SELECT *
    FROM users
    WHERE name = "Alice"
"""
```

## Multi-Class Interfaces

Interfaces span multiple type parameters, defining relationships between types.

```lyric
interface Graph<G, N, E> {
  // Abstract methods bound to type params
  func G.nodes(self) -> [N]
  func N.out_edges(self) -> [E]
  func E.tgt_node(self) -> N

  // Default method
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
  embed DoublyLinked<P, C>    // copies fields and destructors (not methods)
  destructor P { ... }        // can add/override
}
```

### Impl Blocks

```lyric
// Alias: wire interface method → class method
impl Graph<SimpleGraph, SimpleNode, SimpleEdge> {
  G.nodes = SimpleGraph.get_nodes
  N.out_edges = SimpleNode.get_edges
  E.tgt_node = SimpleEdge.get_target
}

// Field bind: wire interface field → concrete field
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

```lyric
pub func count<P, C>(p: P) -> i32 where DoublyLinked<P, C> {
  return len(p.children())
}
```

## Relations

Relations declare ownership between classes using stdlib interfaces. They trigger field injection and auto-generate impl bindings.

**Syntax:**
```
relation [Hint] Parent[:parent_label] owns|refs [Child[:child_label]]
```

- **Hint** — stdlib interface name (ArrayList, OwningList, RefList, HashedList)
- **Labels** — prefix for injected field names. Parent label prefixes parent's fields, child label prefixes child's fields.
- **owns** — cascade destroy children when parent destroyed
- **refs** — unlink children when parent destroyed (no cascade)

### ArrayList — dynamic array ownership

```lyric
class Team {
    name: string
}
class Player {
    name: string
}

relation ArrayList Team:roster owns [Player:team]

impl ArrayList<Team, Player> {
  P.children <-> Team.roster_children
}
```

Injected fields: `Team.roster_children: [Player]`, `Player.team_parent: Team?`, `Player.team_index: i32`.

Functions: `array_append<Team, Player>(t, p)`, `array_remove<Team, Player>(p)`.

### OwningList — doubly-linked list, cascade destroy

```lyric
relation OwningList Team:team owns [Player:player]
```

Injected fields: `Team.team_first: Player?`, `Team.team_last: Player?`, `Player.player_next: Player?`, `Player.player_prev: Player?`, `Player.player_parent: Team?`.

Functions: `dll_append<Team, Player>(t, p)`, `dll_remove<Team, Player>(p)`.

### RefList — doubly-linked list, no cascade

```lyric
relation RefList Room:room refs [Guest:guest]
```

Same fields as OwningList but parent destruction only unlinks, doesn't destroy children.

### HashedList — hash table ownership

```lyric
class Entry {
    key: u64
    value: i32

    pub func hash_key(self) -> u64 { return self.key }
}

relation HashedList Registry:reg owns [Entry:entry]

impl HashedList<Registry, Entry> {
  P.children <-> Registry.reg_children
  P.buckets <-> Registry.reg_buckets
  P.hash_cap <-> Registry.reg_hash_cap
  P.hash_count <-> Registry.reg_hash_count
  C.parent <-> Entry.entry_parent
  C.index <-> Entry.entry_index
}
```

Child must implement `hash_key(self) -> u64`. Functions: `hash_insert`, `hash_lookup`, `hash_remove`, `hash_init`.

## Builtins

**Core:**
- `len(x)` — length of slice, string, or map
- `append(slice, elem)` — returns new slice with element appended
- `push_bytes(dst, src)` — append string `src` bytes to string `dst` in-place
- `println(x)`, `print(x)`, `eprint(x)`, `eprintln(x)` — output (auto `to_string`)
- `isnull(x)` — test if optional/class is null

**Strings:**
- `hash_string(s) -> u64`
- `itoa(n) -> string` — integer to string
- `atoi(s) -> (i64, bool)` — string to integer
- `char_to_string(b) -> string` — single byte to string

**IO/OS:**
- `read_file(path) -> (string, bool)`, `write_file(path, content) -> bool`
- `os_args() -> [string]`, `os_exit(code)`, `os_getwd() -> string`
- `exec_command(name, args) -> (string, bool)`
- `path_join(parts: [string]) -> string`, `path_dir(p)`, `path_base(p)`, `path_ext(p)`
- `list_dir(path) -> ([string], bool)`, `file_exists(path) -> bool`, `mkdtemp() -> string`

**Testing:** `assert(cond, msg)`, `assert_eq(actual, expected, msg)`. See [Testing](#testing).

**Operators:**
- `x!` — unwrap optional (panic if null)
- `expr as T` — type cast
- `x[i]` — index into slice/string
- `x[lo:hi]` — sub-slice
- `+` — addition for numerics; concatenation for strings and slices (returns new value)

## Testing

Lyric has built-in testing support. No frameworks, no ceremony — just assertions, a naming convention, and a CLI command.

### Test Functions

Any function whose name starts with `test_` is a test. No arguments, no return value:

```lyric
func test_lexer_keywords() {
    let lex = Lexer { source: "if else while" }
    let tok = lex.next()
    assert_eq(tok.kind, TIf, "expected if keyword")
}

func test_lower_type_primitives() {
    let lowerer = Lowerer { temp_id: 0, scope: Dict<Sym, LType> {} }
    let te = TypeExpr { kind: TEIdent, name: "i32" }
    let lt = lowerer.lower_type(te)
    assert(!isnull(lt), "should resolve i32")
    assert_eq(lt!.kind, TyI32, "should be TyI32")
}
```

### Assertions

Two builtins, both compiler-provided (they inject file and line automatically):

| Builtin | Behavior |
|---|---|
| `assert(cond, msg)` | If `cond` is false, prints `FAIL file:line: msg` and exits with code 1 |
| `assert_eq(a, b, msg)` | If `a != b`, prints `FAIL file:line: msg` with expected/actual values and exits with code 1 |

`assert_eq` uses auto-generated `to_string()` for enums (variant name), structs and classes (field dump, e.g., `Pos{line: 1, col: 5}`), and primitives.

### Running Tests

```
lyric test test_lexer.ly lexer.ly ast.ly
```

`lyric test` compiles all listed `.ly` files, discovers `test_*` functions, generates a `main()` that calls each one, compiles the C output with gcc, and runs it:

```
PASS  test_lexer_keywords (0.1ms)
PASS  test_lexer_strings (0.1ms)
FAIL  test_lexer_escapes
  assert_eq failed at test_lexer.ly:47
    expected: TStringLit
    got:      TError

3 tests, 2 passed, 1 failed
```

Tests run sequentially in declaration order. A failed assertion exits that test immediately but does not stop the suite — remaining tests still run.

### Test File Conventions

- Test files are regular `.ly` files — no special syntax or annotations
- Name test files `test_*.ly` by convention (not enforced)
- Test functions can use all language features including classes, generics, and relations
- Tests share a compilation unit with the code they test

## Stdlib Classes

- **`Sym`** — interned symbol with pre-computed FNV-1a hash. Create via `sym("name")` or backtick syntax `` `name` `` (which desugars to `sym("name")` at parse time). Methods: `get_name() -> string`, `get_hash() -> u64`. Implements `Hashable`.
- **`Dict<K,V>`** — generic hash table where `K: Hashable`. Methods: `set(key, val)`, `get(key) -> V?`, `has(key) -> bool`, `remove(key)`, `keys() -> [K]`, `len() -> i32`. Constructor: `Dict<K,V>()`.
- **`Hashable`** — interface requiring `get_hash(self) -> u64`. `Sym` implements it. **`string` does NOT implement `Hashable`** — wrap strings with `sym()` to use as hash keys.
- **`Comparable`** — interface for ordering (numeric, string, bool).
- **`Equatable`** — interface for equality comparison (numeric, string, bool).
- **`Error`** — for `(T, error)` returns. `message() -> string`. Create via `Error { msg: "..." }`.
- **`StringBuilder`** — `write(s)`, `write_byte(b)`, `to_string()`, `len()`. Create via `new_string_builder()`.

## Concurrency

```lyric
spawn { ... }
let ch: channel<i32> = make_channel<i32>()
ch.send(value)
let v = ch.receive()
select { case v = <- ch => ... default => ... }
lock(mutex) { ... }
```

Compiles to pthreads in the C backend. `spawn` auto-captures referenced variables into a context struct.

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

C backend: Duff's device state machine with `_init/_next/_free`. The stdlib provides `range(start, end) -> gen i32` for common iteration.

## Memory Model

- **Primitives** — registers/stack, copied by value.
- **Structs/Tuples** — stack, copied on assign. `mut` params pass by reference (scoped, cannot escape).
- **Classes** — slab-allocated. Two modes: **AoS** (default, pointer-based) and **SoA** (parallel arrays, u32 handles, enabled via `--soa` flag). Owned classes destroyed by owner; non-owned classes ref-counted.
- **Slices `[T]`** — fat pointer (data + len + cap). Assignment copies backing data. Parameter passing shares backing data (no copy). `let ref` creates a zero-copy view.
- **Relations** — ownership graph. `owns` = cascade destroy, `refs` = auto-unlink on destroy. No dangling references.
- **Scope-exit freeing** — escape analysis injects `StSliceFree`/`StSlabFree` for locally-allocated slices and classes that don't escape their scope.
- **`final`** — optional pre-destroy hook on classes, called before auto-generated destructor.
- **`trusted`** — blocks/functions where manual `ref(x)`/`unref(x)` is allowed and auto ref-counting is disabled. For stdlib containers.
- **`destroys`** — compiler-inferred annotation on functions that may destroy class instances. References become dead after such calls.
- **`mut resize`** — annotation on parameters that may grow/shrink a slice. Prevents use while element references exist.
- **No GC** — deterministic destruction via ownership + ref counting.

## Design Annotations

```lyric
class Foo {
  why: "Explanation of design intent"
}

doc "Section" {
  "Documentation content"
}

invariant: "claim" verified_at: "date"
source: ["file.go"]
```

These are for human/AI understanding, not runtime behavior.

**Note**: Annotation keywords (`source`, `why`, `doc`, `fake`, `field`, `lock`, `implements`) are **contextual** — they lex as identifiers and are only interpreted as keywords in annotation contexts. They CAN be used as variable/field names. The parser uses targeted lookahead to disambiguate (e.g., `lock` is only a keyword when followed by `(`).

## Known Gotchas

- **Annotation keywords are contextual** — `source`, `why`, `doc`, `fake`, `field`, `lock`, `implements` can be used as variable/field names (they lex as identifiers).
- **`fn` vs `func`** — both work for declarations; `fn` also works in type syntax (`fn(i32) -> bool`).
- **Structs are value types** — must read→modify→write-back for nested fields. Recurring source of bugs.
- **`string` is NOT `Hashable`** — use `sym("key")` or backtick `` `key` `` for hash table keys.
- **Enum variant construction** — must use positional args: `Variant(a, b, c)`. Named args like `Variant(x: a, y: b)` are not supported in call-syntax construction (use struct literal syntax `Struct { x: a }` for structs only).
- **`append(slice, item)` builtin** — exists for plain slices; for relation-owned lists use `array_append<P,C>(parent, child)`.
- **`mut` on class params** — don't do it. Classes are already pointers; `mut` creates double-pointer segfaults.
- **Tuple access** — use `._0`, `._1` (underscore-prefixed indices), not `.0`, `.1`.
- **`null` and `null`** — both accepted by the lexer (mapped to same token). Book convention uses `null`.

## Self-Hosting Status

Lyric is **self-hosting** as of June 2026. The bootstrap compiler (`lyric.c`, ~90K lines of C) is compiled from ~30K lines of Lyric source across 12 packages (`src/ast/`, `src/lir/`, `src/parser/`, etc.) plus stdlib. The compiler achieves a **fixed point**: `bootstrap2.c == bootstrap3.c`. 78 tests pass. The legacy Go compiler has been moved to `legacy/go-compiler/`.

**Compiler architecture notes** (learned during bootstrap):
- `CheckFile` uses three-phase processing: Phase 0 (preregister type names), Phase 1 (register signatures), Phase 2 (check bodies) across ALL files before proceeding.
- `DesugarDestructors` wraps each relation's destructor body in a `StmtBlock` to avoid variable name collisions.
- `MergeStdlib` merges stdlib into the first block only (not every block) to avoid duplicate type definitions. Must merge ALL files BEFORE desugar/check.
- Monomorphization is iterative — Phase 2 re-collects from specialized bodies.
- **AST `Expr` is a VALUE TYPE** — any copy loses checker annotations; always use `&slice[i]`.

**Key design rules for bootstrap code**:
- Classes for most things, structs only for simple value types (Pos, Span, LexerState)
- ArrayList relations for parent→child ownership
- Dict<K,V> for hash tables, Sym for identifiers (hash once)
- All parameters must be named: `func foo(x: i32)` not `func foo(i32)`
