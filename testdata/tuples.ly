lyric tuples {

struct Point { x: i32  y: i32 }

// Basic tuple creation and field access
func test_basic() {
  let t = (42, 99)
  println(t._0)
  println(t._1)
}

// 3-element tuple
func test_triple() {
  let t = (1, 2, 3)
  println(t._0)
  println(t._1)
  println(t._2)
}

// Tuple return
func make_pair() -> (i32, string) {
  return (10, "hello")
}

func test_return() {
  let p = make_pair()
  println(p._0)
  println(p._1)
}

// Tuple with struct
func make_point_pair() -> (Point, i32) {
  return (Point { x: 3, y: 4 }, 99)
}

func test_struct_in_tuple() {
  let t = make_point_pair()
  println(t._0.x)
  println(t._0.y)
  println(t._1)
}

// Tuple of two structs
func make_two_points() -> (Point, Point) {
  return (Point { x: 1, y: 2 }, Point { x: 3, y: 4 })
}

func test_two_structs() {
  let t = make_two_points()
  println(t._0.x)
  println(t._1.y)
}

// Tuple as function argument
func sum_pair(p: (i32, i32)) -> i32 {
  return p._0 + p._1
}

func test_arg() {
  let result = sum_pair((5, 7))
  println(result)
}

func main() {
  test_basic()
  test_triple()
  test_return()
  test_struct_in_tuple()
  test_two_structs()
  test_arg()
}

}
