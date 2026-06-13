// nested_try.ly — tests nested ? operator in expressions

lyric nested_try_demo {
  import errors from "errors"

  func parse_int(s: string) -> (i32, error) {
    if s == "42" {
      return (42, null)
    }
    return (0, errors.New(f"invalid: {s}"))
  }

  func double(x: i32) -> i32 {
    return x * 2
  }

  func process(s: string) -> (i32, error) {
    // Nested ? — pass result of parse_int? directly to double
    let result = double(parse_int(s)?)
    return (result, null)
  }

  func add_parsed(a: string, b: string) -> (i32, error) {
    // Multiple nested ? in one expression
    let sum = parse_int(a)? + parse_int(b)?
    return (sum, null)
  }

  func main() {
    let (r1, e1) = process("42")
    if e1 == null {
      println(f"process result: {r1}")
    }

    let (r2, e2) = add_parsed("42", "42")
    if e2 == null {
      println(f"add result: {r2}")
    }
  }
}
