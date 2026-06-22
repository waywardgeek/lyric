// Regression test: bare `return` inside `main()` must compile.
// Previously the C backend lowered Lyric `return` (no value) to a bare
// `return;` even inside `main`, which is declared as `int main(...)`
// in the emitted C, so gcc rejected it with
//   "'return' with no value, in function returning non-void"
// The fix: inside `main`, lower a bare `return` as `return 0;`.

lyric test_return_in_main {
  func main() {
    let n = 3
    if n > 0 {
      println("early return path")
      return
    }
    // Should be unreachable when n > 0.
    println("fallthrough path")
  }
}
