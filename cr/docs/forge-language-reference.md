# Forge Language Reference (Bootstrap Edition)

Concise reference for writing Forge code. Based on parser, checker, and test files as of 2026-06-06.

## File Structure

```forge
forge BlockName {
  // types, functions, relations, impls
}
```

Each `.fg` file has one or more `forge` blocks.

## Primitives

`bool`, `u8`, `u16`, `u32`, `u64`, `i8`, `i16`, `i32`, `i64`, `f32`, `f64`, `string`, `any`

- Default integer literal: `i32`. Cast with `<u64>(x)`.
- `int`/`uint` — platform-width, Go interop only.
- `any` — empty interface / `void*`.
- Character literals: `'A'` → `u8` (65). Escapes: `\n \r \t \\ \' \" \0 \x##`.

## Type Expressions

```
T?                  // optional (nullable)
[T]                 // slice (fat pointer: data + len + cap)
(T1, T2)            // tuple
fn(T1, T2) -> R     // function type
chan T               // channel
```

## Structs (Value Types)

```forge
struct Point {
  x: f64
  y: f64
}
```

- Copied by value. No methods. No generics.
- Direct field access: `p.x`.
- Fields can have defaults: `width: i32 = 800`.

## Classes (Heap-Allocated, By Reference)

```forge
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

```forge
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

## Enums

```forge
// Simple
enum Color { Red Green Blue }

// With associated data
enum Shape {
  Circle(radius: f64)
  Rect(w: f64, h: f64)
  Point
}
```

Match on enums:
```forge
match shape {
  Circle(r) => { println(f"radius: {r}") }
  Rect(w, h) => { println(f"{w}x{h}") }
  Point => { println("point") }
}
```

### Multi-pattern match arms

Multiple patterns per arm separated by `|`:
```forge
match kind {
  OPlus | OMinus => { PREC_ADDITIVE }
  OStar | OSlash | OPercent => { PREC_MULT }
  OEqEq | OBangEq => { PREC_EQUALITY }
  _ => { PREC_NONE }
}
```

This works for simple variants, variants with bindings (if all alternatives bind the same names), and wildcard `_`.

### Match as expression

`match` can be used as an expression (returns a value):
```forge
let prec = match kind {
  OPlus => { 9 }
  OStar => { 10 }
  _ => { 0 }
}
```

### Match on non-enum values

Match works on enum types. For non-enum dispatch, use `if`/`else if` chains.

## Type Aliases

```forge
type Name = string
type Callback = fn(i32) -> bool
```

## Variables

```forge
let x = 42              // immutable
let mut y: i32 = 0      // mutable, typed
```

## Control Flow

```forge
if cond { ... } else if cond2 { ... } else { ... }

while cond { ... }

for item in collection { ... }
for item, idx in collection { ... }

match expr {
  Pattern => ...
}
```

## Functions

```forge
func add(a: i32, b: i32) -> i32 { return a + b }

pub func public_fn() { ... }

// Generic
func identity<T>(x: T) -> T { return x }

// External method (multi-class interface pattern)
func T.method(self) -> i32 { ... }

// Lambdas
let f = (x: i32) -> i32 { return x * 2 }
```

## Try Operator and Error Handling

Forge uses `(T, error)` tuples for error handling, similar to Go.

### The `error` interface

`error` is a built-in interface declared in stdlib:

```forge
interface error {
  func error.message(self) -> string
}
```

Any class with a `message(self) -> string` method satisfies `error` via structural subtyping. The stdlib provides a default concrete implementation:

```forge
class Error {
    msg: string
    pub func message(self) -> string { return self.msg }
}
```

### Custom error types

