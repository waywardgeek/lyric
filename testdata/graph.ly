// graph.ly — user-authored multi-class generic interfaces (v6)
//
// 🚧 FUTURE SYNTAX — this file does NOT parse with the current compiler.
//    It is the canonical design sketch for Phase 4 of the multi-class
//    interface redesign.  Tracks the post-pivot design (no `constraint`
//    aliases, no `Collection` middleware as algorithm substrate).  See
//    cr/docs/multi-class-interface-redesign.md §9 Phase 4 for the
//    implementation plan that will make this file compile.
//
// Companion to tree.ly.  Demonstrates the relation-equivalence design
// (Bill + Hewitt, 2026-06-22→23 evening review):
//
//   1. Algorithms live as default methods inside user-authored generic
//      interfaces.  No `Collection` middleware in the user-facing
//      surface; no `constraint` aliases.  The interface IS the surface
//      algorithms quantify over, declared in one place.
//
//   2. Interfaces declare ABSTRACT RELATIONS using the existing Phase 3
//      relation syntax.  `Collection` reappears as the minimum-shape
//      vocabulary for relation slots (iter + count + parent-back).
//      Concrete user hints (`ArrayList`, `DoublyLinked`, `HashedList`)
//      all auto-satisfy `Collection`.
//
//   3. Abstract relations omit `owns`/`refs` when the algorithm doesn't
//      care.  Read-only graph algorithms (bfs, total_weight, etc.) are
//      compatible with either ownership kind.  Mutator interfaces
//      (`MutableDirectedGraph`) tighten the requirement to `owns` so
//      the destructor cascades correctly.
//
//   4. Impl blocks bind concrete relations to abstract relations via
//      RELATION EQUIVALENCE.  One line per relation, not per accessor:
//          G:plabel/N:clabel = ConcreteP:plabel/ConcreteC:clabel
//      Non-relation surface (fields, methods) maps via the v5 syntax:
//          E.member = ConcreteType.accessor
//      Empty impl body = auto-derive on exact name match across both
//      relation-key and member buckets.
//
//   5. Interface composition is `extends Parent<...>` — pure desugar-
//      time method-aggregation (no vtable widening).  A child interface
//      may re-declare a parent's abstract relation with a tightened
//      kind (`owns` constraint) but never with a loosened one.
//
//   6. Partial impls (leaving a type var open) make the impl itself
//      generic.  Monomorphized per use site.
//
//   7. Capability primitives (single-class interfaces like Numeric<T>)
//      live in stdlib; users compose them via where-clauses.
//
// Rules carried over unchanged from earlier rounds:
//   - Methods are class-scoped functions (UFCS at call sites).
//   - Field/method unification (field auto-getter + setter sugar).
//   - Phase 3 labels-as-scopes: `person.outgoing.iter()`, `len(person.outgoing)`.
//   - Generic type params explicit between `func` and the name:
//     `pub func<G, N, E> G.method(self) -> ...`.

// =========================================================================
// PART 1.  Graph interfaces  (would live in `graph` package)
// =========================================================================

// DirectedGraph<G, N, E> is the surface every directed-graph algorithm
// needs.  Three abstract relations + non-relation accessors + default
// algorithms.  The relations are kind-unspecified — either owns or
// refs satisfies — because the algorithms here only read.

interface DirectedGraph<G, N, E> {
    // Abstract relations — keyed by (parent:plabel, child:clabel).
    // `Collection` is the minimum-shape hint (iter + count + parent-back).
    // Concrete hints (ArrayList/DoublyLinked/HashedList) all auto-satisfy.
    // Owns/refs unspecified → either kind satisfies.
    relation Collection G:nodes    [N:graph]
    relation Collection N:outgoing [E:src]
    relation Collection N:incoming [E:dst]

    // Default algorithms — monomorphized into the impl at desugar time.
    // Bodies use the abstract relations via dotted scope; impl-time
    // substitution rewrites `self.nodes.iter()` → user's relation.

    pub func G.count_edges(self) -> i32 {
        let mut total: i32 = 0
        for n in self.nodes.iter() {
            for _e in n.outgoing.iter() { total = total + 1 }
        }
        return total
    }

    pub func N.successors(self) -> [N] {
        let mut result: [N] = []
        for e in self.outgoing.iter() { result = append(result, e.src) }
        return result
    }

    pub func N.predecessors(self) -> [N] {
        let mut result: [N] = []
        for e in self.incoming.iter() { result = append(result, e.dst) }
        return result
    }

