lyric Interfaces {
  interface Graph<G, N, E> {
    func G.nodes(self) -> [N]
    func N.out_edges(self) -> [E]
    func E.tgt_node(self) -> N

    // Default implementation: count all edges in the graph
    pub func count_edges(graph: G) -> i32 {
      let mut total: i32 = 0
      let nodes = graph.nodes()
      let mut i: i32 = 0
      while i < len(nodes) {
        let edges = nodes[i].out_edges()
        total = total + len(edges)
        i = i + 1
      }
      return total
    }
  }

  // DoublyLinked with default fields and a default append method
  interface DoublyLinked<P, C> {
    func P.children(self) -> [C]
    func C.label(self) -> string

    // Default fields injected into concrete classes via relation
    field P.first: C?
    field P.last: C?
    field C.prev: C?
    field C.next: C?
    field C.parent: P?

    // Default method: append child to parent's linked list
    pub func dll_append(parent: P, child: C) {
      let old_last = parent.last()
      parent.set_last(child)
      child.set_prev(old_last)
      child.set_parent(parent)
      if isnull(old_last) {
        parent.set_first(child)
      } else {
        old_last!.set_next(child)
      }
    }

    // Destructor for parent: cascade destroy all owned children
    destructor P {
      while !isnull(self.first()) {
        self.first()!.destroy()
      }
    }

    // Destructor for child: unlink from parent's doubly-linked list
    destructor C {
      let p = self.parent()
      let prev_node = self.prev()
      let next_node = self.next()
      if !isnull(prev_node) {
        prev_node!.set_next(next_node)
      } else if !isnull(p) {
        p!.set_first(next_node)
      }
      if !isnull(next_node) {
        next_node!.set_prev(prev_node)
      } else if !isnull(p) {
        p!.set_last(prev_node)
      }
      self.set_parent(null)
      self.set_prev(null)
      self.set_next(null)
    }
  }

  // Classes use DIFFERENT method names than the interface
  class SimpleGraph {
    node_list: [SimpleNode]

    pub func get_nodes(self) -> [SimpleNode] {
      return self.node_list
    }
  }

  class SimpleNode {
    name: string
    edges: [SimpleEdge]

    pub func get_edges(self) -> [SimpleEdge] {
      return self.edges
    }
  }

  class SimpleEdge {
    target: SimpleNode

    pub func get_target(self) -> SimpleNode {
      return self.target
    }
  }

  // impl block wires different-named methods to interface methods
  impl Graph<SimpleGraph, SimpleNode, SimpleEdge> {
    G.nodes = SimpleGraph.get_nodes
    N.out_edges = SimpleNode.get_edges
    E.tgt_node = SimpleEdge.get_target
  }

  // Folder and File classes — relation will inject default fields
  class Folder {
    name: string
    items: [File]
  }

  class File { title: string }

  // This relation injects DoublyLinked default fields into Folder and File
  relation DoublyLinked Folder:files owns [File:src]

  impl DoublyLinked<Folder, File> {
    P.children <-> Folder.items
    C.label <-> File.title
  }

  pub func count_children<P, C>(p: P) -> i32 where DoublyLinked<P, C> {
    let kids = p.children()
    return len(kids)
  }

  func main() {
    let n2 = SimpleNode { name: "B", edges: [] }
    let e1 = SimpleEdge { target: n2 }
    let n1 = SimpleNode { name: "A", edges: [e1] }
    let g = SimpleGraph { node_list: [n1, n2] }
    let count = count_edges<SimpleGraph, SimpleNode, SimpleEdge>(g)
    println(count)

    let f1 = File { title: "readme.txt" }
    let f2 = File { title: "data.csv" }
    let dir = Folder { name: "docs", items: [f1, f2] }
    let num = count_children<Folder, File>(dir)
    println(num)

    // Test default append method
    let dir2 = Folder { name: "empty", items: [] }
    let f3 = File { title: "new.txt" }
    let f4 = File { title: "old.txt" }
    dll_append<Folder, File>(dir2, f3)
    dll_append<Folder, File>(dir2, f4)
    if !isnull(dir2.files_first) {
      println("append works")
    }
    if !isnull(f3.src.parent) {
      println("parent set")
    }

    // Test child destroy (unlink f3 from list, f4 should become first)
    f3.destroy()
    if isnull(f3.src.parent) {
      println("child unlinked")
    }
    if !isnull(dir2.files_first) {
      println(dir2.files_first!.title)
    }

    // Test parent destroy (cascade destroys remaining children)
    let dir3 = Folder { name: "temp", items: [] }
    let f5 = File { title: "a.txt" }
    let f6 = File { title: "b.txt" }
    dll_append<Folder, File>(dir3, f5)
    dll_append<Folder, File>(dir3, f6)
    dir3.destroy()
    if isnull(f5.src.parent) {
      println("cascade works")
    }
  }
}
