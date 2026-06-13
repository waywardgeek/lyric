lyric struct_lit {

struct Point {
  x: i32
  y: i32
}

struct Named {
  name: string
  value: i32
}

// Basic struct literal
func test_basic() {
  let p = Point { x: 10, y: 20 }
  println(p.x)
  println(p.y)
}

// Struct literal inside parenthesized expression
func test_parens() {
  let p = (Point { x: 1, y: 2 })
  println(p.x)
  println(p.y)
}

// Struct literal as function argument
func print_point(p: Point) {
  println(f"{p.x},{p.y}")
}

func test_arg() {
  print_point(Point { x: 5, y: 6 })
}

// Struct literal in variable with explicit type
func test_typed() {
  let n: Named = Named { name: "hello", value: 42 }
  println(n.name)
  println(n.value)
}

// Nested struct (struct containing struct)
struct Rect {
  top_left: Point
  bottom_right: Point
}

func test_nested() {
  let r = Rect {
    top_left: Point { x: 0, y: 0 },
    bottom_right: Point { x: 100, y: 200 }
  }
  println(r.top_left.x)
  println(r.bottom_right.y)
}

// Empty struct
struct Empty {}

func test_empty() {
  let e = Empty {}
  println("empty ok")
}

func main() {
  test_basic()
  test_parens()
  test_arg()
  test_typed()
  test_nested()
  test_empty()
}

}
