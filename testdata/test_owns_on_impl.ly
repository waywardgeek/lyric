// Acceptance test for ownership annotation on impl blocks
// (redesign §3.9). Three exercise points:
//
// (1) User-defined hint interface + user-authored `owns` impl,
//     NO matching relation. Verifies that field injection and
//     destructor synthesis fire purely from the impl annotation —
//     the capability that was relation-only before §3.9.
//
// (2) `refs` variant on a separate hint. The refs-kind destructor
//     unlinks the back-pointer rather than cascading; verifying
//     the back-pointer is null after parent.destroy() proves the
//     refs-kind destructor (not the owns-kind) was synthesized.
//
// (3) Regression: existing `relation` surface still works
//     end-to-end (relation → desugar pass 3A synthesizes an
//     ownership impl → pass 3B materializes fields/mappings →
//     pass 4 synthesizes destructors).
lyric OwnsOnImpl {

  // ===== (1) user-defined hint interface, owns =====
  interface MyOwn<P, C> {
    field P.kids: [C]
    field C.parent: P?
    field C.idx: i32

    destructor owns P {
      for k in self.kids {
        k.destroy()
      }
    }
    destructor owns C { }
    destructor refs P { }
    destructor refs C { }
  }

  class Owner { name: string }
  class Owned { name: string }

  // User-authored ownership impl, NO matching relation.
  impl MyOwn<Owner, Owned> owns { }

  func test_owns_on_impl_user_defined_hint_owns() {
    let o = Owner { name: "o1" }
    let c1 = Owned { name: "c1" }
    let c2 = Owned { name: "c2" }
    // Field `kids` was injected on Owner via §3.9 pass 3B.
    o.kids = append(o.kids, c1)
    o.kids = append(o.kids, c2)
    c1.parent = o
    c1.idx = 0
    c2.parent = o
    c2.idx = 1
    assert_eq(len(o.kids), 2, "field injected on Owner from user-authored owns impl")
    assert_eq(c1.parent!.name, "o1", "back-pointer injected on Owned")
    assert_eq(c2.idx, 1, "third injected field")
    // The owns destructor synthesized onto Owner walks self.kids and
    // calls .destroy() on each child. Reaching this point without
    // panic is the test — for an unsupported pairing the synthesized
    // destructor wouldn't exist and `o.destroy()` would just be the
    // empty default.
    o.destroy()
  }

  // ===== (2) refs variant on a separate user-defined hint =====
  // The refs-kind destructor unlinks the back-pointer instead of
  // cascading. After parent.destroy(), child.owner must be null.
  interface MyRef<P, C> {
    field P.items: [C]
    field C.owner: P?

    destructor owns P {
      for it in self.items {
        it.destroy()
      }
    }
    destructor refs P {
      for it in self.items {
        it.owner = null
      }
    }
    destructor owns C { }
    destructor refs C { }
  }

  class RefOwner { name: string }
  class RefItem { name: string }

  impl MyRef<RefOwner, RefItem> refs { }

  func test_refs_on_impl_user_defined_hint_refs() {
    let o = RefOwner { name: "ro" }
    let it = RefItem { name: "it" }
    o.items = append(o.items, it)
    it.owner = o
    assert_eq(it.owner!.name, "ro", "refs back-pointer set")
    o.destroy()
    // If the owns destructor had fired by mistake, `it` would have
    // been destroyed and `it.owner` access would crash. The refs
    // destructor instead nulls the back-pointer.
    assert_eq(isnull(it.owner), true, "refs destructor unlinks back-pointer")
  }

  // ===== (3) regression: relation surface still works =====
  // The relation desugars via pass 3A into a labeled+annotated impl,
  // which pass 3B then materializes the same way as the user-
  // authored impls above. Coexistence is the whole point of §3.9.
  class Team { name: string }
  class Player { name: string }
  relation ArrayList Team:roster owns [Player:team]

  func test_relation_surface_regression() {
    let t = Team { name: "T" }
    let p = Player { name: "P" }
    t.roster_append(p)
    assert_eq(len(t.roster_children), 1, "relation-surface field still injected")
    assert_eq(p.team_parent!.name, "T", "relation-surface back-pointer still injected")
    t.destroy()
    assert_eq(isnull(p.team_parent), true, "relation-surface destructor still fires")
  }
}
