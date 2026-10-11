// Package cmd wires the command tree to process arguments, signals and exit
// codes. os.Exit is called only in main.
package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/peterbourgon/ff/v3/ffcli"

	"github.com/atakankarsli/sure/internal/cli/shared"
	"github.com/atakankarsli/sure/internal/report"
)

// Run executes one invocation and returns its exit code.
func Run(args []string, deps shared.Deps) int {
	if len(args) == 1 && (args[0] == "--version" || args[0] == "-version") {
		fmt.Fprintln(deps.Stdout, deps.Version)
		return report.ExitClear
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := RootCommand(deps)
	// Help goes to stdout when asked for; flag errors and their usage go to
	// stderr so stdout never mixes text into a JSON document.
	out := deps.Stderr
	if wantsHelp(args) {
		out = deps.Stdout
	}
	setOutput(root, out)

	if err := root.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return report.ExitClear
		}
		fmt.Fprintln(deps.Stderr, "Error:", err)
		return report.ExitUsage
	}
	err := root.Run(ctx)
	var code shared.ExitCode
	switch {
	case err == nil:
		return report.ExitClear
	case errors.As(err, &code):
		return int(code)
	case errors.Is(err, flag.ErrHelp):
		return report.ExitUsage
	default:
		fmt.Fprintln(deps.Stderr, "Error:", err)
		return report.ExitUsage
	}
}

func setOutput(c *ffcli.Command, w io.Writer) {
	if c.FlagSet != nil {
		c.FlagSet.SetOutput(w)
	}
	for _, s := range c.Subcommands {
		setOutput(s, w)
	}
}

func wantsHelp(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		switch strings.TrimLeft(a, "-") {
		case "h", "help":
			if strings.HasPrefix(a, "-") {
				return true
			}
		}
	}
	return false
}
