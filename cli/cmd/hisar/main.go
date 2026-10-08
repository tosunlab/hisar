package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "usage: hisar <command>")
	os.Exit(2)
}
