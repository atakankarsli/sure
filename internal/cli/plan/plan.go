// Package plan implements `sure plan`.
package plan

import (
	"context"
	"flag"

	"github.com/peterbourgon/ff/v3/ffcli"

	"github.com/atakankarsli/sure/internal/cli/shared"
	"github.com/atakankarsli/sure/internal/contracts"
	"github.com/atakankarsli/sure/internal/engine"
	"github.com/atakankarsli/sure/internal/report"
)

func Command(deps shared.Deps) *ffcli.Command {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	sel := shared.BindSelection(fs)
	output := shared.BindOutput(fs, deps.Getenv)
	return &ffcli.Command{
		Name:       "plan",
		ShortUsage: "sure plan --repo <dir> (--all | --changed [--base <ref>] | --contract <id>) [flags]",
		ShortHelp:  "Show what a check would select and send, offline.",
		LongHelp: `Resolves trusted contracts, captures the working tree, selects contracts and
collects their evidence exactly as check does, then stops before any provider
call. Exit 0 unless the invocation or config is invalid (2).`,
		FlagSet:   fs,
		UsageFunc: shared.DefaultUsageFunc,
		Exec: func(ctx context.Context, args []string) error {
			format, err := shared.NormalizeFormat(*output)
			if err != nil {
				return err
			}
			opts, err := sel.Options()
			if err == nil {
				err = shared.RejectArgs(args)
			}
			var p report.Plan
			if err != nil {
				p = report.Plan{SchemaVersion: report.PlanSchema, Status: "invalid", ExitCode: report.ExitUsage,
					Contracts: []report.ContractEvidence{}, Outcomes: []report.PlannedCheck{},
					Errors:          []report.Error{{Code: "invalid_flags", Message: err.Error()}},
					UnmappedChanges: []string{}, ContractChanges: []contracts.Change{}, NextActions: []report.Action{}}
			} else {
				p = engine.Plan(ctx, opts, deps.Engine())
			}
			var md func() string
			if format != shared.FormatJSON {
				md = func() string { return report.PlanMarkdown(p) }
			}
			if err := shared.Print(deps.Stdout, format, p, md); err != nil {
				return err
			}
			return shared.Exit(p.ExitCode)
		},
	}
}
