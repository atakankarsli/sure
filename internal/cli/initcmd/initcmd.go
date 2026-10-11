// Package initcmd implements `sure init`.
package initcmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/peterbourgon/ff/v3/ffcli"

	"github.com/atakankarsli/sure/internal/cli/shared"
	"github.com/atakankarsli/sure/internal/contracts"
	"github.com/atakankarsli/sure/internal/gitview"
	"github.com/atakankarsli/sure/internal/report"
)

const configTemplate = `schema_version: sure.config/v1
contracts: .sure/contracts
source_roots:
%s
# Removed from evidence before any request is built. Secrets, VCS metadata and
# dependency folders are always excluded as well.
exclude:
  - "**/*.test.*"
  - "**/*.spec.*"
  - "**/__tests__/**"
  - "**/*_test.go"
  - "**/test/**"
  - "**/tests/**"
  - "**/*Tests/**"
  - "**/testdata/**"
  - "**/dist/**"
  - "**/build/**"
  - "**/.next/**"
  - "**/coverage/**"
  - "**/vendor/**"
provider:
  # jev or anthropic. Jev runs through the Vercel AI Gateway when
  # AI_GATEWAY_API_KEY is set, else directly with TYPESAFE_API_KEY; pin it with
  # transport: gateway | typesafe. Claude reads SURE_ANTHROPIC_API_KEY.
  name: jev
limits:
  concurrency: 4
  max_file_bytes: 262144
  max_state_bytes: 65536
  max_request_bytes: 98304
  max_estimated_tokens: 30000
# Unvalidated starting policy; tune only on development data.
policy:
  version: experimental-v1
  min_selected_probability: 0.90
  min_probability_margin: 0.20
  min_location_probability: 0.50
`

const contractTemplate = `---
schema_version: sure.contract/v1
id: feature.behavior
severity: error
evidence: code
scope:
  - src/features/feature/**
context:
  - src/domain/model.ts
---
# Feature behavior

Copy this file into .sure/contracts/, name the feature, and write one
behavioral outcome per check. Write what the product must do, from accepted
requirements, not a description of the current code.

## FEAT-001 — Short name of the outcome
Given <precondition>, when <event>, <observable outcome> must hold.
`

const gitignore = "cache/\nrecordings/\n"

func Command(deps shared.Deps) *ffcli.Command {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	repo := fs.String("repo", ".", "Repository to initialize")
	asJSON := fs.Bool("json", false, "Print one JSON document")
	return &ffcli.Command{
		Name:       "init",
		ShortUsage: "sure init [--json]",
		ShortHelp:  "Create .sure/ with a config and a contract template.",
		LongHelp: `Creates .sure/config.yaml, an empty .sure/contracts/ directory and a contract
template. Existing files are never overwritten. Does not call a provider and
does not write any requirement.`,
		FlagSet:   fs,
		UsageFunc: shared.DefaultUsageFunc,
		Exec: func(ctx context.Context, args []string) error {
			res := report.Init{SchemaVersion: report.InitSchema, Created: []string{}, Existing: []string{}, Errors: []report.Error{}}
			if len(args) > 0 {
				res.Errors = append(res.Errors, report.Error{Code: "invalid_flags", Message: "unexpected argument " + args[0]})
			} else if err := scaffold(ctx, *repo, &res); err != nil {
				res.Errors = append(res.Errors, report.Error{Code: "init_failed", Message: err.Error()})
			}
			if len(res.Errors) > 0 {
				res.ExitCode = report.ExitUsage
			}
			if err := shared.Print(deps.Stdout, *asJSON, res, func() string { return human(res, deps.Style()) }); err != nil {
				return err
			}
			return shared.Exit(res.ExitCode)
		},
	}
}

func scaffold(ctx context.Context, dir string, res *report.Init) error {
	repo, err := gitview.Open(ctx, dir)
	if err != nil {
		return err
	}
	root := repo.Root
	roots := "  - ."
	if info, err := os.Stat(filepath.Join(root, "src")); err == nil && info.IsDir() {
		roots = "  - src"
	}
	files := []struct {
		rel     string
		content string
	}{
		{contracts.ConfigPath, fmt.Sprintf(configTemplate, roots)},
		{".sure/.gitignore", gitignore},
		{contracts.DefaultDir + "/.gitkeep", ""},
		{".sure/contract-template.md", contractTemplate},
	}
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f.rel))
		if _, err := os.Lstat(p); err == nil {
			res.Existing = append(res.Existing, f.rel)
			continue
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		fh, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			if errors.Is(err, fs.ErrExist) {
				res.Existing = append(res.Existing, f.rel)
				continue
			}
			return err
		}
		_, werr := fh.WriteString(f.content)
		if cerr := fh.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			return werr
		}
		res.Created = append(res.Created, f.rel)
	}
	return nil
}

func human(r report.Init, st report.Style) string {
	var b strings.Builder
	for _, e := range r.Errors {
		fmt.Fprintf(&b, "%s %s\n", st.Red("✗"), e.Message)
	}
	for _, c := range r.Created {
		fmt.Fprintf(&b, "%s %s\n", st.Green("+"), c)
	}
	for _, c := range r.Existing {
		fmt.Fprintf(&b, "%s %s %s\n", st.Dim("="), c, st.Dim("(kept)"))
	}
	if len(r.Created) > 0 {
		b.WriteString(st.Dim("\nNext: write a contract in .sure/contracts/ (copy .sure/contract-template.md), run `sure validate`, commit .sure/, then run `sure`.") + "\n")
	}
	return b.String()
}
