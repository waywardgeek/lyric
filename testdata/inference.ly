// inference.ly — tests call-site type inference for generic functions
lyric inference_demo {

  func identity<T>(x: T) -> T {
    return x
  }

  func first<T>(xs: [T]) -> T? {
    if len(xs) == 0 {
      return null
    }
    return xs[0]
  }

  func transform<T, U>(xs: [T], f: (T) -> U) -> [U] {
    let mut result: [U] = []
    for x in xs {
      result.push(f(x))
    }
    return result
  }

  func max_val<T: Comparable>(a: T, b: T) -> T {
    if a > b {
      return a
    }
    return b
  }

  func main() -> unit {
    // Single type param, inferred from argument
    let x = identity(42)
    println(f"identity(42) = {x}")

    let s = identity("hello")
    println(f"identity(hello) = {s}")

    // Inferred from list argument
    let nums: [i32] = [10, 20, 30]
    let f = first(nums)
    println(f"first([10,20,30]) = {f!}")

    // Two type params — T inferred from nums, U inferred from lambda return type
    let to_str = |n: i32| -> string { return f"{n * 2}" }
    let doubled = transform(nums, to_str)
    println(f"doubled: {doubled.join(", ")}")

    // Constraint-based, inferred
    let m = max_val(10, 20)
    println(f"max(10, 20) = {m}")

    // Explicit type args still work
    let big: i64 = 100
    let y = identity<i64>(big)
    println(f"identity<i64>(100) = {y}")
  }
}
