lyric mut_params {
  // Test mut parameter support — pass structs by mutable reference

  struct Point {
      x: i32
      y: i32
  }

  func translate(mut p: Point, dx: i32, dy: i32) {
      p.x = p.x + dx
      p.y = p.y + dy
  }

  func set_x(mut p: Point, val: i32) {
      p.x = val
  }

  func test_mut_param() {
      let mut p = Point { x: 10, y: 20 }
      translate(mut p, 5, 3)
      assert_eq(p.x, 15)
      assert_eq(p.y, 23)
  }

  func test_mut_set() {
      let mut p = Point { x: 1, y: 2 }
      set_x(mut p, 99)
      assert_eq(p.x, 99)
      assert_eq(p.y, 2)
  }

  func test_mut_chain() {
      let mut p = Point { x: 0, y: 0 }
      translate(mut p, 1, 2)
      translate(mut p, 3, 4)
      assert_eq(p.x, 4)
      assert_eq(p.y, 6)
  }
}
