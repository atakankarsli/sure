package report

import (
	"fmt"
	"strings"
	"text/tabwriter"
)

// RunMarkdown renders a run for people. Agents should use JSON.
func RunMarkdown(r Run) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Sure check: %s (exit %d)\n\n", strings.ToUpper(strings.ReplaceAll(r.Gate, "_", " ")), r.ExitCode)
	var meta []string
	meta = append(meta, "status "+r.Status)
	if r.Snapshot != nil {
		s := "scope " + r.Snapshot.Scope
		if r.Snapshot.BaseRef != nil && r.Snapshot.BaseCommit != nil {
			s += fmt.Sprintf(" (base %s → %s)", *r.Snapshot.BaseRef, short(*r.Snapshot.BaseCommit))
		}
		meta = append(meta, s)
	}
	if r.Trust != nil {
		t := "trust " + r.Trust.Mode
		if r.Trust.ContractsCommit != nil {
			t += " @ " + short(*r.Trust.ContractsCommit)
		}
		meta = append(meta, t)
	}
	if r.Provider != nil {
		meta = append(meta, fmt.Sprintf("provider %s %s (%s)", r.Provider.Name, r.Provider.RequestedModel, r.Provider.Mode))
	}
	b.WriteString(strings.Join(meta, " · ") + "\n\n")
	s := r.Summary
	fmt.Fprintf(&b, "%d of %d checks selected, %d evaluated: %d supported, %d violated, %d unknown, %d error, %d not checked.\n",
		s.Selected, s.Total, s.Evaluated, s.Supported, s.Violated, s.Unknown, s.Error, s.NotChecked)

	section := func(title, verdict string, detail bool) {
		var rows []Check
		for _, c := range r.Checks {
			if c.Verdict == verdict {
				rows = append(rows, c)
			}
		}
		if len(rows) == 0 {
			return
		}
		fmt.Fprintf(&b, "\n## %s\n\n", title)
		for _, c := range rows {
			if !detail {
				fmt.Fprintf(&b, "- %s — %s\n", c.ID, c.Title)
				continue
			}
			fmt.Fprintf(&b, "- **%s** — %s (%s)\n", c.ID, c.Title, c.Severity)
			fmt.Fprintf(&b, "  - Requirement: %s\n", oneLine(c.Requirement))
			fmt.Fprintf(&b, "  - %s [%s]\n", c.Message, c.ReasonCode)
			if c.Regression != RegressionNotAssessed {
				fmt.Fprintf(&b, "  - Regression: %s\n", c.Regression)
			}
			for _, l := range c.Locations {
				if l.Role == RoleModelSelected {
					fmt.Fprintf(&b, "  - Selected region: %s:%d-%d\n", l.Path, l.StartLine, l.EndLine)
				}
			}
			for _, a := range c.NextActions {
				switch {
				case a.Kind == ActionRecheck:
					fmt.Fprintf(&b, "  - Recheck: `%s`\n", strings.Join(a.Argv, " "))
				case a.Path != "" && a.StartLine > 0:
					fmt.Fprintf(&b, "  - %s: %s:%d-%d\n", a.Kind, a.Path, a.StartLine, a.EndLine)
				case a.Path != "":
					fmt.Fprintf(&b, "  - %s: %s\n", a.Kind, a.Path)
				case a.Description != "":
					fmt.Fprintf(&b, "  - %s: %s\n", a.Kind, a.Description)
				}
			}
		}
	}
	section("Violated", VerdictViolated, true)
	section("Errors", VerdictError, true)
	section("Unknown", VerdictUnknown, true)
	section("Supported", VerdictSupported, false)
	section("Not checked", VerdictNotChecked, false)

	if len(r.UnmappedChanges) > 0 {
		b.WriteString("\n## Changed source covered by no contract\n\n")
		for _, u := range r.UnmappedChanges {
			fmt.Fprintf(&b, "- %s\n", u)
		}
	}
	if len(r.ContractChanges) > 0 {
		b.WriteString("\n## Contract changes needing review\n\n")
		for _, c := range r.ContractChanges {
			fmt.Fprintf(&b, "- %s: %s\n", c.Kind, c.Detail)
		}
	}
	if len(r.Errors) > 0 {
		b.WriteString("\n## Run errors\n\n")
		for _, e := range r.Errors {
			fmt.Fprintf(&b, "- %s: %s\n", e.Code, e.Message)
		}
	}
	if r.Provider != nil && r.Provider.Usage.Requests > 0 {
		u := r.Provider.Usage
		fmt.Fprintf(&b, "\n%d requests (%d live, %d cached), %d input / %d output tokens reported, ~%d estimated. %d ms total.\n",
			u.Requests, u.LiveRequests, u.CacheHits, u.InputTokens, u.OutputTokens, u.EstimatedInputTokens, r.Timing.TotalMS)
	}
	return b.String()
}

// RunTable renders one row per check.
func RunTable(r Run) string {
	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "CHECK\tVERDICT\tSEVERITY\tREASON")
	for _, c := range r.Checks {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", c.ID, c.Verdict, c.Severity, c.ReasonCode)
	}
	tw.Flush()
	fmt.Fprintf(&b, "gate=%s status=%s exit=%d\n", r.Gate, r.Status, r.ExitCode)
	for _, e := range r.Errors {
		fmt.Fprintf(&b, "error %s: %s\n", e.Code, e.Message)
	}
	return b.String()
}

func PlanMarkdown(p Plan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Sure plan: %s\n\n", p.Status)
	for _, e := range p.Errors {
		fmt.Fprintf(&b, "- error %s: %s\n", e.Code, e.Message)
	}
	if p.Status == "invalid" {
		return b.String()
	}
	s := p.Summary
	fmt.Fprintf(&b, "%d of %d checks selected: %d would be evaluated in %d requests (~%d tokens), %d are already unknown.\n",
		s.Selected, s.Total, s.WouldEvaluate, s.Requests, s.EstimatedTokens, s.KnownUnknown)
	if p.Snapshot != nil {
		fmt.Fprintf(&b, "Source digest %s", short(p.Snapshot.SourceDigest))
		if len(p.Snapshot.ChangedPaths) > 0 {
			fmt.Fprintf(&b, ", %d changed paths", len(p.Snapshot.ChangedPaths))
		}
		b.WriteString(".\n")
	}
	b.WriteString("\n## Contracts\n\n")
	for _, c := range p.Contracts {
		if !c.Selected {
			fmt.Fprintf(&b, "- %s: not selected\n", c.ID)
			continue
		}
		fmt.Fprintf(&b, "- %s (%s): %d files, %d bytes, ~%d tokens, %s\n", c.ID, c.SelectedBy, len(c.Files), c.StateBytes, c.EstimatedTokens, c.Decision)
		for _, m := range c.Missing {
			fmt.Fprintf(&b, "  - missing %s (%s)\n", m.Path, m.Reason)
		}
	}
	if len(p.UnmappedChanges) > 0 {
		b.WriteString("\n## Changed source covered by no contract\n\n")
		for _, u := range p.UnmappedChanges {
			fmt.Fprintf(&b, "- %s\n", u)
		}
	}
	if len(p.ContractChanges) > 0 {
		b.WriteString("\n## Contract changes needing review\n\n")
		for _, c := range p.ContractChanges {
			fmt.Fprintf(&b, "- %s: %s\n", c.Kind, c.Detail)
		}
	}
	return b.String()
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if r := []rune(s); len(r) > 240 {
		return string(r[:240]) + "…"
	}
	return s
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
