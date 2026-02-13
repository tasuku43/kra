//go:build experimental

package main

import (
	"os"

	"github.com/tasuku43/kra/internal/cli"
)

var version = "dev"

func main() {
	c := cli.New(os.Stdout, os.Stderr)
	c.Version = version
	os.Exit(c.Run(os.Args[1:]))
}
