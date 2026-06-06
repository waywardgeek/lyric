/* forge_runtime.h — Minimal C runtime for Forge-compiled programs.
 *
 * Provides macros for:
 *   - Dynamic slices (FORGE_SLICE_DEF, forge_push, forge_pop, forge_slice_lit)
 *   - Optionals (FORGE_OPT_DEF, forge_some, forge_none, forge_isnull)
 *   - Error results (FORGE_RESULT_DEF, forge_ok, forge_err)
 *   - String helpers (forge_contains, forge_index_of, forge_replace, forge_join, etc.)
 *   - Formatting (forge_sprintf)
 */

#ifndef FORGE_RUNTIME_H
#define FORGE_RUNTIME_H

#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>
#include <stdbool.h>
#include <string.h>
#include <ctype.h>

/* -------------------------------------------------------------------------
 * Dynamic Slices
 * -------------------------------------------------------------------------
 * Usage:
 *   FORGE_SLICE_DEF(int32_t, ForgeSlice_int32_t)
 *   ForgeSlice_int32_t xs = forge_slice_empty(ForgeSlice_int32_t);
 *   forge_push(&xs, 42, ForgeSlice_int32_t);
 *   int32_t val = xs.data[0];
 */

#define FORGE_SLICE_DEF(ElemType, SliceName) \
    typedef struct { ElemType* data; int32_t len; int32_t cap; } SliceName;

/* Create an empty slice */
#define forge_slice_empty(SliceName) ((SliceName){.data = NULL, .len = 0, .cap = 0})

/* Push an element (grows by 2x when full) */
#define forge_push(slice_ptr, elem, SliceName) do { \
    if ((slice_ptr)->len >= (slice_ptr)->cap) { \
        int32_t _newcap = (slice_ptr)->cap == 0 ? 4 : (slice_ptr)->cap * 2; \
        (slice_ptr)->data = realloc((slice_ptr)->data, sizeof(*(slice_ptr)->data) * _newcap); \
        (slice_ptr)->cap = _newcap; \
    } \
    (slice_ptr)->data[(slice_ptr)->len++] = (elem); \
} while(0)

/* Pop the last element (returns it). Caller must check len > 0. */
#define forge_pop(slice_ptr) ((slice_ptr)->data[--(slice_ptr)->len])

/* Sub-slice: creates a new slice view [low:high). Shares underlying data. */
#define forge_subslice(slice, low, high, SliceName) ({ \
    SliceName _s; \
    _s.data = (slice).data + (low); \
    _s.len = (high) - (low); \
    _s.cap = (slice).cap - (low); \
    _s; \
})

/* Create a slice from an initializer list */
#define forge_slice_lit(SliceName, ElemType, ...) ({ \
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
#define forge_contains(slice, elem) ({ \
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
 *   FORGE_OPT_DEF(int32_t, ForgeOpt_int32_t)
 *   ForgeOpt_int32_t x = forge_some(42, ForgeOpt_int32_t);
 *   ForgeOpt_int32_t y = forge_none(ForgeOpt_int32_t);
 *   if (!forge_isnull(x)) { use(x.val); }
 */

#define FORGE_OPT_DEF(ElemType, OptName) \
    typedef struct { bool has; ElemType val; } OptName;

#define forge_some(value, OptName) ((OptName){.has = true, .val = (value)})
#define forge_none(OptName) ((OptName){.has = false})
#define forge_isnull(opt) (!(opt).has)
#define forge_unwrap(opt) ((opt).val)

/* -------------------------------------------------------------------------
 * Error Results  —  {bool is_err; T value; const char* error}
 * -------------------------------------------------------------------------
 * Usage:
 *   FORGE_RESULT_DEF(int32_t, ForgeResult_int32_t)
 *   ForgeResult_int32_t r = forge_ok(42, ForgeResult_int32_t);
 *   ForgeResult_int32_t e = forge_err("failed", ForgeResult_int32_t);
 */

#define FORGE_RESULT_DEF(ElemType, ResultName) \
    typedef struct { bool is_err; ElemType value; const char* error; } ResultName;

