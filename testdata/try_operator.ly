lyric try_operator {
  import errors from "errors"
  import fmt from "fmt"

  func divide(a: i32, b: i32) -> (i32, error) {
    if b == 0 {
      return (0, errors.New("division by zero"))
    }
    return (a / b, null)
  }

  func parse_number(s: string) -> (i32, error) {
    if s == "42" {
      return (42, null)
    }
    return (0, errors.New(f"unknown number: {s}"))
  }

  // ? in let statement
  func compute(x: i32) -> (i32, error) {
    let result = divide(x, 2)?
    let doubled = divide(result * 4, 2)?
    return (doubled, null)
  }

  // ? in expression statement (discard value)
  func validate(x: i32) -> (i32, error) {
    divide(x, 1)?
    return (x, null)
  }

  // chain multiple ? calls
  func chain() -> (i32, error) {
    let a = divide(100, 2)?
    let b = divide(a, 5)?
    let c = divide(b, 2)?
    return (c, null)
  }

  func main() {
    // Success cases
    let (r1, err1) = compute(20)
    if err1 != null {
      println(f"unexpected error: {err1}")
    }
    println(f"compute(20) = {r1}")

    let (r2, err2) = chain()
    if err2 != null {
      println(f"unexpected error: {err2}")
    }
    println(f"chain() = {r2}")

    let (r3, err3) = validate(7)
    if err3 != null {
      println(f"unexpected error: {err3}")
    }
    println(f"validate(7) = {r3}")

    // Error case
    let (_, err4) = divide(10, 0)
    if err4 != null {
      println(f"expected error: {err4}")
    }
  }
}
