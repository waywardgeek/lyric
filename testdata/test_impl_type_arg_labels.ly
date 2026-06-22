// Acceptance test for labels-on-impl-type-vars (redesign §3.8).
// Verifies both forms produce equivalent label-prefixed members:
//   (1) relation-synthesized impl with per-type-var labels (today's
//       canonical surface — relation desugars internally);
//   (2) user-authored `impl X<A:label, B:label2> { }` syntax
//       parses and merges with a matching relation declaration.
lyric ImplTypeArgLabels {
    class Team { name: string }
    class Player { name: string }

    // Form (1) only — relation synthesizes the labeled impl internally.
    relation ArrayList Team:roster owns [Player:team]

    func test_relation_synthesized_labels() {
        let t = Team { name: "A" }
        let p = Player { name: "P1" }
        t.roster_append(p)
        assert_eq(len(t.roster_children), 1, "label-prefixed parent field via relation")
        assert_eq(p.team_parent!.name, "A", "label-prefixed child back-ptr via relation")
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
        g.rounds_append(r)
        assert_eq(len(g.rounds_children), 1, "user-labeled + relation injects fields")
        assert_eq(r.game_parent!.name, "G", "user-labeled child back-ptr resolves")
    }
}
