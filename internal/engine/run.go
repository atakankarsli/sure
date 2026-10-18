package engine

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/atakankarsli/sure/internal/cache"
	"github.com/atakankarsli/sure/internal/contracts"
	"github.com/atakankarsli/sure/internal/evidence"
	"github.com/atakankarsli/sure/internal/provider"
	"github.com/atakankarsli/sure/internal/provider/replay"
	"github.com/atakankarsli/sure/internal/report"
)

type result struct {
	resp provider.Response
	err  error
}

// Check runs a full check and always returns a report; failures are folded
// into it with the matching exit code.
func Check(ctx context.Context, opts Options, deps Deps) report.Run {
	start := deps.Now()
	run := report.Run{
		SchemaVersion:   report.RunSchema,
		RunID:           deps.NewRunID(),
		Contracts:       []report.ContractEvidence{},
		Checks:          []report.Check{},
		Errors:          []report.Error{},
		UnmappedChanges: []string{},
		ContractChanges: []contracts.Change{},
		NextActions:     []report.Action{},
	}
	finish := func() report.Run {
		run.Timing.TotalMS = deps.Now().Sub(start).Milliseconds()
		return run
	}
	p, err := prepare(ctx, opts, deps)
	if err != nil {
		setFailure(&run, err)
		return finish()
	}
	run.Snapshot, run.Trust, run.Provider = header(p)
	run.UnmappedChanges = nonNil(p.unmapped)
	run.ContractChanges = nonNilChanges(p.changes)
	run.Timing.CollectionMS = p.collectedAt.Milliseconds()

	results := map[string]result{}
	var base map[string]string
	if !p.noChanges && p.needsProvider() {
		prov, err := buildProvider(p, opts, deps)
		if err != nil {
			setFailure(&run, err)
			return finish()
		}
		modelStart := deps.Now()
		results = evaluate(ctx, p, prov, deps)
		if opts.AssessBase && opts.Mode == ModeChanged {
			base = assessBase(ctx, p, prov, results)
		}
		run.Timing.ModelMS = deps.Now().Sub(modelStart).Milliseconds()
	}

	var evaluated int
	run.Checks, run.Errors, evaluated = buildChecks(p, results, base)
	run.Contracts = evidenceReport(p, results)
	run.Confidence = confidence(run.Checks, run.Contracts)
	run.Summary = summarize(run.Checks, evaluated)
	addUsage(run.Provider, p, results)
	run.NextActions = topLevelActions(p)
	run.Status, run.Gate, run.ExitCode = conclude(p, run.Checks, run.Errors)
	return finish()
}

// Plan prepares everything a check would do without calling a provider.
func Plan(ctx context.Context, opts Options, deps Deps) report.Plan {
	out := report.Plan{
		SchemaVersion:   report.PlanSchema,
		Status:          "ok",
		Contracts:       []report.ContractEvidence{},
		Outcomes:        []report.PlannedCheck{},
		Errors:          []report.Error{},
		UnmappedChanges: []string{},
		ContractChanges: []contracts.Change{},
		NextActions:     []report.Action{},
	}
	p, err := prepare(ctx, opts, deps)
	if err != nil {
		f := asFailure(err)
		out.Status, out.ExitCode = "invalid", f.exit
		out.Errors = append(out.Errors, report.Error{Code: f.code, Message: f.msg})
		return out
	}
	out.Snapshot, out.Trust, out.Provider = header(p)
	out.UnmappedChanges = nonNil(p.unmapped)
	out.ContractChanges = nonNilChanges(p.changes)
	out.Contracts = evidenceReport(p, nil)
	out.NextActions = topLevelActions(p)
	if p.noChanges {
		out.Status = report.StatusNoChanges
	}
	for _, cp := range p.plans {
		out.Summary.Total += len(cp.c.Checks)
		if cp.selected {
			out.Summary.Selected += len(cp.c.Checks)
			if cp.preset == nil {
				out.Summary.Requests++
				out.Summary.EstimatedTokens += cp.req.estTokens
			}
		}
		for _, ch := range cp.c.Checks {
			pc := report.PlannedCheck{ID: cp.c.CheckID(ch), Selected: cp.selected}
			switch {
			case !cp.selected:
				pc.Planned = report.VerdictNotChecked
			case cp.preset != nil:
				pc.Planned, pc.ReasonCode = cp.preset.verdict, cp.preset.reason
				out.Summary.KnownUnknown++
			default:
				pc.Planned = "evaluate"
				out.Summary.WouldEvaluate++
			}
			out.Outcomes = append(out.Outcomes, pc)
		}
	}
	return out
}

