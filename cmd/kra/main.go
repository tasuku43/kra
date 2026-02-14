package main

import (
	"os"

	"github.com/tasuku43/kra/internal/cli"
)

var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	c := cli.New(os.Stdout, os.Stderr)
	c.Version = version
	c.Commit = commit
	c.Date = date
	os.Exit(c.Run(os.Args[1:]))
}
