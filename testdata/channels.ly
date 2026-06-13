// channels.ly — channel operations

lyric channels_demo {
  func producer(ch: channel<i32>, count: i32) {
    let mut i: i32 = 0
    while i < count {
      ch.send(i)
      i = i + 1
    }
    ch.close()
  }

  func main() {
    // Buffered channel
    let ch = make_channel<i32>(10)

    // Send and receive
    ch.send(42)
    let val = ch.receive()
    println(f"received: {val}")

    // Unbuffered channel
    let done = make_channel<bool>()

    spawn {
      let ch2 = make_channel<string>(1)
      ch2.send("hello")
      let msg = ch2.receive()
      println(msg)
      done.send(true)
    }

    done.receive()
    println("all done")
  }
}
