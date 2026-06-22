// Acceptance test for labels-on-impl-type-vars (redesign §3.8) AND
// the dotted-scope call form (redesign §3.1).
// Phase 3e: bare-flat textual-prefix access (e.g. `t.roster_children`)
// is now rejected — the dotted-scope form is the only user-visible path.
// Both forms previously existed; the flat-form assertions in this file
// were deleted by the Phase 3e commit.
lyric ImplTypeArgLabels {
    class Team { name: string }
    class Player { name: string }

    relation ArrayList Team:roster owns [Player:team]

    func test_relation_synthesized_labels_dotted() {
        let t = Team { name: "B" }
        let p1 = Player { name: "P1" }
        let p2 = Player { name: "P2" }

        // Dotted-scope form (redesign §3.1): `t.roster.append(p)`
        // resolves via the checker rewrite to the mangled storage name
        // `t.__roster_append(p)` registered on Team.
        t.roster.append(p1)
        t.roster.append(p2)
        assert_eq(len(t.roster.children), 2, "dotted parent field len")
        assert_eq(t.roster.children[0].name, "P1", "dotted parent field index")
        assert_eq(p1.team.parent!.name, "B", "dotted child back-ptr")
        assert_eq(p2.team.index, 1, "dotted child index field")
    }

    // User-authored labeled impl. The existing-impl-merge path in
    // desugar_relations sees the labels are already set on the impl-
    // type-arg slots and leaves them alone; the relation adds field-
    // bind mappings into the same impl block.
    class Game { name: string }
    class Round { name: string }
    relation ArrayList Game:rounds owns [Round:game]
    impl ArrayList<Game:rounds, Round:game> { }

    func test_user_authored_labels_merge() {
        let g = Game { name: "G" }
        let r = Round { name: "R1" }

        g.rounds.append(r)
        assert_eq(len(g.rounds.children), 1, "user-labeled dotted parent field")
        assert_eq(r.game.parent!.name, "G", "user-labeled dotted child back-ptr")
    }
}
