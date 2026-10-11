// Package engine plans and runs a check: it resolves the trusted contracts,
// captures the working tree once, selects contracts, collects evidence, asks
// the provider fixed questions and turns answers into a report. It never
// builds, runs or edits the target application.
package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/atakankarsli/sure/internal/contracts"
	"github.com/atakankarsli/sure/internal/evidence"
	"github.com/atakankarsli/sure/internal/gitview"
	"github.com/atakankarsli/sure/internal/provider"
	"github.com/atakankarsli/sure/internal/report"
)

const (
	// ModeAuto checks the contracts touched by changes against the base, or
	// every contract when nothing changed.
	ModeAuto     = "auto"
	ModeAll      = "all"
	ModeChanged  = "changed"
	ModeContract = "contract"
)

type Options struct {
	Repo         string
	Mode         string
	Contracts    []string
	BaseRef      string
	ContractsRef string
	Draft        bool
	Provider     string
	NoCache      bool
	RecordDir    string
	ReplayDir    string
	AssessBase   bool
}

// ProviderSpec is the provider choice after config and flags are merged.
type ProviderSpec struct {
	Name      string
	Transport string
	Model     string
	Effort    string
	BaseURL   string
}

// Factory builds providers. Identity must work without credentials so plans
// and replays can describe a provider that is never called.
type Factory interface {
	Identity(spec ProviderSpec) (provider.Identity, error)
	New(spec ProviderSpec) (provider.Provider, error)
}

type Deps struct {
	Getenv   func(string) string
	Now      func() time.Time
	NewRunID func() string
	Factory  Factory
	// Stderr receives warnings; Progress, when set, receives one line per
	// decided contract.
	Stderr   io.Writer
	Progress io.Writer
}

// failure is an error that stops a run before evaluation, with its exit code.
type failure struct {
	code string
	exit int
	msg  string
}

func (f *failure) Error() string { return f.msg }

func fail(code string, exit int, format string, args ...any) *failure {
	return &failure{code: code, exit: exit, msg: fmt.Sprintf(format, args...)}
}

// prepared is everything known before the first provider call.
type prepared struct {
	opts       Options
	repo       *gitview.Repo
	head       string
	baseCommit string
	baseName   string
	// autoAll is set when auto mode found no changes and fell back to all.
	autoAll      bool
	trustCommit  string
	trustRef     string
	trusted      *contracts.Set
	changes      []contracts.Change
	snap         *evidence.Snapshot
	changed      []gitview.Change
	changedPaths []string
	selected     map[string]string
	unmapped     []string
	spec         ProviderSpec
	identity     provider.Identity
	plans        []contractPlan
	collectedAt  time.Duration
	noChanges    bool
}

// contractPlan is the evidence and request for one trusted contract.
type contractPlan struct {
	c        contracts.Contract
	selected bool
	by       string
	bundle   evidence.Bundle
	req      built
	// preset holds a non-provider outcome (missing context, visual, budget)
	// for every check; when nil the contract needs a provider call.
	preset *outcome
}

type outcome struct {
	verdict string
	reason  string
	message string
	actions []report.Action
}

func prepare(ctx context.Context, opts Options, deps Deps) (*prepared, error) {
	start := deps.Now()
	p := &prepared{opts: opts, selected: map[string]string{}}
	repo, err := gitview.Open(ctx, opts.Repo)
	if err != nil {
		return nil, fail("invalid_repo", report.ExitUsage, "%v", err)
	}
	p.repo = repo
	if p.head, err = repo.Head(ctx); err != nil {
		return nil, contextFailure(ctx, err)
	}

	if opts.Mode == "" {
		opts.Mode = ModeAuto
		p.opts.Mode = ModeAuto
	}
	if opts.Mode == ModeChanged && (!repo.IsGit || p.head == "") {
		return nil, fail("changed_requires_git", report.ExitUsage, "--changed needs a Git repository with at least one commit")
	}
	if (opts.Mode == ModeChanged || opts.Mode == ModeAuto) && repo.IsGit && p.head != "" {
		ref := opts.BaseRef
		if ref == "" {
			ref = repo.DefaultBase(ctx)
		}
		p.baseName = ref
		base, err := repo.ResolveCommit(ctx, ref)
		if err != nil {
			return nil, fail("invalid_base", report.ExitUsage, "--base: %v", err)
		}
		if p.baseCommit, err = repo.MergeBase(ctx, base, p.head); err != nil {
			return nil, fail("invalid_base", report.ExitUsage, "--base: %v", err)
		}
	}

	if err := p.loadTrust(ctx); err != nil {
		return nil, err
	}

	cfg := p.trusted.Config
	if p.snap, err = evidence.Capture(ctx, repo.Worktree(), cfg); err != nil {
		if ctx.Err() != nil {
			return nil, contextFailure(ctx, err)
		}
		return nil, fail("snapshot_failed", report.ExitUsage, "capture source: %v", err)
	}

	if err := p.selectContracts(ctx); err != nil {
		return nil, err
	}

	p.spec = ProviderSpec{
		Name:      cfg.Provider.Name,
		Transport: cfg.Provider.Transport,
		Model:     cfg.Provider.Model,
		Effort:    cfg.Provider.Effort,
		BaseURL:   deps.Getenv("SURE_PROVIDER_BASE_URL"),
	}
	if opts.Provider != "" && opts.Provider != provider.NameReplay && opts.Provider != cfg.Provider.Name {
		p.spec = ProviderSpec{Name: opts.Provider, BaseURL: p.spec.BaseURL}
	}
	if p.identity, err = deps.Factory.Identity(p.spec); err != nil {
		return nil, fail("invalid_provider", report.ExitUsage, "%v", err)
	}

	if err := p.planEvidence(ctx); err != nil {
		return nil, err
	}
	p.collectedAt = deps.Now().Sub(start)
	return p, nil
}

