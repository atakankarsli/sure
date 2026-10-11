// Package check implements `sure check`.
package check

import (
	"context"
	"errors"
	"flag"
	"time"

	"github.com/peterbourgon/ff/v3/ffcli"

	"github.com/atakankarsli/sure/internal/cli/shared"
	"github.com/atakankarsli/sure/internal/contracts"
	"github.com/atakankarsli/sure/internal/engine"
	"github.com/atakankarsli/sure/internal/provider"
	"github.com/atakankarsli/sure/internal/report"
)

func Command(deps shared.Deps) *ffcli.Command {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	sel := shared.BindSelection(fs)
	noCache := fs.Bool("no-cache", false, "Do not read or write cached decisions")
	record := fs.String("record", "", "Write every decision, with its full request, to this local directory")
	timeout := fs.Duration("timeout", 10*time.Minute, "Deadline for the whole run")
	assessBase := fs.Bool("assess-base", false, "With --changed: re-check violated checks against the base to label introduced regressions")
	output := shared.BindOutput(fs, deps.Getenv)

	return &ffcli.Command{
		Name:       "check",
		ShortUsage: "sure check --repo <dir> (--all | --changed [--base <ref>] | --contract <id>) [flags]",
		ShortHelp:  "Check source against trusted contracts and report diagnostics.",
		LongHelp: `Collects the source each selected contract names, asks the decision provider
one fixed question per check, and reports supported, violated, unknown, error
or not_checked for every check. Never builds or runs the target application.

Exit codes: 0 clear, 1 violation, 2 invalid invocation or config, 3 incomplete,
10 provider failure, 11 rate limit, 12 deadline, 13 malformed decision, 130 canceled.`,
		FlagSet:   fs,
		UsageFunc: shared.DefaultUsageFunc,
		Exec: func(ctx context.Context, args []string) error {
			format, err := shared.NormalizeFormat(*output)
			if err != nil {
				return err
			}
			var run report.Run
			opts, optErr := sel.Options()
			if optErr == nil {
				optErr = shared.RejectArgs(args)
			}
			switch {
			case optErr != nil:
				run = invalid(deps, optErr)
			case *record != "" && opts.Provider == provider.NameReplay:
				run = invalid(deps, errors.New("--record cannot be combined with --provider replay"))
			case *assessBase && opts.Mode != engine.ModeChanged:
				run = invalid(deps, errors.New("--assess-base only applies with --changed"))
			default:
				opts.NoCache, opts.RecordDir, opts.AssessBase = *noCache, *record, *assessBase
				runCtx, cancel := context.WithTimeout(ctx, *timeout)
				defer cancel()
				run = engine.Check(runCtx, opts, deps.Engine())
			}
			if err := shared.Print(deps.Stdout, format, run, render(format, run)); err != nil {
				return err
			}
			return shared.Exit(run.ExitCode)
		},
	}
}

func render(format string, run report.Run) func() string {
	switch format {
	case shared.FormatMarkdown:
		return func() string { return report.RunMarkdown(run) }
	case shared.FormatTable:
		return func() string { return report.RunTable(run) }
	}
	return nil
}

func invalid(deps shared.Deps, err error) report.Run {
	return report.Run{
		SchemaVersion:   report.RunSchema,
		RunID:           deps.NewRunID(),
		Status:          report.StatusFailed,
		Gate:            report.GateError,
		ExitCode:        report.ExitUsage,
		Contracts:       []report.ContractEvidence{},
		Checks:          []report.Check{},
		Errors:          []report.Error{{Code: "invalid_flags", Message: err.Error()}},
		UnmappedChanges: []string{},
		ContractChanges: []contracts.Change{},
		NextActions:     []report.Action{},
	}
}
