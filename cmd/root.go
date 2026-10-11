package cmd

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/peterbourgon/ff/v3/ffcli"

	"github.com/atakankarsli/sure/internal/cli/registry"
	"github.com/atakankarsli/sure/internal/cli/shared"
)

type commandGroup struct {
	Title    string
	Commands []string
}

var rootGroups = []commandGroup{
	{Title: "CONTRACTS", Commands: []string{"init", "contracts"}},
	{Title: "CHECKING", Commands: []string{"plan", "check"}},
	{Title: "UTILITY", Commands: []string{"doctor", "schema", "version"}},
}

// RootCommand builds the command tree. Flag state lives in this call, not in
// package variables, so every invocation starts clean.
func RootCommand(deps shared.Deps) *ffcli.Command {
	fs := flag.NewFlagSet("sure", flag.ContinueOnError)
	version := fs.Bool("version", false, "Print version and exit")
	root := &ffcli.Command{
		Name:        "sure",
		ShortUsage:  "sure <subcommand> [flags]",
		ShortHelp:   "Check source code against versioned product contracts written in English.",
		FlagSet:     fs,
		UsageFunc:   RootUsageFunc,
		Subcommands: registry.Subcommands(deps),
	}
	root.Exec = func(_ context.Context, args []string) error {
		if *version {
			_, err := fmt.Fprintln(deps.Stdout, deps.Version)
			return err
		}
		if len(args) > 0 {
			return fmt.Errorf("unknown command %q", args[0])
		}
		return flag.ErrHelp
	}
	return root
}

func RootUsageFunc(c *ffcli.Command) string {
	var b strings.Builder
	b.WriteString("DESCRIPTION\n  " + c.ShortHelp + "\n\n")
	b.WriteString("USAGE\n  " + c.ShortUsage + "\n\n")
	subs := map[string]*ffcli.Command{}
	for _, s := range c.Subcommands {
		subs[s.Name] = s
	}
	for _, g := range rootGroups {
		b.WriteString(g.Title + "\n")
		tw := tabwriter.NewWriter(&b, 0, 2, 2, ' ', 0)
		for _, name := range g.Commands {
			if s, ok := subs[name]; ok {
				fmt.Fprintf(tw, "  %s\t%s\n", s.Name, s.ShortHelp)
			}
		}
		tw.Flush()
		b.WriteString("\n")
	}
	b.WriteString("FLAGS\n  --version  Print version and exit\n\n")
	b.WriteString("Use \"sure <command> --help\" for more information about a command.")
	return b.String()
}
