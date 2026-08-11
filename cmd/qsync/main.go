package main

import (
	"os"

	"github.com/mamorett/PhotoLib/internal/cli"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	cli.Version = version
	code := cli.Run(os.Args[1:], os.Stdout, os.Stderr)
	os.Exit(int(code))
}
