// graph.ly — capability-interface design for graph algorithms (v3)
//
// Companion to tree.ly.  Both files demonstrate the layered design
// agreed in the multi-class-interface-redesign discussion:
//
//   Storage     = relations (labels-as-scopes; auto-injected accessors).
//   Capability  = small multi-class interfaces.
//   Algorithm   = free functions with where-clauses (UFCS at call site).
//   Composition = constraint aliases bundle related capabilities.
//   Bridge      = aggressive auto-derive; explicit impl when ambiguous.
//
// Key rules (proposed for the spec, not yet adopted):
//
//   1. Methods are class-scoped functions.  `func T.method(self, ...)`.
//      UFCS: `obj.method(args)` and (for zero-arg) `obj.method`.  At
//      module top level the receiver is explicit; inside a class body
//      or single-class impl block it's implicit; inside a multi-class
//      impl block it uses the interface's type-variable name.
//
//   2. Fields/methods unified.  A field `name: T` auto-defines getter
//      `T.name(self) -> T` and setter `obj.name = v`.  Same-named
//      field + method = compile error.
//
//   3. Auto-derive of interface satisfaction (3 rules, in order):
//      a. Explicit `impl` always wins.
//      b. Unique name+signature match in the class scope auto-satisfies
//         (declared methods, field-derived getters, relation accessors —
//         all in one pot).
//      c. No match OR ambiguous match → error; user provides impl.
//
//   4. Generic-on-numeric.  Numeric is a built-in constraint covering
//      i8..i64, u8..u64, f32, f64.  `Numeric<T>` requires `zero`, `one`,
//      `add`, `mul` (static + instance).
//
//   5. Method-call type args banned.  `obj.method<T>(args)` is a parse
//      error; receiver and where-clause pin all type params.  Free-
//      function calls still accept type args for the rare case where T
//      appears only in the return type.
//
//   6. Relation-derived methods fully monomorphized at desugar.  By
//      checker time, every `obj.method(args)` is a concrete call.
//
//   7. Labels in where-clauses (`ManyToOne<N:out, E:source>`) name
//      *which* relation a constraint binds to when multiple match.

// =========================================================================
// PART 1.  Capability interfaces  (would live in `graph` package)
// =========================================================================
//
// ManyToOne bundles what every relation hint provides: parent-side
// iteration + count, child-side back-pointer.  The 3 stdlib hints
// (ArrayList, DoublyLinked, HashedList) all auto-derive ManyToOne.

interface ManyToOne<P, C> {
    pub func P.iter(self) -> gen C
    pub func P.count(self) -> i32
    pub func C.parent(self) -> P?
}

// Numeric is a built-in constraint (added in this redesign) covering
// the integer and floating types.  Algorithms parameterize over W so
// users with f32 or i64 weights aren't forced to f64.

interface Numeric<T> {
    pub func T.zero() -> T          // static factory
    pub func T.one() -> T           // static factory
    pub func T.add(self, other: T) -> T
    pub func T.mul(self, other: T) -> T
}

interface EdgeWeight<E, W: Numeric> {
    pub func E.weight(self) -> W
}

// =========================================================================
// PART 2.  Constraint aliases  (the algorithm-category names)
// =========================================================================
//
// Constraint aliases are pure names for constraint bundles.  They
// have no methods of their own.  Algorithms reference them in where-
// clauses to avoid repetition.

constraint DirectedGraph<G, N, E> =
    ManyToOne<G:nodes, N:graph> +
    ManyToOne<N:out,   E:source> +
    ManyToOne<N:in,    E:target>

constraint WeightedDirectedGraph<G, N, E, W> =
    DirectedGraph<G, N, E> + EdgeWeight<E, W>

// =========================================================================
// PART 3.  Algorithm free functions
// =========================================================================
//
// Free functions with where-clauses.  UFCS makes them callable as
// methods on the receiver: `net.bfs(alice)`, `net.total_weight()`.
// Type parameters are inferred from the signature and where-clause
// (no explicit `<G, N, E>` declaration needed).

pub func G.count_edges(self) -> i32 where DirectedGraph<G, N, E> {
    let mut total: i32 = 0
    for n in self.nodes.iter() {
        total = total + n.out.count
    }
    return total
}

pub func N.successors(self) -> [N] where DirectedGraph<G, N, E> {
    let mut result: [N] = []
    for e in self.out.iter() {
        result = append(result, e.target)
    }
    return result
}

pub func N.predecessors(self) -> [N] where DirectedGraph<G, N, E> {
    let mut result: [N] = []
    for e in self.in.iter() {
        result = append(result, e.source)
    }
    return result
}

pub func G.has_edge(self, src: N, dst: N) -> bool where DirectedGraph<G, N, E> {
    for e in src.out.iter() {
        if e.target == dst { return true }
    }
    return false
}

pub func G.bfs(self, start: N) -> [N] where DirectedGraph<G, N, E> {
    // TODO(hashable): visited-tracking uses linear-search `[N]` slices
    // to stay free of stdlib hashing.  Switch to Dict<N, bool> for O(1)
    // membership once `Hashable.equals` is restored on the interface.
    let mut found = false
    for n in self.nodes.iter() {
        if n == start { found = true }
    }
    if !found { panic("bfs: start node not in graph") }

    let mut order: [N] = []
    let mut visited: [N] = [start]
    let mut queue: [N] = [start]
    while !queue.is_empty() {
        let n = queue[0]
        queue = queue[1:]
        order = append(order, n)
        for e in n.out.iter() {
            let m = e.target
            if !visited.contains(m) {
                visited = append(visited, m)
                queue = append(queue, m)
            }
        }
    }
    return order
}

