package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: codex-usage-client <init|run|install-service|uninstall-service>")
		os.Exit(2)
	}
	fmt.Fprintf(os.Stderr, "command %q is not implemented yet\n", os.Args[1])
	os.Exit(2)
}
