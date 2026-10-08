package main

import (
	"fmt"
	"os"
)

var version = "0.0.0-dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("switchyard", version)
		return
	}
	fmt.Fprintln(os.Stderr, "usage: switchyard --version")
	os.Exit(2)
}
