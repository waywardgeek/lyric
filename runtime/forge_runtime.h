/* forge_runtime.h — Minimal C runtime for Forge-compiled programs.
 *
 * Provides macros for:
 *   - Dynamic slices (FORGE_SLICE_DEF, forge_push, forge_pop, forge_slice_lit)
 *   - Length-prefixed strings (forge_string = [u8], FORGE_STR, helpers)
 *   - Optionals (FORGE_OPT_DEF, forge_some, forge_none, forge_isnull)
 *   - Error results (FORGE_RESULT_DEF, forge_ok, forge_err)
 *   - Formatting (forge_sprintf)
 *
 * Strings: forge_string is a length-prefixed byte slice (ForgeSlice_uint8_t).
 * Embedded \0 is legal. All string operations are length-aware.
 * Heap-allocated strings carry a hidden trailing \0 past .len for C interop
 * convenience, but .len never includes it.
 */

#ifndef FORGE_RUNTIME_H
#define FORGE_RUNTIME_H

#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>
#include <stdbool.h>
#include <string.h>
#include <stdarg.h>
#include <ctype.h>
#include <unistd.h>
#include <dirent.h>
#include <sys/stat.h>


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

/* Push an element (grows by 2x when full).
 * When cap==0, always malloc fresh (data may point to a static string literal). */
#define forge_push(slice_ptr, elem, SliceName) do { \
    if ((slice_ptr)->len >= (slice_ptr)->cap) { \
        int32_t _newcap = (slice_ptr)->cap == 0 ? 4 : (slice_ptr)->cap * 2; \
        void* _old = (slice_ptr)->cap == 0 ? NULL : (slice_ptr)->data; \
        void* _new = realloc(_old, sizeof(*(slice_ptr)->data) * _newcap); \
        if ((slice_ptr)->cap == 0 && (slice_ptr)->len > 0) { \
            memcpy(_new, (slice_ptr)->data, sizeof(*(slice_ptr)->data) * (slice_ptr)->len); \
        } \
        (slice_ptr)->data = _new; \
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
 * Strings  —  length-prefixed byte slice
 * -------------------------------------------------------------------------
 * forge_string = ForgeSlice_uint8_t = { uint8_t* data; int32_t len, cap; }
 * Embedded \0 is legal. All operations are length-aware.
 *
 * Usage:
 *   forge_string s = FORGE_STR("hello");  // from literal
 *   forge_string t = forge_str_from_cstr(cstr);  // from C string
 *   if (forge_str_eq(s, t)) { ... }
 *   forge_string sub = forge_subslice(s, 1, 3, forge_string);  // "el"
 */

FORGE_SLICE_DEF(uint8_t, ForgeSlice_uint8_t)
typedef ForgeSlice_uint8_t forge_string;
#ifndef FORGE_SLICE_FORGE_STRING_DEFINED
#define FORGE_SLICE_FORGE_STRING_DEFINED
FORGE_SLICE_DEF(forge_string, ForgeSlice_forge_string)
#endif

/* Create a string from a C string literal (compile-time length via sizeof) */
#define FORGE_STR(lit) ((forge_string){ \
    .data = (uint8_t*)(lit), \
    .len = (int32_t)(sizeof(lit) - 1), \
    .cap = (int32_t)(sizeof(lit) - 1) \
})

#define FORGE_STR_EMPTY ((forge_string){.data = NULL, .len = 0, .cap = 0})

/* Bulk-append bytes from src string to dst string.
 * Grows dst with doubling strategy, then memcpy.
 * Handles cap==0 (static string literal data) safely. */
static inline void forge_push_bytes(forge_string* dst, forge_string src) {
    if (src.len == 0) return;
    int32_t needed = dst->len + src.len;
    if (needed > dst->cap) {
        int32_t newcap = dst->cap == 0 ? 64 : dst->cap;
        while (newcap < needed) newcap *= 2;
        uint8_t* old = dst->cap == 0 ? NULL : dst->data;
        uint8_t* buf = (uint8_t*)realloc(old, newcap);
        if (dst->cap == 0 && dst->len > 0) {
            memcpy(buf, dst->data, dst->len);
        }
        dst->data = buf;
        dst->cap = newcap;
    }
    memcpy(dst->data + dst->len, src.data, src.len);
    dst->len = needed;
}

