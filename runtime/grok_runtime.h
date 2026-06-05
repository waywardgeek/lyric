/* grok_runtime.h — Minimal C runtime for Grok-compiled programs.
 *
 * Provides macros for:
 *   - Dynamic slices (GROK_SLICE_DEF, grok_push, grok_pop, grok_slice_lit)
 *   - Optionals (GROK_OPT_DEF, grok_some, grok_none, grok_isnull)
 *   - Error results (GROK_RESULT_DEF, grok_ok, grok_err)
 *   - String helpers (grok_contains, grok_index_of, grok_replace, grok_join, etc.)
 *   - Formatting (grok_sprintf)
 */

#ifndef GROK_RUNTIME_H
#define GROK_RUNTIME_H

#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>
#include <stdbool.h>
#include <string.h>

/* -------------------------------------------------------------------------
 * Dynamic Slices
 * -------------------------------------------------------------------------
 * Usage:
 *   GROK_SLICE_DEF(int32_t, GrokSlice_int32_t)
 *   GrokSlice_int32_t xs = grok_slice_empty(GrokSlice_int32_t);
 *   grok_push(&xs, 42, GrokSlice_int32_t);
 *   int32_t val = xs.data[0];
 */

#define GROK_SLICE_DEF(ElemType, SliceName) \
    typedef struct { ElemType* data; int32_t len; int32_t cap; } SliceName;

/* Create an empty slice */
#define grok_slice_empty(SliceName) ((SliceName){.data = NULL, .len = 0, .cap = 0})

/* Push an element (grows by 2x when full) */
#define grok_push(slice_ptr, elem, SliceName) do { \
    if ((slice_ptr)->len >= (slice_ptr)->cap) { \
        int32_t _newcap = (slice_ptr)->cap == 0 ? 4 : (slice_ptr)->cap * 2; \
        (slice_ptr)->data = realloc((slice_ptr)->data, sizeof(*(slice_ptr)->data) * _newcap); \
        (slice_ptr)->cap = _newcap; \
    } \
    (slice_ptr)->data[(slice_ptr)->len++] = (elem); \
} while(0)

/* Pop the last element (returns it). Caller must check len > 0. */
#define grok_pop(slice_ptr) ((slice_ptr)->data[--(slice_ptr)->len])

/* Create a slice from an initializer list */
#define grok_slice_lit(SliceName, ElemType, ...) ({ \
    ElemType _init[] = {__VA_ARGS__}; \
    int32_t _n = sizeof(_init) / sizeof(_init[0]); \
    SliceName _s; \
    _s.data = malloc(sizeof(ElemType) * _n); \
    memcpy(_s.data, _init, sizeof(ElemType) * _n); \
    _s.len = _n; \
    _s.cap = _n; \
    _s; \
})

/* Slice contains an element (linear scan) */
#define grok_contains(slice, elem) ({ \
    bool _found = false; \
    for (int32_t _i = 0; _i < (slice).len; _i++) { \
        if ((slice).data[_i] == (elem)) { _found = true; break; } \
    } \
    _found; \
})

/* -------------------------------------------------------------------------
 * Optionals  —  {bool has; T val}
 * -------------------------------------------------------------------------
 * Usage:
 *   GROK_OPT_DEF(int32_t, GrokOpt_int32_t)
 *   GrokOpt_int32_t x = grok_some(42, GrokOpt_int32_t);
 *   GrokOpt_int32_t y = grok_none(GrokOpt_int32_t);
 *   if (!grok_isnull(x)) { use(x.val); }
 */

#define GROK_OPT_DEF(ElemType, OptName) \
    typedef struct { bool has; ElemType val; } OptName;

#define grok_some(value, OptName) ((OptName){.has = true, .val = (value)})
#define grok_none(OptName) ((OptName){.has = false})
#define grok_isnull(opt) (!(opt).has)
#define grok_unwrap(opt) ((opt).val)

