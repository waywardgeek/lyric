# Forge — A Typed Language for Design and Implementation

*Bill Cox & CodeRhapsody — June 2026*

**Source code & tools:** [github.com/waywardgeek/forge](https://github.com/waywardgeek/forge)

---

## Purpose

Forge is a typed language with two modes of use:

**`.forge` files — understandings.** Declaration-only Forge: types, signatures,
interfaces, annotations, doc blocks, invariants, ownership relations. No function
bodies. The design artifact for Understanding-Driven Development (UDD). Verified
against implementations written in any language. Every `.forge` file is valid Forge.

**`.fg` files — code.** Full Forge with function bodies, executable control flow,
and real semantics. Compilable and runnable. An optional capability — UDD does not
require production code to be written in Forge. The compiler exists to prove the
language design is sound: if the notation is precise enough to verify against real
implementations, then function bodies are all that's missing to make it a real language.

Both modes are designed to be:
- **Read primarily by AI** — dense with meaning, minimal ceremony, no noise
- **Written primarily by AI, reviewed by humans** — the AI writes after implementation;
  the human reviews for accuracy
- **Verified** — `.forge` files are structurally checked against implementations;
  `.fg` files are type-checked and compiled
- **Language-agnostic in intent** — `.forge` files describe design regardless of
  implementation language; `.fg` files compile to Go or C

---

## Design Philosophy

**Permissive by default.** `.forge` files accept design patterns common across
languages, imposing only *sound* constraints — constraints that improve design quality
regardless of target language. The key example: typed function signatures. Every
language has types (even dynamically-typed ones); requiring them catches design
errors the way TypeScript improves on JavaScript.

**No language-specific restrictions.** If Python allows circular imports, Forge does
not forbid them. If Rust requires explicit lifetimes, Forge does not require them.
The `.forge` file describes the design, not the target language's rules.

**Sound constraints Forge does impose:**
- Every parameter and return value must have a type (may be a type variable)
- `self` declares method receivers (mutation is implicit — no `mut self` distinction
  in current implementation)
- Thread safety annotations must be consistent (`guarded_by(mu)` requires `mu` to exist)

**Constraints Forge deliberately does NOT impose:**
- Import ordering or circularity restrictions
- Memory management model (ownership, borrowing, GC)
- Error handling strategy (`(T, error)` is idiomatic but not the only option)
- Naming conventions (follow the implementation language's conventions)

---

## Design Lineage

Forge inherits its core from **Rune**, Bill Cox's systems language. The inheritance
is selective.

**Kept from Rune:**
- The numeric type tower (`u8`–`u256`, `i8`–`i256`, `f32`/`f64`/`f128`)
- `string`, `bool`
- `T?` for optional values
- `T | U` for typed unions
- `[T]` for sequences, `map[K]V` for associative containers
- `enum`, `struct`, `class`
- `relation` for ownership/reference structure, with `owns`/`refs`, labels, and hints

**Dropped from Rune:**
- **Secret propagation in the type system** — research goal, not design tool
- **Optional types everywhere** — Rune allowed omitting types to feel like Python;
  Forge requires every parameter and return value to be typed
- **Memory safety mechanisms in the language** — ownership enforcement is the
  compiler's job; `.forge` files express intent via `relation`
- **Python-style goals** — Forge appeals to the AI reading it, not Python programmers

**Added in Forge:**
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
- **Three-zone `.forge` files** — human-reviewed declarations + auto-generated
  function index + auto-generated dependencies

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

// Platform-width integers (Go interop only)
int   uint    // NOT part of the Forge numeric tower
```

**Default integer literal type:** `i32`. Cast with `<u64>(x)`.

**Character literals:** `'A'` → `u8` constant (value 65). Supports escape sequences:
`\n`, `\r`, `\t`, `\\`, `\'`, `\"`, `\0`, `\x##` (hex byte).

`null` is the nil literal for optional types. `let x = null` without a type annotation
is a checker error; use `let x: T? = null`.

`error` is a built-in interface, not a primitive — see the Interfaces section.

---

## Composite Types

```
T?            // optional: T or null
T | U         // union: T or U (exhaustively typed)
[T]           // slice of T (fat pointer: data + len + cap)
map[K]V       // associative array (Go maps)
(T, U)        // anonymous tuple (positional)
(T, U, V)     // triple, etc.
chan T         // CSP channel
fn(T, U) -> V // function type
```

**String as byte slice:** `string` is `[u8]` internally. String indexing (`s[i]`)
returns `u8`. C interop via `forge_str_to_cstr()` null-terminates on demand.

**Tuples:** Anonymous tuples `(T, U)` are positional. Access fields with `.0`, `.1`
notation (not yet implemented — use destructuring `let (a, b) = expr`).

**Function type syntax:** `fn(T, U) -> V` is the canonical form. `func` is the
keyword for function declarations; `fn` is for type syntax only. `fn` is a contextual
keyword and can be used as a variable name.

---

## Generics and Type Variables

**The rule:** Every function parameter and return value must declare a type. A type
may be a *type variable* — a placeholder resolved at the call site.

### Type Variables

Type variables are declared explicitly using angle brackets `<>`. They must be
explicitly declared — they are never inferred from context. This prevents typos
from silently becoming type variables:

```forge
func identity<T>(x: T) -> T
func first<T>(xs: [T]) -> T?
func transform<T, U>(xs: [T], f: fn(T) -> U) -> [U]
func zip<T, U>(xs: [T], ys: [U]) -> [(T, U)]
```

### Call-Site Type Arguments

Generic functions support both explicit type arguments and type inference:

```forge
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
```forge
func clamp<T: Comparable>(value: T, lo: T, hi: T) -> T
```

**`where` clause:**
```forge
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
Forge names the capability: `T: Integer`. One constraint names what the type must be.

---

## Visibility

Default **private** (unexported in Go). Use `pub` to make a declaration public:

```forge
pub func add(x: i32, y: i32) -> i32    // exported
func helper(x: i32) -> i32              // unexported

pub struct Point { x: f64  y: f64 }     // exported
pub class Counter { ... }               // exported
pub enum Color { Red Green Blue }       // exported
```

In Go output, `pub` → uppercase first letter, no `pub` → lowercase.

Fields use `pub` prefix: `pub name: string`.

**Naming conventions follow the implementation language.** A Go project's `.forge`
files use PascalCase for exported names and camelCase for unexported names.

---

## Functions

### Declarations

All parameters must be named and typed. Return type follows `->`:

```forge
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

```forge
func T.method(self) -> i32 { ... }
func T.mutate(self, x: i32) { ... }
```

The lowerer passes `fn.ReceiverType`; the checker defines `self` in scope.

### Lambdas

```forge
let double = (x: i32) -> i32 { return x * 2 }
let greet = (name: string) -> string { return "hello " + name }
```

Lambda parameters must have explicit types. Lambdas capture variables from their
enclosing scope. In C backend, captured variables are passed via auto-generated
context structs with capture-by-reference via pointer redirection.

### Function Annotations (.forge files)

```forge
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

```forge
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

```forge
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

```forge
match shape {
    Circle(r) => { println(f"radius: {r}") }
    Rect(w, h) => { println(f"{w}x{h}") }
    Point => { println("point") }
}
```

**Multi-pattern arms:** Multiple patterns per arm separated by `|`:
```forge
match kind {
    OPlus | OMinus => { PREC_ADDITIVE }
    OStar | OSlash => { PREC_MULT }
    _ => { PREC_NONE }
}
```

**Match as expression:**
```forge
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

```forge
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
```forge
let c = Counter { count: 0, name: "main" }
```

Fields not specified are zero-initialized. Fields can have defaults:
```forge
class Config {
    timeout: u32 = 30
    retries: i32 = 3
}
let cfg = Config {}  // uses defaults
```

**Explicit constructor** — `func ClassName(self, ...)`:
```forge
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

```forge
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

### No Inheritance

Forge does not support classical inheritance. Subtype relationships are expressed
through interface satisfaction only. Shared behavior belongs in interfaces or
in separate classes held as dependencies.

---

## Interfaces and Multi-Class Contracts

Interfaces are first-class declarations. They can span multiple type parameters,
defining relationships between types:

```forge
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

```forge
interface OwningList<P, C> {
    embed DoublyLinked<P, C>    // copies fields and destructors
    destructor P { ... }        // can add/override
}
```

Embed copies only fields and destructors — not methods.

### Impl Blocks

Wire interface methods to concrete class methods:

```forge
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
```forge
pub func count<P, C>(p: P) -> i32 where DoublyLinked<P, C> {
    return len(p.children())
}
```

### The `error` Interface

`error` is a built-in interface. Any class with a `message(self) -> string`
method satisfies it via structural subtyping:

```forge
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

```forge
relation ArrayList Team:roster owns [Player:team]
```

**Injected fields:** `Team.roster_children: [Player]`, `Player.team_parent: Team?`,
`Player.team_index: i32`.

**Functions:** `array_append<Team, Player>(t, p)`, `array_remove<Team, Player>(p)`.

### OwningList — Doubly-Linked List, Cascade Destroy

```forge
relation OwningList Team:team owns [Player:player]
```

**Injected fields:** `Team.team_first: Player?`, `Team.team_last: Player?`,
`Player.player_next: Player?`, `Player.player_prev: Player?`,
`Player.player_parent: Team?`.

**Functions:** `dll_append<Team, Player>(t, p)`, `dll_remove<Team, Player>(p)`.

### RefList — Doubly-Linked List, No Cascade

Same fields as OwningList but parent destruction only unlinks, doesn't destroy children.

### HashedList — Hash Table Ownership

```forge
relation HashedList Registry:reg owns [Entry:entry]
```

Child must implement `hash_key(self) -> u64`. Open-addressing hash table with 75%
load factor rehash and linear probing.

**Functions:** `hash_insert`, `hash_lookup`, `hash_remove`, `hash_init`.

### Generic Type Parameters in Relations

Relations support generic participants:
```forge
relation HashedList Dict<V>:d owns [DictEntry<V>:d]
```

---

## Variables and Constants

```forge
let x = 42              // immutable, type inferred
let mut y: i32 = 0      // mutable, type annotated
```

**Top-level constants** inside `forge` blocks:
```forge
forge parser {
    let PREC_NONE: i32 = 0
    let PREC_OR: i32 = 1
}
```

These compile to `static` globals in C, `const` or `var` in Go.

---

## Control Flow

```forge
if cond { ... } else if cond2 { ... } else { ... }

while cond { ... }

for item in collection { ... }
for item, idx in collection { ... }

match expr {
    Pattern => { ... }
    _ => { ... }
}
```

### Block Scoping

Any `{ }` block creates a new scope. Variables declared inside are local to that block:

```forge
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

### No Ternary If-Expression

Use `let mut` + if-else:
```forge
let mut result: i32 = 0
if cond { result = a } else { result = b }
```

---

## Type Casts

Angle-bracket prefix syntax:
```forge
let x: i32 = 42
let y: i64 = <i64>(x)        // widen
let z: i32 = <i32>(y)        // narrow (may truncate)
```

Only numeric ↔ numeric casts are supported. All casts are unchecked.

---

## Optional Operations

### Unwrap: `expr!`

Extracts the inner `T` from `T?`. Panics if null:
```forge
let value: i32 = x!
```

### Null Check: `isnull(expr)`

Returns `true` if optional is null:
```forge
if !isnull(result) {
    println(f"found: {result!}")
}
```

### Implicit Wrapping

`T` is assignable to `T?` without explicit conversion:
```forge
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
```forge
func divide(a: i32, b: i32) -> (i32, error) {
    if b == 0 {
        return (0, Error { msg: "division by zero" })
    }
    return (a / b, null)
}
```

### The `?` Operator

Propagates errors from `(T, error)` returns:
```forge
func compute(x: i32) -> (i32, error) {
    let result = divide(x, 2)?    // early returns on error
    return (result, null)
}
```

**Rules:**
- Operand must return `(T, error)`
- Enclosing function must also return `(..., error)`
- Statement-level only
- After `let x = foo()?`, `x` is `T?` (not `T`) — use `x!` to unwrap.
  This is a known ergonomic issue.

**Implementation:** The lowerer desugars `?` via `hoistNestedTry()` which introduces
SSA temps for nested try expressions.

---

## Union Types

```forge
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

```forge
let msg = f"Hello {name}, count={x + 1}"
```

Expressions inside `{ }` are evaluated and converted to strings.

---

## Concurrency (Go Backend)

```forge
spawn { ... }                          // goroutine
let ch: chan i32 = make_chan()          // channel
ch <- value                            // send
let v = <- ch                          // receive
select {                               // multiplex
    case v = <- ch => { ... }
    default => { ... }
}
lock(mutex) { ... }                    // scoped mutex
```

**C backend:** Channels use pthreads macros (`FORGE_CHAN_DEF/IMPL`), spawn uses
auto-capture context structs with `pthread_create`, mutex uses `pthread_mutex_t`.

---

## Generators

```forge
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

---

## Built-in Functions

| Function | Signature | Description |
|---|---|---|
| `println(...)` | `any... -> unit` | Print with newline |
| `print(...)` | `any... -> unit` | Print without newline |
| `eprintln(...)` | `any... -> unit` | Print to stderr with newline |
| `eprint(...)` | `any... -> unit` | Print to stderr |
| `len(x)` | `[T] \| string -> i32` | Length |
| `append(xs, elem)` | `([T], T) -> [T]` | Append element |
| `isnull(x)` | `T? -> bool` | Check if optional is null |
| `hash_string(s)` | `string -> u64` | FNV-1a hash |
| `itoa(n)` | `int -> string` | Integer to string |
| `atoi(s)` | `string -> (i64, bool)` | String to integer |
| `char_to_string(b)` | `u8 -> string` | Byte to string |
| `assert(cond, msg)` | `(bool, string) -> unit` | Fail test if false (see [Testing](#testing)) |
| `assert_eq(a, b, msg)` | `(T, T, string) -> unit` | Fail test if not equal (see [Testing](#testing)) |

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

### Built-in Methods

**String methods:** `s.len()`, `s.contains(sub)`, `s.has_prefix(pre)`,
`s.has_suffix(suf)`, `s.to_upper()`, `s.to_lower()`, `s.trim()`,
`s.replace(old, new)`, `s.split(sep)`, `s.index_of(sub)`, `s.repeat(n)`.

**List methods:** `xs.len()`, `xs.push(item)`, `xs.pop()`, `xs.contains(item)`,
`xs.reverse()`, `xs.join(sep)`.

**Map methods:** `m.len()`, `m.contains_key(k)`, `m.keys()`, `m.values()`.

---

## Testing

Testing is a first-class feature of Forge, not an afterthought bolted on via libraries. The design is minimal and opinionated: two assertion builtins, a naming convention, and a CLI command. No test frameworks, no assertion libraries, no mock systems.

### Design Rationale

Forge is a language for writing compilers and systems software, primarily by AI agents. The testing system reflects what those users actually need:

1. **Fast feedback** — write a test, run it, see what broke. No configuration, no build files, no test runner setup.
2. **Runtime verification** — Forge's type checker has intentional gaps (cross-file resolution produces warnings, not errors). Tests catch what the checker misses.
3. **Minimal ceremony** — a test is just a function. No test classes, no decorators, no registration. If it starts with `test_`, it's a test.

### Test Functions

A test function has no arguments and no return value. Its name starts with `test_`:

```forge
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

```forge
assert(len(tokens) > 0, "lexer should produce at least one token")
assert(!isnull(result), "parse should succeed")
```

Output on failure:
```
FAIL  test_lexer_basic
  assert failed at test_lexer.fg:15
    lexer should produce at least one token
```

#### `assert_eq(actual: T, expected: T, message: string)`

If `actual != expected`, prints the failure message along with both values, then terminates the test:

```forge
assert_eq(tok.kind, TLet, "first token kind")
assert_eq(result.name, "main", "parsed function name")
assert_eq(count, 42, "element count")
```

Output on failure:
```
FAIL  test_lexer_basic
  assert_eq failed at test_lexer.fg:16
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

### The `forge test` Command

```
forge test [files...]
```

`forge test` compiles all listed `.fg` files together, discovers all `test_*` functions, generates a `main()` that invokes each test with timing and result tracking, compiles with gcc, and runs the binary.

Example:
```
forge test test_lexer.fg lexer.fg ast.fg
```

Output:
```
PASS  test_lexer_keywords (0.1ms)
PASS  test_lexer_strings (0.2ms)
PASS  test_lexer_numbers (0.1ms)
FAIL  test_lexer_escapes
  assert_eq failed at test_lexer.fg:47
    expected: TStringLit
    got:      TError

4 tests, 3 passed, 1 failed
```

**Execution model:**
- Tests run sequentially in source declaration order
- A failed assertion stops that test function immediately (no partial execution)
- The suite continues — remaining tests still run
- Exit code: 0 if all pass, 1 if any fail

**No test discovery from directories.** You explicitly list files. This matches Forge's current compilation model (no module system, no implicit file discovery). When modules arrive, `forge test` will gain directory-based discovery.

### Test File Conventions

- Test files are regular `.fg` files — no special syntax, no annotations
- Name test files `test_*.fg` by convention (not enforced by the compiler)
- Place test files alongside the code they test
- Helper functions used only by tests can live in test files (they're just regular functions)

### What Is Not Included (and Why)

| Feature | Reason for exclusion |
|---|---|
| Test fixtures / setUp / tearDown | Enterprise pattern. Tests should be self-contained. |
| Mocking | Forge is for compilers, not web services. Use real objects. |
| Property-based testing | Requires random generation — out of scope for bootstrap. |
| Code coverage | Requires C instrumentation. Potential future addition. |
| Snapshot testing | Too complex for the value it provides at this stage. |
| Test filtering (`--filter`) | Nice-to-have. Could add `forge test --filter lexer` later. |
| Parallel execution | Sequential is simpler and sufficient for compiler tests. |
| Subtests / nested tests | Adds complexity without clear benefit for Forge's use cases. |

The testing system can grow, but the baseline is intentionally small. Two builtins, one convention, one command.

---

- **`Sym`** — interned symbol wrapping string + hash. Create via `sym("name")`.
  Methods: `get_name() -> string`, `get_hash() -> u64`. Hash once, compare by
  integer ("integer war" — avoid repeated string hashing).
- **`Error`** — standard error class. `message() -> string`.
  Create via `Error { msg: "..." }`.
- **`StringBuilder`** — mutable string builder. `write(s)`, `write_byte(b)`,
  `to_string()`, `len()`. Create via `new_string_builder()`.
- **`Dict<V>`** — generic string-keyed hash table using Sym keys and HashedList.
  API: `dict_new()`, `dict_set(d, key, value)`, `dict_get(d, key)`,
  `dict_has(d, key)`, `dict_remove(d, key)`.

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

## The .forge File Layer

`.forge` files use Forge syntax with one additional top-level construct: the
`forge` block. Everything inside is valid Forge plus declarations specific to
the design artifact.

### The `forge` Block

```forge
forge ModuleName {
    // types, functions, relations, impls, constants, annotations
}
```

Each `.forge` file has one `forge` block. `.fg` files also use `forge` blocks
(one or more per file).

### `why:` — One-Line Purpose

Attaches to any declaration:
```forge
class Executor {
    why: "Central dispatch for all tool calls. Single instance per agent."
    active: map[u32]Job  guarded_by(mu)
    mu: lock
}
```

### `doc "Section": """..."""` — Narrative Blocks

Named narrative sections, parsed as first-class content:
```forge
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

```forge
invariant: "every registered name maps to exactly one handler"
invariant: "running job count never exceeds pool size"
    verified_at: "a3f9c12"
```

Cannot be mechanically verified. `verified_at:` stamps flag when a human last
checked the claim against source.

### Thread Safety Annotations

```forge
field: Type   guarded_by(lockname)

func foo()
    requires_lock(lockname)
    excludes_lock(lockname)
    concurrent: true | false
    spawns:
```

### Source and Fake Links

```forge
source: ["pkg/tools/executor.go", "pkg/tools/process_manager.go"]
fake:   "pkg/tools/fake_executor.go"
```

### The Three Zones

Each `.forge` file has three zones:

1. **Human-reviewed zone** — type declarations, doc blocks, invariants, `why:`.
   AI-written, human-reviewed.
2. **Auto-generated function index** — `forge update` scans source, writes
   function signatures with line numbers as `// func ...` comments. Enables
   surgical source reads via line-number jumps.
3. **Auto-generated dependencies** — `forge update` writes import/type
   dependencies.

Zone 1 is the understanding. Zones 2 and 3 are mechanical aids maintained by
the `forge update` tool.

---

## Contextual Keywords

The following keywords are **contextual** — they lex as identifiers and are only
interpreted as keywords in specific parser contexts:

`doc`, `why`, `source`, `fake`, `field`, `lock`, `implements`, `fn`

They CAN be used as variable names, field names, or function names:
```forge
let source = "hello"      // valid — source is just an identifier here
let field = 42             // valid
```

The parser uses targeted lookahead to disambiguate: `lock` is only a keyword when
followed by `(`, `field` only in annotation context, etc. `parsePrimaryExpr` has
NO keyword rewrite — expressions always see identifiers.

---

## Compilation

### File Structure

```forge
forge BlockName {
    // types, functions, relations, impls, constants
}
```

### Multi-File Compilation

```
forge compile --c file1.fg file2.fg ...
```

Multiple `.fg` files are parsed independently, then merged into a single
compilation unit via `MergeFiles()`. The checker uses two-phase processing:
register all types/functions across ALL blocks first (phase 1), then check
bodies (phase 2). This ensures cross-file type references resolve correctly.

### Compilation Pipeline

```
Parse → MergeStdlib → DesugarAll → Check → MergeFiles → Lower → Optimize → Backend
```

**Desugar order** (MUST run in this sequence):
1. InterfaceEmbeds (flatten embedded interfaces)
2. InterfaceFields (field → getter/setter methods)
3. Relations (inject fields + impl blocks)
4. Destructors (generate destroy methods from owns relations)
5. DefaultImpls (extract interface methods to top-level functions)

### Go Backend

Pass-through generics (Go has native generics). Outputs `.go` files.

### C Backend

Requires monomorphized LIR (C has no generics). Outputs `.c` files using
`forge_runtime.h`. Compile with `gcc -std=gnu11 -I runtime`.

**Monomorphization** is LIR→LIR: specializes generic functions/classes for each
concrete type instantiation. Iterative — after specializing, re-collects from
specialized bodies for transitive instantiations.

### Toolchain Commands

| Command | Description |
|---|---|
| `forge compile [--c\|--go] file.fg ...` | Compile to C or Go |
| `forge verify file.forge` | Check .forge against source |
| `forge update file.forge` | Refresh function index and deps |
| `forge fmt file.fg` | Format source (comment-preserving) |
| `forge gen pkg/path/` | Scaffold .forge from Go packages |
| `forge test file.fg ...` | Compile, discover `test_*` functions, run tests |

---

## Known Gotchas

- **`?` returns optional** — after `let x = foo()?`, `x` is `T?`, not `T`.
  Use `x!` to unwrap.
- **No ternary** — use `let mut` + if-else.
- **No `is` operator** — use helper functions with `match`.
- **Enum construction is positional only** — `Variant(a, b)`, not `Variant(x: a)`.
- **Struct literal ambiguity** — `Ident {` is ambiguous between struct literal
  and variable + block in statement context. Parser uses `exprDepth` counter:
  inside parens/brackets/arg lists (`exprDepth > 0`), always struct literal.
  At statement level, uses `isStructLitAhead()` lookahead.
- **`append` vs `array_append`** — `append(slice, item)` for plain slices;
  `array_append<P,C>(parent, child)` for relation-owned lists.
- **`forge fmt` bug** — keywords inside string literals tokenized as keywords.
- **Platform `int`/`uint`** — only for Go interop, not part of numeric tower.

---

## What Forge Is Not

**`.forge` files** are not a programming language. They contain no executable code.
A `.forge` file is a structured design artifact — compressed, checkable understanding
consumed primarily by AI, verified by a static tool against implementations in any
language.

**`.fg` files** are a programming language. They compile, run, and are type-checked.

Neither mode is:
- **UML** — UML is for human visual parsing; Forge is for AI context and static verification
- **A schema language** — Protobuf describes serialization; Forge describes system design
- **Comments** — comments explain code; Forge files describe the *system* above any implementation
- **Documentation** — documentation decays without enforcement; Forge files are verified
  at commit time and the AI has skin in the game keeping them accurate

The closest analog is a typed IDL extended with design documentation, thread safety,
invariants, and ownership — where the primary consumer is an AI working with the code.

---

*Forge is what a codebase would tell you if it could talk.*

---

## Why Forge — The Performance and Safety Story

Forge is designed to become the world's fastest language for memory-intensive
applications — which in a data center is most of them — while simultaneously
being the most memory-safe language that doesn't use garbage collection.

### Relations Eliminate Manual Destructors

In C++, manual destructors are a primary source of memory safety bugs: use-after-free,
double-free, dangling pointers, and memory leaks. Rust addresses this with borrow
checking and lifetimes, but at enormous cognitive cost — engineers spend significant
time fighting the borrow checker.

Forge takes a different approach: **relations declare ownership, and the compiler
generates all destructors automatically.**

```forge
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
manually reorganizes their data. Forge's relation system knows the ownership graph
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

Forge's C backend can use relations to generate SoA layout: each relation field
becomes a separate array. Getters and setters index into the correct array. The
class handle becomes a 32-bit index, not a 64-bit pointer — halving pointer storage.

**This is the DataDraw insight that Bill proved at scale:** relation-based code
generation with SoA layout produced 10x performance improvements in EDA tools
processing billions of objects. Forge brings this to a general-purpose language.

### Multi-Class Interfaces: Expressiveness Without Inheritance

Most languages force a choice: either graph algorithms know about your concrete
types (not reusable) or they use heavyweight inheritance/visitor patterns (verbose
and fragile).

Forge's multi-class interfaces let you write graph algorithms ONCE and bind them
to ANY concrete types via impl blocks:

```forge
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

Forge's memory model:
- **Relations** declare ownership → compiler generates destructors
- **Cascade deletion** through `owns` relations → no leaks
- **Back-pointers** maintained automatically → no dangling references
- **Deterministic destruction** → predictable latency (no GC pauses)
- **No borrow checker** → no lifetime annotations, no fighting the compiler

The cost: you must declare your ownership graph via relations. But you were going
to design that ownership graph anyway — Forge just makes it explicit and verifiable
rather than implicit and error-prone.

For the rare case of shared ownership without a clear parent, ref-counting is
available (deferred — not yet implemented).

### The Result

A language that is:
1. **Faster than C++** for memory-intensive applications (SoA layout, 32-bit handles)
2. **Safer than C++** (no manual destructors, no use-after-free)
3. **More expressive than Rust** (multi-class interfaces, no borrow checker friction)
4. **Simpler than both** (relations replace pages of boilerplate)

---

## Standard Library Reference

The stdlib (`stdlib/std.fg` and `stdlib/string.fg`) is auto-imported into all
Forge programs. It provides ownership data structures, hash tables, string
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
```forge
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
```forge
relation OwningList Document:doc owns [Paragraph:para]
```

### RefList<P, C> — Doubly-Linked List, No Cascade

Embeds `DoublyLinked<P, C>`. Parent death unlinks children but does NOT destroy
them — children survive independently.

**Usage:**
```forge
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
```forge
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

### Dict<V> — Generic String-Keyed Hash Table

Built on HashedList with Sym keys. The convenience layer for hash maps when
you don't need the full relation machinery.

**Functions:**
| Function | Description |
|---|---|
| `dict_new<V>() -> Dict<V>` | Create empty dictionary |
| `dict_set<V>(d, key: string, value: V)` | Set key-value pair (replaces if exists) |
| `dict_get<V>(d, key: string) -> DictEntry<V>?` | Get entry by key (null if missing) |
| `dict_has<V>(d, key: string) -> bool` | Check if key exists |
| `dict_remove<V>(d, key: string) -> bool` | Remove by key |

**DictEntry<V> fields:** `key: Sym?`, `value: V`.

**Usage:**
```forge
let d = dict_new<i32>()
dict_set<i32>(d, "x", 42)
dict_set<i32>(d, "y", 99)
if dict_has<i32>(d, "x") {
    let entry = dict_get<i32>(d, "x")
    println(entry!.value)  // 42
}
dict_remove<i32>(d, "x")
```

### Sym — Interned Symbol

Wraps a string with a pre-computed FNV-1a hash. Hash is computed once at creation;
all subsequent operations use the u64 hash for O(1) comparison. This is the
"integer war" principle: avoid repeated string hashing in hot paths.

**Construction:** `sym("name")` (not `pub` — avoids Go name collision)

**Methods:**
| Method | Return | Description |
|---|---|---|
| `get_name(self)` | `string` | Original string |
| `get_hash(self)` | `u64` | Pre-computed FNV-1a hash |

**Usage:**
```forge
let s = sym("identifier")
let h = s.get_hash()       // u64, computed once
let n = s.get_name()       // "identifier"
```

### Error Handling

**`error` interface:** Any class with `message(self) -> string` satisfies it.

**`Error` class:** Default concrete implementation.
```forge
let e = Error { msg: "something went wrong" }
println(e.message())  // "something went wrong"
```

**Custom errors:** Just implement `message`:
```forge
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
```forge
let sb = new_string_builder()
sb.write("hello")
sb.write_byte(' ')
sb.write("world")
println(sb.to_string())  // "hello world"
```

### String Utilities (stdlib/string.fg)

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
```forge
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
```forge
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
```forge
let stdin: Reader = os_stdin()
let stdout: Writer = os_stdout()
let stderr: Writer = os_stderr()
```

**Network I/O** (future):
```forge
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
