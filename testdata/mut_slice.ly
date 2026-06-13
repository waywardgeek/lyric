lyric mut_slice {
  // Test mut parameter support for slices and primitives (not just structs)

  func add_item(mut items: [i32], val: i32) {
    items = append(items, val)
  }

  func test_mut_slice_append() {
    let mut list: [i32] = []
    add_item(mut list, 10)
    add_item(mut list, 20)
    assert_eq(len(list), 2)
    assert_eq(list[0], 10)
    assert_eq(list[1], 20)
  }

  func add_name(mut names: [string], name: string) {
    names = append(names, name)
  }

  func test_mut_string_slice() {
    let mut names: [string] = []
    add_name(mut names, "hello")
    add_name(mut names, "world")
    assert_eq(len(names), 2)
  }

  func double_it(mut val: i32) {
    val = val * 2
  }

  func test_mut_primitive() {
    let mut x: i32 = 10
    double_it(mut x)
    assert_eq(x, 20)
  }

  func swap(mut a: i32, mut b: i32) {
    let tmp = a
    a = b
    b = tmp
  }

  func test_mut_swap() {
    let mut x: i32 = 1
    let mut y: i32 = 2
    swap(mut x, mut y)
    assert_eq(x, 2)
    assert_eq(y, 1)
  }
}
