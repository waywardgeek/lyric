package main

import (
	"fmt"
	"strings"
)

type Displayable interface {
	Display() string
}

type Prioritizable interface {
	Priority() int32
}

type Task struct {
	Title string
	Desc string
	Prio int32
	Done bool
}

func NewTask(title string, desc string, prio int32, done bool) *Task {
	return &Task{
		Title: title,
		Desc: desc,
		Prio: prio,
		Done: done,
	}
}

func (r *Task) Display() string {
	return fmt.Sprintf("[%v] %v: %v", r.Prio, r.Title, r.Desc)
}

func (r *Task) Priority() int32 {
	return r.Prio
}

func (r *Task) Is_done() bool {
	return r.Done
}

type TaskList struct {
	Name string
	Tasks []Task
}

func NewTaskList(name string) *TaskList {
	return &TaskList{
		Name: name,
	}
}

func (r *TaskList) Add(task Task) {
}

func (r *TaskList) Summary() string {
	return fmt.Sprintf("Task list '%v'", r.Name)
}

func Show(item Displayable) string {
	return item.Display()
}

func Highest_priority(a int32, b int32) int32 {
	if a > b {
		return a
	}
	return b
}

func Repeat_string(s string, n int32) string {
	result := ""
	var i int32 = int32(0)
	for i < n {
		result = result + s
		i = i + int32(1)
	}
	return result
}

func Fizzbuzz(n int32) string {
	if n % int32(15) == int32(0) {
		return "FizzBuzz"
	} else if n % int32(3) == int32(0) {
		return "Fizz"
	} else if n % int32(5) == int32(0) {
		return "Buzz"
	}
	return fmt.Sprintf("%v", n)
}

func main() {
	t1 := &Task{Title: "Design Grok", Desc: "Language spec", Prio: 3, Done: false}
	t2 := &Task{Title: "Build parser", Desc: "Recursive descent", Prio: 5, Done: false}
	t3 := &Task{Title: "Write tests", Desc: "Full coverage", Prio: 4, Done: true}
	fmt.Println(Show(t1))
	fmt.Println(Show(t2))
	fmt.Println(Show(t3))
	max_p := Highest_priority(t1.Priority(), t2.Priority())
	fmt.Println(fmt.Sprintf("Highest priority: %v", max_p))
	separator := Repeat_string("-", int32(30))
	fmt.Println(separator)
	var i int32 = int32(1)
	results := ""
	for i <= int32(15) {
		if i > int32(1) {
			results = results + ", "
		}
		results = results + Fizzbuzz(i)
		i = i + int32(1)
	}
	fmt.Println(fmt.Sprintf("FizzBuzz: %v", results))
	if t3.Is_done() {
		fmt.Println("Tests are done!")
	}
	fmt.Println(strings.ToUpper("grok is working!"))
}
