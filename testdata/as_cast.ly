// as_cast.ly — tests 'expr as Type' cast syntax
lyric as_cast_test {
  func test_basic_cast() {
    let x: i32 = 42
    let y: i64 = x as i64
    assert_eq(y, 42 as i64, "i32 to i64 cast")
  }

  func test_downcast() {
    let big: i64 = 100
    let small: i32 = big as i32
    assert_eq(small, 100, "i64 to i32 cast")
  }

  func test_cast_in_expression() {
    let a: i32 = 10
    let b: i64 = (a as i64) + (20 as i64)
    assert_eq(b, 30 as i64, "cast in arithmetic")
  }
}
