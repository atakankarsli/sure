// Package validate implements `sure validate`.
package validate

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
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	repo := fs.String("repo", ".", "Repository to read")
	ref := fs.String("ref", "", "Validate the contracts committed at this ref instead of the working tree")
	asJSON := fs.Bool("json", false, "Print one JSON document")
	return &ffcli.Command{
		Name:       "validate",
		ShortUsage: "sure validate [--ref <ref>] [--json]",
		ShortHelp:  "Check .sure/ config and contracts for mistakes, offline.",
		FlagSet:    fs,
		UsageFunc:  shared.DefaultUsageFunc,
		Exec: func(ctx context.Context, args []string) error {
			v := report.Validation{SchemaVersion: report.ValidationSchema, Source: "worktree",
				Contracts: []report.ValidatedContract{}, Issues: []contracts.Issue{}, Errors: []report.Error{}}
			if len(args) > 0 {
				v.Errors = append(v.Errors, report.Error{Code: "invalid_flags", Message: "unexpected argument " + args[0]})
			} else if err := validate(ctx, *repo, *ref, &v); err != nil {
				v.Errors = append(v.Errors, report.Error{Code: "unreadable", Message: err.Error()})
			}
			v.Valid = len(v.Issues) == 0 && len(v.Errors) == 0
			if !v.Valid {
				v.ExitCode = report.ExitUsage
			}
			if err := shared.Print(deps.Stdout, *asJSON, v, func() string { return human(v, deps.Style()) }); err != nil {
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

func human(v report.Validation, st report.Style) string {
	var b strings.Builder
	if v.Valid {
		n := 0
		for _, c := range v.Contracts {
			n += len(c.Checks)
		}
		fmt.Fprintf(&b, "%s %d contracts, %d checks are valid\n", st.Green("✓"), len(v.Contracts), n)
		for _, c := range v.Contracts {
			fmt.Fprintf(&b, "  %s %s\n", c.ID, st.Dim(fmt.Sprintf("%d checks · %s · %s", len(c.Checks), c.Severity, c.Evidence)))
		}
		return b.String()
	}
	fmt.Fprintf(&b, "%s .sure/ has %d problems\n", st.Red("✗"), len(v.Issues)+len(v.Errors))
	for _, is := range v.Issues {
		fmt.Fprintf(&b, "  %s %s\n", is.String(), st.Dim("["+is.Code+"]"))
	}
	for _, e := range v.Errors {
		fmt.Fprintf(&b, "  %s %s\n", e.Message, st.Dim("["+e.Code+"]"))
	}
	return b.String()
}
