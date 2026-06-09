# Forge Language Reference (Bootstrap Edition)

Concise reference for writing Forge code. Based on parser, checker, and test files as of 2026-06-07.

## Bootstrap Philosophy

**The #1 goal of the bootstrap is to make Forge as awesome as Go — maybe even better — for writing compilers.** The bootstrap compiler is being ported from Go to Forge. Every time we hit jank, friction, or missing features, we fix the compiler rather than work around the issue. The bootstrap process is the design feedback loop that makes Forge great.

**Key principles:**
- Don't work around language issues — fix them in the compiler
- If something feels janky, that's a signal to improve the language
- The bootstrap .fg files (ast.fg, lexer.fg, parser.fg, expr_parser.fg) are the primary test of language ergonomics
- Target: C backend via monomorphization (not Go backend)

## File Structure

```forge
forge BlockName {
  // types, functions, relations, impls, constants
}
```

The `forge` wrapper is **optional**. Bare `.fg` files with top-level declarations are valid — the module name is derived from the filename:

```forge
// mymodule.fg — no wrapper needed
enum Color { Red, Green, Blue }
func greet(name: string) -> string { return f"Hello {name}" }
```

Each `.fg` file has one or more `forge` blocks (or bare declarations). Multiple `.fg` files can be compiled together: `forge compile --c file1.fg file2.fg ...` — they are merged into a single compilation unit.

## Newlines and Multi-line Expressions

Newlines are statement terminators. However, **inside `()` and `[]` brackets, newlines are treated as whitespace**, enabling multi-line expressions:

```forge
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

`bool`, `u8`, `u16`, `u32`, `u64`, `i8`, `i16`, `i32`, `i64`, `f32`, `f64`, `string`, `any`

- Default integer literal: `i32`. Cast with `x as u64`.
- `int`/`uint` — platform-width, Go interop only.
- `any` — empty interface / `void*`.
- Character literals: `'A'` → `u8` (65). Escapes: `\n \r \t \\ \' \" \0 \x##`.

## Type Expressions

```
T?                  // optional (nullable)
[T]                 // slice (fat pointer: data + len + cap)
map[K]V             // associative map (K must be comparable)
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

### `if let` — conditional pattern match

Extract a single variant without a full `match`:

```forge
if let Circle(r) = shape {
    println(f"radius: {r}")
} else {
    println("not a circle")
}
```

Bindings (`r`) are scoped to the then-block. The `else` branch is optional.

### `let..else` — assertive pattern extract

Extract variant data into the surrounding scope, with a mandatory diverging `else`:

```forge
let Circle(r) = shape else {
    return -1.0
}
// r is now in scope
println(f"radius: {r}")
```

The `else` block must diverge (`return`, `break`, `continue`). Bindings escape into the outer scope — this avoids rightward drift for the common "extract or bail" pattern.

## Type Aliases

```forge
type Name = string
type Callback = fn(i32) -> bool
```

## Variables & Constants

```forge
let x = 42              // immutable
let mut y: i32 = 0      // mutable, typed
```

**Top-level constants** can be declared directly inside `forge` blocks:

```forge
forge parser {
  let PREC_NONE: i32 = 0
  let PREC_OR: i32 = 1
  // ...
}
```

These compile to `static` globals in C.

## Control Flow

```forge
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

```forge
if expr.kind is ExprCall { ... }
if not (node is Leaf) { ... }
```

Returns `bool`. Does not bind variables — use `if let` for destructuring.
```

### Block Scoping

Forge has block-level scoping like Go. Any `{ }` block creates a new scope — variables declared inside are local to that block:

```forge
func example() {
    let x = 1
    {
        let x = 2       // shadows outer x, legal
        println(x)      // prints 2
    }
    println(x)          // prints 1
}
```

This is used internally by the compiler for code generation (e.g., destructor body injection wraps each relation's cleanup in a block to avoid variable name collisions). Block scoping works at all pipeline levels: AST (`StmtBlock`), LIR (`LStmtBlock`), and C backend (emits `{ }`). Block scoping is not yet exposed as user-facing Forge syntax but the infrastructure is complete.

## Maps

```forge
let empty: map[string]i32 = {:}
let keywords = {"if": TIf, "else": TElse, "for": TFor}
```

**Operations:**
```forge
m[key]              // lookup — returns V? (null if absent)
m[key] = value      // insert or update
delete(m, key)      // remove key
len(m)              // entry count
for key, value in m { ... }   // iteration (unordered)
```

Keys must be comparable (primitives, `string`, `Sym`, structs with all-comparable fields).

**Note:** The bootstrap currently uses `Dict<V>` (string-keyed via `Sym`) and `HashedList` relations for hash tables. `map[K]V` is the general-purpose built-in that supersedes these for non-relational use cases.

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
  let data = read_file("x.txt")?    // propagates error on failure, data is string
  return (data, null)
}
```

`?` unwraps the success value: after `let x = foo()?`, `x` is `T` (not `T?`). If the error is non-nil, the function returns immediately with the error.

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

