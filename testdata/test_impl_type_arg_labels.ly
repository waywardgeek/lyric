// Acceptance test for labels-on-impl-type-vars (redesign §3.8) AND
// the Phase 3c-capability dotted-scope call form (redesign §3.1).
// Both ride on the same per-type-var label foundation.
lyric ImplTypeArgLabels {
    class Team { name: string }
    class Player { name: string }

    // Form (1) only — relation synthesizes the labeled impl internally.
    relation ArrayList Team:roster owns [Player:team]

    func test_relation_synthesized_labels_flat() {
        let t = Team { name: "A" }
        let p = Player { name: "P1" }
        t.roster_append(p)
        assert_eq(len(t.roster_children), 1, "label-prefixed parent field via relation")
        assert_eq(p.team_parent!.name, "A", "label-prefixed child back-ptr via relation")
    }

    func test_relation_synthesized_labels_dotted() {
        let t = Team { name: "B" }
        let p1 = Player { name: "P1" }
        let p2 = Player { name: "P2" }

        // Phase 3c-capability dotted-scope form: `t.roster.append(p)`
        // is sugar for the flat `t.roster_append(p)`. Both forms must
        // compile and produce identical runtime behavior.
        t.roster.append(p1)
        t.roster.append(p2)
        assert_eq(len(t.roster.children), 2, "dotted parent field len")
        assert_eq(t.roster.children[0].name, "P1", "dotted parent field index")
        assert_eq(p1.team.parent!.name, "B", "dotted child back-ptr")
        assert_eq(p2.team.index, 1, "dotted child index field")

        // Flat form still works on the same data.
        assert_eq(len(t.roster_children), 2, "flat len after dotted appends")
        assert_eq(t.roster_children[0].name, "P1", "flat parity")
    }

    // Form (2) — user-authored labeled impl. The existing-impl-merge
    // path in desugar_relations sees the labels are already set on
    // the impl-type-arg slots and leaves them alone; the relation
    // adds field-bind mappings into the same impl block.
    class Game { name: string }
    class Round { name: string }
    relation ArrayList Game:rounds owns [Round:game]
    impl ArrayList<Game:rounds, Round:game> { }

    func test_user_authored_labels_merge() {
        let g = Game { name: "G" }
        let r = Round { name: "R1" }

        // Dotted form on user-authored labeled impl.
        g.rounds.append(r)
        assert_eq(len(g.rounds.children), 1, "user-labeled dotted parent field")
        assert_eq(r.game.parent!.name, "G", "user-labeled dotted child back-ptr")

        // Flat form parity.
        assert_eq(len(g.rounds_children), 1, "user-labeled flat parent field")
        assert_eq(r.game_parent!.name, "G", "user-labeled flat child back-ptr")
    }
}