#define forge_ok(val, ResultName) ((ResultName){.is_err = false, .value = (val), .error = NULL})
#define forge_err(msg, ResultName) ((ResultName){.is_err = true, .error = (msg)})
#define forge_is_err(r) ((r).is_err)

/* -------------------------------------------------------------------------
 * String helpers
 * -------------------------------------------------------------------------
 * These return heap-allocated strings where needed. The Forge C runtime
 * does not yet have a GC — these leak. That's fine for now.
 */

static inline bool forge_str_contains(const char* s, const char* sub) {
    return strstr(s, sub) != NULL;
}

static inline int32_t forge_str_index_of(const char* s, const char* sub) {
    const char* p = strstr(s, sub);
    if (p == NULL) return -1;
    return (int32_t)(p - s);
}

static inline bool forge_str_has_prefix(const char* s, const char* prefix) {
    return strncmp(s, prefix, strlen(prefix)) == 0;
}

static inline bool forge_str_has_suffix(const char* s, const char* suffix) {
    size_t slen = strlen(s), suflen = strlen(suffix);
    if (slen < suflen) return false;
    return strcmp(s + slen - suflen, suffix) == 0;
}

// FNV-1a hash for strings
static inline uint64_t forge_hash_string(const char* s) {
    uint64_t h = 14695981039346656037ULL;
    for (const char* p = s; *p; p++) {
        h ^= (uint64_t)(unsigned char)*p;
        h *= 1099511628211ULL;
    }
    return h;
}

static inline const char* forge_str_replace(const char* s, const char* old, const char* new_s) {
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

static inline const char* forge_str_repeat(const char* s, int32_t n) {
    size_t slen = strlen(s);
    char* result = malloc(slen * n + 1);
    result[0] = '\0';
    for (int32_t i = 0; i < n; i++) {
        strcat(result, s);
    }
    return result;
}

static inline const char* forge_str_join(const char* sep, const char** parts, int32_t count) {
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

/* forge_sprintf — heap-allocated formatted string */
static inline const char* forge_sprintf(const char* fmt, ...) {
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
static inline const char* forge_bool_str(bool b) {
    return b ? "true" : "false";
}

/* String case conversion — heap-allocated result */
static inline const char* forge_toupper(const char* s) {
    size_t len = strlen(s);
    char* r = (char*)malloc(len + 1);
    for (size_t i = 0; i < len; i++) r[i] = (char)toupper((unsigned char)s[i]);
    r[len] = '\0';
    return r;
}

static inline const char* forge_tolower(const char* s) {
    size_t len = strlen(s);
    char* r = (char*)malloc(len + 1);
    for (size_t i = 0; i < len; i++) r[i] = (char)tolower((unsigned char)s[i]);
    r[len] = '\0';
    return r;
}

/* -------------------------------------------------------------------------
 * Channels (pthreads-based, buffered and unbuffered)
 * -------------------------------------------------------------------------
 * Usage:
 *   FORGE_CHAN_DEF(int32_t, ForgeChan_int32_t)
 *   ForgeChan_int32_t* ch = forge_chan_make_int32_t(10);  // buffered
 *   forge_chan_send_int32_t(ch, 42);
 *   int32_t val = forge_chan_recv_int32_t(ch);
 *   forge_chan_close_int32_t(ch);
 *   forge_chan_free_int32_t(ch);
 */
#include <pthread.h>

#define FORGE_CHAN_DEF(ElemType, ChanName) \
    typedef struct { \
        ElemType* buf; \
        int32_t cap; \
        int32_t len; \
        int32_t head; \
        int32_t tail; \
        bool closed; \
        pthread_mutex_t mu; \
        pthread_cond_t not_empty; \
        pthread_cond_t not_full; \
    } ChanName;

#define FORGE_CHAN_IMPL(ElemType, ChanName, Suffix) \
    static inline ChanName* forge_chan_make_##Suffix(int32_t capacity) { \
        ChanName* ch = calloc(1, sizeof(ChanName)); \
        ch->cap = capacity > 0 ? capacity : 1; \
        ch->buf = malloc(sizeof(ElemType) * ch->cap); \
        pthread_mutex_init(&ch->mu, NULL); \
        pthread_cond_init(&ch->not_empty, NULL); \
        pthread_cond_init(&ch->not_full, NULL); \
        return ch; \
    } \
    static inline void forge_chan_send_##Suffix(ChanName* ch, ElemType val) { \
        pthread_mutex_lock(&ch->mu); \
        while (ch->len >= ch->cap && !ch->closed) { \
            pthread_cond_wait(&ch->not_full, &ch->mu); \
        } \
        if (!ch->closed) { \
            ch->buf[ch->tail] = val; \
            ch->tail = (ch->tail + 1) % ch->cap; \
            ch->len++; \
            pthread_cond_signal(&ch->not_empty); \
        } \
        pthread_mutex_unlock(&ch->mu); \
    } \
    static inline ElemType forge_chan_recv_##Suffix(ChanName* ch) { \
        pthread_mutex_lock(&ch->mu); \
        while (ch->len == 0 && !ch->closed) { \
            pthread_cond_wait(&ch->not_empty, &ch->mu); \
        } \
        ElemType val; memset(&val, 0, sizeof(val)); \
        if (ch->len > 0) { \
            val = ch->buf[ch->head]; \
            ch->head = (ch->head + 1) % ch->cap; \
            ch->len--; \
            pthread_cond_signal(&ch->not_full); \
        } \
        pthread_mutex_unlock(&ch->mu); \
        return val; \
    } \
    static inline void forge_chan_close_##Suffix(ChanName* ch) { \
        pthread_mutex_lock(&ch->mu); \
        ch->closed = true; \
        pthread_cond_broadcast(&ch->not_empty); \
        pthread_cond_broadcast(&ch->not_full); \
        pthread_mutex_unlock(&ch->mu); \
    } \
    static inline void forge_chan_free_##Suffix(ChanName* ch) { \
        pthread_mutex_destroy(&ch->mu); \
        pthread_cond_destroy(&ch->not_empty); \
        pthread_cond_destroy(&ch->not_full); \
        free(ch->buf); \
        free(ch); \
    }

