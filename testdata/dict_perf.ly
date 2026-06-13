// Test Dict<Sym, i32> performance with many entries

func main() {
  let d = Dict<Sym, i32>()
  let mut i = 0
  // Insert 10000 entries
  while i < 10000 {
    let key = sym(f"key_{i}")
    d.set(key, i)
    i = i + 1
  }
  // Look up all entries
  let mut found = 0
  i = 0
  while i < 10000 {
    let key = sym(f"key_{i}")
    let entry = d.get(key)
    if !isnull(entry) {
      found = found + 1
    }
    i = i + 1
  }
  println(found)
}
