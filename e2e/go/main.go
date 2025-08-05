package main

import (
	"fmt"
	"runtime"
)

func main() {
	fmt.Printf("Hello, world from %s %s!\n", runtime.GOOS, runtime.GOARCH)
}
