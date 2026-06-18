// Test trusted func with ref/unref on expressions

class Box {
  value: i32
}

trusted func Box.inc_ref(self) {
  ref self
}

trusted func Box.dec_ref(self) {
  unref self
}

trusted func test_basic_ref() {
  let b = Box { value: 42 }
  ref b
  ref b
  unref b
  unref b
  println(f"value: {b.value}")
}

trusted func test_self_ref() {
  let b = Box { value: 99 }
  b.inc_ref()
  b.dec_ref()
  println(f"self ref: {b.value}")
}

func main() {
  test_basic_ref()
  test_self_ref()
  println("trusted ref/unref OK")
}
