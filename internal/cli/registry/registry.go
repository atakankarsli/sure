// Package registry lists every Sure subcommand.
package registry

import (
	"context"
	"flag"
	"fmt"

	"github.com/peterbourgon/ff/v3/ffcli"

	"github.com/atakankarsli/sure/internal/cli/check"
	"github.com/atakankarsli/sure/internal/cli/contractscmd"
	"github.com/atakankarsli/sure/internal/cli/doctor"
	"github.com/atakankarsli/sure/internal/cli/initcmd"
	"github.com/atakankarsli/sure/internal/cli/plan"
	"github.com/atakankarsli/sure/internal/cli/schemacmd"
	"github.com/atakankarsli/sure/internal/cli/shared"
)

func versionCommand(deps shared.Deps) *ffcli.Command {
	return &ffcli.Command{
		Name:       "version",
		ShortUsage: "sure version",
		ShortHelp:  "Print version information and exit.",
		FlagSet:    flag.NewFlagSet("version", flag.ContinueOnError),
		UsageFunc:  shared.DefaultUsageFunc,
		Exec: func(context.Context, []string) error {
			_, err := fmt.Fprintln(deps.Stdout, deps.Version)
			return err
		},
	}
}

// Subcommands builds fresh command state for one invocation.
func Subcommands(deps shared.Deps) []*ffcli.Command {
	return []*ffcli.Command{
		initcmd.Command(deps),
		contractscmd.Command(deps),
		plan.Command(deps),
		check.Command(deps),
		doctor.Command(deps),
		schemacmd.Command(deps),
		versionCommand(deps),
	}
}
