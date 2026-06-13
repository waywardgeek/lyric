// mylib/types.ly — exported types
lyric mylib {
  pub struct Point {
    x: i32
    y: i32
  }

  pub func new_point(x: i32, y: i32) -> Point {
    return Point { x: x, y: y }
  }
}
