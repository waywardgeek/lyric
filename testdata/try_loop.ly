// Minimal repro for self-compilation hang in parse_paren_lambda
// Key features: while loop, try operator, (T?, error) return, append

class Item {
  name: string
}

func make_item(s: string) -> (Item?, error) {
  if s == "" {
    return (null, "empty")
  }
  return (Item { name: s }, null)
}

func collect(names: [string]) -> ([Item], error) {
  let mut items: [Item] = []
  let mut i = 0
  while i < len(names) {
    let item = make_item(names[i])?
    items = append(items, item!)
    i = i + 1
  }
  return (items, null)
}

func main() {
  let names = ["a", "b", "c"]
  let result = collect(names)
  if result._1 == null {
    println(len(result._0))
  }
}
