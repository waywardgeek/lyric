// tree.ly — capability-interface design for tree algorithms (v3)
//
// Companion to graph.ly.  See its preamble for the design rules.
// This file exercises a SELF-RELATION (same class on both sides),
// which is the second nontrivial case for labels-as-scopes after the
// multi-relation-same-pair case in graph.ly.

// =========================================================================
// PART 1.  Capability interfaces
// =========================================================================
//
// ManyToOne is reused from graph.ly's design (would be in a shared
// `relations` package).  Redeclared here for self-containment.

interface ManyToOne<P, C> {
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
// TreeNode<N> captures the self-relation on N: children scope on the
// parent side, parent scope on the child side.  Two labels because
// self-relations live in one class's namespace and must distinguish
// the two sides.

constraint TreeNode<N> = ManyToOne<N:children, N:parent>

constraint Tree<T, N> = HasRoot<T, N> + TreeNode<N>

// =========================================================================
// PART 3.  Algorithm free functions
// =========================================================================
//
// Node-only algorithms (subtree traversal) constrain on `TreeNode<N>`.
// Tree-spanning algorithms (full traversals from root) constrain on
// the full `Tree<T, N>`.  Tighter constraints = clearer requirements.

pub func N.is_leaf(self) -> bool where TreeNode<N> {
    return self.children.count == 0
}

pub func N.is_root(self) -> bool where TreeNode<N> {
    return isnull(self.parent)              // auto-projection: child-side scope yields parent
}

pub func N.depth(self) -> i32 where TreeNode<N> {
    let mut d: i32 = 0
    let mut cur: N? = self.parent
    while !isnull(cur) {
        d = d + 1
        cur = cur!.parent
    }
    return d
}

pub func N.subtree_size(self) -> i32 where TreeNode<N> {
    let mut total: i32 = 1
    for c in self.children.iter() {
        total = total + c.subtree_size()
    }
    return total
}

pub func N.path_to_root(self) -> [N] where TreeNode<N> {
    let mut result: [N] = [self]
    let mut cur: N? = self.parent
    while !isnull(cur) {
        result = append(result, cur!)
        cur = cur!.parent
    }
    return result
}

pub func N.common_ancestor(self, other: N) -> N? where TreeNode<N> {
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

pub func T.count_nodes(self) -> i32 where Tree<T, N> {
    let r = self.root
    if isnull(r) { return 0 }
    return r!.subtree_size()
}

pub func T.walk(self) -> [N] where Tree<T, N> {
    let r = self.root
    if isnull(r) { return [] }
    return collect_pre_order(r!, [])
}

pub func collect_pre_order<N>(n: N, acc: [N]) -> [N] where TreeNode<N> {
    let mut r = append(acc, n)
    for c in n.children.iter() {
        r = collect_pre_order(c, r)
    }
    return r
}

// =========================================================================
// PART 4.  Filesystem  (semantic labels match — zero impl blocks)
// =========================================================================

class Filesystem {
    name: string
    root: Folder?            // singleton root — plain field, satisfies HasRoot
}

class Folder {
    name: string
    size_bytes: i64
}

// Self-relation with role-named labels.  Distinct labels mandatory
// in self-relations (one namespace).  Labels match TreeNode's expected
// names, so ManyToOne<Folder:children, Folder:parent> auto-derives.
relation DoublyLinked Folder:children owns [Folder:parent]

// Auto-derivation status — all green, no impl blocks:
//   HasRoot<Filesystem, Folder>           — Filesystem.root field
//   ManyToOne<Folder:children, Folder:parent> — Folder self-relation
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
// auto-derived because the user's relation labels happen to match the
// constraint's expected names.  Zero impl blocks.
//
// Three design points the self-relation case exercises that graph.ly
// doesn't:
//
//   1. **Distinct labels mandatory** in self-relations.  `Folder:children
//      owns [Folder:parent]` uses two labels because both sides live in
//      Folder's namespace; same-letter (legal in cross-class relations)
//      would collide here.
//
//   2. **Auto-projection of singleton scopes.**  `node.parent` is the
//      parent Node (typed `N?`), not a scope handle.  Without it, the
//      body would say `node.parent.parent` — once for the scope name,
//      once for the back-pointer field inside.  Auto-projection lets
//      `is_root(n) = isnull(n.parent)` read as intended.
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
//   - Self-relation auto-derive: the same ManyToOne mechanism that
//     works for cross-class relations works unchanged for self-
//     relations, given distinct labels.
//   - Algorithm constraint tightening: `T.count_nodes` requires the
//     full Tree (needs root); `N.subtree_size` requires only TreeNode
//     (works on any subtree, no enclosing root needed).
