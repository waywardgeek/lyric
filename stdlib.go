package main

import (
	"fmt"
	"strings"
	"strconv"
)

func main() {
	greeting := strings.ToUpper("hello, grok!")
	fmt.Println(greeting)
	num := int32(42)
	s := strconv.Itoa(int(num))
	fmt.Println("The answer is " + s)
	var x int32 = int32(100)
	var y int64 = int64(x)
	var z int32 = int32(y)
	fmt.Println(fmt.Sprintf("x=%v, y=%v, z=%v", x, y, z))
}
