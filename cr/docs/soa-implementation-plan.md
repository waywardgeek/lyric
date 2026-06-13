# SoA Memory Layout Implementation Plan

## Goal
Add `--soa` compiler flag. Default stays AoS (current behavior). When `--soa` is passed, class instances use Structure-of-Arrays layout with `uint32_t` index handles instead of `ClassName*` pointers.

## Current State (AoS)
- Classes use `ClassName*` pointers (NULL = none)
- Block-based allocation: linked list of fixed-size blocks (LYRIC_SLAB_BLOCK = 256)
- `lyric_next` field appended to each struct for free-list linkage
- `memory.ly` rewrites ExClassAlloc → ExSlabAlloc, field inits → StSlabSet
- C backend emits slab infrastructure, alloc/free functions, `ptr->field` access
- 76/76 tests pass, self-compile fixed point ✅

## Design: SoA Layout

### Handles
- `uint32_t` indices: 0 = null, valid ≥ 1, array index = handle - 1
- All class references become `uint32_t` in generated C
- Optional class → `uint32_t` (0 = none, no wrapper needed — 0 IS the null sentinel)
- NULL checks → `handle == 0` checks

### Slab Structure (SoA)
```c
typedef struct {
    int32_t*  field1;      // parallel array per field
    uint8_t*  field2;
    uint32_t* lyric_next;  // free-list chain
    uint32_t  used;        // high-water mark
    uint32_t  cap;         // allocated capacity
    uint32_t  free_head;   // head of free list (0 = empty)
} LyricSlab_Counter;

static LyricSlab_Counter _lyric_slab_Counter = {0};
```

### Field Access (SoA)
```c
// AoS: ptr->field
// SoA: _lyric_slab_Counter.field[handle - 1]
```

### Alloc (SoA)
```c
static uint32_t _lyric_slab_alloc_Counter(void) {
    if (_lyric_slab_Counter.free_head) {
        uint32_t h = _lyric_slab_Counter.free_head;
        _lyric_slab_Counter.free_head = _lyric_slab_Counter.lyric_next[h - 1];
        // Zero all field arrays at index h-1
        return h;
    }
    if (_lyric_slab_Counter.used == _lyric_slab_Counter.cap) {
        uint32_t new_cap = _lyric_slab_Counter.cap ? _lyric_slab_Counter.cap * 2 : 8;
        // realloc each field array individually
        _lyric_slab_Counter.field1 = realloc(..., new_cap * sizeof(int32_t));
        _lyric_slab_Counter.field2 = realloc(..., new_cap * sizeof(uint8_t));
        _lyric_slab_Counter.lyric_next = realloc(..., new_cap * sizeof(uint32_t));
        // Zero new capacity
        _lyric_slab_Counter.cap = new_cap;
    }
    return ++_lyric_slab_Counter.used;
}
```

### Free (SoA)
```c
static void _lyric_slab_free_Counter(uint32_t h) {
    if (!h) return;
    _lyric_slab_Counter.lyric_next[h - 1] = _lyric_slab_Counter.free_head;
    _lyric_slab_Counter.free_head = h;
}
```

## Implementation Steps

### Step 1: CLI Flag + LProgram Field
- Add `--soa` flag to `cmd_compile` and `cmd_test` in `main.ly`
- Set `prog.slab_mode_soa = true` (new bool on LProgram, alongside existing `slab_mode`)
- Both AoS and SoA set `slab_mode = true`; `slab_mode_soa` distinguishes layout

### Step 2: LIR Type Changes
- No new LIR nodes needed — ExSlabAlloc, ExSlabGet, StSlabSet, StSlabFree already exist
- The C backend decides AoS vs SoA emission based on `prog.slab_mode_soa`
- `c_type()` must emit `uint32_t` for TyClassHandle when `slab_mode_soa`
- Optional class types need no wrapper in SoA (0 is already null)

### Step 3: C Backend — `c_type()` Changes
- When `slab_mode_soa`: `TyClassHandle` → `uint32_t` (not `ClassName*`)
- When `slab_mode_soa`: `TyOptional(TyClassHandle)` → `uint32_t` (0 = none)
- `zero_value()`: class handle → `0` (not `NULL`)

### Step 4: C Backend — `emit_slab_infrastructure()` SoA Path
- Emit per-field parallel arrays instead of block-based struct arrays
- Emit `uint32_t`-based alloc (realloc per field) and free functions
- No `lyric_next` struct field — it's a parallel array in the slab

### Step 5: C Backend — Field Access
- `ExSlabGet` / `ExStructField` on class receiver:
  - AoS: `ptr->field`
  - SoA: `_lyric_slab_ClassName.field[handle - 1]`
- `StSlabSet`:
  - AoS: `ptr->field = val`
  - SoA: `_lyric_slab_ClassName.field[handle - 1] = val`
- `ValClassFieldRef` (mutating method on class field):
  - AoS: `ptr->field` (lvalue)
  - SoA: `_lyric_slab_ClassName.field[handle - 1]` (lvalue)

### Step 6: C Backend — Null Checks
- `ExIsNull` on class handle:
  - AoS: `ptr == NULL`
  - SoA: `handle == 0`
- `ExWrapOptional` on class:
  - AoS: pass pointer through (non-null = some)
  - SoA: pass handle through (non-zero = some)
- `ExUnwrapOptional` on class:
  - AoS: just use the pointer
  - SoA: just use the handle (it's already `uint32_t`)

### Step 7: C Backend — `emit_class_decl()` SoA Path
- In SoA mode, classes do NOT emit a C struct at all (fields are in slab arrays)
- But we still need forward declarations for type resolution
- Option: emit `typedef uint32_t ClassName;` as a handle alias

### Step 8: C Backend — to_string, destroy, method receivers
- `self` parameter type: `uint32_t` instead of `ClassName*`
- Field access in methods: `_lyric_slab_ClassName.field[self - 1]` 
- `to_string` functions: use slab access

### Step 9: Tests
- All 76 existing tests must pass under both `--soa` and default (AoS)
- Add `soa_test.ly` that specifically tests SoA-sensitive patterns
- `test_lyric.sh` runs tests in AoS mode (default); add `test_lyric.sh --soa` mode
- Self-compile with `--soa` would be the ultimate validation but is not required initially

### Step 10: Verify Self-Compile Fixed Point
- Default (AoS) fixed point must still hold
- SoA fixed point: compile compiler with `--soa`, verify it produces same output

## Risk Assessment

### Known Hard Parts
1. **Optional class flattening** — `TyOptional(TyClassHandle)` becomes just `uint32_t` in SoA. The unwrap/wrap/isnull emit paths all need updating.
2. **ExStructField on class receivers** — the C backend uses this for both struct and class field access. Must detect class receiver and emit slab access in SoA mode.
3. **Method receiver types** — `self: ClassName` becomes `self: uint32_t` in SoA. All method calls pass handle, not pointer.
4. **Slices of classes** — `[ClassName]` stays as `LyricSlice_uint32_t` in SoA. Slice push/pop work on handles.

### What's Easier This Time
- LIR dump (`--lir-dump`) available for debugging
- memory.ly rewrite pass already works — no changes needed there
- All the LIR node types already exist
- Clean 76/76 baseline with fixed point

## Estimated Scope
- ~200-400 lines changes in `c_backend.ly` (new SoA emission paths, conditional logic)
- ~10 lines in `main.ly` (CLI flag)
- ~5 lines in `lir.ly` (new bool field)
- ~0 lines in `memory.ly` (no changes — it already produces the right LIR nodes)
- ~50 lines new test (`soa_test.ly`)
