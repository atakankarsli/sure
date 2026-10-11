// Package contractscmd implements `sure contracts` and its subcommands.
package contractscmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/peterbourgon/ff/v3/ffcli"

	"github.com/atakankarsli/sure/internal/cli/shared"
	"github.com/atakankarsli/sure/internal/contracts"
	"github.com/atakankarsli/sure/internal/gitview"
	"github.com/atakankarsli/sure/internal/report"
)

func Command(deps shared.Deps) *ffcli.Command {
	fs := flag.NewFlagSet("contracts", flag.ContinueOnError)
	return &ffcli.Command{
		Name:        "contracts",
		ShortUsage:  "sure contracts <subcommand> [flags]",
		ShortHelp:   "Work with feature contracts.",
		FlagSet:     fs,
		UsageFunc:   shared.DefaultUsageFunc,
		Subcommands: []*ffcli.Command{validateCommand(deps)},
		Exec: func(context.Context, []string) error {
			return flag.ErrHelp
		},
	}
}

func validateCommand(deps shared.Deps) *ffcli.Command {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	repo := fs.String("repo", ".", "Repository to read")
	ref := fs.String("ref", "", "Validate the contracts committed at this ref instead of the working tree")
	output := shared.BindOutput(fs, deps.Getenv)
	return &ffcli.Command{
		Name:       "validate",
		ShortUsage: "sure contracts validate --repo <dir> [--ref <ref>] [--output json]",
		ShortHelp:  "Strictly validate the config and every contract, offline.",
		FlagSet:    fs,
		UsageFunc:  shared.DefaultUsageFunc,
		Exec: func(ctx context.Context, args []string) error {
			format, err := shared.NormalizeFormat(*output)
			if err != nil {
				return err
			}
			v := report.Validation{SchemaVersion: report.ValidationSchema, Source: "worktree",
				Contracts: []report.ValidatedContract{}, Issues: []contracts.Issue{}, Errors: []report.Error{}}
			if err := shared.RejectArgs(args); err != nil {
				v.Errors = append(v.Errors, report.Error{Code: "invalid_flags", Message: err.Error()})
			} else if err := validate(ctx, *repo, *ref, &v); err != nil {
				v.Errors = append(v.Errors, report.Error{Code: "unreadable", Message: err.Error()})
			}
			v.Valid = len(v.Issues) == 0 && len(v.Errors) == 0
			if !v.Valid {
				v.ExitCode = report.ExitUsage
			}
			var md func() string
			if format != shared.FormatJSON {
				md = func() string { return markdown(v) }
			}
			if err := shared.Print(deps.Stdout, format, v, md); err != nil {
				return err
			}
			return shared.Exit(v.ExitCode)
		},
	}
}

func validate(ctx context.Context, dir, ref string, v *report.Validation) error {
	repo, err := gitview.Open(ctx, dir)
	if err != nil {
		return err
	}
	src := repo.Worktree()
	if ref != "" {
		commit, err := repo.ResolveCommit(ctx, ref)
		if err != nil {
			return fmt.Errorf("--ref: %w", err)
		}
		src, v.Source = repo.Tree(commit), commit
	}
	set, err := contracts.Load(ctx, src)
	if err != nil {
		if errors.Is(err, contracts.ErrNotInitialized) {
			v.Issues = append(v.Issues, contracts.Issue{File: contracts.ConfigPath, Code: "not_initialized", Message: err.Error()})
			return nil
		}
		return err
	}
	v.Issues = append(v.Issues, set.Issues...)
	for _, c := range set.Contracts {
		vc := report.ValidatedContract{ID: c.ID, Path: c.Path, Severity: c.Severity, Evidence: c.Evidence, Checks: []string{}}
		for _, ch := range c.Checks {
			vc.Checks = append(vc.Checks, c.CheckID(ch))
		}
		v.Contracts = append(v.Contracts, vc)
	}
	return nil
}

func markdown(v report.Validation) string {
	var b strings.Builder
	if v.Valid {
		n := 0
		for _, c := range v.Contracts {
			n += len(c.Checks)
		}
		fmt.Fprintf(&b, "Contracts are valid: %d contracts, %d checks (%s).\n", len(v.Contracts), n, v.Source)
		for _, c := range v.Contracts {
			fmt.Fprintf(&b, "- %s (%s, %s): %s\n", c.ID, c.Severity, c.Evidence, strings.Join(c.Checks, ", "))
		}
		return b.String()
	}
	fmt.Fprintf(&b, "Contracts are invalid (%s):\n", v.Source)
	for _, is := range v.Issues {
		fmt.Fprintf(&b, "- %s [%s]\n", is.String(), is.Code)
	}
	for _, e := range v.Errors {
		fmt.Fprintf(&b, "- %s [%s]\n", e.Message, e.Code)
	}
	return b.String()
}
