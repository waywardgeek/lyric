// Test if let and let..else syntax

enum Shape {
    Circle(radius: f64)
    Rect(w: f64, h: f64)
}

func get_radius(s: Shape) -> f64 {
    if let Circle(r) = s {
        return r
    }
    return 0.0
}

func get_radius_or_neg(s: Shape) -> f64 {
    if let Circle(r) = s {
        return r
    } else {
        return -1.0
    }
}

func get_radius_let_else(s: Shape) -> f64 {
    let Circle(r) = s else {
        return -1.0
    }
    return r
}

func get_area_rect(s: Shape) -> f64 {
    let Rect(w, h) = s else {
        return -1.0
    }
    return w * h
}

func test_if_let_match() {
    let c = Circle(3.14)
    let r = get_radius(c)
    assert(r == 3.14, "if let should extract Circle radius")
}

func test_if_let_no_match() {
    let rect = Rect(2.0, 3.0)
    let r = get_radius(rect)
    assert(r == 0.0, "if let should fall through for non-Circle")
}

func test_if_let_else_match() {
    let c = Circle(5.0)
    let r = get_radius_or_neg(c)
    assert(r == 5.0, "if let else should extract Circle radius")
}

func test_if_let_else_no_match() {
    let rect = Rect(2.0, 3.0)
    let r = get_radius_or_neg(rect)
    assert(r == -1.0, "if let else should return -1 for non-Circle")
}

func test_let_else_match() {
    let c = Circle(7.0)
    let r = get_radius_let_else(c)
    assert(r == 7.0, "let else should extract Circle radius")
}

func test_let_else_no_match() {
    let rect = Rect(2.0, 3.0)
    let r = get_radius_let_else(rect)
    assert(r == -1.0, "let else should return -1 for non-Circle")
}

func test_let_else_rect() {
    let rect = Rect(4.0, 5.0)
    let area = get_area_rect(rect)
    assert(area == 20.0, "let else rect should compute area")
}
