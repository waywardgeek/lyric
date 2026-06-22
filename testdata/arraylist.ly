// arraylist.ly — tests ArrayList relation from stdlib
lyric arraylist {

  class Team { name: string }
  class Player { name: string }

  relation ArrayList Team:roster owns [Player:team]

  func main() {
    let t = Team { name: "Wolves" }
    let p1 = Player { name: "Alice" }
    let p2 = Player { name: "Bob" }
    let p3 = Player { name: "Carol" }

    array_append<Team, Player>(t, p1)
    array_append<Team, Player>(t, p2)
    array_append<Team, Player>(t, p3)

    println(len(t.roster.children))
    println(p1.team.index)
    println(p2.team.index)
    println(p3.team.index)

    // Remove middle element (Bob) — Carol should swap into Bob's slot
    array_remove<Team, Player>(p2)
    println(len(t.roster.children))
    println(p3.team.index)

    // Parent destroy — cascade
    let t2 = Team { name: "Bears" }
    let p4 = Player { name: "Dan" }
    array_append<Team, Player>(t2, p4)
    t2.destroy()
    println(isnull(p4.team.parent))
  }
}
