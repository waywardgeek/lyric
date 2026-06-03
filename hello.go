package main

type Point struct {
	X float64
	Y float64
}

func Factorial(n int32) int32 {
	if n <= int32(1) {
		return int32(1)
	}
	return n * Factorial(n - int32(1))
}

func Abs(p Point) float64 {
	return p.X * p.X + p.Y * p.Y
}

func main() {
	_ = Factorial(int32(5))
	p := Point{X: 3.0, Y: 4.0}
	_ = Abs(p)
	var i int32 = int32(0)
	for i < int32(10) {
		i = i + int32(1)
	}
}
