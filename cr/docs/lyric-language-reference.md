# Lyric Language Reference

Daily-driver reference for Lyric (`.ly`) code. Every feature listed here is implemented today — verified against the bootstrap compiler in `src/`, the stdlib in `stdlib/`, and the test suite in `testdata/`. For aspirational features see `lyric-language-spec.md`. For the active backlog see `cr/docs/bootstrap-roadmap.md` and `TODO.md`.

*Source of truth: bootstrap compiler self-compiles to a fixed point. Last reviewed 2026-06-20.*

---

## 1. File Structure

```lyric
// optional: lyric <BlockName> { ... } wrapper. When absent, bare top-level
// declarations belong to the directory's package.

import ast
import parser

func main() {
    println("hello")
}
```

- A **package** is a directory of `.ly` files. All files in a directory share one namespace.
- A **module** is a directory containing a `lyric.mod` marker file. The file's *content* is reserved for future use; only its presence matters today.
- Newlines terminate statements. Inside `()` and `[]` newlines are whitespace. Inside `{}` they are not (statement blocks).
- Comments: `//` to end of line. No block comments.
- One file may contain multiple `lyric BlockName { ... }` blocks.

## 2. Lexical Structure

### Hard keywords
```
as       break       case        class        continue     destructor
else     enum        false       for          from         func
if       impl        import      in           interface    is
let      lyric       match       mut          null         owns
pub      refs        relation    return       select       self
spawn    struct      true        type         where        while
yield
```

### Contextual keywords (lex as identifiers; can be used as identifier names)
- Modifiers: `permanent`, `trusted`, `final`
- Field annotations: `field`, `lock`, `implements`, `guarded_by`
- Type names: `gen`, `channel`, `unit`, `error`, `fn`, `map`
- Built-ins used as functions: `ref`, `unref`, `isnull`, `sym`, `default`

### Literals

| Form | Examples | Notes |
|---|---|---|
| Integer | `42`, `1_000_000` | Default type `i32`. Underscores allowed. No `0x`/`0o`/`0b` prefixes (roadmap). No type suffix. |
| Float | `1.5`, `3.14` | Default type `f64`. `1.` alone does not parse. |
| Char | `'a'`, `'\n'`, `'\xFF'` | A `u8` byte. Escapes: `\n \r \t \\ \' \" \0 \xHH`. No `\u` (roadmap: UTF-8). |
| String | `"hello"` | UTF-8 bytes. Same escapes as char. |
| Triple | `"""..."""` | Preserves newlines. Content is `str_trim`-ed. |
| F-string | `f"x={x + 1}"` | `{expr}` interpolates; `{{` and `}}` escape braces. |
| Backtick sym | `` `name` `` | Sugar for `sym("name")`. |
| Bool | `true`, `false` | |
| Null | `null` | The only null literal. `nil` is not accepted. |

## 3. Primitive Types

| Type | Implemented |
|---|---|
| `bool` | ✓ |
| `i8 i16 i32 i64` | ✓ |
| `u8 u16 u32 u64` | ✓ |
| `f32 f64` | ✓ |
| `string` | ✓ (a byte slice — see §6) |
| `unit` | ✓ (function return only; the void type) |
| `any` | ✓ (top type / interface dispatch) |
| `error` | ✓ (built-in interface — see §11) |
| `int`, `uint` | ✓ (platform-width; mostly for stdlib interop) |

Larger int/float widths (`i128/i256/u128/u256/f128`) are on the roadmap.

- Default integer literal type: `i32`. Cast with `x as u64`.
- Implicit numeric widening: smaller → larger (`i32 + i64` → `i64`). Cross-sign assignment is also implicit (loose; explicit `as` is good style).

## 4. Composite Types

```
T?              // optional (nullable)
T | U           // union (tagged)
[T]             // slice — fat pointer (data + len + cap)
(T, U)          // tuple — positional, access via ._0 ._1
fn(T1, T2) -> R // function type (also accepts func(...) or T -> U for single-arg)
channel<T>      // CSP channel
lock            // mutex type (lowercase)
gen T           // generator return type
```

