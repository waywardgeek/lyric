package main

type Shape interface {
	isShape()
}

type ShapeCircle struct {
	Radius float64
}

func (ShapeCircle) isShape() {}

type ShapeRectangle struct {
	Width float64
	Height float64
}

func (ShapeRectangle) isShape() {}

type ShapeEmpty struct{}

func (ShapeEmpty) isShape() {}

type Counter struct {
	Count int32
}

func NewCounter(count int32) *Counter {
	return &Counter{
		Count: count,
	}
}

func (r *Counter) Increment() {
	r.Count = r.Count + 1
}

func (r *Counter) Get() int32 {
	return r.Count
}

func Describe(x int32) int32 {
	result := func() int32 {
		switch _m := x; _m {
		case 0:
			return int32(10)
		case 1:
			return int32(20)
		default:
			return int32(30)
		}
	}()
	return result
}

func Abs(x int32) int32 {
	if x < int32(0) {
		return int32(0) - x
	} else {
		return x
	}
}

func SumList(xs []int32) int32 {
	var total int32 = int32(0)
	for _, x := range xs {
		total = total + x
	}
	return total
}

func Apply(f func(int32) int32, x int32) int32 {
	return f(x)
}

func ApplyBinOp(f func(int32, int32) int32, a int32, b int32) int32 {
	return f(a, b)
}

func main() {
	ctr := &Counter{Count: 0}
	ctr.Increment()
	_ = ctr.Get()
	_ = Describe(int32(1))
	_ = Abs(-int32(5))
	nums := []int32{int32(1), int32(2), int32(3)}
	_ = SumList(nums)
	scores := map[string]int32{"alice": int32(100), "bob": int32(95)}
	_ = scores
	_ = Apply(Abs, int32(42))
	double := func(x int32) int32 {
		return x * 2
	}
	_ = Apply(double, int32(5))
	add := func(a int32, b int32) int32 {
		return a + b
	}
	_ = ApplyBinOp(add, int32(3), int32(4))
}
