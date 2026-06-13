// spawn.ly — spawn statement (goroutine launching)

lyric spawn_demo {
  func main() {
    let done = make_channel<bool>(1)

    // Basic spawn with captured variable
    spawn {
      let x = 42
      println(f"hello from goroutine: {x}")
      done.send(true)
    }
    done.receive()

    // Spawn with loop
    spawn {
      for i in [1, 2, 3] {
        println(f"item: {i}")
      }
      done.send(true)
    }
    done.receive()

    // Another spawn
    spawn {
      println("third goroutine")
      done.send(true)
    }
    done.receive()

    println("all done")
  }
}
