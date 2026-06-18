lyric trusted_rc {

  class Parent { name: string }
  class Child { name: string }
  relation ArrayList Parent refs [Child]

  func main() {
    {
      let p = Parent { name: "Bob" }
      let c = Child { name: "Carol" }
      array_append<Parent, Child>(p, c)
      c.parent = p
      println(p.name)
      println(c.name)
    }
    println("done")
  }
}
