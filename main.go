package main

import (
	"os"

	"github.com/atakankarsli/sure/cmd"
	"github.com/atakankarsli/sure/internal/cli/shared"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	info := version
	if commit != "none" {
		info += " (" + commit + ", " + date + ")"
	}
	os.Exit(cmd.Run(os.Args[1:], shared.DefaultDeps(info)))
}
