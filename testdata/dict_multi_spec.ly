// Test: multiple Dict specializations shouldn't have last-writer-wins contamination
// This is the minimal reproduction of the bootstrap self-compilation Dict bug.

func test_multi_dict() {
  let d1 = Dict<Sym, i32>()
  d1.set(`alpha`, 1)
  d1.set(`beta`, 2)

  let d2 = Dict<Sym, string>()
  d2.set(`hello`, "world")
  d2.set(`foo`, "bar")

  // Access both — forces both specializations
  assert(d1.has(`alpha`), "d1 should have alpha")
  assert_eq(d1.get(`alpha`)!.value, 1, "d1 alpha should be 1")

  assert(d2.has(`hello`), "d2 should have hello")
  assert_eq(d2.get(`hello`)!.value, "world", "d2 hello should be world")

  // Test keys
  let k1 = d1.keys()
  let k2 = d2.keys()
  assert_eq(len(k1), 2, "d1 should have 2 keys")
  assert_eq(len(k2), 2, "d2 should have 2 keys")

  // Test remove
  assert(d1.remove(`alpha`), "d1 remove alpha should return true")
  assert_eq(len(d1.keys()), 1, "d1 should have 1 key after remove")

  print("PASS")
}

func main() {
  test_multi_dict()
}
