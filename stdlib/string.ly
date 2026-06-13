// string.ly — String utilities for the Lyric standard library
// Since string = [u8], all functions operate on byte slices.

lyric string_utils {

  // --- Comparison / Search ---

  // Check if haystack contains needle
  pub func str_contains(haystack: string, needle: string) -> bool {
    return str_index_of(haystack, needle) >= 0
  }

  // Check if s starts with prefix
  pub func str_has_prefix(s: string, prefix: string) -> bool {
    if len(prefix) > len(s) {
      return false
    }
    let mut i: i32 = 0
    while i < len(prefix) {
      if s[i] != prefix[i] {
        return false
      }
      i = i + 1
    }
    return true
  }

  // Check if s ends with suffix
  pub func str_has_suffix(s: string, suffix: string) -> bool {
    if len(suffix) > len(s) {
      return false
    }
    let offset = len(s) - len(suffix)
    let mut i: i32 = 0
    while i < len(suffix) {
      if s[offset + i] != suffix[i] {
        return false
      }
      i = i + 1
    }
    return true
  }

  // Find first occurrence of needle in haystack. Returns -1 if not found.
  pub func str_index_of(haystack: string, needle: string) -> i32 {
    if len(needle) == 0 {
      return 0
    }
    if len(needle) > len(haystack) {
      return -1
    }
    let limit = len(haystack) - len(needle)
    let mut i: i32 = 0
    while i <= limit {
      let mut j: i32 = 0
      while j < len(needle) && haystack[i + j] == needle[j] {
        j = j + 1
      }
      if j == len(needle) {
        return i
      }
      i = i + 1
    }
    return -1
  }

  // --- Splitting ---

  // Split string by separator, returning at most n parts.
  // If n <= 0, returns all parts (same as str_split).
  // If n == 1, returns [s] unchanged.
  // Otherwise, splits into at most n parts: the last part contains the remainder.
  pub func str_split_n(s: string, sep: string, n: i32) -> [string] {
    if n == 1 {
      return [s]
    }
    let mut result: [string] = []
    let mut start: i32 = 0
    let mut count: i32 = 1
    while start <= len(s) {
      if n > 0 && count >= n {
        result = append(result, s[start:len(s)])
        return result
      }
      let idx = str_index_of(s[start:len(s)], sep)
      if idx < 0 {
        result = append(result, s[start:len(s)])
        return result
      }
      result = append(result, s[start:start + idx])
      start = start + idx + len(sep)
      count = count + 1
    }
    return result
  }

  // Split string by separator. Returns a slice of strings.
  pub func str_split(s: string, sep: string) -> [string] {
    let mut result: [string] = []
    if len(sep) == 0 {
      // Split into individual bytes
      let mut i: i32 = 0
      while i < len(s) {
        result = append(result, char_to_string(s[i]))
        i = i + 1
      }
      return result
    }
    let mut start: i32 = 0
    while start <= len(s) {
      let idx = str_index_of(s[start:len(s)], sep)
      if idx < 0 {
        result = append(result, s[start:len(s)])
        return result
      }
      result = append(result, s[start:start + idx])
      start = start + idx + len(sep)
    }
    return result
  }

  // --- Trimming ---

  func is_whitespace(ch: u8) -> bool {
    return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
  }

  // Trim whitespace from both ends
  pub func str_trim(s: string) -> string {
    let mut lo: i32 = 0
    while lo < len(s) && is_whitespace(s[lo]) {
      lo = lo + 1
    }
    let mut hi: i32 = len(s)
    while hi > lo && is_whitespace(s[hi - 1]) {
      hi = hi - 1
    }
    return s[lo:hi]
  }

  // Trim whitespace from left
  pub func str_trim_left(s: string) -> string {
    let mut lo: i32 = 0
    while lo < len(s) && is_whitespace(s[lo]) {
      lo = lo + 1
    }
    return s[lo:len(s)]
  }

  // Trim whitespace from right
  pub func str_trim_right(s: string) -> string {
    let mut hi: i32 = len(s)
    while hi > 0 && is_whitespace(s[hi - 1]) {
      hi = hi - 1
    }
    return s[0:hi]
  }

  // --- Case conversion ---

  pub func str_to_upper(s: string) -> string {
    let buf = new_string_builder()
    let mut i: i32 = 0
    while i < len(s) {
      let ch = s[i]
      if ch >= 'a' && ch <= 'z' {
        buf.write_byte(ch - (32 as u8))
      } else {
        buf.write_byte(ch)
      }
      i = i + 1
    }
    return buf.to_string()
  }

  pub func str_to_lower(s: string) -> string {
    let buf = new_string_builder()
    let mut i: i32 = 0
    while i < len(s) {
      let ch = s[i]
      if ch >= 'A' && ch <= 'Z' {
        buf.write_byte(ch + (32 as u8))
      } else {
        buf.write_byte(ch)
      }
      i = i + 1
    }
    return buf.to_string()
  }

  // --- Replacement ---

  // Replace all occurrences of old with new_str in s
  pub func str_replace(s: string, old: string, new_str: string) -> string {
    if len(old) == 0 {
      return s
    }
    let buf = new_string_builder()
    let mut i: i32 = 0
    while i < len(s) {
      let remaining = s[i:len(s)]
      if str_has_prefix(remaining, old) {
        buf.write(new_str)
        i = i + len(old)
      } else {
        buf.write_byte(s[i])
        i = i + 1
      }
    }
    return buf.to_string()
  }

  // --- Repetition ---

  // Repeat string n times
  pub func str_repeat(s: string, n: i32) -> string {
    let buf = new_string_builder()
    let mut i: i32 = 0
    while i < n {
      buf.write(s)
      i = i + 1
    }
    return buf.to_string()
  }

  // --- Join ---

  // Concatenate two strings. Returns a new string containing all bytes of a followed by all
  // bytes of b. Since string = [u8], this is simple byte-level concatenation with no encoding
  // assumptions. Equivalent to a StringBuilder write(a) + write(b), or the Go a + b idiom.
  //
  // Usage:
  //   let s = str_concat("hello ", "world")   // → "hello world"
  //   let path = str_concat(dir, "/file.ly")
  //
  // For joining more than two strings, use a StringBuilder or str_join.
  pub func str_concat(a: string, b: string) -> string {
    let buf = new_string_builder()
    buf.write(a)
    buf.write(b)
    return buf.to_string()
  }

  // Join a slice of strings with a separator
  pub func str_join(parts: [string], sep: string) -> string {
    let buf = new_string_builder()
    let mut i: i32 = 0
    while i < len(parts) {
      if i > 0 {
        buf.write(sep)
      }
      buf.write(parts[i])
      i = i + 1
    }
    return buf.to_string()
  }

}
