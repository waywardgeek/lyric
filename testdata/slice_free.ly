// Test slice scope-exit freeing.
// Each function tests a specific ownership pattern.

struct Holder {
  items: [i32]
}

class Box {
  data: [i32]
}

// Case 1: Local slice, no escape — should be freed
func test_local_no_escape() {
  let mut temps: [i32] = []
  temps.push(1)
  temps.push(2)
  let mut sum = 0
  let mut i = 0
  while i < len(temps) {
    sum = sum + temps[i]
    i = i + 1
  }
  assert_eq(sum, 3, "local no escape")
}

// Case 2: Local slice returned — should NOT be freed
func make_list() -> [i32] {
  let mut result: [i32] = []
  result.push(10)
  result.push(20)
  return result
}

func test_return_no_free() {
  let r = make_list()
  assert_eq(len(r), 2, "returned slice len")
  assert_eq(r[0], 10, "returned slice [0]")
  assert_eq(r[1], 20, "returned slice [1]")
}

// Case 3: Local slice stored in struct field — should NOT be freed
func test_struct_field_escape() {
  let mut items: [i32] = []
  items.push(42)
  let h = Holder { items: items }
  assert_eq(len(h.items), 1, "struct field len")
  assert_eq(h.items[0], 42, "struct field value")
}

// Case 4: Local slice stored in class field — should NOT be freed
func test_class_field_escape() {
  let mut data: [i32] = []
  data.push(99)
  let b = Box { data: data }
  assert_eq(len(b.data), 1, "class field len")
  assert_eq(b.data[0], 99, "class field value")
}

// Case 5: Local slice passed to function that stores it — should NOT be freed
func store_in_holder(items: [i32]) -> Holder {
  return Holder { items: items }
}

func test_escape_via_call() {
  let mut items: [i32] = []
  items.push(7)
  let h = store_in_holder(items)
  assert_eq(h.items[0], 7, "escaped via call")
}

// Case 6: Multiple locals, some escape some don't
func test_mixed_escape() {
  let mut freed_later: [i32] = []
  freed_later.push(1)

  let mut escapes: [i32] = []
  escapes.push(2)
  let h = Holder { items: escapes }

  assert_eq(freed_later[0], 1, "non-escaped still valid")
  assert_eq(h.items[0], 2, "escaped still valid")
}

// Case 7: Slice in inner scope — should be freed at inner scope exit
func test_inner_scope_free() {
  let mut outer = 0
  if true {
    let mut inner: [i32] = []
    inner.push(5)
    outer = inner[0]
  }
  assert_eq(outer, 5, "inner scope value")
}

// Case 8: Slice created in loop — each iteration's slice should be freed
func test_loop_free() {
  let mut total = 0
  let mut i = 0
  while i < 3 {
    let mut tmp: [i32] = []
    tmp.push(i + 1)
    total = total + tmp[0]
    i = i + 1
  }
  assert_eq(total, 6, "loop free")
}

// Case 9: Transitive escape — func A passes to func B which stores it
func store_indirect(items: [i32]) -> Holder {
  return store_in_holder(items)
}

func test_transitive_escape() {
  let mut items: [i32] = []
  items.push(33)
  let h = store_indirect(items)
  assert_eq(h.items[0], 33, "transitive escape")
}

// Case 10: Slice from function call (not ExMakeSlice) — should NOT be freed
func test_non_owned_slice() {
  let items = make_list()
  // items was returned from make_list, we own it now
  // but it wasn't created via ExMakeSlice in THIS scope
  assert_eq(items[0], 10, "non-owned slice")
}
