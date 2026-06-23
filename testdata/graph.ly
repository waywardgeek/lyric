// graph.ly — user-authored multi-class generic interfaces (v7)
//
// Companion to tree.ly.  Demonstrates the Wave-1 design (fields and
// methods only, no relation equivalence).  This is the design that
// the spec's §Interfaces and Multi-Class Contracts documents as the
// canonical multi-class interface example.
//
// Design summary (Bill + Hewitt, 2026-06-23 morning review):
//
//   1. Algorithms live as default methods inside user-authored generic
//      interfaces.  The interface declares the abstract surface
//      (methods with type-var receivers like `func G.nodes(self) -> [N]`)
//      and the default algorithm bodies that use that surface.
//
//   2. Interface composition is `extends Parent<...>` — pure desugar-
//      time aggregation (copy parent methods into child).  No vtable
//      widening, no runtime IS-A.  Child may override parent methods.
//
//   3. Impl blocks bind concrete classes to interface type vars via
//      alias mappings:
//          T.member = ConcreteType.accessor
//      where T is the interface's type-var name (G/N/E/W in graph
//      interfaces) and ConcreteType.accessor is the concrete class's
//      method or field-derived getter.
//
//   4. Empty impl body triggers auto-derive on exact-name match.  For
//      each unsatisfied abstract method, the compiler looks up the
//      concrete class bound to its type-var and checks for a method
//      with the same name and compatible signature.  Missing matches
//      surface as ONE diagnostic listing every unsatisfied member.
//
//   5. Partial impls (leaving a type var open) make the impl itself
//      generic.  Monomorphized per use site.
//
//   6. Relation accessors (post-Phase-3e dotted scope: `n.outgoing.iter()`)
//      are valid right-hand-side targets in alias mappings.  A binding
//      `N.outgoing_edges = Route.a` synthesizes
//      `pub func Route.outgoing_edges(self) -> [Via] { return self.a.iter() }`.
//
// 🚧 STATUS (post-Wave-1 ship 2026-06-23):
//   • Wave 1 lexer/parser pieces (extends, partial impls, empty-body
//     impl + auto-derive diagnostic) ARE shipped. This file parses
//     fully.
//   • Default-method emission for multi-class interfaces (4w1-a) is
//     NOT shipped — the receiver-only type inference yields G but
//     can't recover N and E for Graph<G,N,E>.count_edges. Until
//     4w1-a lands, the default-method bodies in DirectedGraph and
//     WeightedDirectedGraph below are aspirational. Calls to them
//     from main() fail at the checker with "unknown method: iter"
//     / "field not found" because the synthesized method bodies
//     can't resolve N's surface.
//   • Relation equivalence (Wave 2) is the other pending piece —
//     `G:nodes/N:graph = Net:routes/Route:net` syntax NOT in lexer.
//
// 🚧 NOT IN THIS FILE — deferred to Wave 2 (relation equivalence):
//      relation Collection G:nodes [N:graph]    (abstract relations)
//      G:nodes/N:graph = Net:routes/Route:net   (relation equivalence)
//   Both are pure ergonomic sugar over Wave 1; the same user code
//   compiles with field/method bindings only (this file).  See
//   cr/docs/multi-class-interface-redesign.md §9 Phase 4 Wave 2.

// =========================================================================
// PART 1.  Graph interfaces  (would live in `graph` package)
// =========================================================================

// DirectedGraph<G, N, E> declares the abstract surface every directed-
// graph algorithm needs, plus default algorithm bodies.

interface DirectedGraph<G, N, E> {
    // Abstract surface — every impl must satisfy these five.
    pub func G.nodes(self) -> [N]
    pub func N.outgoing_edges(self) -> [E]
    pub func N.incoming_edges(self) -> [E]
    pub func E.src(self) -> N
    pub func E.dst(self) -> N

    // Default algorithms — monomorphized into the impl at desugar time.

    pub func G.count_edges(self) -> i32 {
        let mut total: i32 = 0
        for n in self.nodes() {
            for _e in n.outgoing_edges() { total = total + 1 }
        }
        return total
    }

    pub func N.successors(self) -> [N] {
        let mut result: [N] = []
        for e in self.outgoing_edges() { result = append(result, e.dst()) }
        return result
    }

