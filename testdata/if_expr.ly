// if_expr.ly — tests if/else as expression
lyric if_expr_test {
  func test_basic_if_expr() {
    let x = if true { 42 } else { 0 }
    assert_eq(x, 42, "if true should return then branch")
  }

  func test_false_branch() {
    let x = if false { 42 } else { 99 }
    assert_eq(x, 99, "if false should return else branch")
  }

  func test_if_expr_with_variable() {
    let cond = true
    let x = if cond { 10 } else { 20 }
    assert_eq(x, 10, "if variable condition")
  }

  func test_if_expr_in_arithmetic() {
    let a = 5
    let b = if a > 3 { a * 2 } else { a }
    assert_eq(b, 10, "if expr in let binding with arithmetic")
  }

  func test_if_expr_string() {
    let s = if true { "hello" } else { "world" }
    assert_eq(s, "hello", "if expr with strings")
  }
}
