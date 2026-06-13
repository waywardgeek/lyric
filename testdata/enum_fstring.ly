// Test enum to_string in f-strings

lyric enum_fstring {

enum Color { Red Green Blue }

func main() {
  let c = Green
  println(f"color is {c}")
  let b = Blue
  println(f"also {b}")
}

}
