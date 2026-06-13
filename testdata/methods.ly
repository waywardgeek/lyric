// methods.ly — tests built-in methods on string, list, and map types
lyric methods_test {

  func main() {
    // --- String methods ---
    let s = "Hello, World!"

    // Length
    println(f"len: {s.len()}")

    // Case conversion
    println(s.to_upper())
    println(s.to_lower())

    // Search
    println(f"contains 'World': {s.contains("World")}")
    println(f"has_prefix 'Hello': {s.has_prefix("Hello")}")
    println(f"has_suffix '!': {s.has_suffix("!")}")
    println(f"index_of 'World': {s.index_of("World")}")

    // Transform
    let csv = "a,b,c,d"
    let parts = csv.split(",")
    println(f"split: {parts.len()} parts")
    println(f"join: {parts.join(" | ")}")

    let padded = "  hello  "
    println(f"trim: '{padded.trim()}'")

    let replaced = s.replace("World", "Lyric")
    println(f"replace: {replaced}")

    let stars = "*".repeat(5)
    println(f"repeat: {stars}")

    // --- List methods ---
    let mut items: [i32] = []
    items.push(10)
    items.push(20)
    items.push(30)
    println(f"list len: {items.len()}")
    println(f"contains 20: {items.contains(20)}")
    println(f"last: {items.pop()}")
    println(f"after pop: {items.len()}")

    // --- Dict methods ---
    let mut scores = Dict<Sym, i32>()
    scores.set(`alice`, 95)
    scores.set(`bob`, 87)
    scores.set(`carol`, 92)
    println(f"has alice: {scores.has(`alice`)}")

    let keys = scores.keys()
    println(f"keys count: {keys.len()}")

    let removed = scores.remove(`bob`)
    println(f"removed bob: {removed}")
    println(f"has bob after remove: {scores.has(`bob`)}")
  }
}