func (p *prepared) needsProvider() bool {
	for _, cp := range p.plans {
		if cp.selected && cp.preset == nil {
			return true
		}
	}
	return false
}

func asFailure(err error) *failure {
	var f *failure
	if errors.As(err, &f) {
		return f
	}
	return &failure{code: "internal_error", exit: report.ExitUsage, msg: err.Error()}
}

func setFailure(run *report.Run, err error) {
	f := asFailure(err)
	run.Status, run.Gate, run.ExitCode = report.StatusFailed, report.GateError, f.exit
	run.Errors = append(run.Errors, report.Error{Code: f.code, Message: f.msg})
}

func header(p *prepared) (*report.Snapshot, *report.Trust, *report.Provider) {
	snap := &report.Snapshot{
		Kind:         "worktree",
		Scope:        p.opts.Mode,
		Selection:    p.opts.Mode,
		SourceDigest: p.snap.Digest(),
		ChangedPaths: nonNil(p.changedPaths),
	}
	if p.opts.Mode == ModeAuto {
		snap.Scope, snap.Selection = ModeChanged, ModeChanged
		if p.autoAll {
			snap.Scope, snap.Selection = ModeAll, "all_no_changes"
			if p.baseCommit == "" {
				snap.Selection = "all_no_base"
			}
		}
	}
	if p.baseCommit != "" {
		snap.BaseRef, snap.BaseCommit = report.Ptr(p.baseName), report.Ptr(p.baseCommit)
	}
	if p.head != "" {
		snap.HeadCommit = report.Ptr(p.head)
	}
	trust := &report.Trust{
		Mode:           "protected",
		ContractDigest: p.trusted.Digest(),
		PolicyVersion:  p.trusted.Config.Policy.Version,
		MinProbability: p.trusted.Config.Policy.MinSelectedProbability,
	}
	switch {
	case p.adhocOnly:
		trust.Mode = "adhoc"
	case p.opts.Draft:
		trust.Mode = "draft"
	default:
		trust.ContractsRef, trust.ContractsCommit = report.Ptr(p.trustRef), report.Ptr(p.trustCommit)
	}
	id := p.identity
	prov := &report.Provider{
		Name:              id.Name,
		Transport:         id.Transport,
		Endpoint:          id.Endpoint,
		RequestedModel:    id.Model,
		ReturnedModels:    []string{},
		Mode:              "none",
		PromptVersion:     PromptVersion,
		ProbabilitySource: id.ProbabilitySource,
		LongContext:       id.LongContext,
	}
	return snap, trust, prov
}

func buildProvider(p *prepared, opts Options, deps Deps) (provider.Provider, error) {
	if opts.Provider == provider.NameReplay {
		if opts.ReplayDir == "" {
			return nil, fail("missing_recordings", report.ExitUsage, "--provider replay needs --recordings <dir>")
		}
		return &replay.Player{Dir: opts.ReplayDir, Replayed: p.identity}, nil
	}
	live, err := deps.Factory.New(p.spec)
	if err != nil {
		if provider.KindOf(err) == provider.ErrMissingKey {
			return nil, fail(string(provider.ErrMissingKey), report.ExitUsage, "%v", err)
		}
		return nil, fail("invalid_provider", report.ExitUsage, "%v", err)
	}
	prov := live
	if !opts.NoCache {
		dir, err := cache.DefaultDir(deps.Getenv)
		if err == nil {
			var c *cache.Cache
			if c, err = cache.Open(dir, cache.Namespace(p.repo.Root)); err == nil {
				w := cache.Wrap(prov, c, PromptVersion, evidence.CollectorVersion)
				w.OnPutError = func(err error) { fmt.Fprintf(deps.Stderr, "sure: cache write failed: %v\n", err) }
				prov = w
			}
		}
		if err != nil {
			fmt.Fprintf(deps.Stderr, "sure: cache disabled: %v\n", err)
		}
	}
	if opts.RecordDir != "" {
		prov = &replay.Recorder{Dir: opts.RecordDir, Inner: prov, IncludeRequest: true}
	}
	return prov, nil
}