func (p *prepared) loadTrust(ctx context.Context) error {
	wt, wtErr := contracts.Load(ctx, p.repo.Worktree())
	if wtErr != nil && !errors.Is(wtErr, contracts.ErrNotInitialized) {
		return contextFailure(ctx, wtErr)
	}
	if errors.Is(wtErr, contracts.ErrNotInitialized) && p.opts.ContractsRef == "" {
		if _, err := p.repo.Worktree().Read(ctx, ".sure", 0); errors.Is(err, gitview.ErrNotFound) {
			return fail("not_initialized", report.ExitUsage, "no .sure/ in %s", p.repo.Root)
		}
	}
	if p.opts.Draft {
		if wtErr != nil {
			return fail("not_initialized", report.ExitUsage, "%v", wtErr)
		}
		if !wt.Valid() {
			return issuesFailure(wt.Issues)
		}
		p.trusted = wt
		return nil
	}

	ref := p.opts.ContractsRef
	switch {
	case ref != "":
	case p.baseCommit != "":
		ref = p.baseCommit
	case p.head != "":
		ref = "HEAD"
	default:
		return fail("no_trusted_base", report.ExitUsage,
			"no commit holds trusted contracts yet; commit .sure/ or run with --draft")
	}
	if !p.repo.IsGit {
		return fail("no_trusted_base", report.ExitUsage, "trusted contracts need a Git repository; run with --draft")
	}
	commit, err := p.repo.ResolveCommit(ctx, ref)
	if err != nil {
		return fail("invalid_contracts_ref", report.ExitUsage, "--contracts-ref: %v", err)
	}
	trusted, err := contracts.Load(ctx, p.repo.Tree(commit))
	if err != nil {
		if errors.Is(err, contracts.ErrNotInitialized) {
			return fail("no_trusted_contracts", report.ExitUsage,
				"%s has no %s; commit the contracts first or run with --draft", short(commit), contracts.ConfigPath)
		}
		return contextFailure(ctx, err)
	}
	if !trusted.Valid() {
		return issuesFailure(trusted.Issues)
	}
	p.trusted, p.trustCommit, p.trustRef = trusted, commit, ref
	if p.opts.ContractsRef == "" && ref == p.baseCommit {
		p.trustRef = commit
	}
	p.changes = contracts.Diff(trusted, wt, wtErr)
	return nil
}

func issuesFailure(issues []contracts.Issue) *failure {
	msgs := make([]string, 0, len(issues))
	for _, is := range issues {
		msgs = append(msgs, is.String())
	}
	return fail("invalid_contracts", report.ExitUsage, "contracts are invalid: %s", strings.Join(msgs, "; "))
}

// dependencyFiles change what the code means without touching it; a change to
// any of them selects every contract.
var dependencyFiles = map[string]bool{
	"package.json": true, "package-lock.json": true, "npm-shrinkwrap.json": true, "yarn.lock": true,
	"pnpm-lock.yaml": true, "bun.lockb": true, "bun.lock": true, "deno.json": true, "deno.lock": true,
	"go.mod": true, "go.sum": true, "go.work": true, "Cargo.toml": true, "Cargo.lock": true,
	"Package.swift": true, "Package.resolved": true, "Podfile": true, "Podfile.lock": true,
	"build.gradle": true, "build.gradle.kts": true, "settings.gradle": true, "settings.gradle.kts": true,
	"gradle.properties": true, "pom.xml": true, "requirements.txt": true, "pyproject.toml": true,
	"poetry.lock": true, "uv.lock": true, "Pipfile.lock": true, "Gemfile": true, "Gemfile.lock": true,
	"composer.json": true, "composer.lock": true, "tsconfig.json": true, "jsconfig.json": true,
	"babel.config.js": true, ".babelrc": true, "project.pbxproj": true,
}

