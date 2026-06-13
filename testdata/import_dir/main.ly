// main.ly — imports a directory module
lyric main {
  import mylib

  func main() {
    let p = mylib.new_point(1, 2)
    let sum = mylib.add(p.x, p.y)
    print(sum)
  }
}
