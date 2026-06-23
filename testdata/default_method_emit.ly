// default_method_emit.ly — Phase 4 Wave 1 4w1-a regression test.
//
// Exercises the desugar pass `desugar_specialize_default_impls`:
// a user-authored multi-class interface declares a default method
// whose body uses the abstract surface on a non-receiver type-var
// (count_edges iterates self.nodes() then n.outgoing_edges()).
//
// Receiver-only type inference can recover G from g.count_edges()
// but cannot recover N. The desugar pass substitutes G→MyGraph and
// N→MyNode at desugar time using the impl block's bindings, emits
// a concrete MyGraph.count_edges, and the body's n.outgoing_edges()
// resolves through the alias wrapper MyNode_DiGraph_outgoing_edges
// the lowerer synthesizes from the impl's `N.outgoing_edges =
// MyNode.outgoing_edges` mapping.
//
// Also exercises the alias-wrapper self-recursion fix in the
// lowerer: the wrapper body calls the user-helper by its mangled
// C name directly (ExCall), bypassing impl_method_renames so the
// wrapper doesn't recurse into itself.

lyric default_method_emit {

  interface DiGraph<G, N, E> {
    pub func G.nodes(self) -> [N]
    pub func N.outgoing_edges(self) -> [E]

    pub func G.count_edges(self) -> i32 {
      let mut total: i32 = 0
      for n in self.nodes() {
        for _e in n.outgoing_edges() { total = total + 1 }
      }
      return total
    }

    pub func G.has_self_loop(self) -> bool {
      for n in self.nodes() {
        if len(n.outgoing_edges()) > 0 { return true }
      }
      return false
    }
  }

  class MyGraph { name: string }
  class MyNode  { id: i32 }
  class MyEdge  { weight: i32 }

  relation ArrayList MyGraph:my_nodes owns [MyNode:graph]
  relation ArrayList MyNode:my_edges  refs [MyEdge:src]

  pub func MyGraph.nodes(self) -> [MyNode] {
    let mut r: [MyNode] = []
    for n in self.my_nodes.children() { r = append(r, n) }
    return r
  }

  pub func MyNode.outgoing_edges(self) -> [MyEdge] {
    let mut r: [MyEdge] = []
    for e in self.my_edges.children() { r = append(r, e) }
    return r
  }

  impl DiGraph<MyGraph, MyNode, MyEdge> {
    G.nodes          = MyGraph.nodes
    N.outgoing_edges = MyNode.outgoing_edges
  }

  func main() -> i32 {
    // Build a graph with 2 nodes and 3 edges.
    let g = MyGraph { name: "g" }
    let n1 = MyNode { id: 1 }
    let n2 = MyNode { id: 2 }
    array_append<MyGraph, MyNode>(g, n1)
    array_append<MyGraph, MyNode>(g, n2)
    let e1 = MyEdge { weight: 5 }
    let e2 = MyEdge { weight: 3 }
    let e3 = MyEdge { weight: 7 }
    array_append<MyNode, MyEdge>(n1, e1)
    array_append<MyNode, MyEdge>(n1, e2)
    array_append<MyNode, MyEdge>(n2, e3)

    // Default method specialized into MyGraph.count_edges by
    // desugar_specialize_default_impls.
    let c = g.count_edges()
    assert_eq(c, 3, "count_edges should sum edges across all nodes")

    // Second default method, same specialization mechanism.
    let h = g.has_self_loop()
    assert_eq(h, true, "has_self_loop true when any node has outgoing edges")

    // Empty graph exercises the false branch.
    let g2 = MyGraph { name: "empty" }
    assert_eq(g2.count_edges(), 0, "empty graph: zero edges")
    assert_eq(g2.has_self_loop(), false, "empty graph: no self loop")

    return 0
  }
}