var dependencyPrefixes = []string{"tsconfig.", "next.config.", "vite.config.", "webpack.config.", "metro.config."}

func invalidatesPlan(cfg contracts.Config, p string) bool {
	if p == contracts.ConfigPath || strings.HasPrefix(p, cfg.Contracts+"/") {
		return true
	}
	base := path.Base(p)
	if dependencyFiles[base] {
		return true
	}
	for _, pre := range dependencyPrefixes {
		if strings.HasPrefix(base, pre) {
			return true
		}
	}
	return false
}

func (p *prepared) selectContracts(ctx context.Context) error {
	set := p.trusted
	switch p.opts.Mode {
	case ModeAll:
		for _, c := range set.Contracts {
			p.selected[c.ID] = "all"
		}
	case ModeContract:
		for _, arg := range p.opts.Contracts {
			id, _, _ := strings.Cut(arg, "/")
			matched := false
			for _, c := range set.Contracts {
				if c.ID == id || strings.HasPrefix(c.ID, id+".") {
					p.selected[c.ID] = "contract"
					matched = true
				}
			}
			if !matched {
				ids := make([]string, 0, len(set.Contracts))
				for _, c := range set.Contracts {
					ids = append(ids, c.ID)
				}
				return fail("unknown_contract", report.ExitUsage, "no trusted contract matches %q; known: %s", arg, strings.Join(ids, ", "))
			}
		}
	case ModeAuto:
		if p.baseCommit == "" {
			p.selectAll("all")
			p.autoAll = true
			return nil
		}
		if err := p.selectChanged(ctx); err != nil {
			return err
		}
		if p.noChanges {
			p.noChanges = false
			p.autoAll = true
			p.selectAll("all")
		}
	case ModeChanged:
		return p.selectChanged(ctx)
	default:
		return fail("invalid_mode", report.ExitUsage, "choose one of --all, --changed or --contract")
	}
	return nil
}

func (p *prepared) selectAll(by string) {
	for _, c := range p.trusted.Contracts {
		p.selected[c.ID] = by
	}
}

// selectChanged selects every contract whose scope or context covers a path
// that differs from the base, and records changed source no contract covers.
func (p *prepared) selectChanged(ctx context.Context) error {
	set := p.trusted
	changes, err := p.repo.Changes(ctx, p.baseCommit)
	if err != nil {
		return contextFailure(ctx, err)
	}
	p.changed = changes
	seen := map[string]bool{}
	for _, c := range changes {
		for _, x := range c.Paths() {
			if !seen[x] {
				seen[x] = true
				p.changedPaths = append(p.changedPaths, x)
			}
		}
	}
	sort.Strings(p.changedPaths)
	if len(p.changedPaths) == 0 {
		p.noChanges = true
		return nil
	}
	for _, x := range p.changedPaths {
		if invalidatesPlan(set.Config, x) {
			for _, c := range set.Contracts {
				if _, ok := p.selected[c.ID]; !ok {
					p.selected[c.ID] = "plan_invalidated:" + x
				}
			}
		}
	}
	for _, x := range p.changedPaths {
		covered := false
		for _, c := range set.Contracts {
			if c.Covers(x) {
				covered = true
				if _, ok := p.selected[c.ID]; !ok {
					p.selected[c.ID] = "changed:" + x
				}
			}
		}
		if !covered && set.Config.Allowed(x) {
			p.unmapped = append(p.unmapped, x)
		}
	}
	return nil
}

