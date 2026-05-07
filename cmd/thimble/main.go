// Command thimble is the operator's entry point. It defers all flag
// parsing, argument validation, and side effects to internal/cli; this
// file exists only to fail-fast on the error returned from cli.Run.
package main

import (
	"fmt"
	"os"

	"github.com/cartine/thimble/internal/cli"
)

func main() {
	if err := cli.Run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "thimble:", err)
		os.Exit(1)
	}
}
