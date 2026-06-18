// owning_list.ly — tests OwningList relation from stdlib (embeds DoublyLinked)

lyric owning_list {

  class Team { name: string }
  class Player { name: string }

  relation OwningList Team:team owns [Player:player]

  func main() {
    let t = Team { name: "Wolves" }
    let p1 = Player { name: "Alice" }
    let p2 = Player { name: "Bob" }
    let p3 = Player { name: "Carol" }

    t.team_append(p1)
    t.team_append(p2)
    t.team_append(p3)

    // Walk the list
    let mut cur = t.team_first
    while !isnull(cur) {
      println(cur!.name)
      cur = cur!.player_next
    }

    // Remove middle element
    t.team_remove(p2)
    println("after remove:")
    cur = t.team_first
    while !isnull(cur) {
      println(cur!.name)
      cur = cur!.player_next
    }

    // Cascade destroy — should destroy all remaining children
    t.destroy()
    println(f"p1 parent null: {isnull(p1.player_parent)}")
    println(f"p3 parent null: {isnull(p3.player_parent)}")
  }
}