/* Create from null-terminated C string (heap-copies) */
static inline forge_string forge_str_from_cstr(const char* s) {
    if (!s) return (forge_string){.data = NULL, .len = 0, .cap = 0};
    int32_t n = (int32_t)strlen(s);
    uint8_t* buf = (uint8_t*)malloc(n + 1);
    memcpy(buf, s, n + 1); /* trailing \0 for C interop */
    return (forge_string){.data = buf, .len = n, .cap = n};
}

/* Create from raw bytes (heap-copies, adds hidden trailing \0) */
static inline forge_string forge_str_from_bytes(const void* data, int32_t len) {
    uint8_t* buf = (uint8_t*)malloc(len + 1);
    memcpy(buf, data, len);
    buf[len] = '\0';
    return (forge_string){.data = buf, .len = len, .cap = len};
}

/* Equality (length-aware, handles embedded \0) */
static inline bool forge_str_eq(forge_string a, forge_string b) {
    if (a.len != b.len) return false;
    if (a.len == 0) return true;
    return memcmp(a.data, b.data, a.len) == 0;
}

/* Lexicographic comparison */
static inline int forge_str_cmp(forge_string a, forge_string b) {
    int32_t min = a.len < b.len ? a.len : b.len;
    int r = min > 0 ? memcmp(a.data, b.data, min) : 0;
    if (r != 0) return r;
    return (a.len > b.len) - (a.len < b.len);
}

/* Concatenate two strings (heap-allocates) */
static inline forge_string forge_str_concat(forge_string a, forge_string b) {
    int32_t total = a.len + b.len;
    uint8_t* buf = (uint8_t*)malloc(total + 1);
    if (a.len > 0) memcpy(buf, a.data, a.len);
    if (b.len > 0) memcpy(buf + a.len, b.data, b.len);
    buf[total] = '\0';
    return (forge_string){.data = buf, .len = total, .cap = total};
}

/* Length-aware memmem (find needle in haystack) */
static inline const uint8_t* forge_memmem(const uint8_t* h, int32_t hlen,
                                            const uint8_t* n, int32_t nlen) {
    if (nlen == 0) return h;
    if (nlen > hlen) return NULL;
    for (int32_t i = 0; i <= hlen - nlen; i++) {
        if (memcmp(h + i, n, nlen) == 0) return h + i;
    }
    return NULL;
}

/* Contains */
static inline bool forge_str_contains(forge_string s, forge_string sub) {
    return forge_memmem(s.data, s.len, sub.data, sub.len) != NULL;
}

/* Index of substring (-1 if not found) */
static inline int32_t forge_str_index_of(forge_string s, forge_string sub) {
    const uint8_t* p = forge_memmem(s.data, s.len, sub.data, sub.len);
    if (!p) return -1;
    return (int32_t)(p - s.data);
}

/* Has prefix */
static inline bool forge_str_has_prefix(forge_string s, forge_string prefix) {
    if (prefix.len > s.len) return false;
    return memcmp(s.data, prefix.data, prefix.len) == 0;
}

/* Has suffix */
static inline bool forge_str_has_suffix(forge_string s, forge_string suffix) {
    if (suffix.len > s.len) return false;
    return memcmp(s.data + s.len - suffix.len, suffix.data, suffix.len) == 0;
}

/* FNV-1a hash (length-aware) */
static inline uint64_t forge_hash_string(forge_string s) {
    uint64_t h = 14695981039346656037ULL;
    for (int32_t i = 0; i < s.len; i++) {
        h ^= (uint64_t)s.data[i];
        h *= 1099511628211ULL;
    }
    return h;
}