pub func G.total_weight(self) -> W where WeightedDirectedGraph<G, N, E, W> {
    let mut sum: W = W.zero()
    for n in self.nodes.iter() {
        for e in n.out.iter() {
            sum = sum.add(e.weight)
        }
    }
    return sum
}

// =========================================================================
// PART 4.  FPGA antifuse router  (neutral labels — bridging impls)
// =========================================================================

class Net {
    name: string
}

class Route {
    layer: i32
}

class Via {
    fuse_id: i32
    weight: f32 = 1.0          // f32 — exercises Numeric over a narrower type
}

relation DoublyLinked Net:routes  owns [Route:net]
relation DoublyLinked Route:a     refs [Via:a]
relation DoublyLinked Route:b     refs [Via:b]

// The user's relation labels (a/b) don't match the directional names
// the algorithm expects (out/in/source/target).  Bridge them in a
// grouped impl block on the constraint alias.  Each entry is a label-
// rename alias from algorithm-side label to user-side label.  The
// `Net:nodes → Net:routes` binding for ManyToOne<G:nodes, N:graph>
// also needs bridging because the relation label is `:routes`, not
// `:nodes`.

impl DirectedGraph<Net, Route, Via> {
    ManyToOne<Net:nodes, Route:graph> = ManyToOne<Net:routes, Route:net>
    ManyToOne<Route:out, Via:source>  = ManyToOne<Route:a, Via:a>
    ManyToOne<Route:in,  Via:target>  = ManyToOne<Route:b, Via:b>
}

// EdgeWeight<Via, f32> auto-derives via the `weight: f32` field.
// EdgeWeight<Via, f64> requires an explicit impl if any algorithm
// instantiates at f64 — auto-widen is NOT in the design.  (Users who
// genuinely want f64 either change the field type or write an explicit
// impl that does `self.weight as f64`.)

// Use sites:

pub func count_fuses(net: Net) -> i32 {
    return net.count_edges()
}

pub func sum_weights_f32(net: Net) -> f32 {
    return net.total_weight()          // W inferred = f32 from EdgeWeight<Via, f32>
}

// Router-specific bidirectional walker — uses the raw relations
// directly via labels-as-scopes.  Not a graph algorithm; just user
// code over the same data.
pub func adjacent_vias(r: Route) -> [Via] {
    let mut result: [Via] = []
    for v in r.a.iter() { result = append(result, v) }
    for v in r.b.iter() { result = append(result, v) }
    return result
}

// =========================================================================
// PART 5.  Follower network  (semantic labels match — zero impl blocks)
// =========================================================================

class Network {
    name: string
}

class Person {
    handle: string
}

class Follow {
    since: i64
    weight: f64 = 1.0
}

relation DoublyLinked Network:nodes owns [Person:graph]
relation DoublyLinked Person:out    refs [Follow:source]
relation DoublyLinked Person:in     refs [Follow:target]

// All constraint members auto-derive:
//   ManyToOne<Network:nodes, Person:graph>  — direct (labels match)
//   ManyToOne<Person:out, Follow:source>    — direct
//   ManyToOne<Person:in, Follow:target>     — direct
//   EdgeWeight<Follow, f64>                 — direct (field auto-getter)
//
// No impl block needed for Network/Person/Follow to satisfy
// WeightedDirectedGraph<Network, Person, Follow, f64>.

pub func count_follows(net: Network) -> i32 {
    return net.count_edges()
}

pub func total_engagement(net: Network) -> f64 {
    return net.total_weight()
}

pub func mutual_followers(a: Person, b: Person) -> [Person] {
    let a_preds = a.predecessors()
    let b_preds = b.predecessors()
    let mut result: [Person] = []
    for p in a_preds {
        if b_preds.contains(p) {
            result = append(result, p)
        }
    }
    return result
}

// =========================================================================
// Summary
// =========================================================================
//
//                ┌────────────────────────┬──────────────────────────┐
//                │ FPGA router            │ Follower network         │
//   ─────────────┼────────────────────────┼──────────────────────────┤
//   Labels       │ neutral (routes/a/b)   │ semantic (nodes/out/in)  │
//   Weight type  │ f32                    │ f64                      │
//   Impl blocks  │ 1 (grouped, 3 aliases) │ 0 (all auto-derived)     │
//                └────────────────────────┴──────────────────────────┘
//
// What this final design demonstrates:
//
//   - **ManyToOne is the single unified capability** for relation-shaped
//     storage.  No fine-grained OutEdges/InEdges/EdgeEndpoints zoo.
//   - **Constraint aliases** bundle the capabilities an algorithm
//     category needs (DirectedGraph, WeightedDirectedGraph) without
//     introducing interface-composing-interface.
//   - **Algorithms are free functions** with where-clauses, callable
//     as methods via UFCS.
//   - **Labels in where-clauses** (`ManyToOne<N:out, E:source>`) name
//     which relation a constraint binds to.
//   - **Grouped constraint-alias impl** (`impl DirectedGraph<...>`)
//     organizes related bridging impls together; desugars to separate
//     top-level impls per capability.
//   - **Aggressive auto-derive** does the work whenever labels and
//     types align; explicit impls only when the user has a genuine
//     disambiguation choice (which physical relation is the "out"
//     direction).
//   - **No `_method_aliases` global, no per-relation rename map, no
//     textual-prefix dispatch.**  The Forge bug class is eliminated
//     by construction.