Tuples are positional. Access via `t._0`, `t._1`. Destructure with `let (a, b) = t`.

**Note**: `map[K]V` is reserved syntax that compiles to a stub today. Use `Dict<K,V>` (stdlib) for hash maps.

## 5. Variables and Bindings

```lyric
let x = 42                  // immutable
let mut y: i32 = 0          // mutable
let x: i32                  // declared, assigned later (statement scope)
let _ = expr                // discard
let (a, b) = pair()         // tuple destructure
let ref view = data[5:10]   // zero-copy view
let mut ref buf = packet[0:16]
```

Grammar: `let [mut] [ref] (name | (name1, name2, ...) | Pat(bindings)) [: Type] [= expr]`.

Module-level `let` and `let mut` are allowed (compile to C `static` globals).

- **Copy-on-assign** for value types (primitives, structs, tuples, slices).
- **`ref` bindings** create a zero-copy view; the source must outlive the binding.
- **Move semantics** are inferred: single-use locals transfer ownership (no retain+release).

## 6. Strings

Strings are byte slices (`string == [u8]` internally) with a hidden trailing `\0` for C interop. Indexing returns `u8`; slicing returns a new string sharing backing data.

```lyric
let s = "hello"
let ch = s[0]            // u8 (104)
let sub = s[1:4]          // "ell"
let cat = "hello" + " world"
```

UTF-8-aware operations (`\u{...}` escapes, code-point iteration) are on the roadmap. Today, `len(s)` and `s.char_at(i)` are byte-oriented.

### Built-in string methods
| Method | Returns | Notes |
|---|---|---|
| `s.len()`, `s.length()` | `i32` | byte length |
| `s.is_empty()` | `bool` | |
| `s.contains(sub)` | `bool` | |
| `s.has_prefix(p)`, `s.starts_with(p)` | `bool` | synonyms |
| `s.has_suffix(p)`, `s.ends_with(p)` | `bool` | synonyms |
| `s.index_of(sub)` | `i32` | -1 if not found |
| `s.substring(lo, hi)` | `string` | also use `s[lo:hi]` |
| `s.char_at(i)` | `string` | one-byte string |
| `s.trim()`, `s.to_lower()`, `s.to_upper()` | `string` | |
| `s.replace(old, new)` | `string` | all occurrences |
| `s.repeat(n)` | `string` | |
| `s.split(sep)` | `[string]` | |
| `s.join(parts)` | `string` | (on a separator string) |

### Stdlib string utilities (`stdlib/string.ly`, namespace `string_utils`)
`str_concat`, `str_split_n`, `str_trim_left`, `str_trim_right`, `str_replace`, `str_repeat`, `str_join`, `str_has_prefix`, `str_has_suffix`, `str_index_of`, `str_to_upper`, `str_to_lower`. Same byte-oriented semantics.

## 7. Slices

`[T]` is a dynamic array — fat pointer (data + len + cap). Assignment copies the slice header; backing storage is shared (parameter passing) or copied (variable assignment).

```lyric
let xs: [i32] = [1, 2, 3]
let empty: [string] = []
xs[0]                // index
xs[1:3]              // sub-slice (shares backing)
let more = xs + [4, 5, 6]
```

### Built-ins
- `len(xs)` — length (`i32`)
- `append(xs, elem)` — return new slice

