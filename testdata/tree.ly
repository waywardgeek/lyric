// tree.ly — capability-interface design for tree algorithms (v4)
//
// Companion to graph.ly.  See its preamble for the design rules.
// This file exercises a SELF-RELATION (same class on both sides),
// using the UNLABELED-FLAT form of relation declaration: a single
// self-relation with no labels injects all hint members directly
// into the class namespace.
//
// graph.ly uses LABELED relations because the FPGA case has two
// distinct relations on the same (Route, Via) pair.  tree.ly uses
// UNLABELED because there's only one relation on (Folder, Folder),
// and the flat form reads cleaner.
//
// Algorithm constraints follow suit: graph.ly's constraints carry
// labels (`Collection<N:out, E:source>`); tree.ly's constraints do
// not (`Collection<N, N>`).  The algorithm author chooses; users
// either match or provide a bridging impl.

// =========================================================================
// PART 1.  Capability interfaces
// =========================================================================
//
// Collection is reused from graph.ly's design (would be in a shared
// `relations` package).  Redeclared here for self-containment.

interface Collection<P, C> {
    pub func P.iter(self) -> gen C
    pub func P.count(self) -> i32
    pub func C.parent(self) -> P?
}

// HasRoot — a tree-wrapper type has one root node.  Returns Option
// because an empty tree is a real shape.  Auto-derives from any field
// of type `N?` named `root` (or from any method of that signature).

interface HasRoot<T, N> {
    pub func T.root(self) -> N?
}

// =========================================================================
// PART 2.  Constraint aliases
// =========================================================================
//
// TreeNode<N> requires the unlabeled Collection self-relation on N —
// `iter`, `count`, and `parent` injected flat into the class
// namespace.  Algorithm authors who prefer label-distinguished
// children/parent sides would write `Collection<N:children, N:parent>`
// instead; users would then need a labeled relation
// (`relation DoublyLinked Folder:children owns [Folder:parent]`) to
// satisfy it.  Both styles are legal; tree.ly picks the flat style
// because tree shapes have a natural single-relation reading.

constraint TreeNode<N> = Collection<N, N>

constraint Tree<T, N> = HasRoot<T, N> + TreeNode<N>

// =========================================================================
// PART 3.  Algorithm free functions
// =========================================================================
//
// Node-only algorithms (subtree traversal) constrain on `TreeNode<N>`.
// Tree-spanning algorithms (full traversals from root) constrain on
// the full `Tree<T, N>`.  Tighter constraints = clearer requirements.
//
// All algorithm type parameters are declared explicitly inside `<>`
// per the redesign's §6.1 rule (call-site inference still works for
// callers; only the declaration sites need explicit `<T>`).

pub func<N> N.is_leaf(self) -> bool where TreeNode<N> {
    return self.count == 0
}

pub func<N> N.is_root(self) -> bool where TreeNode<N> {
    return isnull(self.parent)
}

pub func<N> N.depth(self) -> i32 where TreeNode<N> {
    let mut d: i32 = 0
    let mut cur: N? = self.parent
    while !isnull(cur) {
        d = d + 1
        cur = cur!.parent
    }
    return d
}

pub func<N> N.subtree_size(self) -> i32 where TreeNode<N> {
    let mut total: i32 = 1
    for c in self.iter() {
        total = total + c.subtree_size()
    }
    return total
}

pub func<N> N.path_to_root(self) -> [N] where TreeNode<N> {
    let mut result: [N] = [self]
    let mut cur: N? = self.parent
    while !isnull(cur) {
        result = append(result, cur!)
        cur = cur!.parent
    }
    return result
}

pub func<N> N.common_ancestor(self, other: N) -> N? where TreeNode<N> {
    // TODO(hashable): linear-search on the path slice.  Switch to
    // Dict<N, bool> for O(1) lookup once `Hashable.equals` is
    // restored.  Same caveat as graph.ly bfs.
    let on_self_path = self.path_to_root()
    let mut cur: N? = other
    while !isnull(cur) {
        if on_self_path.contains(cur!) {
            return cur
        }
        cur = cur!.parent
    }
    return null
}

pub func<T, N> T.count_nodes(self) -> i32 where Tree<T, N> {
    let r = self.root
    if isnull(r) { return 0 }
    return r!.subtree_size()
}

