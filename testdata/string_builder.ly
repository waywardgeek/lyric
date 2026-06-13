// test_string_builder.ly — Test StringBuilder with append-based write
// Verifies O(n) append instead of O(n²) string concat

func test_basic_write() {
  let sb = new_string_builder()
  sb.write("hello")
  sb.write(" ")
  sb.write("world")
  assert_eq(sb.to_string(), "hello world")
}

func test_write_byte() {
  let sb = new_string_builder()
  sb.write_byte(65 as u8)  // 'A'
  sb.write_byte(66 as u8)  // 'B'
  sb.write_byte(67 as u8)  // 'C'
  assert_eq(sb.to_string(), "ABC")
}

func test_mixed_write() {
  let sb = new_string_builder()
  sb.write("hi")
  sb.write_byte(33 as u8)  // '!'
  sb.write(" ok")
  assert_eq(sb.to_string(), "hi! ok")
}

func test_empty_write() {
  let sb = new_string_builder()
  sb.write("")
  sb.write("x")
  sb.write("")
  assert_eq(sb.to_string(), "x")
}

func test_len() {
  let sb = new_string_builder()
  assert_eq(sb.len(), 0)
  sb.write("abc")
  assert_eq(sb.len(), 3)
  sb.write("de")
  assert_eq(sb.len(), 5)
}

func test_large_append() {
  // Build a large string to verify doubling growth works
  let sb = new_string_builder()
  let mut i = 0
  while i < 1000 {
    sb.write("x")
    i = i + 1
  }
  assert_eq(sb.len(), 1000)
}