### Methods
| Method | Returns | Mutates |
|---|---|---|
| `xs.len()`, `xs.length()` | `i32` | |
| `xs.is_empty()` | `bool` | |
| `xs.push(x)`, `xs.append(x)` | `unit` | ✓ |
| `xs.pop()` | `T` | ✓ |
| `xs.contains(x)` | `bool` | |
| `xs.index_of(x)`, `xs.find(x)` | `i32` | |
| `xs.first()`, `xs.last()` | `T?` | |
| `xs.sort()`, `xs.reverse()`, `xs.clear()` | `unit` | ✓ |
| `xs.remove(x)` | `unit` | ✓ |
| `xs.extend(other)` | `unit` | ✓ |
| `xs.join(sep)` | `string` | (on `[string]`) |
| `xs.slice(lo, hi)` | `[T]` | |

Mutating methods on a class field (e.g. `obj.items.push(x)`) automatically use a reference to avoid copy-then-discard.

## 8. Structs

Value types — copied on assign, no identity, no methods.

```lyric
struct Point { x: f64, y: f64 }
struct Range<T> { lo: T, hi: T }

let p = Point { x: 1.0, y: 2.0 }       // named-field literal
let q = Point { 1.0, 2.0 }             // positional — only inside parens/args/lists
struct Config { timeout: u32 = 30 }     // field defaults
let cfg = Config {}                     // uses defaults
```

Structs cannot be relation targets.

## 9. Classes

Heap-allocated, by reference. Identity. Methods. Eligible for relations and RC.

```lyric
class Counter {
    count: i32
    items: [string]

    func increment(self) { self.count = self.count + 1 }
    func get(self) -> i32 { return self.count }
}

let c = Counter { count: 10 }
c.increment()
```

- Construction: struct-literal `Counter { count: 10 }` when no explicit constructor.
- **Explicit constructor**: `func ClassName(self, args) { ... }` — call site uses `Counter(args)`.
- **Generics**: `class Pair<T> { first: T  second: T }`; construction `Pair<i32> { first: 1, second: 2 }`.
- **`final func name(self)`** — pre-destruction hook, runs before the auto-generated `destroy`.
- **`implements I1, I2`** — declares interface satisfaction (declarative today; method completeness is not yet checker-enforced — runtime/codegen will fail).
- **`permanent class`** — opt out of refcounting / destruction. Never freed. Used for singletons like `SymTable`.
- **No inheritance.** Use interfaces.

### Auto-generated destruction
Every non-permanent class gets a `pub func destroy(mut self)` generated by the compiler from `owns`/`refs` relations. You do not write it. Use `final` for pre-destroy cleanup.

Execution order on destroy: `final` → relation cascade/unlink → slab free.

## 10. Enums

```lyric
enum Color { Red, Green, Blue }                // simple — compiles to C typedef enum

enum Shape {
    Circle(radius: f64)
    Rect(w: f64, h: f64)
    Point
}

let c = Color.Red
```

### Match
```lyric
match shape {
    Circle(r) => { println(f"radius: {r}") }
    Rect(w, h) => { println(f"{w}x{h}") }
    Point => { println("point") }
}

// multi-pattern
match kind {
    OPlus | OMinus => { PREC_ADDITIVE }
    _ => { PREC_NONE }
}

// guards
match token {
    Ident(name) if name == "self" => { handle_self() }
    Ident(name) => { handle_ident(name) }
    _ => {}
}

// match as expression
let prec = match kind {
    OPlus => { 9 }
    _ => { 0 }
}

// qualified-variant patterns
match k { ExprKind.Call(f, args) => { ... } _ => {} }

// tuple patterns
match (x, y) {
    (1, 2) => { ... }
    (a, b) if a > b => { ... }
    (_, _) => {}
}
```

**Context-driven disambiguation**: When two enums share a variant name (e.g. `Pair`), the construction `Pair(a, b)` resolves by the expected field type at the use site.

### Variant check: `is`
```lyric
if expr.kind is ExprCall { ... }
if !(node is Leaf) { ... }
```
`is` returns `bool` and does not bind.

### `if let` and `let..else`
```lyric
if let Circle(r) = shape { println(r) } else { println("not a circle") }

// extract or bail; else must diverge
let Circle(r) = shape else { return -1.0 }
println(r)  // r in scope here
```

