// fstring.ly — tests f-string interpolation
lyric fstring_test {
  import fmt from "fmt"

  func main() {
    let name = "Lyric"
    let version = 1

    // Basic interpolation
    fmt.Println(f"Hello, {name}!")

    // Expression in interpolation
    fmt.Println(f"{name} v{version + 1}")

    // Multiple interpolations
    let a = 3
    let b = 4
    fmt.Println(f"{a} + {b} = {a + b}")

    // No interpolation (just f-string syntax)
    fmt.Println(f"plain string")
  }
}
