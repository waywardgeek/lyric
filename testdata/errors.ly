lyric errors {
  import fmt from "fmt"
  import errors from "errors"

  func divide(a: i32, b: i32) -> (i32, error) {
    if b == 0 {
      return (0, errors.New("division by zero"))
    }
    return (a / b, nil)
  }

  func must_divide(a: i32, b: i32) -> i32 {
    let (result, err) = divide(a, b)
    if err != nil {
      println(f"Error: {err}")
      return 0
    }
    return result
  }

  func main() {
    let (val, err) = divide(10, 2)
    if err != nil {
      println(f"Error: {err}")
    }
    println(f"10 / 2 = {val}")

    let (_, err2) = divide(10, 0)
    if err2 != nil {
      println(f"Expected error: {err2}")
    }
  }
}