func (p *prepared) planEvidence(ctx context.Context) error {
	cfg := p.trusted.Config
	var manifest *evidence.VisualManifest
	var manifestErr error
	if cfg.Visual.Manifest != "" {
		b, err := p.repo.Worktree().Read(ctx, cfg.Visual.Manifest, evidence.MaxVisualManifest)
		switch {
		case errors.Is(err, gitview.ErrNotFound):
		case err != nil:
			manifestErr = err
		default:
			m, err := evidence.ParseVisualManifest(b)
			if err != nil {
				manifestErr = err
			} else {
				manifest = &m
			}
		}
	}
	for _, c := range p.trusted.Contracts {
		by, sel := p.selected[c.ID]
		cp := contractPlan{c: c, selected: sel, by: by}
		if !sel {
			p.plans = append(p.plans, cp)
			continue
		}
		bundle, err := evidence.Collect(ctx, p.snap, cfg, c)
		if err != nil {
			return contextFailure(ctx, err)
		}
		cp.bundle = bundle
		switch {
		case c.Evidence == contracts.EvidenceImage:
			cp.preset = visualOutcome(ctx, manifest, manifestErr, c, p)
		case len(cp.bundle.Missing) > 0:
			var paths []string
			var acts []report.Action
			for _, m := range cp.bundle.Missing {
				paths = append(paths, m.Path+" ("+m.Reason+")")
				acts = append(acts, report.Action{Kind: report.ActionSupplyContext, Path: m.Path,
					Description: "Make this required context file available, or correct the contract's context list."})
			}
			cp.preset = &outcome{verdict: report.VerdictUnknown, reason: "missing_context",
				message: "Required context could not be supplied: " + strings.Join(paths, ", ") + ". Nothing was sent to the provider.",
				actions: acts}
		case !hasScopeFile(cp.bundle):
			cp.preset = &outcome{verdict: report.VerdictUnknown, reason: "empty_scope",
				message: "The contract's scope matches no readable source files, so nothing supports it.",
				actions: []report.Action{{Kind: report.ActionSupplyContext,
					Description: "Point the contract's scope at the code that implements it."}}}
		default:
			req, err := buildRequest(c, cp.bundle, cfg.Limits)
			if err != nil {
				return fail("request_build_failed", report.ExitUsage, "%s: %v", c.ID, err)
			}
			cp.req = req
			if req.overBudget {
				cp.preset = &outcome{verdict: report.VerdictUnknown, reason: "evidence_budget_exceeded",
					message: fmt.Sprintf("The complete evidence (%d bytes of state, about %d tokens) exceeds the configured limits; nothing was sent. Files beyond the budget: %s.",
						req.stateBytes, req.estTokens, strings.Join(req.omitted, ", ")),
					actions: []report.Action{{Kind: report.ActionNarrowScope,
						Description: "Split the contract or narrow its scope so the complete evidence fits the limits."}}}
			}
		}
		p.plans = append(p.plans, cp)
	}
	return nil
}

func visualOutcome(ctx context.Context, m *evidence.VisualManifest, mErr error, c contracts.Contract, p *prepared) *outcome {
	if mErr != nil {
		return &outcome{verdict: report.VerdictUnknown, reason: evidence.VisualManifestError,
			message: "The visual manifest could not be used: " + mErr.Error(),
			actions: []report.Action{{Kind: report.ActionProvideVisual, Description: "Regenerate a valid visual manifest."}}}
	}
	// Each check is assessed against its own manifest entry; the contract
	// outcome reports the first non-supported reason, which every check shares
	// because no configured provider can judge images.
	status := evidence.VisualMissing
	for _, ch := range c.Checks {
		s, _ := evidence.AssessVisual(ctx, m, c.CheckID(ch), p.snap.Digest(), p.repo.Worktree())
		status = s
		if s != evidence.VisualUnsupported {
			break
		}
	}
	msgs := map[string]string{
		evidence.VisualMissing:      "This check needs a visual artifact for the current source, and none is recorded.",
		evidence.VisualStale:        "The recorded visual artifact was produced for different source; it is not reused.",
		evidence.VisualHashMismatch: "The visual artifact's bytes do not match the manifest hash.",
		evidence.VisualUnsafe:       "The visual artifact path is unsafe.",
		evidence.VisualUnsupported:  "This check needs visual judgment, and the configured provider cannot judge images.",
	}
	return &outcome{verdict: report.VerdictUnknown, reason: status, message: msgs[status],
		actions: []report.Action{{Kind: report.ActionProvideVisual,
			Description: "Produce an approved artifact for the current source with the repository's screenshot tooling."}}}
}

// hasScopeFile reports whether the scope itself matched code; context files
// alone never make a contract checkable.
func hasScopeFile(b evidence.Bundle) bool {
	for _, f := range b.Files {
		if f.Role == evidence.RoleScope {
			return true
		}
	}
	return false
}

func contextFailure(ctx context.Context, err error) *failure {
	switch {
	case errors.Is(ctx.Err(), context.Canceled):
		return fail("canceled", report.ExitCanceled, "canceled")
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return fail("deadline_exceeded", report.ExitTimeout, "run deadline exceeded")
	}
	return fail("io_error", report.ExitUsage, "%v", err)
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
