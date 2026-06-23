// dll_children_and_multirel.ly — regression for two paired fixes:
//   1. stdlib DoublyLinked now exposes `pub func P.children(self) -> gen C`
//      as a zero-allocation forward iterator. Per-label injection means
//      `parent.label.children()` walks just that relation's list.
//   2. desugar_relations now treats per-side labels as part of the
//      impl-block identity in the find-or-create lookup. Two
//      DoublyLinked relations on the same (P, C) pair with different
//      labels produce two independent impls (and thus two independent
//      sets of label-prefixed storage + iter wrappers).
//
// Before fix 2, the second relation's labels were silently dropped
// and `obj.<second_label>.append(...)` failed with "field 'X' not
// found".

class Node { name: string }
class Edge { id: i32 }
relation DoublyLinked Node:fwd  refs [Edge:a]
relation DoublyLinked Node:bwd  refs [Edge:b]

pub func test_single_dll() {
  let n = Node { name: "single" }
  n.fwd.append(Edge { id: 1 })
  n.fwd.append(Edge { id: 2 })
  n.fwd.append(Edge { id: 4 })
  let mut s: i32 = 0
  for e in n.fwd.children() { s = s + e.id }
  assert_eq(s, 7)
}

pub func test_two_dlls_independent() {
  let n = Node { name: "two" }
  n.fwd.append(Edge { id: 1 })
  n.fwd.append(Edge { id: 2 })
  n.bwd.append(Edge { id: 8 })
  n.bwd.append(Edge { id: 16 })
  let mut f: i32 = 0
  for e in n.fwd.children() { f = f + e.id }
  let mut b: i32 = 0
  for e in n.bwd.children() { b = b + e.id }
  assert_eq(f, 3)
  assert_eq(b, 24)
}

pub func test_empty_children_is_noop() {
  let n = Node { name: "empty" }
  let mut count: i32 = 0
  for _e in n.fwd.children() { count = count + 1 }
  for _e in n.bwd.children() { count = count + 1 }
  assert_eq(count, 0)
}
