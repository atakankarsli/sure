// Package check implements the default command: `sure [contract...]`.
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

const LongHelp = `Checks your code against the contracts in .sure/contracts and prints how sure
the decision model is that they hold. With no arguments it checks the contracts
touched by changes against the default branch (origin/HEAD, main or master),
or every contract when nothing changed. Name contracts, or id prefixes, to
check only those. Never builds or runs your application.

Exit codes: 0 pass, 1 violated, 2 usage or config error, 3 not sure
(unknown check, uncovered change, contract edits), 10-13 provider failure,
130 canceled.`

// Flags are the check flags; the root command and the `check` alias share
// them.
type Flags struct {
	repo, base, contractsRef, provider, record, replay *string
	all, dryRun, asJSON, draft, noCache, assessBase    *bool
	verbose                                            *bool
	timeout                                            *time.Duration
}

func Bind(fs *flag.FlagSet) *Flags {
	return &Flags{
		asJSON:       fs.Bool("json", false, "Print one JSON document (for agents and CI)"),
		dryRun:       fs.Bool("dry-run", false, "Show what would be checked and sent; no model call"),
		all:          fs.Bool("all", false, "Check every contract"),
		base:         fs.String("base", "", "Compare against this ref (default: origin/HEAD, main or master)"),
		repo:         fs.String("repo", ".", "Repository to check"),
		provider:     fs.String("provider", "", "jev or anthropic (default: from .sure/config.yaml)"),
		record:       fs.String("record", "", "Save decisions with their full requests to this local directory"),
		replay:       fs.String("replay", "", "Answer from recorded decisions in this directory; never calls a model"),
		noCache:      fs.Bool("no-cache", false, "Ignore and do not write cached decisions"),
		draft:        fs.Bool("draft", false, "Use uncommitted contracts (not a protected check)"),
		contractsRef: fs.String("contracts-ref", "", "Read trusted contracts from this ref"),
		assessBase:   fs.Bool("assess-base", false, "Also ask about the base to label violations this change introduced"),
		timeout:      fs.Duration("timeout", 10*time.Minute, "Deadline for the whole run"),
		verbose:      fs.Bool("verbose", false, "Print one progress line per contract to stderr"),
	}
}

func (f *Flags) options(args []string) (engine.Options, error) {
	o := engine.Options{
		Repo:         *f.repo,
		Mode:         engine.ModeAuto,
		Contracts:    args,
		BaseRef:      *f.base,
		ContractsRef: *f.contractsRef,
		Draft:        *f.draft,
		Provider:     *f.provider,
		NoCache:      *f.noCache,
		RecordDir:    *f.record,
		AssessBase:   *f.assessBase,
	}
	switch {
	case len(args) > 0 && *f.all:
		return o, errors.New("name contracts or pass --all, not both")
	case len(args) > 0:
		o.Mode = engine.ModeContract
	case *f.all:
		o.Mode = engine.ModeAll
	}
	switch o.Provider {
	case "", provider.NameJev, provider.NameAnthropic:
	default:
		return o, errors.New("--provider must be jev or anthropic")
	}
	if *f.replay != "" {
		if o.Provider != "" || o.RecordDir != "" {
			return o, errors.New("--replay cannot be combined with --provider or --record")
		}
		o.Provider, o.ReplayDir = provider.NameReplay, *f.replay
	}
	if o.Draft && o.ContractsRef != "" {
		return o, errors.New("--draft and --contracts-ref cannot be combined")
	}
	if o.AssessBase && o.Mode != engine.ModeAuto {
		return o, errors.New("--assess-base applies to change checks only")
	}
	return o, nil
}

// Exec runs a check, or a dry run, and prints the result.
func Exec(deps shared.Deps, fs *flag.FlagSet, f *Flags) func(context.Context, []string) error {
	return func(ctx context.Context, args []string) error {
		args, err := shared.ParseInterspersed(fs, args)
		if err != nil {
			return err
		}
		opts, err := f.options(args)
		if *f.dryRun {
			var p report.Plan
			if err != nil {
				p = report.Plan{SchemaVersion: report.PlanSchema, Status: "invalid", ExitCode: report.ExitUsage,
					Contracts: []report.ContractEvidence{}, Outcomes: []report.PlannedCheck{},
					Errors:          []report.Error{{Code: "invalid_flags", Message: err.Error()}},
					UnmappedChanges: []string{}, ContractChanges: []contracts.Change{}, NextActions: []report.Action{}}
			} else {
				p = engine.Plan(ctx, opts, deps.Engine(*f.verbose))
			}
			if err := shared.Print(deps.Stdout, *f.asJSON, p, func() string { return report.PlanHuman(p, deps.Style()) }); err != nil {
				return err
			}
			return shared.Exit(p.ExitCode)
		}
		var run report.Run
		if err != nil {
			run = invalid(deps, err)
		} else {
			runCtx, cancel := context.WithTimeout(ctx, *f.timeout)
			defer cancel()
			run = engine.Check(runCtx, opts, deps.Engine(*f.verbose))
		}
		if err := shared.Print(deps.Stdout, *f.asJSON, run, func() string { return report.Human(run, deps.Style()) }); err != nil {
			return err
		}
		return shared.Exit(run.ExitCode)
	}
}

// Command is the `check` alias of the root command.
func Command(deps shared.Deps) *ffcli.Command {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	f := Bind(fs)
	return &ffcli.Command{
		Name:       "check",
		ShortUsage: "sure check [contract...] [flags]",
		ShortHelp:  "Same as `sure`.",
		LongHelp:   LongHelp,
		FlagSet:    fs,
		UsageFunc:  shared.DefaultUsageFunc,
		Exec:       Exec(deps, fs, f),
	}
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
