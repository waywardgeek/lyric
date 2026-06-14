lyric method_iface_test {

  pub interface MyList<P, C> {
    field P.items: [C]
    field C.owner: P?
    field C.pos: i32

    pub func P.add(self, child: C) {
      let kids = self.items()
      let num: i32 = len(kids)
      child.set_pos(num)
      child.set_owner(self)
      self.set_items(append(kids, child))
    }

    pub func P.count(self) -> i32 {
      return len(self.items()) as i32
    }
  }

  class Widget {
    label: string
  }

  class Panel {}

  relation MyList Panel:w owns [Widget:p]

  func test_method_syntax() {
    let panel = Panel {}
    let w1 = Widget { label: "button" }
    let w2 = Widget { label: "text" }

    panel.add(w1)
    panel.add(w2)

    assert_eq(panel.count(), 2)

    let kids = panel.w_items()
    assert_eq(kids[0].label, "button")
    assert_eq(kids[1].label, "text")

    println("PASS: test_method_syntax")
  }

  func main() {
    test_method_syntax()
  }
}
