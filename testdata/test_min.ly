lyric test_min {

  func test_return_value() {
    let (f, e) = parse_file("lyric t { func f() { return 42 } }", "test.ly")
    if e != null { print("return 42: FAIL") } else { print("return 42: OK") }
  }

  func test_expr_stmt() {
    let (f, e) = parse_file("lyric t { func f() { 42 } }", "test.ly")
    if e != null { print("expr 42: FAIL") } else { print("expr 42: OK") }
  }

  func test_break() {
    let (f, e) = parse_file("lyric t { func f() { break } }", "test.ly")
    if e != null { print("break: FAIL") } else { print("break: OK") }
  }

  func test_nested_block() {
    let (f, e) = parse_file("lyric t { func f() { { } } }", "test.ly")
    if e != null { print("nested {}: FAIL") } else { print("nested {}: OK") }
  }

  func test_while() {
    let (f, e) = parse_file("lyric t { func f() { while true { } } }", "test.ly")
    if e != null { print("while: FAIL") } else { print("while: OK") }
  }
}
