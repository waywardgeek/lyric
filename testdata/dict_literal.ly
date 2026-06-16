lyric dict_literal {
  func main() {
    // Basic dict literal with string keys (auto-interned as Sym)
    let d = {"name": "Alice", "city": "Denver"}
    let name_entry = d.get(`name`)
    if !isnull(name_entry) {
      print(name_entry!.value + "\n")
    }
    let city_entry = d.get(`city`)
    if !isnull(city_entry) {
      print(city_entry!.value + "\n")
    }

    // Dict literal with integer values
    let scores = {"alice": 95, "bob": 87}
    let alice_entry = scores.get(`alice`)
    if !isnull(alice_entry) {
      print(itoa(alice_entry!.value as i64) + "\n")
    }

    // Empty dict literal
    let empty: Dict<Sym, string> = {}
    print("ok\n")
  }
}