/* Spawn a function in a new thread (fire-and-forget detached thread) */
static inline void forge_spawn(void* (*func)(void*), void* arg) {
    pthread_t thread;
    pthread_create(&thread, NULL, func, arg);
    pthread_detach(thread);
}

/* -------------------------------------------------------------------------
 * Tagged Unions (for ad-hoc union types like string | i32 | bool)
 * -------------------------------------------------------------------------
 * Tag constants identify which member is active.
 */
#define FORGE_UNION_TAG_I32    0
#define FORGE_UNION_TAG_I64    1
#define FORGE_UNION_TAG_F32    2
#define FORGE_UNION_TAG_F64    3
#define FORGE_UNION_TAG_BOOL   4
#define FORGE_UNION_TAG_STRING 5
#define FORGE_UNION_TAG_PTR    6

typedef struct {
    int tag;
    union {
        int32_t  as_i32;
        int64_t  as_i64;
        float    as_f32;
        double   as_f64;
        bool     as_bool;
        const char* as_string;
        void*    as_ptr;
    } data;
} ForgeUnion;

static inline ForgeUnion forge_union_i32(int32_t v)       { return (ForgeUnion){FORGE_UNION_TAG_I32, {.as_i32 = v}}; }
static inline ForgeUnion forge_union_i64(int64_t v)       { return (ForgeUnion){FORGE_UNION_TAG_I64, {.as_i64 = v}}; }
static inline ForgeUnion forge_union_f32(float v)         { return (ForgeUnion){FORGE_UNION_TAG_F32, {.as_f32 = v}}; }
static inline ForgeUnion forge_union_f64(double v)        { return (ForgeUnion){FORGE_UNION_TAG_F64, {.as_f64 = v}}; }
static inline ForgeUnion forge_union_bool(bool v)         { return (ForgeUnion){FORGE_UNION_TAG_BOOL, {.as_bool = v}}; }
static inline ForgeUnion forge_union_string(const char* v){ return (ForgeUnion){FORGE_UNION_TAG_STRING, {.as_string = v}}; }
static inline ForgeUnion forge_union_ptr(void* v)         { return (ForgeUnion){FORGE_UNION_TAG_PTR, {.as_ptr = v}}; }

#endif /* FORGE_RUNTIME_H */
