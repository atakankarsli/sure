// Package registry lists every Sure subcommand.
package registry

import (
	"context"
	"flag"
	"fmt"

	"github.com/peterbourgon/ff/v3/ffcli"

	"github.com/atakankarsli/sure/internal/cli/check"
	"github.com/atakankarsli/sure/internal/cli/doctor"
	"github.com/atakankarsli/sure/internal/cli/initcmd"
	"github.com/atakankarsli/sure/internal/cli/schemacmd"
	"github.com/atakankarsli/sure/internal/cli/shared"
	"github.com/atakankarsli/sure/internal/cli/validate"
)

// Visible is the command list shown in help, in order.
var Visible = []string{"init", "validate", "doctor"}

func versionCommand(deps shared.Deps) *ffcli.Command {
	return &ffcli.Command{
		Name:       "version",
		ShortUsage: "sure version",
		ShortHelp:  "Print the version.",
		FlagSet:    flag.NewFlagSet("version", flag.ContinueOnError),
		UsageFunc:  shared.DefaultUsageFunc,
		Exec: func(context.Context, []string) error {
			_, err := fmt.Fprintln(deps.Stdout, deps.Version)
			return err
		},
	}
}

// Subcommands builds fresh command state for one invocation. `check`,
// `schema` and `version` work but are not listed in help.
func Subcommands(deps shared.Deps) []*ffcli.Command {
	return []*ffcli.Command{
		initcmd.Command(deps),
		validate.Command(deps),
		doctor.Command(deps),
		check.Command(deps),
		schemacmd.Command(deps),
		versionCommand(deps),
	}
}
