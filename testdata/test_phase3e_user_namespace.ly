// Phase 3e acceptance: user namespace coexists with scope-injected
// storage. Per redesign §3.1, with the dotted-scope sugar being the
// only user-visible access path, the storage name for relation-injected
// fields is mangled (`__roster_children`) and therefore cannot collide
// with a user-declared field named `roster_children`. Both names are
// accessible side-by-side; both round-trip correctly.
lyric Phase3eUserNamespace {
  class Team {
    name: string
    // User-declared field that happens to share the textual prefix the
    // pre-Phase-3e compiler used for the relation-injected storage.
    // Today, with mangling, this field lives in its own namespace.
    roster_children: i32
  }
  class Player { name: string }

  relation ArrayList Team:roster owns [Player:team]

  func test_user_field_coexists_with_relation_scope() {
    let t = Team { name: "Wolves", roster_children: 42 }
    let p1 = Player { name: "Alice" }
    let p2 = Player { name: "Bob" }
    t.roster.append(p1)
    t.roster.append(p2)

    // The relation-injected storage is reached via the dotted-scope
    // sugar; the user-declared field is reached by its declared name.
    assert_eq(len(t.roster.children), 2, "relation storage holds 2 players")
    assert_eq(t.roster_children, 42, "user-declared field is unaffected")

    // Mutating the user field doesn't perturb the relation storage.
    t.roster_children = 99
    assert_eq(t.roster_children, 99, "user field is mutable")
    assert_eq(len(t.roster.children), 2, "relation storage unchanged by user write")

    // Cascade destroy still works.
    t.destroy()
    assert_eq(isnull(p1.team.parent), true, "cascade destroyed back-pointer")
  }
}
