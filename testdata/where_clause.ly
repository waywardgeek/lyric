// Test: where clause constraints

lyric where_test {

func max_val<T>(a: T, b: T) -> T
  where T: Comparable
{
  if a > b {
    return a
  }
  return b
}

func main() {
  let result = max_val<i32>(10, 20)
  println(f"max: {result}")
}

}
