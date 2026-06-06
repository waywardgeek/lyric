# Pre-Bootstrap Sprint

Everything needed before we can start porting the Forge compiler to Forge.

## 1. Strings as `[u8]`

**Current**: `string` is `char*` in C backend, `string` in Go backend. String indexing returns `string`. Strings are opaque — can't iterate bytes, can't write string algorithms in Forge.

**Change**: `type string = [u8]`. String becomes a slice of bytes.

### What This Enables
- `s[i]` returns `u8`
- `s[0:5]` slices work
- `len(s)` returns byte count
- `append(s, byte)` works
- `for c in s` iterates bytes
- String library writable in pure Forge
- Lexer can be pure Forge (character-by-character scanning)

### Implementation

**Type system**: Register `string` as a type alias for `[u8]` in the checker. `TypeString` still exists as a convenience but resolves to `Sequence(U8)` during type checking.

**String literals**: The lowerer emits `[u8]` slice initialization instead of a string value. In LIR, a string literal becomes an `LExprBuiltin("str_from_lit", [LValLitStr])` or similar, since we need the C backend to call a runtime helper.

**C backend**: `string` → `forge_slice_u8`. String literals emit `forge_str_lit("hello")` which copies bytes into a heap-allocated slice. Add to `forge_runtime.h`:

```c
static inline forge_slice_u8 forge_str_lit(const char* s) {
    size_t n = strlen(s);
    forge_slice_u8 r;
    r.data = (uint8_t*)malloc(n);
    r.len = (int32_t)n;
    r.cap = (int32_t)n;
    memcpy(r.data, (const uint8_t*)s, n);
    return r;
}

// Null-terminate for C interop (printf, fopen, etc.)
// Caller must free the result.
static inline char* forge_str_to_cstr(forge_slice_u8 s) {
    char* r = (char*)malloc(s.len + 1);
    memcpy(r, s.data, s.len);
    r[s.len] = '\0';
    return r;
}
```

**Go backend**: Strings stay as Go `string`. Add conversion helpers: `[]byte(s)` / `string(b)` at boundaries. Since Go strings are already UTF-8 byte sequences, this is mostly transparent.

**F-strings**: `f"hello {name}"` — each literal part becomes a `[u8]`, interpolated values get `to_string()` or format builtin, then all parts are concatenated. The lowerer already emits `LExprFormat` with parts — the C backend needs to build a `forge_slice_u8` instead of `char*`.

**Null terminator**: NOT stored in the slice. Pure byte count. The `forge_str_to_cstr()` bridge adds `\0` when passing to C functions (file I/O, printf, exec).

### Migration
- Existing `.fg` test files using `string` keep working — it's the same type, just now a slice alias.
- `hash_string` builtin stays (operates on `[u8]` now — same bytes).
- `print`/`println` need to accept `[u8]` — C backend uses `fwrite(s.data, 1, s.len, stdout)` instead of `printf("%s", s)`.

## 2. Character Literals

**Syntax**: `'c'` for any ASCII printable character. Evaluates to a `u8` constant (the ASCII value).

**Escape sequences** (same set for both character literals and string literals):
- `'\n'` — newline (0x0A)
- `'\r'` — carriage return (0x0D)
- `'\t'` — tab (0x09)
- `'\\'` — backslash
- `'\''` — single quote (in char literals)
- `'\"'` — double quote (in string literals, already works)
- `'\0'` — null byte (0x00)
- `'\x41'` — hex byte (two hex digits, 0x00–0xFF)

Character literals are NOT a new type — they're `u8` constants. `'A'` is `65u8`.

### Implementation

**Lexer**: When `peek() == '\''`, scan a character literal. Handle escape sequences. Emit a new token `TCharLit` with the byte value stored as the token text (or as an integer).

**Parser**: `TCharLit` → `ExprKind.IntLit` with value set to the ASCII byte value, type annotated as `u8`. Alternatively, add `ExprKind.CharLit` if we want to preserve source fidelity for `forge fmt`.

**Checker**: If we use `IntLit`, no changes needed (already typed). If `CharLit`, resolve to `u8`.

**Lowerer**: `CharLit` → `LValLitInt` with the byte value, type `LTyUint{Bits: 8}`.

**C/Go backends**: Emit the integer constant. `'A'` → `65` in C, `uint8(65)` in Go.

**String literal escape sequences**: The lexer already handles `\"` and `\\` in strings. Extend `scanString()` to also handle `\n`, `\r`, `\t`, `\0`, `\x##`. Store the actual byte values in the string token, not the escape text.

**String literals contain arbitrary UTF-8**: Since strings are `[u8]`, a string literal `"café"` stores the raw UTF-8 bytes (`[99, 97, 102, 195, 169]`). No special handling needed — the lexer already reads UTF-8 source, we just preserve the bytes.

## 3. String Standard Library (`stdlib/string.fg`)

Written in pure Forge once strings are `[u8]`. All functions operate on byte slices.

```
forge string_lib {
  // Comparison
  pub func str_eq(a: string, b: string) -> bool
  pub func str_cmp(a: string, b: string) -> i32    // -1, 0, 1

  // Search
  pub func contains(haystack: string, needle: string) -> bool
  pub func has_prefix(s: string, prefix: string) -> bool
  pub func has_suffix(s: string, suffix: string) -> bool
  pub func index_of(haystack: string, needle: string) -> i32   // -1 if not found

  // Manipulation
  pub func substring(s: string, start: i32, end: i32) -> string  // alias for s[start:end]
  pub func trim_space(s: string) -> string
  pub func to_upper(s: string) -> string    // ASCII only
  pub func to_lower(s: string) -> string    // ASCII only
  pub func replace(s: string, old: string, new_str: string) -> string
  pub func repeat(s: string, count: i32) -> string

  // Split/Join
  pub func split(s: string, sep: string) -> [string]
  pub func join(parts: [string], sep: string) -> string

  // Conversion
  pub func itoa(n: i64) -> string
  pub func atoi(s: string) -> i64    // or (i64, bool) for error
  pub func u64_to_string(n: u64) -> string

  // Character classification (ASCII)
  pub func is_letter(c: u8) -> bool     // A-Z, a-z
  pub func is_digit(c: u8) -> bool      // 0-9
  pub func is_space(c: u8) -> bool      // space, tab, newline, CR
  pub func is_alnum(c: u8) -> bool      // letter or digit
}
```

