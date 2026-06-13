// concat.ly — test string and slice concatenation

func test_string_concat() {
  let a = "hello"
  let b = " world"
  let c = a + b
  assert_eq(c, "hello world", "string + string")

  // Empty cases
  let d = "" + "foo"
  assert_eq(d, "foo", "empty + string")
  let e = "bar" + ""
  assert_eq(e, "bar", "string + empty")
}

func test_slice_concat() {
  let a = [1, 2, 3]
  let b = [4, 5]
  let c = a + b
  assert_eq(len(c), 5, "slice concat length")
  assert_eq(c[0], 1, "slice concat [0]")
  assert_eq(c[4], 5, "slice concat [4]")

  // Original unchanged
  assert_eq(len(a), 3, "original a unchanged")
}

func test_slice_extend() {
  let mut xs = [1, 2, 3]
  let ys = [4, 5, 6]
  xs.extend(ys)
  assert_eq(len(xs), 6, "extend length")
  assert_eq(xs[0], 1, "extend [0]")
  assert_eq(xs[5], 6, "extend [5]")
}

func test_string_extend() {
  let mut s = "hello"
  let other = " world"
  // Use StringBuilder for in-place string building
  let sb = new_string_builder()
  sb.write(s)
  sb.write(other)
  assert_eq(sb.to_string(), "hello world", "string builder concat")
}
