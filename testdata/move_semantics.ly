// move_semantics.ly — test that ownership transfer skips retain/release

lyric move_semantics {

class Widget {
    value: i32
}

// Test 1: Basic move — x is dead after assignment to y
func test_basic_move() {
    let x = Widget { value: 42 }
    let y = x  // x is never used again → should be a move (no retain)
    println(f"{y.value}")
}

// Test 2: Copy — x is still used after assignment to y
func test_copy_when_live() {
    let x = Widget { value: 10 }
    let y = x  // x IS used again → must retain
    println(f"{y.value}")
    println(f"{x.value}")
}

// Test 3: Move through a chain
func test_chain_move() {
    let a = Widget { value: 99 }
    let b = a  // a dead → move
    let c = b  // b dead → move
    println(f"{c.value}")
}

// Test 4: Function returning a class — caller owns result
func make_widget(v: i32) -> Widget {
    return Widget { value: v }
}

func test_return_ownership() {
    let w = make_widget(7)
    let w2 = w  // w dead → move
    println(f"{w2.value}")
}

// Test 5: Parameter is borrowed — cannot move from it
func take_widget(w: Widget) -> Widget {
    let local = w  // w is a param (borrowed) → must retain, NOT move
    return local
}

func test_param_not_moved() {
    let w = Widget { value: 55 }
    let w2 = take_widget(w)
    println(f"{w2.value}")
}

func main() {
    test_basic_move()
    test_copy_when_live()
    test_chain_move()
    test_return_ownership()
    test_param_not_moved()
}

}
