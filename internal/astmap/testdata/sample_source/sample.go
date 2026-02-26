package sample

import "fmt"

func Add(a, b int) int {
	return a + b
}

func Subtract(a, b int) int {
	return a - b
}

func Greet(name string) string {
	if name == "" {
		return "Hello, World!"
	}
	return fmt.Sprintf("Hello, %s!", name)
}

type Calculator struct {
	Result int
}

func (c *Calculator) Add(n int) {
	c.Result += n
}

func (c *Calculator) Multiply(n int) {
	c.Result *= n
}

func neverCalled() {
	fmt.Println("this function is never called")
}
