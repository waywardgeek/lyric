package main

type Shape interface {
	Area() float64
	Name() string
}

type Printable interface {
	To_string() string
}

type PrintableShape interface {
	Shape
	Printable
}

type Circle struct {
	Radius float64
}

func NewCircle(radius float64) *Circle {
	return &Circle{
		Radius: radius,
	}
}

func (r *Circle) Area() float64 {
	return 3.14159 * r.Radius * r.Radius
}

func (r *Circle) Name() string {
	return "circle"
}

type Rectangle struct {
	Width float64
	Height float64
}

func NewRectangle(width float64, height float64) *Rectangle {
	return &Rectangle{
		Width: width,
		Height: height,
	}
}

func (r *Rectangle) Area() float64 {
	return r.Width * r.Height
}

func (r *Rectangle) Name() string {
	return "rectangle"
}

type Square struct {
	Side float64
}

func NewSquare(side float64) *Square {
	return &Square{
		Side: side,
	}
}

func (r *Square) Area() float64 {
	return r.Side * r.Side
}

func (r *Square) Name() string {
	return "square"
}

func (r *Square) To_string() string {
	return "a square"
}

func Describe(s Shape) string {
	return s.Name()
}

func main() {
	c := &Circle{Radius: 5.0}
	r := &Rectangle{Width: 3.0, Height: 4.0}
	sq := &Square{Side: 7.0}
	_ = c.Area()
	_ = r.Area()
	_ = sq.Area()
	_ = Describe(c)
	_ = Describe(r)
}