/* Replace all occurrences of old with new_s */
static inline forge_string forge_str_replace(forge_string s, forge_string old, forge_string new_s) {
    if (old.len == 0) return forge_str_from_bytes(s.data, s.len);
    /* Count occurrences */
    int count = 0;
    const uint8_t* p = s.data;
    int32_t remaining = s.len;
    while (remaining >= old.len) {
        const uint8_t* found = forge_memmem(p, remaining, old.data, old.len);
        if (!found) break;
        count++;
        int32_t skip = (int32_t)(found - p) + old.len;
        p += skip;
        remaining -= skip;
    }
    if (count == 0) return forge_str_from_bytes(s.data, s.len);
    int32_t total = s.len + count * (new_s.len - old.len);
    uint8_t* buf = (uint8_t*)malloc(total + 1);
    uint8_t* dst = buf;
    p = s.data;
    remaining = s.len;
    while (remaining >= old.len) {
        const uint8_t* found = forge_memmem(p, remaining, old.data, old.len);
        if (!found) break;
        int32_t prefix_len = (int32_t)(found - p);
        memcpy(dst, p, prefix_len);
        dst += prefix_len;
        memcpy(dst, new_s.data, new_s.len);
        dst += new_s.len;
        p = found + old.len;
        remaining -= prefix_len + old.len;
    }
    memcpy(dst, p, remaining);
    dst += remaining;
    buf[total] = '\0';
    return (forge_string){.data = buf, .len = total, .cap = total};
}

/* Repeat string n times */
static inline forge_string forge_str_repeat(forge_string s, int32_t n) {
    if (n <= 0 || s.len == 0) return FORGE_STR_EMPTY;
    int32_t total = s.len * n;
    uint8_t* buf = (uint8_t*)malloc(total + 1);
    for (int32_t i = 0; i < n; i++) {
        memcpy(buf + i * s.len, s.data, s.len);
    }
    buf[total] = '\0';
    return (forge_string){.data = buf, .len = total, .cap = total};
}

/* Join an array of strings with separator */
static inline forge_string forge_str_join(forge_string sep, forge_string* parts, int32_t count) {
    if (count == 0) return FORGE_STR_EMPTY;
    int32_t total = 0;
    for (int32_t i = 0; i < count; i++) {
        total += parts[i].len;
        if (i > 0) total += sep.len;
    }
    uint8_t* buf = (uint8_t*)malloc(total + 1);
    uint8_t* dst = buf;
    for (int32_t i = 0; i < count; i++) {
        if (i > 0 && sep.len > 0) { memcpy(dst, sep.data, sep.len); dst += sep.len; }
        if (parts[i].len > 0) { memcpy(dst, parts[i].data, parts[i].len); dst += parts[i].len; }
    }
    buf[total] = '\0';
    return (forge_string){.data = buf, .len = total, .cap = total};
}

/* Split string by separator, returns a slice of strings */
static inline ForgeSlice_forge_string forge_str_split(forge_string s, forge_string sep) {
    ForgeSlice_forge_string result = {.data = NULL, .len = 0, .cap = 0};
    if (sep.len == 0) {
        /* Split into individual bytes */
        for (int32_t i = 0; i < s.len; i++) {
            forge_string ch = forge_str_from_bytes(s.data + i, 1);
            forge_push(&result, ch, forge_string);
        }
        return result;
    }
    const uint8_t* p = s.data;
    int32_t remaining = s.len;
    while (remaining >= 0) {
        const uint8_t* found = (remaining >= sep.len) ?
            forge_memmem(p, remaining, sep.data, sep.len) : NULL;
        if (!found) {
            forge_string part = forge_str_from_bytes(p, remaining);
            forge_push(&result, part, forge_string);
            break;
        }
        int32_t prefix_len = (int32_t)(found - p);
        forge_string part = forge_str_from_bytes(p, prefix_len);
        forge_push(&result, part, forge_string);
        p = found + sep.len;
        remaining -= prefix_len + sep.len;
    }
    return result;
}

/* forge_sprintf — heap-allocated formatted string.
 * NOTE: This uses C's printf family, which doesn't handle embedded \0.
 * Use only for format strings without embedded nulls. */
static inline forge_string forge_sprintf(const char* fmt, ...) {
    va_list args, args2;
    va_start(args, fmt);
    va_copy(args2, args);
    int n = vsnprintf(NULL, 0, fmt, args);
    va_end(args);
    uint8_t* buf = (uint8_t*)malloc(n + 1);
    vsnprintf((char*)buf, n + 1, fmt, args2);
    va_end(args2);
    return (forge_string){.data = buf, .len = (int32_t)n, .cap = (int32_t)n};
}