**Testing:** `assert(cond, msg)`, `assert_eq(actual, expected, msg)`. See [Testing](#testing).

**Operators:** `x!` unwrap optional, `expr as T` cast, `x[i]` index, `x[lo:hi]` slice.

## Testing

Forge has built-in testing support. No frameworks, no ceremony — just assertions, a naming convention, and a CLI command.

### Test Functions

Any function whose name starts with `test_` is a test. No arguments, no return value:

```forge
func test_lexer_keywords() {
    let lex = Lexer { source: "if else while" }
    let tok = lex.next()
    assert_eq(tok.kind, TIf, "expected if keyword")
}

func test_lower_type_primitives() {
    let lowerer = Lowerer { temp_id: 0, scope: Dict<LType> {} }
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
forge test test_lexer.fg lexer.fg ast.fg
```

`forge test` compiles all listed `.fg` files, discovers `test_*` functions, generates a `main()` that calls each one, compiles the C output with gcc, and runs it:

```
PASS  test_lexer_keywords (0.1ms)
PASS  test_lexer_strings (0.1ms)
FAIL  test_lexer_escapes
  assert_eq failed at test_lexer.fg:47
    expected: TStringLit
    got:      TError

3 tests, 2 passed, 1 failed
```

Tests run sequentially in declaration order. A failed assertion exits that test immediately but does not stop the suite — remaining tests still run.

### Test File Conventions

- Test files are regular `.fg` files — no special syntax or annotations
- Name test files `test_*.fg` by convention (not enforced)
- Test functions can use all language features including classes, generics, and relations
- Tests share a compilation unit with the code they test — no import system needed

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

**Note**: Annotation keywords (`source`, `why`, `doc`, `fake`, `field`, `lock`, `implements`) are **contextual** — they lex as identifiers and are only interpreted as keywords in annotation contexts. They CAN be used as variable/field names. The parser uses targeted lookahead to disambiguate (e.g., `lock` is only a keyword when followed by `(`).

## Known Gotchas

- **Annotation keywords are contextual** — `source`, `why`, `doc`, `fake`, `field`, `lock`, `implements` can be used as variable/field names (they lex as identifiers).
- **`fn` vs `func`** — `func` is the only function keyword. `fn` is for type syntax only (e.g., `fn(i32) -> bool`) and is a contextual keyword — it can be used as a variable name.
- **`forge fmt` lexer bug** — keywords inside string literals are tokenized as keywords, causing parse failures on strings containing words like `doc`, `source`, etc. Fix planned.
- **Enum variant construction** — must use positional args: `Variant(a, b, c)`. Named args like `Variant(x: a, y: b)` are not supported in call-syntax construction (use struct literal syntax `Struct { x: a }` for structs only).
- **`append(slice, item)` builtin** — exists for plain slices; for relation-owned lists use `array_append<P,C>(parent, child)`.

## Bootstrap Status

**Completed .fg files**: `bootstrap/ast.fg`, `bootstrap/lexer.fg`, `bootstrap/parser.fg`, `bootstrap/expr_parser.fg`, `bootstrap/desugar.fg`, `bootstrap/checker.fg`, `bootstrap/lir.fg`, `bootstrap/lowerer.fg`

**Compilation**: `forge compile --c bootstrap/*.fg` produces ~30,094 lines of C. **0 GCC errors, 0 warnings, 0 void* violations.** 100/100 C tests pass (lexer 31, parser 52, desugar 12, test_min 5). 8,049 lines Forge → 30,094 lines C. All 249+ Go tests pass.

**Compiler architecture notes** (learned during bootstrap):
- `CheckFile` uses two-phase processing: register all types/functions across blocks first, then check bodies. This is required for multi-file compilation.
- `DesugarDestructors` wraps each relation's destructor body in a `StmtBlock` to avoid variable name collisions when multiple relations contribute to the same `destroy()` method.
- `MergeStdlib` merges stdlib into the first block only (not every block) to avoid duplicate type definitions.
- `EmitC` deduplicates structs, classes, enums, and functions by name before emission (function dedup uses `Receiver.Name` key to avoid colliding methods like `Lexer.peek` and `Parser.peek`).
- Composite type macros are ordered: forward declarations → `FORGE_SLICE_DEF` (uses pointer) → struct/class/enum definitions → `FORGE_OPT_DEF`/`FORGE_RESULT_DEF` (use complete types).
- `func T.method(self)` external method syntax: the lowerer passes `fn.ReceiverType` as the receiver, and the checker defines `self` in scope when `ReceiverType` is set.

**Next to port**: optimizer.fg, monomorphizer.fg, c_backend.fg. Also: implement `forge test` and write bootstrap unit tests.

**Key design rules for bootstrap code**:
- Classes for most things, structs only for simple value types (Pos, Span, LexerState)
- ArrayList relations for parent→child ownership
- Dict<V> for hash tables, Sym for identifiers (hash once)
- All parameters must be named: `func foo(x: i32)` not `func foo(i32)`
