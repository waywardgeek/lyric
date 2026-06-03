package main

import (
	"fmt"
)

func main() {
	name := "Grok"
	version := int32(1)
	fmt.Println(fmt.Sprintf("Hello, %v!", name))
	fmt.Println(fmt.Sprintf("%v v%v", name, version + int32(1)))
	a := int32(3)
	b := int32(4)
	fmt.Println(fmt.Sprintf("%v + %v = %v", a, b, a + b))
	fmt.Println("plain string")
}