/* Bool to string for printf */
static inline const char* forge_bool_str(bool b) {
    return b ? "true" : "false";
}

/* String case conversion */
static inline forge_string forge_toupper(forge_string s) {
    uint8_t* buf = (uint8_t*)malloc(s.len + 1);
    for (int32_t i = 0; i < s.len; i++) buf[i] = (uint8_t)toupper(s.data[i]);
    buf[s.len] = '\0';
    return (forge_string){.data = buf, .len = s.len, .cap = s.len};
}

static inline forge_string forge_tolower(forge_string s) {
    uint8_t* buf = (uint8_t*)malloc(s.len + 1);
    for (int32_t i = 0; i < s.len; i++) buf[i] = (uint8_t)tolower(s.data[i]);
    buf[s.len] = '\0';
    return (forge_string){.data = buf, .len = s.len, .cap = s.len};
}

/* Trim whitespace from both ends */
static inline forge_string forge_str_trim(forge_string s) {
    int32_t start = 0, end = s.len;
    while (start < end && isspace(s.data[start])) start++;
    while (end > start && isspace(s.data[end - 1])) end--;
    if (start == 0 && end == s.len) return s; /* no trim needed, return view */
    return forge_str_from_bytes(s.data + start, end - start);
}

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
 * Error messages remain const char* (C string literals).
 * This is intentional — error messages come from forge_err("msg") literals.
 */

#define FORGE_RESULT_DEF(ElemType, ResultName) \
    typedef struct { bool is_err; ElemType value; const char* error; } ResultName;

#define forge_ok(val, ResultName) ((ResultName){.is_err = false, .value = (val), .error = NULL})
#define forge_err(msg, ResultName) ((ResultName){.is_err = true, .error = (msg)})
#define forge_is_err(r) ((r).is_err)

/* -------------------------------------------------------------------------
 * Channels (pthreads-based, buffered and unbuffered)
 * -------------------------------------------------------------------------
 * Usage:
 *   FORGE_CHAN_DEF(int32_t, ForgeChan_int32_t)
 *   ForgeChan_int32_t* ch = forge_chan_make_int32_t(10);
 *   forge_chan_send_int32_t(ch, 42);
 *   int32_t val = forge_chan_recv_int32_t(ch);
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
    static inline bool forge_chan_tryrecv_##Suffix(ChanName* ch, ElemType* out) { \
        pthread_mutex_lock(&ch->mu); \
        if (ch->len == 0) { \
            pthread_mutex_unlock(&ch->mu); \
            return false; \
        } \
        ElemType val = ch->buf[ch->head]; \
        ch->head = (ch->head + 1) % ch->cap; \
        ch->len--; \
        pthread_cond_signal(&ch->not_full); \
        pthread_mutex_unlock(&ch->mu); \
        if (out) *out = val; \
        return true; \
    } \
    static inline bool forge_chan_trysend_##Suffix(ChanName* ch, ElemType val) { \
        pthread_mutex_lock(&ch->mu); \
        if (ch->len >= ch->cap || ch->closed) { \
            pthread_mutex_unlock(&ch->mu); \
            return false; \
        } \
        ch->buf[ch->tail] = val; \
        ch->tail = (ch->tail + 1) % ch->cap; \
        ch->len++; \
        pthread_cond_signal(&ch->not_empty); \
        pthread_mutex_unlock(&ch->mu); \
        return true; \
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
        forge_string as_string;
        void*    as_ptr;
    } data;
} ForgeUnion;

static inline ForgeUnion forge_union_i32(int32_t v)       { return (ForgeUnion){FORGE_UNION_TAG_I32, {.as_i32 = v}}; }
static inline ForgeUnion forge_union_i64(int64_t v)       { return (ForgeUnion){FORGE_UNION_TAG_I64, {.as_i64 = v}}; }
static inline ForgeUnion forge_union_f32(float v)         { return (ForgeUnion){FORGE_UNION_TAG_F32, {.as_f32 = v}}; }
static inline ForgeUnion forge_union_f64(double v)        { return (ForgeUnion){FORGE_UNION_TAG_F64, {.as_f64 = v}}; }
static inline ForgeUnion forge_union_bool(bool v)         { return (ForgeUnion){FORGE_UNION_TAG_BOOL, {.as_bool = v}}; }
static inline ForgeUnion forge_union_string(forge_string v){ return (ForgeUnion){FORGE_UNION_TAG_STRING, {.as_string = v}}; }
static inline ForgeUnion forge_union_ptr(void* v)         { return (ForgeUnion){FORGE_UNION_TAG_PTR, {.as_ptr = v}}; }

static inline void forge_union_fprint(FILE* f, ForgeUnion u) {
    switch (u.tag) {
    case FORGE_UNION_TAG_I32:    fprintf(f, "%d", u.data.as_i32); break;
    case FORGE_UNION_TAG_I64:    fprintf(f, "%lld", (long long)u.data.as_i64); break;
    case FORGE_UNION_TAG_F32:    fprintf(f, "%g", u.data.as_f32); break;
    case FORGE_UNION_TAG_F64:    fprintf(f, "%g", u.data.as_f64); break;
    case FORGE_UNION_TAG_BOOL:   fprintf(f, "%s", u.data.as_bool ? "true" : "false"); break;
    case FORGE_UNION_TAG_STRING: fprintf(f, "%.*s", (int)u.data.as_string.len, (const char*)u.data.as_string.data); break;
    case FORGE_UNION_TAG_PTR:    fprintf(f, "%p", u.data.as_ptr); break;
    }
}

/* -------------------------------------------------------------------------
 * File I/O
 * -------------------------------------------------------------------------
 */

