// lock.ly — lock statement (mutex scoping)

lyric lock_demo {
  func main() {
    let mut mu: Lock
    let mut count: i32 = 0

    lock(mu) {
      count = count + 1
    }
    lock(mu) {
      count = count + 10
    }

    println(f"final count: {count}")
  }
}

// expected output:
// final count: 11
