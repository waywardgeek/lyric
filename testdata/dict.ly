lyric dict_test {
  func main() {
    // Create a Dict<Sym, i32> using constructor syntax
    let d = Dict<Sym, i32>()

    // Insert values using backtick sym syntax
    d.set(`x`, 10)
    d.set(`y`, 20)
    d.set(`z`, 30)

    // Lookup
    let ex = d.get(`x`)
    if !isnull(ex) {
      println(itoa(ex!.value))
    }

    // Has
    if d.has(`y`) {
      println("has y")
    }
    if !d.has(`w`) {
      println("no w")
    }

    // Keys
    let keys = d.keys()
    println(itoa(len(keys)))

    // Remove
    d.remove(`y`)
    if !d.has(`y`) {
      println("y removed")
    }

    // Sym interning - reference equality
    let a = `hello`
    let b = `hello`
    if a == b {
      println("sym interning works")
    }
    println(a.get_name())
  }
}
