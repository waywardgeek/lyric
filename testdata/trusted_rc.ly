lyric trusted_rc {

  class Parent { name: string }
  class Child { name: string }
  relation RefArrayList Parent refs [Child]

  final func Parent.on_destroy(self) {
    println(f"destroying parent: {self.name}")
  }

  final func Child.on_destroy(self) {
    println(f"destroying child: {self.name}")
  }

  func main() {
    {
      let p = Parent { name: "Bob" }
      let c = Child { name: "Carol" }
      p.append(c)
      println(p.name)
      println(c.name)
    }
    println("done")
  }
}
