package shared

import (
	"flag"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/peterbourgon/ff/v3/ffcli"
)

// DefaultUsageFunc renders --help for every subcommand in one layout.
func DefaultUsageFunc(c *ffcli.Command) string {
	var b strings.Builder
	short := strings.TrimSpace(c.ShortHelp)
	long := strings.TrimSpace(c.LongHelp)
	if short != "" {
		b.WriteString("DESCRIPTION\n  " + short + "\n\n")
	}
	usage := strings.TrimSpace(c.ShortUsage)
	if usage == "" {
		usage = c.Name
	}
	b.WriteString("USAGE\n  " + usage + "\n\n")
	if long != "" {
		b.WriteString(long + "\n\n")
	}
	if len(c.Subcommands) > 0 {
		b.WriteString("SUBCOMMANDS\n")
		tw := tabwriter.NewWriter(&b, 0, 2, 2, ' ', 0)
		for _, sub := range c.Subcommands {
			fmt.Fprintf(tw, "  %s\t%s\n", sub.Name, sub.ShortHelp)
		}
		tw.Flush()
		b.WriteString("\n")
	}
	if c.FlagSet != nil {
		var flags []*flag.Flag
		c.FlagSet.VisitAll(func(f *flag.Flag) { flags = append(flags, f) })
		if len(flags) > 0 {
			b.WriteString("FLAGS\n")
			tw := tabwriter.NewWriter(&b, 0, 2, 2, ' ', 0)
			for _, f := range flags {
				if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "[]" {
					fmt.Fprintf(tw, "  --%s\t%s (default: %s)\n", f.Name, f.Usage, f.DefValue)
				} else {
					fmt.Fprintf(tw, "  --%s\t%s\n", f.Name, f.Usage)
				}
			}
			tw.Flush()
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