These replace the Go `strings`, `strconv`, and `unicode` packages the compiler currently uses.

## 4. File I/O and OS Builtins

The compiler needs to read source files, write output, invoke `cc`, and parse CLI args. These can't be written in Forge — they need C syscall bridges.

### Approach: Builtins (not `extern`)

Register these as builtins in checker + lowerer + C backend, like `len`, `append`, `hash_string`. Simpler than building an `extern` FFI system pre-bootstrap.

```
// File I/O
pub func read_file(path: string) -> (string, bool)     // contents, ok
pub func write_file(path: string, data: string) -> bool // ok

// Console I/O
pub func print(s: string)
pub func println(s: string)
pub func eprint(s: string)
pub func eprintln(s: string)

// OS
pub func os_args() -> [string]
pub func os_exit(code: i32)
pub func os_getwd() -> string

// Process execution (for invoking cc)
pub func exec_command(program: string, args: [string]) -> (string, bool)  // output, ok

// Path manipulation (simple builtins, no need for full filepath package)
pub func path_join(parts: [string]) -> string
pub func path_dir(path: string) -> string
pub func path_base(path: string) -> string
pub func path_ext(path: string) -> string
```

### C Backend Implementation

Each builtin maps to a C function in `forge_runtime.h`. Examples:

```c
static inline forge_result_slice_u8_bool forge_read_file(forge_slice_u8 path) {
    char* cpath = forge_str_to_cstr(path);
    FILE* f = fopen(cpath, "rb");
    free(cpath);
    if (!f) return (forge_result_slice_u8_bool){ .val = {0}, .err = false };
    fseek(f, 0, SEEK_END);
    long n = ftell(f);
    fseek(f, 0, SEEK_SET);
    uint8_t* buf = malloc(n);
    fread(buf, 1, n, f);
    fclose(f);
    forge_slice_u8 s = { buf, (int32_t)n, (int32_t)n };
    return (forge_result_slice_u8_bool){ .val = s, .err = true };
}
```

## 5. Missing Language Features

### 5a. `==` Deep Value Comparison (Rune Semantics)

Following Rune: `==` performs deep recursive value comparison for structs, tuples, and slices (including `[u8]` strings). Class references compare by pointer identity. Functions compare by pointer.

This means `s1 == s2` for strings just works — element-wise byte comparison. No special `str_eq()` needed.

**Implementation**: The C backend currently emits `==` directly (works for scalars and pointers). For structs, slices, and tuples, emit a generated comparison function or inline loop. For `[u8]`: compare lengths, then `memcmp`. For nested structures: recursive field-by-field comparison.

### 5b. Tagged Union / `any` Type

The AST and LIR use `Data any` with type assertions everywhere. The Go compiler has `dataAs[T](data any) *T`. For Forge:

**`any` is just an empty interface** — same as Go. Forge already has interfaces and the C backend has vtable dispatch. In C, `any` = `void*`. Type assertions via match on concrete types (the C backend already handles `ForgeUnion` with type tags for union types).

No new language machinery needed — just register `any` as a built-in empty interface in the checker.

### 5c. Error Model

Forge already has `(T, error)` tuple returns and `?` operator. For the bootstrap, add a concrete `Error` class to stdlib:

```forge
class Error(msg: string) {
    pub func message(self) -> string { return self.msg }
}
```

Compiler code uses `Error(f"unexpected token {tok}")` to create errors, and `?` to propagate them. No language changes needed.

### 5d. StringBuilder

The compiler builds strings extensively (C backend output). Current Go code uses `fmt.Sprintf` and `strings.Builder`. Options:

- **StringBuilder class**: `class StringBuilder()` with `write(s: string)`, `write_byte(b: u8)`, `to_string() -> string`. Backed by `[u8]` with amortized growth. Can be written in pure Forge.
- **F-strings**: Already have `f"..."` — covers most formatting cases.

**Decision**: Add `StringBuilder` to stdlib. F-strings for simple cases, StringBuilder for the C backend's heavy string building.

## 6. Summary: Implementation Order

1. **String escape sequences** in lexer (both string and char literals) — ~50 LOC
2. **Character literals** (`'c'` → `u8`) in lexer/parser/checker — ~100 LOC
3. **String as `[u8]`** — type alias, C backend changes, runtime helpers — ~300 LOC
4. **Deep `==` for composite types** — C backend emits memcmp/field-wise comparison — ~150 LOC
5. **String stdlib** (`stdlib/string.fg`) — pure Forge — ~200 LOC
6. **OS/IO builtins** — checker + lowerer + C backend + runtime — ~400 LOC
7. **`any` type** — checker + lowerer + C backend (boxing/unboxing) — ~300 LOC
8. **StringBuilder** in stdlib — pure Forge — ~50 LOC

**Total: ~1,550 lines of changes**, then we're ready to start porting.

## 7. Open Questions for Bill

1. **Path separator**: Hardcode `/` (Unix only for bootstrap). ✅ Decided.