// evaluate sends one request per selected contract through a bounded worker
// pool. Answers are not interpreted here.
func evaluate(ctx context.Context, p *prepared, prov provider.Provider, deps Deps) map[string]result {
	var jobs []contractPlan
	for _, cp := range p.plans {
		if cp.selected && cp.preset == nil {
			jobs = append(jobs, cp)
		}
	}
	out := make(map[string]result, len(jobs))
	var mu sync.Mutex
	sem := make(chan struct{}, max(1, p.trusted.Config.Limits.Concurrency))
	var wg sync.WaitGroup
	for _, cp := range jobs {
		wg.Add(1)
		go func(cp contractPlan) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				mu.Lock()
				out[cp.c.ID] = result{err: provider.ContextError(ctx)}
				mu.Unlock()
				return
			}
			defer func() { <-sem }()
			resp, err := prov.Evaluate(ctx, cp.req.req)
			if err == nil {
				err = checkShape(cp, resp)
			}
			mu.Lock()
			out[cp.c.ID] = result{resp: resp, err: err}
			mu.Unlock()
			if deps.Progress == nil {
				return
			}
			if err != nil {
				fmt.Fprintf(deps.Progress, "sure: %s: %v\n", cp.c.ID, err)
			} else {
				fmt.Fprintf(deps.Progress, "sure: %s: %d checks (%s)\n", cp.c.ID, len(cp.c.Checks), resp.Source)
			}
		}(cp)
	}
	wg.Wait()
	return out
}

// checkShape re-validates a decision against the request it answers,
// whatever layer (cache, replay) produced it.
func checkShape(cp contractPlan, resp provider.Response) error {
	return provider.ValidateAnswers(cp.req.req, resp.Answers)
}

// assessBase re-asks violated checks against the base commit's source so a
// violation can be called an introduced regression only when the base was
// supported under identical contracts, model and prompt.
func assessBase(ctx context.Context, p *prepared, prov provider.Provider, current map[string]result) map[string]string {
	out := map[string]string{}
	cfg := p.trusted.Config
	var baseSnap *evidence.Snapshot
	for _, cp := range p.plans {
		r, ok := current[cp.c.ID]
		if !ok || r.err != nil {
			continue
		}
		var violated []int
		for i := range cp.c.Checks {
			if v, _, _ := decide(cfg.Policy, r.resp.Answers[cp.req.assessIDs[i]]); v == report.VerdictViolated {
				violated = append(violated, i)
			}
		}
		if len(violated) == 0 {
			continue
		}
		for _, i := range violated {
			out[cp.c.CheckID(cp.c.Checks[i])] = report.RegressionUnknown
		}
		if baseSnap == nil {
			var err error
			if baseSnap, err = evidence.Capture(ctx, p.repo.Tree(p.baseCommit), cfg); err != nil {
				return out
			}
		}
		b, err := evidence.Collect(ctx, baseSnap, cfg, cp.c)
		if err != nil || len(b.Missing) > 0 || len(b.Files) == 0 {
			continue
		}
		req, err := buildRequest(cp.c, b, cfg.Limits)
		if err != nil || req.overBudget {
			continue
		}
		resp, err := prov.Evaluate(ctx, req.req)
		if err != nil || provider.ValidateAnswers(req.req, resp.Answers) != nil {
			continue
		}
		for _, i := range violated {
			switch v, _, _ := decide(cfg.Policy, resp.Answers[req.assessIDs[i]]); v {
			case report.VerdictSupported:
				out[cp.c.CheckID(cp.c.Checks[i])] = report.RegressionIntroduced
			case report.VerdictViolated:
				out[cp.c.CheckID(cp.c.Checks[i])] = report.RegressionPreexisting
			}
		}
	}
	return out
}

