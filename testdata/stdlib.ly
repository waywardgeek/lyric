// stdlib.ly — tests built-in methods and Go stdlib interop
lyric stdlib_test {
  import strconv from "strconv"

  func main() {
    // String methods replace direct stdlib imports
    let greeting = "hello, lyric!"
    println(greeting.to_upper())

    // String conversion — strconv.Itoa expects Go's int, so cast from i32
    let num = 42
    let s = strconv.Itoa(num as int)
    println("The answer is " + s)

    // Numeric casts between Lyric types
    let x: i32 = 100
    let y: i64 = x as i64
    let z: i32 = y as i32
    println(f"x={x}, y={y}, z={z}")

    // String methods
    let csv = "alice,bob,carol"
    let names = csv.split(",")
    println(f"names: {names.join(", ")}")
    println(f"has bob: {csv.contains("bob")}")
    println(f"starts with alice: {csv.has_prefix("alice")}")
  }
}
