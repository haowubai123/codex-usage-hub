package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "serve" {
		fmt.Fprintln(os.Stderr, "usage: codex-usage-server serve --config server.yaml")
		os.Exit(2)
	}
	fmt.Fprintln(os.Stderr, "serve is not implemented yet")
	os.Exit(2)
}