## 11. Interfaces

Multi-class interfaces declare relationships between type parameters.

```lyric
interface Graph<G, N, E> {
    func G.nodes(self) -> [N]
    func N.out_edges(self) -> [E]
    func E.tgt_node(self) -> N

    pub func count_edges(graph: G) -> i32 {
        let mut total = 0
        let nodes = graph.nodes()
        for n in nodes { total = total + len(n.out_edges()) }
        return total
    }
}
```

Relation hints (binary interfaces — see §12) additionally declare
`field T.name: Type` (injected into the implementing class) and
**paired destructors** keyed on the relation kind:

```lyric
interface ArrayList<P, C> {
    field P.children: [C]
    field C.parent:  P?
    field C.index:   i32

    pub trusted func P.append(self, child: C) { ... }
    pub trusted func P.remove(self, child: C) { ... }

    destructor owns P { ... }   // selected by `relation ArrayList X owns [Y]`
    destructor owns C { ... }
    destructor refs P { ... }   // selected by `relation ArrayList X refs [Y]`
    destructor refs C { ... }
}
```

The `owns` / `refs` keyword on the destructor block must match the relation's
kind keyword; the desugar pass copies only the matching pair onto the
concrete classes. A bare `destructor T { ... }` (no kind) defaults to `owns`.

### Impl blocks
```lyric
// Alias: interface method ↔ class method
impl Graph<SimpleGraph, SimpleNode, SimpleEdge> {
    G.nodes = SimpleGraph.get_nodes
    N.out_edges = SimpleNode.get_edges
    E.tgt_node = SimpleEdge.get_target
}

// Field bind: interface field ↔ concrete field
//   (the `<->` token is the bidirectional field-binding arrow.)
impl ArrayList<Team, Player> {
    P.children <-> Team.roster
    C.parent   <-> Player.team
    C.index    <-> Player.team_idx
}

// Inline body
impl Printable<Widget> {
    P.to_string = (self) -> string { return f"Widget({self.name})" }
}
```

**Labeled impl declarations.** Each top-level class type-arg may
carry an optional `:label` that puts the interface's per-side
injected members under a `<label>` sub-scope on that class (see
`cr/docs/multi-class-interface-redesign.md` §3.8):

```lyric
impl ArrayList<Team:roster, Player:team> { ... }
// label-prefixed members: team.roster_children, player.team_parent

// Same interface, same class, distinct labels — non-colliding:
impl DoublyLinked<Node:ready_q,   Node:ready_q_child>   { ... }
impl DoublyLinked<Node:blocked_q, Node:blocked_q_child> { ... }
```

A `relation` declaration is sugar for a labeled impl of the
matching hint interface plus an `owns`/`refs` flag.

🚧 The dotted-scope call form (`team.roster.children`) for accessing
label-prefixed members is design intent per redesign §3.1 but not yet
shipped; today, label-prefixed members are accessed by their flat
textual-prefix names (`team.roster_children`). Phase 3c-capability
ships the dotted form as additive sugar.

### `error` interface
Built-in. Any class with a `message(self) -> string` method satisfies it.

```lyric
class ParseError {
    msg: string
    pub func message(self) -> string { return self.msg }
}
```

### `where` clauses
```lyric
func clamp<T: Comparable>(v: T, lo: T, hi: T) -> T { ... }
func count<P, C>(p: P) -> i32 where DoublyLinked<P, C> { ... }
```

Built-in constraints: `Comparable` (numeric, string, bool), `Equatable` (numeric, string, bool), `Hashable` (`Sym`, numeric, bool — **not** `string`; wrap with `sym()`).

## 12. Relations

Declare ownership using a stdlib interface as a hint. The compiler injects fields, generates impl bindings, and writes destructors.

```
relation Hint Parent[:p_label] (owns | refs) [Child[:c_label]]
```

