// Test embedded null bytes in strings

lyric null_byte {

func main() {
  let s = "hello\x00world"
  println(len(s))
}

}