/* -------------------------------------------------------------------------
 * Error Results  —  {bool is_err; T value; const char* error}
 * -------------------------------------------------------------------------
 * Usage:
 *   GROK_RESULT_DEF(int32_t, GrokResult_int32_t)
 *   GrokResult_int32_t r = grok_ok(42, GrokResult_int32_t);
 *   GrokResult_int32_t e = grok_err("failed", GrokResult_int32_t);
 */

#define GROK_RESULT_DEF(ElemType, ResultName) \
    typedef struct { bool is_err; ElemType value; const char* error; } ResultName;

#define grok_ok(val, ResultName) ((ResultName){.is_err = false, .value = (val), .error = NULL})
#define grok_err(msg, ResultName) ((ResultName){.is_err = true, .error = (msg)})
#define grok_is_err(r) ((r).is_err)

/* -------------------------------------------------------------------------
 * String helpers
 * -------------------------------------------------------------------------
 * These return heap-allocated strings where needed. The Grok C runtime
 * does not yet have a GC — these leak. That's fine for now.
 */

static inline bool grok_str_contains(const char* s, const char* sub) {
    return strstr(s, sub) != NULL;
}

static inline int32_t grok_str_index_of(const char* s, const char* sub) {
    const char* p = strstr(s, sub);
    if (p == NULL) return -1;
    return (int32_t)(p - s);
}

static inline bool grok_str_has_prefix(const char* s, const char* prefix) {
    return strncmp(s, prefix, strlen(prefix)) == 0;
}

static inline bool grok_str_has_suffix(const char* s, const char* suffix) {
    size_t slen = strlen(s), suflen = strlen(suffix);
    if (slen < suflen) return false;
    return strcmp(s + slen - suflen, suffix) == 0;
}

static inline const char* grok_str_replace(const char* s, const char* old, const char* new_s) {
    const char* pos = strstr(s, old);
    if (!pos) {
        char* dup = malloc(strlen(s) + 1);
        strcpy(dup, s);
        return dup;
    }
    size_t oldlen = strlen(old), newlen = strlen(new_s), slen = strlen(s);
    /* Count occurrences */
    int count = 0;
    const char* p = s;
    while ((p = strstr(p, old)) != NULL) { count++; p += oldlen; }
    char* result = malloc(slen + count * (newlen - oldlen) + 1);
    char* dst = result;
    p = s;
    while ((pos = strstr(p, old)) != NULL) {
        memcpy(dst, p, pos - p);
        dst += pos - p;
        memcpy(dst, new_s, newlen);
        dst += newlen;
        p = pos + oldlen;
    }
    strcpy(dst, p);
    return result;
}

static inline const char* grok_str_repeat(const char* s, int32_t n) {
    size_t slen = strlen(s);
    char* result = malloc(slen * n + 1);
    result[0] = '\0';
    for (int32_t i = 0; i < n; i++) {
        strcat(result, s);
    }
    return result;
}

static inline const char* grok_str_join(const char* sep, const char** parts, int32_t count) {
    if (count == 0) {
        char* r = malloc(1);
        r[0] = '\0';
        return r;
    }
    size_t total = 0, seplen = strlen(sep);
    for (int32_t i = 0; i < count; i++) {
        total += strlen(parts[i]);
        if (i > 0) total += seplen;
    }
    char* result = malloc(total + 1);
    result[0] = '\0';
    for (int32_t i = 0; i < count; i++) {
        if (i > 0) strcat(result, sep);
        strcat(result, parts[i]);
    }
    return result;
}

/* grok_sprintf — heap-allocated formatted string */
static inline const char* grok_sprintf(const char* fmt, ...) {
    va_list args, args2;
    va_start(args, fmt);
    va_copy(args2, args);
    int n = vsnprintf(NULL, 0, fmt, args);
    va_end(args);
    char* buf = malloc(n + 1);
    vsnprintf(buf, n + 1, fmt, args2);
    va_end(args2);
    return buf;
}

/* Bool to string for printf */
static inline const char* grok_bool_str(bool b) {
    return b ? "true" : "false";
}

#endif /* GROK_RUNTIME_H */
