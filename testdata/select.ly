// select.ly — select statement (channel multiplexing)

lyric select_demo {
  func main() {
    let ch1 = make_channel<string>(1)
    let ch2 = make_channel<i32>(1)
    let done = make_channel<bool>(1)

    ch1.send("hello")

    // Select with receive binding
    select {
      case msg = ch1.receive() => {
        println(f"got message: {msg}")
      }
      case num = ch2.receive() => {
        println(f"got number: {num}")
      }
    }

    // Select with send and default
    ch2.send(42)
    select {
      case val = ch2.receive() => {
        println(f"received: {val}")
      }
      default => {
        println("no data ready")
      }
    }

    // Select with send case
    spawn {
      let x = ch1.receive()
      println(f"spawned got: {x}")
      done.send(true)
    }

    select {
      case ch1.send("world") => {
        println("sent to ch1")
      }
    }

    done.receive()
    println("select done")
  }
}