- `Hint` ∈ `ArrayList | DoublyLinked | HashedList`.
- `owns`: cascade-destroy children when parent destroyed.
- `refs`: unlink children only (children survive).
- **Labels** prefix the injected field/method names. With no labels,
  members inject flat into the class (only one unlabeled relation per
  `(P, C)` pair is allowed).

```lyric
class Team { name: string }
class Player { name: string }

relation ArrayList Team:roster owns [Player:team]
```

Injected: `Team.roster_children: [Player]`, `Player.team_parent: Team?`, `Player.team_index: i32`.
Functions: `array_append<Team, Player>(t, p)`, `array_remove<Team, Player>(p)`.
Methods: `team.roster_append(p)`, `team.roster_remove(p)`.

| Hint | Functions | Notes |
|---|---|---|
| `ArrayList` | `array_append`, `array_remove` | O(1) swap-remove. Both `owns` and `refs`. |
| `DoublyLinked` | `dll_append`, `dll_remove` | Intrusive doubly-linked list. Both `owns` and `refs`. |
| `HashedList` | `hash_insert`, `hash_lookup`, `hash_remove` | Open-addressing, 75% load factor; child must implement `hash_key(self) -> u64`. Both `owns` and `refs`. |

`owns` / `refs` is orthogonal to the hint name — every hint supports both
modifiers, with the modifier selecting the destructor pair the desugar
pass copies onto the concrete classes (cascade vs unlink). The old
`OwningList` / `RefList` hint names are gone; use `DoublyLinked owns` /
`DoublyLinked refs` instead.

Relations support generic class participants (`relation HashedList Dict<K, V>:d owns [DictEntry<K, V>:d]`).

## 13. Functions

```lyric
func add(a: i32, b: i32) -> i32 { return a + b }

pub func public_fn() { ... }

// generic
func identity<T>(x: T) -> T { return x }

// external method (multi-class interface pattern)
func T.method(self) -> i32 { ... }

// lambda — two equivalent syntaxes
let f = (x: i32) -> i32 { return x * 2 }
let g = |x: i32| -> i32 { x * 2 }
```

`func` declares functions. `fn` is the canonical type-syntax keyword (`fn(i32) -> bool`); `func(...) -> R` and `T -> U` (single-arg) also parse.

Lambdas may not be generic. Lambdas capture variables from their enclosing scope.

### Mutable parameters
```lyric
struct Point { x: i32, y: i32 }
func translate(mut p: Point, dx: i32, dy: i32) { p.x = p.x + dx }

let mut pt = Point { x: 10, y: 20 }
translate(mut pt, 5, 0)        // `mut` required on both sides
translate(mut points[0], 1, 0) // slice elements OK
```

Rules:
- `mut` required on both the parameter and the call site (Swift `inout` pattern).
- Don't use `mut` on class parameters — classes are already references, and `mut` creates double-pointer segfaults.
- `mut self` is accepted but redundant; mutation through `self` is always allowed.

### Field annotations
```lyric
class Counter {
    count: i32 guarded_by(mu)
    mu: lock
}
```
`guarded_by(name)` declares thread-safety intent. It is parsed and stored; runtime enforcement is on the roadmap.

## 14. Optional and Error Handling

### Optional ops
```lyric
let v: i32 = opt!              // unwrap; panics if null
if !isnull(result) { ... }     // explicit null test
if result != null { ... }      // also works (equivalent)
```

Implicit wrapping: `T` is assignable to `T?` without conversion. Empty `[]`, empty `{}`, and `null` infer their type from context (variable type, function param type, field type, return type).

### `(T, error)` and `?`
Go-style. Functions that can fail return `(T, error)`:

```lyric
func divide(a: i32, b: i32) -> (i32, error) {
    if b == 0 { return (0, Error { msg: "division by zero" }) }
    return (a / b, null)
}

func compute(x: i32) -> (i32, error) {
    let r = divide(x, 2)?         // propagates error on failure; r is i32
    return (r, null)
}

let (val, err) = divide(10, 2)    // explicit destructure also works
```