    pub func N.predecessors(self) -> [N] {
        let mut result: [N] = []
        for e in self.incoming_edges() { result = append(result, e.src()) }
        return result
    }

    pub func G.has_edge(self, src: N, dst: N) -> bool {
        for e in src.outgoing_edges() {
            if e.dst() == dst { return true }
        }
        return false
    }

    pub func G.bfs(self, start: N) -> [N] {
        // TODO(hashable): visited-tracking uses linear-search [N] slices
        // to stay free of stdlib hashing.  Switch to Dict<N, bool> once
        // Hashable.equals is restored on the interface.
        let mut found = false
        for n in self.nodes() {
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
            for e in n.outgoing_edges() {
                let m = e.dst()
                if !visited.contains(m) {
                    visited = append(visited, m)
                    queue = append(queue, m)
                }
            }
        }
        return order
    }
}

// WeightedDirectedGraph<G, N, E, W> adds an edge-weight surface and
// one weight-using algorithm.  `extends` is desugar-time method
// aggregation (no runtime IS-A).

interface WeightedDirectedGraph<G, N, E, W> extends DirectedGraph<G, N, E> {
    pub func E.weight(self) -> W

    pub func G.total_weight(self) -> W where Numeric<W> {
        let mut sum: W = W.zero()
        for n in self.nodes() {
            for e in n.outgoing_edges() { sum = sum.add(e.weight()) }
        }
        return sum
    }
}

// =========================================================================
// PART 2.  FPGA antifuse router  (neutral labels — explicit alias mapping)
// =========================================================================
//
// The user's relation labels (a/b) don't match the interface's named
// surface (outgoing_edges/incoming_edges).  The impl block names the
// alias mapping explicitly.  Six abstract members → six binding lines.
//
// Each binding line synthesizes a forwarding wrapper at desugar time.
// E.g. `N.outgoing_edges = Route.outgoing_a` synthesizes
//     pub func Route.outgoing_edges(self) -> [Via] { return self.outgoing_a() }

class Net {
    name: string
}

class Route {
    layer: i32
}

class Via {
    fuse_id: i32
    weight: f32 = 1.0          // exercises Numeric<W> at f32 width
}

relation DoublyLinked Net:routes  owns [Route:net]
relation DoublyLinked Route:a     refs [Via:a_src]
relation DoublyLinked Route:b     refs [Via:b_src]

// Helper methods bridge the relation accessors to interface surface.
// In Wave 2, these can be elided in favor of relation equivalence.

pub func Net.all_routes(self) -> [Route] {
    let mut result: [Route] = []
    for r in self.routes.iter() { result = append(result, r) }
    return result
}

pub func Route.outgoing_a(self) -> [Via] {
    let mut result: [Via] = []
    for v in self.a.iter() { result = append(result, v) }
    return result
}

pub func Route.incoming_b(self) -> [Via] {
    let mut result: [Via] = []
    for v in self.b.iter() { result = append(result, v) }
    return result
}

pub func Via.a_endpoint(self) -> Route { return self.a_src }
pub func Via.b_endpoint(self) -> Route { return self.b_src }

impl WeightedDirectedGraph<Net, Route, Via, f32> {
    G.nodes           = Net.all_routes
    N.outgoing_edges  = Route.outgoing_a
    N.incoming_edges  = Route.incoming_b
    E.src             = Via.a_endpoint
    E.dst             = Via.b_endpoint
    E.weight          = Via.weight       // f32 field auto-getter on E
}

// Use sites — algorithms callable as methods via UFCS.

pub func count_fuses(net: Net) -> i32 {
    return net.count_edges()
}

pub func sum_fuse_weights(net: Net) -> f32 {
    return net.total_weight()
}

// =========================================================================
// PART 3.  Follower network  (semantic labels — empty impl, auto-derive)
// =========================================================================
//
// When the user's relation back-pointer field names AND helper-method
// names match the interface's abstract surface exactly, the impl body
// can be EMPTY.  Auto-derive walks the abstract surface and matches
// against the class scope by exact name.

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

relation DoublyLinked Network:nodes         owns [Person:graph]
relation DoublyLinked Person:outgoing_edges refs [Follow:src]
relation DoublyLinked Person:incoming_edges refs [Follow:dst]