```forge
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

```forge
func load() -> (string, error) {
  let data = read_file("x.txt")?    // propagates error on failure
  return (data, null)
}
```

**Important**: `?` only unwraps the error — the value side remains optional. So after `let x = foo()?`, `x` is `T?` and you need `x!` to unwrap it. This is a known ergonomic issue.

Statement-level only. Containing function must return `(T, error)`.

## F-strings

```forge
let msg = f"Hello {name}, count={x + 1}"
```

## Multi-Class Interfaces

Interfaces span multiple type parameters, defining relationships between types.

```forge
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

```forge
interface OwningList<P, C> {
  embed DoublyLinked<P, C>    // copies fields and destructors
  destructor P { ... }        // can add/override
}
```

### Impl Blocks

```forge
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

```forge
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

```forge
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

```forge
relation OwningList Team:team owns [Player:player]
```

Injected fields: `Team.team_first: Player?`, `Team.team_last: Player?`, `Player.player_next: Player?`, `Player.player_prev: Player?`, `Player.player_parent: Team?`.

Functions: `dll_append<Team, Player>(t, p)`, `dll_remove<Team, Player>(p)`.

### RefList — doubly-linked list, no cascade

```forge
relation RefList Room:room refs [Guest:guest]
```

Same fields as OwningList but parent destruction only unlinks, doesn't destroy children.

### HashedList — hash table ownership

```forge
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

**Core:** `len(x)`, `append(slice, elem)`, `println(x)`, `print(x)`, `eprint(x)`, `eprintln(x)`, `isnull(x)`.

**Strings:** `hash_string(s) -> u64`, `itoa(n) -> string`, `atoi(s) -> (i64, bool)`, `char_to_string(b) -> string`.

**IO/OS:** `read_file(path) -> (string, bool)`, `write_file(path, content) -> bool`, `os_args() -> [string]`, `os_exit(code)`, `os_getwd() -> string`, `exec_command(name, args) -> (string, bool)`, `path_join(a, b)`, `path_dir(p)`, `path_base(p)`, `path_ext(p)`.

**Operators:** `x!` unwrap optional, `<T>(expr)` cast, `x[i]` index, `x[lo:hi]` slice.

## Stdlib Classes

- **`Sym`** — interned symbol. Create via `sym("name")`. `get_name() -> string`, `get_hash() -> u64`.
- **`Error`** — for `(T, error)` returns. `message() -> string`. Create via `Error { msg: "..." }`.
- **`StringBuilder`** — `write(s)`, `write_byte(b)`, `to_string()`, `len()`. Create via `new_string_builder()`.

## Concurrency (Go backend only)

```forge
spawn { ... }
let ch: chan i32 = make_chan()
ch <- value
let v = <- ch
select { case v = <- ch => ... default => ... }
lock(mutex) { ... }
```

## Memory Model

- **Structs** — stack, copied by value.
- **Classes** — heap, passed by reference (pointer).
- **Slices `[T]`** — fat pointer (data + len + cap). Copied by value but shares backing array. Grown via `append()`.
- **Relations** — ownership graph. `.destroy()` cascades through `owns` relations.
- **No GC** — deterministic destruction via ownership.

## Design Annotations

```forge
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

**Note**: `source`, `why`, `doc`, `fake`, and other annotation keywords are reserved — they cannot be used as field or variable names. Use `src` instead of `source`.

## Known Gotchas

- **`source` is a keyword** — can't use as field/param name. Use `src` or `src_text`.
- **`fn` vs `func`** — `fn` is for type syntax only (e.g., `fn(i32) -> bool`), `func` for declarations.
- **No ternary if-expression** — use `let mut x = ...; if cond { x = a } else { x = b }`.
- **`?` returns optional** — after `let x = foo()?`, `x` is `T?`, not `T`. Use `x!` to unwrap.
- **`forge fmt` lexer bug** — keywords inside string literals are tokenized as keywords, causing parse failures on strings containing words like `doc`, `source`, etc.
- **Checker warnings for cross-file refs** — the checker prints "undefined variable" warnings for symbols defined in other `.fg` files. These are non-fatal; the lowerer proceeds.