// ExitFor maps a provider error kind to its process exit code.
func ExitFor(kind provider.ErrorKind) int {
	switch kind {
	case provider.ErrCanceled:
		return report.ExitCanceled
	case provider.ErrDeadline:
		return report.ExitTimeout
	case provider.ErrMalformed:
		return report.ExitMalformed
	case provider.ErrRateLimited:
		return report.ExitRateLimit
	case provider.ErrMissingKey:
		return report.ExitUsage
	default:
		return report.ExitNetwork
	}
}

func evidenceReport(p *prepared, results map[string]result) []report.ContractEvidence {
	out := []report.ContractEvidence{}
	for _, cp := range p.plans {
		ce := report.ContractEvidence{
			Adhoc:    cp.adhoc,
			ID:       cp.c.ID,
			Path:     cp.c.Path,
			Evidence: cp.c.Evidence,
			Selected: cp.selected,
			Files:    nonNilSlice(cp.bundle.Files),
			Missing:  nonNilSlice(cp.bundle.Missing),
			Skipped:  nonNilSlice(cp.bundle.Skipped),
			Decision: "none",
		}
		if cp.selected {
			ce.SelectedBy = cp.by
			ce.StateBytes, ce.RequestBytes, ce.EstimatedTokens = cp.req.stateBytes, cp.req.requestBytes, cp.req.estTokens
		}
		switch r, ok := results[cp.c.ID]; {
		case cp.preset != nil && cp.selected:
			ce.Decision = "not_sent:" + cp.preset.reason
		case ok && r.err != nil:
			ce.Decision = "error"
		case ok:
			ce.Decision = r.resp.Source
		}
		out = append(out, ce)
	}
	return out
}

// confidence fills each contract's lowest check confidence and returns the
// run's. A provider error leaves the affected contract, and the run, without a
// number: nothing was decided.
func confidence(checks []report.Check, contracts []report.ContractEvidence) *report.Confidence {
	perContract := map[string]*float64{}
	failed := map[string]bool{}
	var lowest *float64
	undecided, selected := 0, 0
	for _, c := range checks {
		switch c.Verdict {
		case report.VerdictNotChecked:
			continue
		case report.VerdictError:
			failed[c.Contract] = true
			selected++
			continue
		}
		selected++
		if c.Confidence == nil {
			continue
		}
		v := *c.Confidence
		if c.Probabilities == nil {
			undecided++
		}
		if cur, ok := perContract[c.Contract]; !ok || v < *cur {
			perContract[c.Contract] = report.Ptr(v)
		}
		if lowest == nil || v < *lowest {
			lowest = report.Ptr(v)
		}
	}
	for i := range contracts {
		if v, ok := perContract[contracts[i].ID]; ok && !failed[contracts[i].ID] {
			contracts[i].Confidence = v
		}
	}
	if lowest == nil || len(failed) > 0 || selected == 0 {
		return nil
	}
	return &report.Confidence{Percent: report.Percent(*lowest), Probability: *lowest, Basis: report.ConfidenceBasis, Undecided: undecided}
}

func addUsage(prov *report.Provider, p *prepared, results map[string]result) {
	sources := map[string]bool{}
	models := map[string]bool{}
	for _, cp := range p.plans {
		r, ok := results[cp.c.ID]
		if !ok {
			continue
		}
		prov.Usage.Requests++
		prov.Usage.EstimatedInputTokens += cp.req.estTokens
		if r.err != nil {
			continue
		}
		sources[r.resp.Source] = true
		if r.resp.Model != "" {
			models[r.resp.Model] = true
		}
		switch r.resp.Source {
		case provider.SourceCache:
			prov.Usage.CacheHits++
		case provider.SourceLive:
			prov.Usage.LiveRequests++
			prov.Usage.InputTokens += r.resp.Usage.InputTokens
			prov.Usage.OutputTokens += r.resp.Usage.OutputTokens
		}
	}
	for m := range models {
		prov.ReturnedModels = append(prov.ReturnedModels, m)
	}
	sort.Strings(prov.ReturnedModels)
	switch len(sources) {
	case 0:
		prov.Mode = "none"
	case 1:
		for s := range sources {
			prov.Mode = s
		}
	default:
		prov.Mode = "mixed"
	}
}