    pub func G.has_edge(self, src: N, dst: N) -> bool {
        for e in src.outgoing.iter() {
            if e.src == dst { return true }
        }
        return false
    }

    pub func G.bfs(self, start: N) -> [N] {
        // TODO(hashable): visited-tracking uses linear-search [N] slices
        // to stay free of stdlib hashing.  Switch to Dict<N, bool> once
        // Hashable.equals is restored on the interface.
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
            for e in n.outgoing.iter() {
                let m = e.src
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
// one weight-using algorithm.  `extends` is desugar-time method-
// aggregation (no runtime IS-A).  No new abstract relations; just
// adds a `weight` field on E.

interface WeightedDirectedGraph<G, N, E, W> extends DirectedGraph<G, N, E> {
    field E.weight: W

    pub func G.total_weight(self) -> W where Numeric<W> {
        let mut sum: W = W.zero()
        for n in self.nodes.iter() {
            for e in n.outgoing.iter() { sum = sum.add(e.weight) }
        }
        return sum
    }
}

// MutableDirectedGraph<G, N, E> tightens the :nodes relation to require
// `owns` so destructive operations cascade correctly.  A user whose
// concrete relation is `refs` will be rejected at the impl block (not
// at the algorithm call site) with a clean diagnostic.
//
// NOTE: mid-iteration deletion is undefined in Lyric today (Phase B's
// slice-invalidation bug on relation children, logged in TODO).  Until
// the language specifies it, the safe pattern is two-pass: collect
// dead nodes in a slice, then delete in a second loop.

interface MutableDirectedGraph<G, N, E> extends DirectedGraph<G, N, E> {
    // Tightened: nodes must be owned so remove_node can cascade.
    // outgoing/incoming inherit DirectedGraph's unspecified kind
    // (the edge endpoints don't need to be owned for deletion to work
    // when nodes are owned and edges follow via owns Route → owns Via).
    relation Collection G:nodes owns [N:graph]

    pub func G.remove_node(self, n: N) {
        // Cascades to all edges incident on n via the owns chain.
        self.nodes.remove(n)
    }

    pub func G.prune_dangling(self) {
        // Two-pass: collect, then delete.  See NOTE above.
        let mut dead: [N] = []
        for n in self.nodes.iter() {
            if n.outgoing.is_empty() && n.incoming.is_empty() {
                dead = append(dead, n)
            }
        }
        for n in dead { self.nodes.remove(n) }
    }
}

// =========================================================================
// PART 2.  FPGA antifuse router  (neutral labels — explicit equivalences)
// =========================================================================
//
// The user's relation labels (a/b) don't match the interface's named
// surface (outgoing/incoming).  The impl block names the equivalence
// explicitly.  Three abstract relations → three equivalence lines, plus
// one field equivalence for the weight.

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
relation DoublyLinked Route:a     refs [Via:src]
relation DoublyLinked Route:b     refs [Via:dst]

impl WeightedDirectedGraph<Net, Route, Via, f32> {
    // Relation equivalences (LHS = interface relation key, RHS = concrete):
    G:nodes/N:graph    = Net:routes/Route:net
    N:outgoing/E:src   = Route:a/Via:src
    N:incoming/E:dst   = Route:b/Via:dst

    // Field equivalence (for non-relation surface):
    E.weight = Via.weight
}

// Use sites — algorithms callable as methods via UFCS.

pub func count_fuses(net: Net) -> i32 {
    return net.count_edges()           // default method from DirectedGraph
}

pub func sum_fuse_weights(net: Net) -> f32 {
    return net.total_weight()          // default method from WeightedDirectedGraph
}

// Router-specific bidirectional walker — uses raw relations directly
// via dotted scope.  Not a graph algorithm; just user code over the
// same data.
pub func adjacent_vias(r: Route) -> [Via] {
    let mut result: [Via] = []
    for v in r.a.iter() { result = append(result, v) }
    for v in r.b.iter() { result = append(result, v) }
    return result
}

// =========================================================================
// PART 3.  Follower network  (semantic labels — empty impl, auto-derive)
// =========================================================================
//
// When the user picks labels that match the interface relation keys
// exactly, the impl body can be EMPTY.  Auto-derive walks both buckets:
//
//   Relations (matched by parent-label and child-label exact match):
//     G:nodes/N:graph    ↔ Network:nodes/Person:graph        ✓
//     N:outgoing/E:src   ↔ Person:outgoing/Follow:src        ✓
//     N:incoming/E:dst   ↔ Person:incoming/Follow:dst        ✓
//
//   Fields/methods (matched by exact name in class scope):
//     E.weight           ↔ Follow.weight (auto-getter on f64 field) ✓
//
// All four bindings auto-derive.  Empty body = success.  If any
// member fails to match, the compiler emits one error listing every
// unsatisfied member (§5.9 third shape).

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

relation DoublyLinked Network:nodes    owns [Person:graph]
relation DoublyLinked Person:outgoing  refs [Follow:src]
relation DoublyLinked Person:incoming  refs [Follow:dst]

impl WeightedDirectedGraph<Network, Person, Follow, f64> { }   // all auto-derived

// Use sites — same algorithms, different concrete type.

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

// Network ALSO satisfies MutableDirectedGraph because its :nodes
// relation IS owns.  No additional impl block needed — DirectedGraph's
// auto-derived bindings carry over to MutableDirectedGraph (extends).

impl MutableDirectedGraph<Network, Person, Follow> { }   // also auto-derived

pub func clean_inactive_users(net: Network) {
    net.prune_dangling()
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

relation DoublyLinked FlexNet:routes      owns [FlexRoute:net]
relation DoublyLinked FlexRoute:outgoing  refs [FlexVia:src]
relation DoublyLinked FlexRoute:incoming  refs [FlexVia:dst]

// W remains open — every (FlexNet, FlexRoute, FlexVia<W>) instantiation
// monomorphizes through this impl.  Other bindings auto-derive.
impl<W> WeightedDirectedGraph<FlexNet, FlexRoute, FlexVia<W>, W> {
    G:nodes/N:graph = FlexNet:routes/FlexRoute:net   // labels diverge (routes vs nodes)
}

pub func sum_flex_i32(net: FlexNet) -> i32 {
    return net.total_weight()              // W inferred = i32 via FlexVia<i32>
}

pub func sum_flex_f64(net: FlexNet) -> f64 {
    return net.total_weight()
}

// =========================================================================
// Summary  (what changed from v5)
// =========================================================================
//
//                ┌─────────────────────────┬──────────────────────────┐
//                │ FPGA router (Part 2)    │ Follower network (Part 3)│
//   ─────────────┼─────────────────────────┼──────────────────────────┤
//   Labels       │ neutral (routes/a/b)    │ semantic (nodes/out/in)  │
//   Weight type  │ f32                     │ f64                      │
//   Impl block   │ 1 (3 rel + 1 field)     │ 1 (empty body)           │
//   Mutator impl │ N/A                     │ 1 (empty body)           │
//                └─────────────────────────┴──────────────────────────┘
//
// What this design (v6) demonstrates:
//
//   - **Relation equivalence as the binding primitive.** The LHS
//     `G:nodes/N:graph` is a single typed token (interface relation
//     key) checked against the interface's declared abstract relations
//     in one lookup.  No per-method resolution; iter/count/parent are
//     all subsumed by mapping the relation as a unit.
//
//   - **Omit owns/refs to mean "don't care."** Algorithm interfaces
//     declare abstract relations without an ownership kind; the user's
//     concrete relation can be owns or refs and still satisfy.  Mutator
//     interfaces tighten the requirement (`owns`) where deletion
//     semantics demand it.  No `any` keyword needed.
//
//   - **`extends` for desugar-time method aggregation.** Child
//     interfaces may tighten parent's abstract relations (add `owns`)
//     but never loosen them.  No runtime IS-A relation; no vtable
//     widening.  Bootstrap cost: ~ the old `embed` lowering, adapted
//     for multi-class.
//
//   - **Empty-body impls trigger auto-derive on EXACT match across
//     both buckets** (relation key + member name).  Single error
//     surface lists every unsatisfied member.
//
//   - **Partial impls are generic.** `impl<W> WDG<FlexNet, FlexRoute,
//     FlexVia<W>, W>` leaves W open; monomorphizes per use site.
//
//   - **Algorithm bodies use the interface's named surface
//     (`n.outgoing.iter()`, `e.src`), not the user's labels.** The
//     impl-block relation-equivalence does the translation at desugar.
//
//   - **No `_method_aliases` global, no per-relation rename map, no
//     textual-prefix dispatch, no constraint-alias resolution.** The
//     Forge bug class is eliminated by construction.
