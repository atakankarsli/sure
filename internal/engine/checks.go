package engine

import (
	"fmt"
	"math"

	"github.com/atakankarsli/sure/internal/contracts"
	"github.com/atakankarsli/sure/internal/evidence"
	"github.com/atakankarsli/sure/internal/provider"
	"github.com/atakankarsli/sure/internal/report"
)

// decide applies the acceptance policy: supported or violated only when the
// chosen label's probability and its margin over the runner-up clear the
// thresholds; everything else is unknown.
func decide(pol contracts.Policy, a provider.Answer) (verdict, reason string, prob float64) {
	choice := provider.ArgMax(provider.Question{Options: assessOptions}, a.Probabilities)
	p := a.Probabilities[choice]
	runner := 0.0
	for _, o := range assessOptions {
		if o.Name != choice {
			runner = math.Max(runner, a.Probabilities[o.Name])
		}
	}
	switch {
	case choice == LabelUnknown:
		return report.VerdictUnknown, "model_unknown", p
	case p < pol.MinSelectedProbability || p-runner < pol.MinProbabilityMargin:
		return report.VerdictUnknown, "low_confidence", p
	case choice == LabelSupported:
		return report.VerdictSupported, "source_supports_requirement", p
	default:
		return report.VerdictViolated, "source_conflicts_with_requirement", p
	}
}

// buildChecks turns plans and provider results into the per-check report, in
// trusted contract order. It returns the operational errors (one per failed
// contract) and how many checks a provider actually decided.
func buildChecks(p *prepared, results map[string]result, base map[string]string) ([]report.Check, []report.Error, int) {
	checks := []report.Check{}
	errs := []report.Error{}
	evaluated := 0
	pol := p.trusted.Config.Policy
	for _, cp := range p.plans {
		r, called := results[cp.c.ID]
		if called && r.err != nil {
			kind := provider.KindOf(r.err)
			errs = append(errs, report.Error{Code: string(kind), Message: r.err.Error(), Contract: cp.c.ID})
		}
		for i, ch := range cp.c.Checks {
			id := cp.c.CheckID(ch)
			out := report.Check{
				ID:            id,
				Contract:      cp.c.ID,
				Title:         ch.Title,
				Severity:      cp.c.Severity,
				Requirement:   ch.Requirement,
				MessageOrigin: "template",
				AnalysisLevel: "text-scoped",
				Regression:    report.RegressionNotAssessed,
				Locations:     []report.Location{},
				NextActions:   []report.Action{},
			}
			switch {
			case !cp.selected:
				out.Verdict, out.ReasonCode = report.VerdictNotChecked, "not_selected"
				out.Message = "Not selected for this run; this is not a pass."
				if p.noChanges {
					out.ReasonCode, out.Message = "no_changes", "No source changed, so nothing was checked; this is not a pass."
				}
			case cp.preset != nil:
				out.Verdict, out.ReasonCode, out.Message = cp.preset.verdict, cp.preset.reason, cp.preset.message
				out.Confidence = report.Ptr(0.0)
				out.NextActions = append(out.NextActions, cp.preset.actions...)
				out.NextActions = append(out.NextActions, recheck(p, cp.c.ID))
			case called && r.err != nil:
				out.Verdict, out.ReasonCode = report.VerdictError, string(provider.KindOf(r.err))
				out.Message = "The provider did not return a usable decision: " + r.err.Error()
				out.NextActions = append(out.NextActions, recheck(p, cp.c.ID))
			case called:
				evaluated++
				fillDecision(&out, p, cp, r.resp, i, pol)
				if v, ok := base[id]; ok {
					out.Regression = v
				}
			default:
				out.Verdict, out.ReasonCode = report.VerdictError, "not_evaluated"
				out.Message = "The check was selected but no decision was obtained."
			}
			checks = append(checks, out)
		}
	}
	return checks, errs, evaluated
}