Rules for `?`: operand returns `(T, error)`; enclosing function returns `(..., error)`; statement-level only.

There is **no** `try` / `catch` / `throw`. `?` is the "try operator" in the Rust sense.

## 15. Union Types

```lyric
let value: string | i32 = 42

func process(x: string | i32) -> string {
    match x {
        string => { "it's a string" }
        i32 => { "it's an int" }
    }
}
```

Any member type is assignable to the union; a union is assignable to another type only if all variants match.

## 16. Control Flow

```lyric
if cond { ... } else if cond2 { ... } else { ... }
let r = if cond { a } else { b }           // expression form requires else

while cond { ... }
for item in collection { ... }
for item, idx in collection { ... }
```

Block scoping: any `{ ... }` creates a new scope. `let x = 1; { let x = 2; ... }; ...` works.

## 17. Concurrency

```lyric
spawn { do_work() }

let ch: channel<i32> = make_channel<i32>()           // unbuffered
let ch2 = make_channel<i32>(10)                       // buffered
ch.send(42)
let v = ch.receive()                                  // also: ch.recv()
ch.close()

select {
    case v = ch.receive() => { use(v) }
    case ch2.send(99)     => { ... }
    default               => { ... }
}

let mu: lock
lock(mu) { critical_section() }                       // scoped
```

C backend uses pthreads. **Known limitations** (see roadmap):
- `spawn` captures variables **by pointer** → races possible. Use channels for safe sharing.
- `select` uses a `usleep(100)` polling loop — not real wait/notify.
- `receive()` on a closed channel returns the zero value with no `(value, ok)` signal.

## 18. Generators

```lyric
func range(start: i32, end: i32) -> gen i32 {
    let mut i = start
    while i < end { yield i; i = i + 1 }
}

for val in range(0, 10) { println(val) }
```

The stdlib provides `range(start, end) -> gen i32`. C backend uses a Duff's-device state machine.

## 19. Memory Model

- **Primitives** — registers/stack.
- **Structs / tuples** — stack, copied on assign. `mut` params pass by reference (scoped).
- **Classes** — slab-allocated. Two modes: **AoS** (default, pointer handles) and **SoA** (`--soa` flag, `u32` handles + parallel arrays). Owned classes destroyed via their owning relation; unowned classes are refcounted.
- **Slices `[T]`** — fat pointer. Assignment copies the header; backing storage shared. `let ref` creates a zero-copy view.
- **Relations** — ownership graph. `owns` cascades destroy; `refs` only unlinks.
- **Scope-exit freeing** — escape analysis injects free calls for locally-allocated slices and non-escaping classes.
- **`final`** — pre-destroy hook on classes.
- **`trusted`** — functions / blocks where manual `ref(x)` and `unref(x)` are allowed.
- **`permanent`** — class never freed, never refcounted. Pairing with a relation produces a compiler warning.
- **Inferred moves** — single-use locals transfer ownership instead of retain+release.

There is no GC, no borrow checker, and no lifetime annotations.

## 20. Built-in Functions

| Function | Signature | |
|---|---|---|
| `len(x)` | `[T] \| string -> i32` | |
| `append(xs, e)` | `([T], T) -> [T]` | new slice |
| `push_bytes(dst, src)` | `(mut string, string) -> unit` | in-place byte append |
| `print`, `println`, `eprint`, `eprintln` | `(any...) -> unit` | auto `to_string` |
| `panic(msg)` | `string -> unit` | aborts |
| `exit(code)` | `i32 -> unit` | process exit |
| `isnull(x)` | `T? -> bool` | |
| `assert(cond, msg)` | `(bool, string) -> unit` | injects file/line — see §22 |
| `assert_eq(a, b, msg?)` | `(T, T, string?) -> unit` | message optional |
| `to_string(x)` | `any -> string` | auto-generated for every type |
| `format(fmt, args...)` | | f-string-like |
| `sym(s)` | `string -> Sym` | also `` `name` `` |
| `hash_string(s)` | `string -> u64` | FNV-1a |
| `itoa(n) atoi(s)` | | int↔string; atoi returns `(i64, bool)` |
| `parse_float(s)` | `string -> (f64, bool)` | |
| `char_to_string(b)` | `u8 -> string` | |
| `string_to_bytes(s)`, `bytes_to_string(b)` | | byte-slice / string conversion |
| `new_string_builder()` | `-> StringBuilder` | |
| `new_error(msg)` | `string -> Error` | shortcut |
| `make_channel<T>()` / `make_channel<T>(n)` | | unbuffered / buffered |