// User-defined helpers matching the interface names exactly.
pub func Network.nodes(self) -> [Person] {
    let mut result: [Person] = []
    for p in self.nodes.iter() { result = append(result, p) }
    return result
}

pub func Person.outgoing_edges(self) -> [Follow] {
    let mut result: [Follow] = []
    for e in self.outgoing_edges.iter() { result = append(result, e) }
    return result
}

pub func Person.incoming_edges(self) -> [Follow] {
    let mut result: [Follow] = []
    for e in self.incoming_edges.iter() { result = append(result, e) }
    return result
}

// (Follow.src/dst/weight are accessible directly — relation back-
// pointers and f64 field auto-getters.  All six abstract members
// have an exact-name match in the class scope.)

impl WeightedDirectedGraph<Network, Person, Follow, f64> { }   // all auto-derived

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
        if b_preds.contains(p) { result = append(result, p) }
    }
    return result
}

// =========================================================================
// PART 4.  Partial impl — generic in the weight type
// =========================================================================
//
// A weight-agnostic router data set — the impl leaves W open,
// monomorphizing per use site.  Same engine as generic-function
// specialization.

class FlexNet {
    name: string
}

class FlexRoute {
    layer: i32
}

class FlexVia<W> {           // generic in the weight type
    fuse_id: i32
    weight: W
}

relation DoublyLinked FlexNet:routes              owns [FlexRoute:net]
relation DoublyLinked FlexRoute:outgoing_edges    refs [FlexVia:src]
relation DoublyLinked FlexRoute:incoming_edges    refs [FlexVia:dst]

pub func FlexNet.nodes(self) -> [FlexRoute] {
    let mut result: [FlexRoute] = []
    for r in self.routes.iter() { result = append(result, r) }
    return result
}

pub func FlexRoute.outgoing_edges(self) -> [FlexVia<W>] {
    let mut result: [FlexVia<W>] = []
    for v in self.outgoing_edges.iter() { result = append(result, v) }
    return result
}

pub func FlexRoute.incoming_edges(self) -> [FlexVia<W>] {
    let mut result: [FlexVia<W>] = []
    for v in self.incoming_edges.iter() { result = append(result, v) }
    return result
}

// Partial impl: W remains open; monomorphized per use site.
impl<W> WeightedDirectedGraph<FlexNet, FlexRoute, FlexVia<W>, W> { }

pub func sum_flex_i32(net: FlexNet) -> i32 {
    return net.total_weight()              // W inferred = i32 via FlexVia<i32>
}

pub func sum_flex_f64(net: FlexNet) -> f64 {
    return net.total_weight()
}

// =========================================================================
// Summary
// =========================================================================
//
//                ┌─────────────────────────┬──────────────────────────┐
//                │ FPGA router (Part 2)    │ Follower network (Part 3)│
//   ─────────────┼─────────────────────────┼──────────────────────────┤
//   Labels       │ neutral (routes/a/b)    │ match interface surface  │
//   Weight type  │ f32                     │ f64                      │
//   Impl block   │ 1 (6 alias lines)       │ 1 (empty body)           │
//   Helper fns   │ 5                       │ 3                        │
//                └─────────────────────────┴──────────────────────────┘
//
// What this design (v7, Wave 1) demonstrates:
//
//   - **One user-authored interface per algorithm domain.** Algorithms
//     are default methods on the interface; the interface IS the
//     surface the algorithm quantifies over.
//
//   - **`extends` for interface composition** — desugar-time method
//     aggregation, no runtime IS-A, no vtable widening.
//
//   - **Alias-form impl as the binding primitive.** `T.member =
//     Concrete.accessor` synthesizes a forwarding wrapper at desugar.
//
//   - **Empty-body impls trigger auto-derive on exact name match.**
//     The user opts in to auto-derive by writing the impl (declaring
//     the intent).  Missing bindings surface as one diagnostic.
//
//   - **Partial impls are generic.** `impl<W> WDG<...,W>` leaves W
//     open; monomorphized per use site.
//
//   - **Algorithm bodies use the interface's named surface
//     (`n.outgoing_edges()`, `e.dst()`), not the user's labels.** The
//     impl-block alias does the translation at desugar.
//
//   - **Helper functions bridge relation accessors to interface
//     methods today** (Wave 1).  Wave 2 will absorb the helpers via
//     relation equivalence so user code drops the wrapper boilerplate.
