// Test: a class with TWO ArrayList relations
// Reproduces the bug where array_remove uses wrong label's children accessor

lyric multi_relation {

  class Child1 {
    val: i32
  }

  class Child2 {
    val: i32
  }

  class Parent {
    name: string
  }
  relation ArrayList Parent:c1 owns [Child1:c1]
  relation ArrayList Parent:c2 owns [Child2:c2]

  func main() {
    test_multi_relation()
  }

  func test_multi_relation() {
    let p = Parent { name: "test" }
    let a = Child1 { val: 1 }
    let b = Child2 { val: 2 }
    array_append<Parent, Child1>(p, a)
    array_append<Parent, Child2>(p, b)
    print(len(p.c1.children()))
    print(len(p.c2.children()))
    array_remove<Parent, Child1>(a)
    print(len(p.c1.children()))
    print(len(p.c2.children()))
  }
}