### Character predicates
`char_is_digit`, `char_is_alpha`, `char_is_space`, `char_is_upper`, `char_is_lower`, `char_is_alnum` — all `u8 -> bool`.

### Files & OS
`read_file(path)` → `(string, bool)`, `write_file(path, content)` → `bool`, `os_args()`, `os_exit(code)`, `os_getwd()`, `exec_command(name, args)` → `(string, bool)`, `path_join(parts)`, `path_dir(p)`, `path_base(p)`, `path_ext(p)`, `list_dir(path)` → `([string], bool)`, `file_exists(path)`, `mkdtemp()`.

### Relation functions (manually callable in `trusted` contexts)
`array_append`, `array_remove`, `dll_append`, `dll_remove`, `hash_insert`, `hash_lookup`, `hash_remove`. (`hash_init`, `hash_rehash`, `hash_find_slot` exist but are internal.)

## 21. Stdlib Classes

| Class | Notes |
|---|---|
| `Sym` | Interned symbol with pre-computed FNV-1a hash. `sym("name")` or `` `name` ``. `get_name() -> string`, `get_hash() -> u64`. Implements `Hashable`. |
| `Dict<K, V>` | Hash table where `K: Hashable`. Constructor `Dict<K,V>()`. Methods: `set(k, v)`, `get(k) -> DictEntry<K,V>?`, `has(k) -> bool`, `remove(k) -> bool`, `keys() -> [K]`, `len()` / `length()`. |
| `DictEntry<K, V>` | Fields `key: K`, `value: V`. |
| `Hashable` | Interface with `get_hash(self) -> u64`. (Note: `equals(self, other) -> bool` is on the roadmap.) |
| `Comparable`, `Equatable` | Built-in constraints — see §11. |
| `Error` | `{ msg: string }`. `message() -> string`. |
| `StringBuilder` | `write(s)`, `write_byte(b)`, `to_string()`, `len()`. Create via `new_string_builder()`. |

## 22. Testing

Built into the compiler. No frameworks, no fixtures, no mocks.

- Any function whose name starts with `test_` is a test. No arguments, no return.
- `assert(cond, msg)` and `assert_eq(actual, expected, msg?)` are compiler-provided builtins. They inject file and line into failure messages and exit the test on failure.
- `assert_eq` displays both values via auto-generated `to_string`. Enum variants are shown by name; structs and classes dump fields (`Point{x: 1.0, y: 2.0}`).

```lyric
func test_lexer_basic() {
    let lex = Lexer { source: "let x = 42" }
    let tok = lex.next()
    assert_eq(tok.kind, TLet, "first token should be let")
}
```

Run:
```
lyric test test_lexer.ly lexer.ly ast.ly
```

Output:
```
PASS  test_lexer_keywords
FAIL  test_lexer_escapes
  assert_eq failed at test_lexer.ly:47
    expected: TStringLit
    got:      TError

N tests, P passed, F failed
```

Tests run sequentially in declaration order. A failed assertion stops that one test; the suite continues. Exit code 0/1.

## 23. Toolchain

```
lyric compile <file.ly | dir>  [-o out] [flags]
lyric test    <files.ly...>     [-o out] [flags]
lyric fmt     <files.lyric...>
lyric help
```

