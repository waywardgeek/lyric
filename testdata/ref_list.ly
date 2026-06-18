// ref_list.ly — tests RefList relation from stdlib (embeds DoublyLinked)
// Unlike OwningList, parent destruction unlinks children but does NOT destroy them.

lyric ref_list {

  class Room { name: string }
  class Guest { name: string }

  relation RefList Room:room refs [Guest:guest]

  func main() {
    let r = Room { name: "Lobby" }
    let g1 = Guest { name: "Alice" }
    let g2 = Guest { name: "Bob" }
    let g3 = Guest { name: "Carol" }

    r.append(g1)
    r.append(g2)
    r.append(g3)

    // Walk the list
    let mut cur = r.room_first
    while !isnull(cur) {
      println(cur!.name)
      cur = cur!.guest_next
    }

    // Remove middle element
    r.remove(g2)
    println("after remove:")
    cur = r.room_first
    while !isnull(cur) {
      println(cur!.name)
      cur = cur!.guest_next
    }

    // Destroy parent — children should be unlinked but still alive
    r.destroy()
    println(f"g1 parent null: {isnull(g1.guest_parent)}")
    println(f"g3 parent null: {isnull(g3.guest_parent)}")
    // Children still accessible
    println(f"g1 name: {g1.name}")
    println(f"g2 name: {g2.name}")
    println(f"g3 name: {g3.name}")
  }
}
