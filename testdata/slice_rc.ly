// Test: slices of class handles and structs-with-class-handles need element RC

lyric test_slice_rc {

class Widget {
    value: i32
}

struct Pair {
    a: Widget
    b: Widget
}

// Test 1: slice of class handles
func test_slice_of_classes() {
    let w1 = Widget { value: 1 }
    let w2 = Widget { value: 2 }
    let ws: [Widget] = [w1, w2]
    println(f"{ws[0].value} {ws[1].value}")
}

// Test 2: slice of structs containing class handles
func test_slice_of_structs() {
    let w1 = Widget { value: 10 }
    let w2 = Widget { value: 20 }
    let w3 = Widget { value: 30 }
    let w4 = Widget { value: 40 }
    let pairs: [Pair] = [Pair { a: w1, b: w2 }, Pair { a: w3, b: w4 }]
    println(f"{pairs[0].a.value} {pairs[1].b.value}")
}

// Test 3: plain slice — no RC needed
func test_plain_slice() {
    let nums: [i32] = [1, 2, 3]
    println(f"{nums[0]} {nums[2]}")
}

func main() {
    test_slice_of_classes()
    test_slice_of_structs()
    test_plain_slice()
}

}
