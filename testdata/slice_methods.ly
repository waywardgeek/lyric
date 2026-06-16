// slice_methods.ly — tests slice methods: is_empty, first, last, index_of, remove, clear, reverse
lyric slice_methods {
  func main() {
    // is_empty
    let empty: [i32] = []
    let nonempty: [i32] = [10, 20, 30]
    println(empty.is_empty())
    println(nonempty.is_empty())

    // first and last
    let v = nonempty.first()
    if !isnull(v) { println(v!) }
    let w = nonempty.last()
    if !isnull(w) { println(w!) }

    // first/last on empty
    let a: [i32] = []
    println(isnull(a.first()))
    println(isnull(a.last()))

    // index_of
    let xs: [i32] = [5, 10, 15, 20]
    println(xs.index_of(10))
    println(xs.index_of(99))

    // remove
    let mut ys: [i32] = [1, 2, 3, 4, 5]
    ys.remove(3)
    println(len(ys))
    println(ys[0])
    println(ys[1])
    println(ys[2])
    println(ys[3])

    // clear
    let mut zs: [i32] = [7, 8, 9]
    zs.clear()
    println(len(zs))
    println(zs.is_empty())

    // reverse
    let mut rs: [i32] = [1, 2, 3, 4]
    rs.reverse()
    println(rs[0])
    println(rs[1])
    println(rs[2])
    println(rs[3])
  }
}
