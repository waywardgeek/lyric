// Test let ref and let mut ref bindings

func test_ref_binding() {
  let data: [i32] = [1, 2, 3, 4, 5]
  let ref view = data
  // view should see the same data without copying
  assert_eq(len(view), 5)
  assert_eq(view[0], 1)
  assert_eq(view[4], 5)
  println("ref binding OK")
}

func test_mut_ref_binding() {
  let mut data: [i32] = [10, 20, 30]
  let ref view = data
  // view shares backing data — no scope-exit free on view
  assert_eq(len(view), 3)
  assert_eq(view[0], 10)
  println("mut ref binding OK")
}

func main() {
  test_ref_binding()
  test_mut_ref_binding()
}
