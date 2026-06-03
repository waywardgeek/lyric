package main

type Point struct {
	X float64
	Y float64
}

func Factorial(n int32) int32 {
	if n <= 1 {
		return 1
	}
	return n * Factorial(n - 1)
}

func Abs(p Point) float64 {
	return p.X * p.X + p.Y * p.Y
}

func Main() {
	_ = Factorial(5)
	p := Point{X: 3.0, Y: 4.0}
	_ = Abs(p)
	var i int32 = 0
	for i < 10 {
		i = i + 1
	}
}
