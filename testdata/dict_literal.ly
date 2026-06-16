lyric dict_literal {
  func main() {
    // Dict literal with sym keys
    let d = {`name`: "Alice", `city`: "Denver"}
    let name_entry = d.get(`name`)
    if !isnull(name_entry) {
      print(name_entry!.value + "\n")
    }
    let city_entry = d.get(`city`)
    if !isnull(city_entry) {
      print(city_entry!.value + "\n")
    }

    // Dict literal with integer keys
    let scores = {1: "alice", 2: "bob"}
    let alice_entry = scores.get(1)
    if !isnull(alice_entry) {
      print(alice_entry!.value + "\n")
    }

    // Empty dict literal
    let empty: Dict<Sym, string> = {}
    print("ok\n")
  }
}
