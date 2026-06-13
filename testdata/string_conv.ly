lyric string_conv {
    func main() {
        // itoa
        let s = itoa(42)
        println(s)

        let neg = itoa(-123)
        println(neg)

        // atoi
        let (val, ok) = atoi("99")
        if ok {
            println(val)
        }

        let (val2, ok2) = atoi("not_a_number")
        if !ok2 {
            println("parse failed correctly")
        }

        // char_to_string
        let c: u8 = 'A'
        let cs = char_to_string(c)
        println(cs)

        // StringBuilder
        let sb = new_string_builder()
        sb.write("hello")
        sb.write(" ")
        sb.write("world")
        println(sb.to_string())
        println(sb.len())
    }
}