func topLevelActions(p *prepared) []report.Action {
	out := []report.Action{}
	seen := map[string]bool{}
	for _, c := range p.changes {
		key := c.Contract
		if key == "" {
			key = c.Field
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		desc := "Review the contract change against the trusted base; evaluation used the trusted version."
		if c.Contract == "" {
			desc = "Review the change to " + c.Field + "; evaluation used the trusted configuration."
		}
		out = append(out, report.Action{Kind: report.ActionReviewContract, Path: pathFor(p, c), Description: desc})
	}
	for _, u := range p.unmapped {
		out = append(out, report.Action{Kind: report.ActionMapChange, Path: u,
			Description: "This changed source file is covered by no contract; add it to a contract's scope or context, or exclude it."})
	}
	return out
}

func pathFor(p *prepared, c contracts.Change) string {
	if c.Contract == "" {
		return c.Field
	}
	if tc, ok := p.trusted.Contract(c.Contract); ok {
		return tc.Path
	}
	return ""
}

func summarize(checks []report.Check, evaluated int) report.Summary {
	s := report.Summary{Total: len(checks), Evaluated: evaluated}
	for _, c := range checks {
		if c.Verdict != report.VerdictNotChecked {
			s.Selected++
		}
		switch c.Verdict {
		case report.VerdictSupported:
			s.Supported++
		case report.VerdictViolated:
			s.Violated++
		case report.VerdictUnknown:
			s.Unknown++
		case report.VerdictError:
			s.Error++
		case report.VerdictNotChecked:
			s.NotChecked++
		}
	}
	return s
}

// conclude derives status, gate and exit code. Operational failure outranks
// a violation, which outranks incompleteness.
func conclude(p *prepared, checks []report.Check, errs []report.Error) (string, string, int) {
	if p.noChanges {
		return report.StatusNoChanges, report.GateNotApplicable, report.ExitClear
	}
	op := 0
	for _, e := range errs {
		code := ExitFor(provider.ErrorKind(e.Code))
		for _, pri := range report.OperationalPriority {
			if code == pri && (op == 0 || indexOf(report.OperationalPriority, pri) < indexOf(report.OperationalPriority, op)) {
				op = pri
			}
		}
	}
	violated, incomplete, selected := false, len(p.unmapped) > 0 || len(p.changes) > 0, 0
	for _, c := range checks {
		if c.Verdict != report.VerdictNotChecked {
			selected++
		}
		switch c.Verdict {
		case report.VerdictViolated:
			if c.Severity == contracts.SeverityError {
				violated = true
			}
		case report.VerdictUnknown, report.VerdictError:
			incomplete = true
		}
	}
	status := report.StatusCompleted
	if incomplete {
		status = report.StatusIncomplete
	}
	switch {
	case op != 0:
		return report.StatusFailed, report.GateError, op
	case violated:
		return status, report.GateViolated, report.ExitViolation
	case incomplete:
		return status, report.GateIncomplete, report.ExitIncomplete
	case selected == 0:
		return report.StatusNoApplicable, report.GateNotApplicable, report.ExitClear
	default:
		return status, report.GateClear, report.ExitClear
	}
}

func indexOf(list []int, v int) int {
	for i, x := range list {
		if x == v {
			return i
		}
	}
	return len(list)
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func nonNilChanges(s []contracts.Change) []contracts.Change {
	if s == nil {
		return []contracts.Change{}
	}
	return s
}

func nonNilSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
