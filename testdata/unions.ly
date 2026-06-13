// unions.ly — end-to-end test for union types

lyric unions {

func describe(val: string | i32) -> string {
    return match val {
        string => { "it's a string" }
        i32 => { "it's an int" }
    }
}

func stmt_match(val: string | i32) {
    match val {
        string => { println("string!") }
        i32 => { println("int!") }
    }
}

func with_default(val: string | i32 | bool) -> string {
    return match val {
        string => { "string" }
        _ => { "other" }
    }
}

func main() {
    let a: string | i32 = 42
    let b: string | i32 = "hello"
    println(describe(a))
    println(describe(b))
    stmt_match(a)
    stmt_match(b)
    
    let c: string | i32 | bool = true
    println(with_default(c))
}

}
