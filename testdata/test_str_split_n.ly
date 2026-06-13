lyric test_split {
  pub func main() {
    // Basic split with limit
    let parts = str_split_n("fmt.Println.extra", ".", 2)
    println(len(parts))       // 2
    println(parts[0])         // fmt
    println(parts[1])         // Println.extra

    // n=1 returns whole string
    let one = str_split_n("a.b.c", ".", 1)
    println(len(one))         // 1
    println(one[0])           // a.b.c

    // n=0 or negative: all parts
    let all = str_split_n("a.b.c", ".", 0)
    println(len(all))         // 3

    // No separator found
    let none = str_split_n("hello", ".", 2)
    println(len(none))        // 1
    println(none[0])          // hello

    println("str_split_n OK")
  }
}
