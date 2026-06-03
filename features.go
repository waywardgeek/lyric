package main

type Vec2 struct {
	X float64
	Y float64
}

func Widen(x int32) int64 {
	var y int64 = int64(100)
	return int64(x) + y
}

func SumUntil(limit int32) int32 {
	var sum int32 = int32(0)
	var i int32 = int32(0)
	for true {
		if i >= limit {
			break
		}
		if i == int32(3) {
			i = i + int32(1)
			continue
		}
		sum = sum + i
		i = i + int32(1)
	}
	return sum
}

func Classify(x int32) int32 {
	switch x {
	case 0:
		return int32(0)
	case 1:
		return int32(1)
	default:
		return int32(2)
	}
	return int32(2)
}

func Dot(a Vec2, b Vec2) float64 {
	return a.X * b.X + a.Y * b.Y
}

func SumList(xs []int32) int32 {
	var total int32 = int32(0)
	for _, x := range xs {
		total = total + x
	}
	return total
}

func main() {
	_ = Widen(int32(42))
	_ = SumUntil(int32(10))
	_ = Classify(int32(5))
	a := Vec2{X: 1.0, Y: 2.0}
	b := Vec2{X: 3.0, Y: 4.0}
	_ = Dot(a, b)
	nums := []int32{int32(1), int32(2), int32(3)}
	_ = SumList(nums)
	scores := map[string]int32{"alice": int32(100), "bob": int32(95)}
	_ = scores
}