typedef struct { forge_string _0; bool _1; } forge_str_bool_t;

static inline forge_str_bool_t forge_read_file(forge_string path) {
    /* Need null-terminated path for fopen */
    char* cpath = (char*)malloc(path.len + 1);
    memcpy(cpath, path.data, path.len);
    cpath[path.len] = '\0';
    FILE* f = fopen(cpath, "rb");
    free(cpath);
    if (!f) { forge_str_bool_t r = {FORGE_STR_EMPTY, false}; return r; }
    fseek(f, 0, SEEK_END);
    long n = ftell(f);
    fseek(f, 0, SEEK_SET);
    uint8_t* buf = (uint8_t*)malloc(n + 1);
    fread(buf, 1, n, f);
    fclose(f);
    buf[n] = '\0';
    forge_str_bool_t r = {{.data = buf, .len = (int32_t)n, .cap = (int32_t)n}, true};
    return r;
}

static inline bool forge_write_file(forge_string path, forge_string data) {
    char* cpath = (char*)malloc(path.len + 1);
    memcpy(cpath, path.data, path.len);
    cpath[path.len] = '\0';
    FILE* f = fopen(cpath, "wb");
    free(cpath);
    if (!f) return false;
    size_t written = fwrite(data.data, 1, data.len, f);
    fclose(f);
    return (int32_t)written == data.len;
}

/* -------------------------------------------------------------------------
 * OS
 * -------------------------------------------------------------------------
 */

static inline forge_string forge_getwd(void) {
    static char buf[4096];
    if (getcwd(buf, sizeof(buf))) return forge_str_from_cstr(buf);
    return FORGE_STR_EMPTY;
}

static inline ForgeSlice_forge_string forge_list_dir(forge_string path) {
    char* cpath = (char*)malloc(path.len + 1);
    memcpy(cpath, path.data, path.len);
    cpath[path.len] = '\0';
    DIR* d = opendir(cpath);
    free(cpath);
    ForgeSlice_forge_string result = {NULL, 0, 0};
    if (!d) return result;
    struct dirent* entry;
    while ((entry = readdir(d)) != NULL) {
        if (entry->d_name[0] == '.' && (entry->d_name[1] == '\0' ||
            (entry->d_name[1] == '.' && entry->d_name[2] == '\0'))) continue;
        forge_string name = forge_str_from_cstr(entry->d_name);
        forge_push((&result), name, forge_string);
    }
    closedir(d);
    return result;
}

