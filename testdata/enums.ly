// enums.ly — end-to-end test for enum types

lyric enums {

enum Color { Red Green Blue }

enum Shape {
    Circle(radius: f64)
    Rectangle(width: f64, height: f64)
    Point
}

func color_name(c: Color) -> string {
    return match c {
        Red => { "red" }
        Green => { "green" }
        Blue => { "blue" }
    }
}

func area(s: Shape) -> f64 {
    return match s {
        Circle(r) => { 3.14159 * r * r }
        Rectangle(w, h) => { w * h }
        Point => { 0.0 }
    }
}

func main() {
    let c = Red
    println(color_name(c))
    
    let s1 = Circle(5.0)
    let s2 = Rectangle(3.0, 4.0)
    let s3 = Point
    println(area(s1))
    println(area(s2))
    println(area(s3))
}

}
