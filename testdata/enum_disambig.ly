// Test: enum variant disambiguation with data fields
// Two enums with the same variant name "Pair", used in struct literal fields.
// The checker must resolve based on expected field type.

enum Shape {
  Circle(radius: f64)
  Pair(a: f64, b: f64)
}

enum Items {
  Single(name: string)
  Pair(x: string, y: string)
}

struct Container {
  shape: Shape
  items: Items
}

func test_disambig_data() {
  let c = Container {
    shape: Pair(1.0, 2.0),
    items: Pair("hello", "world"),
  }
  match c.shape {
    Pair(a, b) => { println(a, b) }
    _ => { println("wrong shape") }
  }
  match c.items {
    Pair(x, y) => { println(x, y) }
    _ => { println("wrong items") }
  }
}

func main() {
  test_disambig_data()
}
