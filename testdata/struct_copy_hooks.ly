// struct_copy_hooks.ly — test RC for class handles embedded in structs/tuples

lyric struct_copy_hooks {

class Widget {
    value: i32
}

// Test 1: Struct with class field — copy must retain, scope exit releases
struct Pair {
    a: Widget
    b: Widget
}

func test_struct_copy() {
    let w1 = Widget { value: 1 }
    let w2 = Widget { value: 2 }
    let p = Pair { a: w1, b: w2 }
    let q = p  // must retain p.a and p.b
    println(f"{q.a.value} {q.b.value}")
    // scope exit: release p.a, p.b, q.a, q.b, w1, w2
}

// Test 2: Nested struct containing class handle
struct Inner {
    w: Widget
}

struct Outer {
    inner: Inner
    x: i32
}

func test_nested_struct() {
    let w = Widget { value: 42 }
    let o = Outer { inner: Inner { w: w }, x: 10 }
    let o2 = o  // must retain o.inner.w
    println(f"{o2.inner.w.value} {o2.x}")
}

// Test 3: Struct with no class fields — should NOT get hooks
struct Point {
    x: i32
    y: i32
}

func test_plain_struct() {
    let p = Point { x: 1, y: 2 }
    let q = p  // no RC needed
    println(f"{q.x} {q.y}")
}

// Test 4: Function returning struct with class field
func make_pair(v1: i32, v2: i32) -> Pair {
    return Pair { a: Widget { value: v1 }, b: Widget { value: v2 } }
}

func test_return_struct() {
    let p = make_pair(10, 20)
    println(f"{p.a.value} {p.b.value}")
}

// Test 5: Struct copy where source is dead (move — no retain needed)
func test_struct_move() {
    let w = Widget { value: 77 }
    let p = Pair { a: w, b: w }
    let q = p  // p is dead → move (no retain needed for embedded refs)
    println(f"{q.a.value} {q.b.value}")
}

func main() {
    test_struct_copy()
    test_nested_struct()
    test_plain_struct()
    test_return_struct()
    test_struct_move()
}

}
