// owning_list.ly — tests OwningList relation from stdlib (embeds DoublyLinked)

lyric owning_list {

  class Team { name: string }
  class Player { name: string }

  relation DoublyLinked Team:team owns [Player:player]

  func main() {
    let t = Team { name: "Wolves" }
    let p1 = Player { name: "Alice" }
    let p2 = Player { name: "Bob" }
    let p3 = Player { name: "Carol" }

    t.team.append(p1)
    t.team.append(p2)
    t.team.append(p3)

    // Walk the list
    let mut cur = t.team.first
    while !isnull(cur) {
      println(cur!.name)
      cur = cur!.player.next
    }

    // Remove middle element
    t.team.remove(p2)
    println("after remove:")
    cur = t.team.first
    while !isnull(cur) {
      println(cur!.name)
      cur = cur!.player.next
    }

    // Cascade destroy — should destroy all remaining children
    t.destroy()
    println(f"p1 parent null: {isnull(p1.player.parent)}")
    println(f"p3 parent null: {isnull(p3.player.parent)}")
  }
}