Subcommand resolution accepts unique prefixes (`lyric c` = compile).

### Compile flags
| Flag | Effect |
|---|---|
| `-o name` | Output binary name |
| `--soa` | Struct-of-Arrays slab layout |
| `--detect-uaf` | Mark freed slots; check on every access (debug builds) |
| `--rc-free` | Auto-destroy on ref-count zero (default on) |
| `--no-rc` | Disable auto-destruction; only decrement |
| `--lir-dump path` | Write LIR text dump |

### Module mode
If the target directory contains a `lyric.mod` marker, all `.ly` files in that directory's **top level** are compiled together. `lyric.mod`'s content is reserved. Recursive subdirectory discovery and recursive import resolution are on the roadmap.

### `lyric fmt`
Reformats the human-reviewed zone of a `.lyric` file. Zones 2 & 3 (function index / dependencies) are preserved verbatim. (Generating zones 2 & 3 lives in `lyre`, not in the Lyric compiler.)

## 24. Operator Precedence

Lowest first:
```
 1  ||         (or)
 2  &&         (and)
 3  |          (bitwise OR)    — same level as C
 4  ^          (bitwise XOR)
 5  &          (bitwise AND)
 6  == !=
 7  < <= > >=
 8  << >>
 9  + -
10  * / %
11  - !        (unary)
12  . () [] ! ? is as   (postfix)
```

Compound assigns: `+= -= *= /=`. Bitwise compound assigns (`%= &= |= ^= <<= >>=`) and unary `~` are on the roadmap; bitwise-op precedence is scheduled to move above `==`/`<` (above all non-integer operators) — see roadmap.

## 25. Imports

```lyric
import ast                          // package in directory ./ast/
import v2 from "parser/v2"          // aliased nested path
```

Three syntactic forms parse: `import name`, `import alias from "path"`, `import "path"` (no alias — known crasher today; avoid).

**Current limitations** (roadmap):
- Single level only — imports of imports are not resolved.
- `pub` visibility is not enforced — all imported declarations are accessible.
- No cycle detection.

## 26. Known Gotchas

- **`null`** is the only null literal; `nil` is not accepted.
- **Cross-sign integer assignment** is currently implicit and lossy. Use explicit `as` for narrowing.
- **`null` is currently assignable to any type** (should be tightened to `T?`/class/interface/error).
- **`as` casts accept any target type** today (not just numeric ↔ numeric); narrow uses of this are recommended.
- **`implements I`** is declarative — method completeness is not yet checker-enforced.
- **`if`/`match` as expressions** do not yet unify branch types — the checker takes the first branch's type.
- **Structs are value types** — read → modify → write back when modifying nested struct fields.
- **`string` is NOT `Hashable`** — wrap with `sym()` for hash keys.
- **`mut` on class parameters** — don't. Classes are already references.
- **`fn` vs `func`** — both work; `fn` is preferred for type syntax.
- **Tuple field access** — `t._0`, `t._1` (underscore-prefixed).
- **`recv`** is a synonym for channel `receive()`.
- **`length()`** is a synonym for `len()` on Dict and slices.
- **C identifier collisions** — the C backend appends `_` to identifiers colliding with C keywords. Mostly invisible at the Lyric level.
- **`map[K]V`** parses but compiles to a stub today. Use `Dict<K,V>`.

## 27. Self-Hosting Status

Lyric is **self-hosting** as of June 2026. The bootstrap compiler is written in Lyric (~30K lines across `src/`) and compiles itself to a fixed point. The runtime header (`runtime/lyric_runtime.h`) provides slab allocators, strings, channels, and the test-harness macros.

**For language design changes**: read the spec first (`lyric-language-spec.md`), this reference, and the roadmap (`cr/docs/bootstrap-roadmap.md`). The language is designed for AI-assisted compiler engineering — every feature exists because it earns its keep in the bootstrap source.
