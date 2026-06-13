// any_type.ly — test interface{}-style union type
// 'any' is just a type alias for a union of all primitive types

lyric any_type {
    type Any = string | i32 | i64 | f32 | f64 | bool

    func store(x: Any) -> Any {
        return x
    }

    func main() {
        let a: Any = 42
        let b: Any = "hello"
        let c: Any = true
        println(a)
        println(b)
        println(c)

        let d = store(99)
        println(d)
    }
}