func fillDecision(out *report.Check, p *prepared, cp contractPlan, resp provider.Response, i int, pol contracts.Policy) {
	a := resp.Answers[cp.req.assessIDs[i]]
	verdict, reason, prob := decide(pol, a)
	out.Verdict, out.ReasonCode = verdict, reason
	out.Confidence = report.Ptr(a.Probabilities[LabelSupported])
	out.Probabilities = &report.Probabilities{
		Supported: a.Probabilities[LabelSupported],
		Violated:  a.Probabilities[LabelViolated],
		Unknown:   a.Probabilities[LabelUnknown],
	}
	out.ProbabilitySource = report.Ptr(p.identity.ProbabilitySource)
	out.ModelConfidence = a.Confidence

	inspected := func() {
		for _, f := range cp.bundle.Files {
			out.Locations = append(out.Locations, report.Location{Path: f.Path, StartLine: 1, EndLine: max(1, f.Lines), Role: report.RoleInspected})
		}
	}
	switch verdict {
	case report.VerdictSupported:
		out.Message = fmt.Sprintf("The supplied source supports this requirement within the recorded scope (p=%.2f).", prob)
		inspected()
	case report.VerdictViolated:
		region, ok := locate(cp, resp, i, pol)
		if ok {
			f := fileByID(cp.bundle, region.FileID)
			out.Locations = append(out.Locations, report.Location{Path: region.Path, StartLine: region.StartLine,
				EndLine: region.EndLine, Role: report.RoleModelSelected, Excerpt: evidence.Excerpt(f, region.StartLine, region.EndLine)})
			out.Message = fmt.Sprintf("The decision model reports a conflict with this requirement (p=%.2f). Inspect the selected source region.", prob)
			out.NextActions = append(out.NextActions, report.Action{Kind: report.ActionInspectSource, Path: region.Path,
				StartLine: region.StartLine, EndLine: region.EndLine})
		} else {
			inspected()
			out.Message = fmt.Sprintf("The decision model reports a conflict with this requirement (p=%.2f); no single region was selected, so inspect the listed files.", prob)
			for _, f := range cp.bundle.Files {
				out.NextActions = append(out.NextActions, report.Action{Kind: report.ActionInspectSource, Path: f.Path,
					StartLine: 1, EndLine: max(1, f.Lines)})
			}
		}
		out.NextActions = append(out.NextActions, recheck(p, cp.c.ID))
	default:
		inspected()
		if reason == "model_unknown" {
			out.Message = fmt.Sprintf("The decision model could not decide from the supplied evidence (p(unknown)=%.2f).", prob)
		} else {
			out.Message = fmt.Sprintf("The decision did not meet the acceptance policy %s (top label at p=%.2f); treated as unknown.", pol.Version, prob)
		}
		out.NextActions = append(out.NextActions,
			report.Action{Kind: report.ActionSupplyContext,
				Description: "Add the files this requirement depends on to the contract's scope or context, or state the requirement more precisely."},
			recheck(p, cp.c.ID))
	}
}

// locate maps the localization answer back to a real region. A missing or
// unconfident answer yields no location rather than a guessed one.
func locate(cp contractPlan, resp provider.Response, i int, pol contracts.Policy) (evidence.Region, bool) {
	if i >= len(cp.req.locateIDs) {
		return evidence.Region{}, false
	}
	a, ok := resp.Answers[cp.req.locateIDs[i]]
	if !ok || a.Choice == RegionNone || a.Probabilities[a.Choice] < pol.MinLocationProbability {
		return evidence.Region{}, false
	}
	for _, r := range cp.bundle.Regions {
		if r.ID == a.Choice {
			return r, true
		}
	}
	return evidence.Region{}, false
}

func fileByID(b evidence.Bundle, id string) evidence.File {
	for _, f := range b.Files {
		if f.ID == id {
			return f
		}
	}
	return evidence.File{}
}

// recheck preserves the repository, trust and provider of this run so the
// next agent checks the same immutable expectation.
func recheck(p *prepared, contractID string) report.Action {
	argv := []string{"sure", contractID, "--repo", p.opts.Repo}
	if p.opts.Draft {
		argv = append(argv, "--draft")
	} else {
		argv = append(argv, "--contracts-ref", p.trustCommit)
	}
	switch {
	case p.opts.Provider == provider.NameReplay:
		argv = append(argv, "--replay", p.opts.ReplayDir)
	case p.opts.Provider != "":
		argv = append(argv, "--provider", p.opts.Provider)
	}
	return report.Action{Kind: report.ActionRecheck, Argv: append(argv, "--json")}
}
