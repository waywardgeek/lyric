// destroy_shared.ly — Two parent classes both own a child via separate relations.
// Destroying one parent cascade-destroys the child, which auto-removes itself
// from the other parent. Then we allocate a new child and verify slab reuse
// (same pointer as the destroyed child).

lyric destroy_shared {

  class TeamA { name: string }
  class TeamB { name: string }
  class Player { name: string }

  relation DoublyLinked TeamA:team_a owns [Player:pa]
  relation DoublyLinked TeamB:team_b owns [Player:pb]

  func main() {
    let a = TeamA { name: "Alphas" }
    let b = TeamB { name: "Betas" }
    let p = Player { name: "Alice" }

    // Alice belongs to both teams
    dll_append<TeamA, Player>(a, p)
    dll_append<TeamB, Player>(b, p)

    println(f"a has player: {!isnull(a.team_a.first)}")
    println(f"b has player: {!isnull(b.team_b.first)}")

    // Remember Alice's pointer for slab reuse check
    // TODO: This is a use-after-free — p is dangling after a.destroy().
    // Once we have use-after-free detection, this test should be updated
    // to only compare the new allocation against a saved opaque handle.
    let old_ptr = p

    // Destroy team A — should cascade-destroy Alice,
    // which should auto-remove her from team B
    a.destroy()

    println(f"b has player after destroy: {!isnull(b.team_b.first)}")

    // Allocate a new player — slab should reuse Alice's slot
    let p2 = Player { name: "Bob" }
    println(f"slab reuse: {p2 == old_ptr}")

    // The new player is independent
    println(f"p2 name: {p2.name}")
  }
}
