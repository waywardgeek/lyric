// generics.ly — end-to-end test for generic functions and types

lyric generics {

func identity<T>(x: T) -> T {
    return x
}

func max<T: Comparable>(a: T, b: T) -> T {
    if a > b {
        return a
    }
    return b
}

func apply<T, U>(x: T, f: T -> U) -> U {
    return f(x)
}

func main() {
    let a = identity<i32>(42)
    let b = identity<string>("hello")
    let c = max<i32>(10, 20)
    let d = identity<i64>(100)
    println(f"identity<i32>(42) = {a}")
    println(f"identity<string>(\"hello\") = {b}")
    println(f"max<i32>(10, 20) = {c}")
    println(f"identity<i64>(100) = {d}")
}

}
