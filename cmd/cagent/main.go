package main

import (
	"fmt"
	"os"

	"github.com/davidroman0O/cagent/internal/cli"
)

var version = "dev"

func main() {
	if err := cli.NewRoot(version).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
