package cmd

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/peterbourgon/ff/v3/ffcli"

	"github.com/atakankarsli/sure/internal/cli/check"
	"github.com/atakankarsli/sure/internal/cli/registry"
	"github.com/atakankarsli/sure/internal/cli/shared"
)

// RootCommand builds the command tree. Bare `sure` runs a check; flag state
// lives in this call, not in package variables.
func RootCommand(deps shared.Deps) *ffcli.Command {
	fs := flag.NewFlagSet("sure", flag.ContinueOnError)
	version := fs.Bool("version", false, "Print the version")
	flags := check.Bind(fs)
	run := check.Exec(deps, fs, flags)
	root := &ffcli.Command{
		Name:        "sure",
		ShortUsage:  "sure [contract...] [flags]",
		ShortHelp:   "How sure are we that the code still does what the contracts say?",
		LongHelp:    check.LongHelp,
		FlagSet:     fs,
		UsageFunc:   RootUsageFunc,
		Subcommands: registry.Subcommands(deps),
	}
	root.Exec = func(ctx context.Context, args []string) error {
		if *version {
			_, err := fmt.Fprintln(deps.Stdout, deps.Version)
			return err
		}
		return run(ctx, args)
	}
	return root
}

func RootUsageFunc(c *ffcli.Command) string {
	var b strings.Builder
	b.WriteString(c.ShortHelp + "\n\n")
	b.WriteString("USAGE\n")
	tw := tabwriter.NewWriter(&b, 0, 2, 2, ' ', 0)
	fmt.Fprintf(tw, "  sure [contract...]\tCheck the contracts your changes touch (or the ones named)\n")
	subs := map[string]*ffcli.Command{}
	for _, s := range c.Subcommands {
		subs[s.Name] = s
	}
	for _, name := range registry.Visible {
		if s, ok := subs[name]; ok {
			fmt.Fprintf(tw, "  sure %s\t%s\n", s.Name, s.ShortHelp)
		}
	}
	tw.Flush()
	for _, g := range []struct {
		title string
		names []string
	}{
		{"FLAGS", []string{"expect", "checks", "scope", "json", "dry-run", "all", "base", "provider"}},
		{"MORE", []string{"record", "replay", "no-cache", "draft", "contracts-ref", "assess-base", "repo", "timeout", "verbose", "version"}},
	} {
		b.WriteString("\n" + g.title + "\n")
		tw = tabwriter.NewWriter(&b, 0, 2, 2, ' ', 0)
		for _, name := range g.names {
			f := c.FlagSet.Lookup(name)
			if f == nil {
				continue
			}
			def := ""
			if f.DefValue != "" && f.DefValue != "false" {
				def = " (default " + f.DefValue + ")"
			}
			fmt.Fprintf(tw, "  --%s\t%s%s\n", f.Name, f.Usage, def)
		}
		tw.Flush()
	}
	b.WriteString("\n" + c.LongHelp + "\n")
	return b.String()
}
