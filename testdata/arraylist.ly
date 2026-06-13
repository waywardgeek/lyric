// arraylist.ly — tests ArrayList relation from stdlib
lyric arraylist {

  class Team { name: string }
  class Player { name: string }

  relation ArrayList Team:roster owns [Player:team]

  impl ArrayList<Team, Player> {
    P.children <-> Team.roster_children
  }

  func main() {
    let t = Team { name: "Wolves" }
    let p1 = Player { name: "Alice" }
    let p2 = Player { name: "Bob" }
    let p3 = Player { name: "Carol" }

    array_append<Team, Player>(t, p1)
    array_append<Team, Player>(t, p2)
    array_append<Team, Player>(t, p3)

    println(len(t.roster_children))
    println(p1.team_index)
    println(p2.team_index)
    println(p3.team_index)

    // Remove middle element (Bob) — Carol should swap into Bob's slot
    array_remove<Team, Player>(p2)
    println(len(t.roster_children))
    println(p3.team_index)

    // Parent destroy — cascade
    let t2 = Team { name: "Bears" }
    let p4 = Player { name: "Dan" }
    array_append<Team, Player>(t2, p4)
    t2.destroy()
    println(isnull(p4.team_parent))
  }
}
