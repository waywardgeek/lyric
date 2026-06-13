lyric is_operator {
    enum Shape {
        Circle(radius: f64)
        Rectangle(width: f64, height: f64)
        Point
    }

    func describe(s: Shape) -> string {
        if s is Circle {
            return "circle"
        }
        if s is Rectangle {
            return "rectangle"
        }
        return "point"
    }

    func test_is_basic() {
        let c = Circle(3.14)
        assert(c is Circle, "should be Circle")
        assert(!(c is Rectangle), "should not be Rectangle")
        assert(!(c is Point), "should not be Point")

        let r = Rectangle(2.0, 3.0)
        assert(r is Rectangle, "should be Rectangle")
        assert(!(r is Circle), "should not be Circle")

        let p = Point
        assert(p is Point, "should be Point")
        assert(!(p is Circle), "should not be Circle")
    }

    func test_is_in_conditions() {
        let s = Circle(1.0)
        let result = describe(s)
        assert_eq(result, "circle")

        let r = Rectangle(1.0, 2.0)
        let result2 = describe(r)
        assert_eq(result2, "rectangle")

        let p = Point
        let result3 = describe(p)
        assert_eq(result3, "point")
    }
}