static inline bool forge_file_exists(forge_string path) {
    char* cpath = (char*)malloc(path.len + 1);
    memcpy(cpath, path.data, path.len);
    cpath[path.len] = '\0';
    struct stat st;
    bool exists = (stat(cpath, &st) == 0);
    free(cpath);
    return exists;
}

static inline forge_string forge_mkdtemp(forge_string prefix) {
    char tmpl[4096];
    int n = snprintf(tmpl, sizeof(tmpl), "/tmp/%.*s-XXXXXX", (int)prefix.len, (const char*)prefix.data);
    (void)n;
    char* result = mkdtemp(tmpl);
    if (!result) return FORGE_STR_EMPTY;
    return forge_str_from_cstr(result);
}

/* -------------------------------------------------------------------------
 * Path manipulation
 * -------------------------------------------------------------------------
 */

static inline forge_string forge_path_dir(forge_string path) {
    /* Find last '/' */
    for (int32_t i = path.len - 1; i >= 0; i--) {
        if (path.data[i] == '/') {
            return forge_str_from_bytes(path.data, i);
        }
    }
    return FORGE_STR(".");
}

static inline forge_string forge_path_base(forge_string path) {
    for (int32_t i = path.len - 1; i >= 0; i--) {
        if (path.data[i] == '/') {
            return forge_str_from_bytes(path.data + i + 1, path.len - i - 1);
        }
    }
    return forge_str_from_bytes(path.data, path.len);
}

static inline forge_string forge_path_ext(forge_string path) {
    /* Find last '.' after last '/' */
    int32_t start = 0;
    for (int32_t i = path.len - 1; i >= 0; i--) {
        if (path.data[i] == '/') { start = i + 1; break; }
    }
    for (int32_t i = path.len - 1; i >= start; i--) {
        if (path.data[i] == '.') {
            return forge_str_from_bytes(path.data + i, path.len - i);
        }
    }
    return FORGE_STR_EMPTY;
}

/* -------------------------------------------------------------------------
 * String conversion
 * -------------------------------------------------------------------------
 */

static inline forge_string forge_itoa(int64_t n) {
    char buf[32];
    int len = snprintf(buf, sizeof(buf), "%lld", (long long)n);
    return forge_str_from_bytes(buf, len);
}

typedef struct { int64_t _0; bool _1; } forge_atoi_result;

static inline forge_atoi_result forge_atoi(forge_string s) {
    /* Need null-terminated for strtoll */
    char* cstr = (char*)malloc(s.len + 1);
    memcpy(cstr, s.data, s.len);
    cstr[s.len] = '\0';
    char* end;
    long long v = strtoll(cstr, &end, 10);
    bool ok = (*end == '\0' && end != cstr);
    free(cstr);
    return (forge_atoi_result){ ._0 = (int64_t)v, ._1 = ok };
}

typedef struct { double _0; bool _1; } forge_parse_float_result;

static inline forge_parse_float_result forge_parse_float(forge_string s) {
    char* cstr = (char*)malloc(s.len + 1);
    memcpy(cstr, s.data, s.len);
    cstr[s.len] = '\0';
    char* end;
    double v = strtod(cstr, &end);
    bool ok = (*end == '\0' && end != cstr);
    free(cstr);
    return (forge_parse_float_result){ ._0 = v, ._1 = ok };
}

static inline forge_string forge_char_to_string(uint8_t c) {
    uint8_t* buf = (uint8_t*)malloc(2);
    buf[0] = c;
    buf[1] = '\0';
    return (forge_string){.data = buf, .len = 1, .cap = 1};
}

/* Print a forge_string to a FILE* (length-aware, handles embedded \0) */
static inline void forge_fprint_str(FILE* f, forge_string s) {
    if (s.len > 0) fwrite(s.data, 1, s.len, f);
}

/* panic: print message to stderr and abort */
static inline void forge_panic(forge_string msg) {
    fprintf(stderr, "panic: %.*s\n", (int)msg.len, (const char*)msg.data);
    exit(1);
}

#endif /* FORGE_RUNTIME_H */
