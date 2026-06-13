lyric optionals {
  import fmt from "fmt"

  // Optional return with value wrapping
  func find(xs: [i32], target: i32) -> i32? {
    for x in xs {
      if x == target {
        return x
      }
    }
    return nil
  }

  // len and append builtins
  func build_list() -> [i32] {
    let mut xs: [i32] = []
    xs = append(xs, 1)
    xs = append(xs, 2)
    xs = append(xs, 3)
    return xs
  }

  // Optional string
  func find_name(names: [string], target: string) -> string? {
    for name in names {
      if name == target {
        return name
      }
    }
    return nil
  }

  func main() {
    let xs = [10, 20, 30, 40, 50]

    // Test unwrap with !
    let found = find(xs, 30)
    if !isnull(found) {
      println(f"found: {found!}")
    }

    // Test isnull
    let missing = find(xs, 99)
    if isnull(missing) {
      println("not found: correct")
    }

    // Test len/append
    let ys = build_list()
    println(f"built: {len(ys)} items")

    // Test optional string with unwrap
    let names = ["alice", "bob", "charlie"]
    let result = find_name(names, "bob")
    if !isnull(result) {
      println(f"found name: {result!}")
    }

    // Test chained: find and unwrap
    println(f"direct unwrap: {find(xs, 20)!}")
  }
}