pub func<T, N> T.walk(self) -> [N] where Tree<T, N> {
    let r = self.root
    if isnull(r) { return [] }
    return collect_pre_order(r!, [])
}

pub func<N> collect_pre_order(n: N, acc: [N]) -> [N] where TreeNode<N> {
    let mut r = append(acc, n)
    for c in n.iter() {
        r = collect_pre_order(c, r)
    }
    return r
}

// =========================================================================
// PART 4.  Filesystem  (unlabeled self-relation — flat injection)
// =========================================================================

class Filesystem {
    name: string
    root: Folder?            // singleton root — plain field, satisfies HasRoot
}

class Folder {
    name: string
    size_bytes: i64
}

// Unlabeled self-relation: a single relation between (Folder, Folder)
// needs no labels.  All hint members inject flat into Folder:
//   Parent-side: first, last, iter(), count, append(Folder), remove(Folder)
//   Child-side:  parent, next, prev
// P-side and C-side names don't collide (children/iter/count/append
// vs. parent/next/prev), so a single unlabeled self-relation is safe.
relation DoublyLinked Folder owns [Folder]

// Auto-derivation status — all green, no impl blocks:
//   HasRoot<Filesystem, Folder>   — Filesystem.root field
//   Collection<Folder, Folder>     — Folder's unlabeled self-relation
//                                   gives iter/count/parent directly.
// Together: Tree<Filesystem, Folder> is satisfied.

pub func count_folders(fs: Filesystem) -> i32 {
    return fs.count_nodes()
}

pub func print_tree(fs: Filesystem) {
    for f in fs.walk() {
        let d = f.depth()
        let mut indent = ""
        let mut i: i32 = 0
        while i < d {
            indent = indent + "  "
            i = i + 1
        }
        println(f"{indent}{f.name}/")
    }
}

pub func deepest_common(a: Folder, b: Folder) -> Folder? {
    return a.common_ancestor(b)
}

// =========================================================================
// Summary
// =========================================================================
//
// One Tree library, one implementation.  All algorithm satisfaction is
// auto-derived because the user's unlabeled self-relation gives
// exactly what `Collection<N, N>` requires.  Zero impl blocks.
//
// Three design points the self-relation case exercises that graph.ly
// doesn't:
//
//   1. **Unlabeled-flat injection for single self-relations.**
//      `relation DoublyLinked Folder owns [Folder]` injects all hint
//      members directly into Folder — no `.label.` prefix — because
//      P-side and C-side member names don't collide and there's only
//      one such relation.  Reads naturally: `folder.parent`,
//      `folder.iter()`, `folder.append(child)`.
//
//   2. **Constraint signature follows the relation style.**
//      `TreeNode<N> = Collection<N, N>` carries no labels because the
//      user's relation carries no labels.  graph.ly's
//      `DirectedGraph<G, N, E>` carries labels (`:out`, `:in`,
//      `:source`, `:target`) because its users have multiple relations
//      on the same `(P, C)` pair.  Algorithm authors choose; tree-
//      shaped data picks flat.
//
//   3. **Constraint subset for node-only algorithms.**  `subtree_size`
//      constrains on `TreeNode<N>` (just the self-relation), not the
//      full `Tree<T, N>`.  Algorithms ask for what they need; users
//      get those algorithms even on partial implementations (e.g., a
//      Node disconnected from any Tree-wrapper still has subtree_size,
//      depth, common_ancestor).
//
// What this final design demonstrates beyond graph.ly:
//
//   - Constraint composition: `Tree<T, N> = HasRoot<T, N> + TreeNode<N>`
//     bundles a wrapper-side capability with a node-side capability.
//   - Self-relation flat-form auto-derive: the same Collection
//     mechanism that works for cross-class relations works unchanged
//     for self-relations, because flat injection means the required
//     surface lives directly on N.
//   - Algorithm constraint tightening: `T.count_nodes` requires the
//     full Tree (needs root); `N.subtree_size` requires only TreeNode
//     (works on any subtree, no enclosing root needed).
//   - Explicit `<N>` and `<T, N>` on every algorithm declaration
//     per the redesign's explicit-type-params-on-decl rule.
