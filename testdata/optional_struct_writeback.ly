// Test: writing through optional struct field on a class should modify the original

struct Inner {
  value: i32
}

class Outer {
  data: Inner?
}

func main() {
  let o = Outer { data: Inner { value: 10 } }

  // This should modify o.data.value to 42
  o.data!.value = 42

  // Read it back
  let v = o.data!.value
  assert_eq(v, 42, "optional struct write-through should work")
  print("PASS")
}
