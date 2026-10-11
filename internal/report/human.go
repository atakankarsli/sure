package report

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

// Style colors human output; the zero value prints plain text.
type Style struct{ Color bool }

func (s Style) wrap(code, text string) string {
	if !s.Color {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func (s Style) Green(t string) string  { return s.wrap("32", t) }
func (s Style) Red(t string) string    { return s.wrap("31", t) }
func (s Style) Yellow(t string) string { return s.wrap("33", t) }
func (s Style) Dim(t string) string    { return s.wrap("2", t) }
func (s Style) Bold(t string) string   { return s.wrap("1", t) }

// Human renders a run as a short report: one line per selected contract with
// its confidence, the failing checks beneath it, then one summary line.
func Human(r Run, st Style) string {
	var b strings.Builder
	if r.Snapshot == nil {
		for _, e := range r.Errors {
			fmt.Fprintf(&b, "%s %s\n", st.Red("✗"), e.Message)
			if hint := hintFor(e.Code); hint != "" {
				fmt.Fprintf(&b, "  %s\n", st.Dim(hint))
			}
		}
		return b.String()
	}
	b.WriteString(st.Bold("sure") + st.Dim(" · "+headline(r)) + "\n\n")

	byContract := map[string][]Check{}
	for _, c := range r.Checks {
		byContract[c.Contract] = append(byContract[c.Contract], c)
	}
	notSelected := 0
	tw := tabwriter.NewWriter(&b, 0, 2, 3, ' ', 0)
	var details []string
	for _, ce := range r.Contracts {
		if !ce.Selected {
			notSelected++
			continue
		}
		mark, pct := contractMark(ce, byContract[ce.ID], st)
		fmt.Fprintf(tw, "  %s %s\t%s\n", mark, ce.ID, pct)
		tw.Flush()
		failing := false
		for _, c := range byContract[ce.ID] {
			failing = failing || c.Verdict == VerdictViolated || c.Verdict == VerdictUnknown
		}
		for _, c := range byContract[ce.ID] {
			if c.Verdict == VerdictSupported || c.Verdict == VerdictError {
				continue
			}
			pct := "   "
			if c.Confidence != nil {
				pct = fmt.Sprintf("%2d%%", Percent(*c.Confidence))
			}
			fmt.Fprintf(&b, "      %s %s  %s %s\n", st.Dim(localID(c.ID)), pct, c.Title, st.Dim("· "+checkReason(c, r.Trust)))
			for _, l := range c.Locations {
				if l.Role == RoleModelSelected {
					fmt.Fprintf(&b, "                   %s\n", st.Dim(fmt.Sprintf("→ %s:%d-%d", l.Path, l.StartLine, l.EndLine)))
				}
			}
		}
		if failing && len(ce.Files) > 0 {
			paths := make([]string, len(ce.Files))
			for i, f := range ce.Files {
				paths[i] = f.Path
			}
			fmt.Fprintf(&b, "      %s\n", st.Dim("read  "+list(paths, 4)))
		}
	}
	tw.Flush()
	if notSelected > 0 {
		fmt.Fprintf(&b, "  %s\n", st.Dim(fmt.Sprintf("· %d %s not affected", notSelected, plural(notSelected, "contract", "contracts"))))
	}
	if len(r.UnmappedChanges) > 0 {
		details = append(details, st.Yellow("⚠")+fmt.Sprintf(" %d changed %s covered by no contract: %s",
			len(r.UnmappedChanges), plural(len(r.UnmappedChanges), "file is", "files are"), list(r.UnmappedChanges, 3)))
	}
	if len(r.ContractChanges) > 0 {
		details = append(details, st.Yellow("⚠")+fmt.Sprintf(" .sure/ differs from the trusted base (%d %s); the trusted contracts were checked",
			len(r.ContractChanges), plural(len(r.ContractChanges), "change", "changes")))
	}
	for _, e := range r.Errors {
		details = append(details, st.Red("✗")+" "+e.Message)
	}
	if len(details) > 0 {
		b.WriteString("\n")
		for _, d := range details {
			b.WriteString("  " + d + "\n")
		}
	}
	b.WriteString("\n" + footer(r, st) + "\n")
	if next := nextStep(r); next != "" {
		b.WriteString(st.Dim(next) + "\n")
	}
	return b.String()
}

func headline(r Run) string {
	s := r.Snapshot
	selected := 0
	for _, c := range r.Contracts {
		if c.Selected {
			selected++
		}
	}
	var parts []string
	base := ""
	if s.BaseRef != nil {
		base = *s.BaseRef
	}
	switch s.Selection {
	case "changed":
		parts = append(parts, fmt.Sprintf("%d of %d %s", selected, len(r.Contracts), plural(len(r.Contracts), "contract", "contracts")),
			fmt.Sprintf("%d changed %s vs %s", len(s.ChangedPaths), plural(len(s.ChangedPaths), "file", "files"), base))
	case "all_no_changes":
		parts = append(parts, fmt.Sprintf("all %d %s", selected, plural(selected, "contract", "contracts")), "no changes vs "+base)
	case "contract":
		parts = append(parts, fmt.Sprintf("%d %s", selected, plural(selected, "contract", "contracts")))
	default:
		parts = append(parts, fmt.Sprintf("all %d %s", selected, plural(selected, "contract", "contracts")))
	}
	if r.Provider != nil {
		p := r.Provider.Name + "/" + r.Provider.RequestedModel
		if r.Provider.Mode == "cache" || r.Provider.Mode == "replay" {
			p += " (" + r.Provider.Mode + ")"
		}
		parts = append(parts, p)
	}
	if r.Trust != nil && r.Trust.Mode == "draft" {
		parts = append(parts, "draft contracts")
	}
	return strings.Join(parts, " · ")
}

func contractMark(ce ContractEvidence, checks []Check, st Style) (string, string) {
	worst := VerdictSupported
	for _, c := range checks {
		switch {
		case c.Verdict == VerdictError:
			worst = VerdictError
		case c.Verdict == VerdictViolated && worst != VerdictError:
			worst = VerdictViolated
		case c.Verdict == VerdictUnknown && worst == VerdictSupported:
			worst = VerdictUnknown
		}
	}
	pct := "—"
	if ce.Confidence != nil {
		pct = fmt.Sprintf("%3d%%", Percent(*ce.Confidence))
	}
	switch worst {
	case VerdictSupported:
		return st.Green("✓"), st.Green(pct)
	case VerdictViolated:
		return st.Red("✗"), st.Red(pct)
	case VerdictError:
		return st.Red("!"), st.Red(pct)
	default:
		return st.Yellow("○"), st.Yellow(pct)
	}
}

func checkReason(c Check, t *Trust) string {
	switch c.ReasonCode {
	case "source_conflicts_with_requirement":
		return "violated"
	case "low_confidence":
		if t != nil && t.MinProbability > 0 {
			return fmt.Sprintf("not sure enough (needs %d%%)", Percent(t.MinProbability))
		}
		return "not sure enough"
	case "model_unknown":
		return "the model could not decide from these files"
	case "model_refused":
		return "the model declined to answer"
	case "missing_context":
		return "missing context file"
	case "empty_scope":
		return "scope matches no files"
	case "evidence_budget_exceeded":
		return "too much code for one request; narrow the scope"
	case "missing_visual_evidence", "stale_evidence", "artifact_hash_mismatch", "unsupported_evidence_kind", "unsafe_artifact_path", "invalid_visual_manifest":
		return "needs visual evidence (" + strings.ReplaceAll(c.ReasonCode, "_", " ") + ")"
	}
	if c.Verdict == VerdictError {
		return "provider error: " + c.ReasonCode
	}
	return c.ReasonCode
}

func footer(r Run, st Style) string {
	s := r.Summary
	var parts []string
	switch {
	case r.Confidence != nil:
		text := fmt.Sprintf("%d%% sure", r.Confidence.Percent)
		switch r.Gate {
		case GateClear:
			text = st.Green(st.Bold(text))
		case GateViolated, GateError:
			text = st.Red(st.Bold(text))
		default:
			text = st.Yellow(st.Bold(text))
		}
		parts = append(parts, text)
	case r.Status == StatusNoApplicable:
		parts = append(parts, st.Bold("nothing to check"))
	case r.Status == StatusNoChanges:
		parts = append(parts, st.Bold("no changes"))
	default:
		parts = append(parts, st.Red(st.Bold("not sure")))
	}
	if s.Violated > 0 {
		parts = append(parts, fmt.Sprintf("%d violated", s.Violated))
	}
	if s.Unknown > 0 {
		parts = append(parts, fmt.Sprintf("%d not sure", s.Unknown))
	}
	if s.Error > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", s.Error))
	}
	if s.Supported > 0 {
		parts = append(parts, fmt.Sprintf("%d passed", s.Supported))
	}
	parts = append(parts, (time.Duration(r.Timing.TotalMS) * time.Millisecond).Round(100*time.Millisecond).String())
	return strings.Join(parts, st.Dim(" · "))
}

func nextStep(r Run) string {
	var argv []string
	for _, c := range r.Checks {
		for _, a := range c.NextActions {
			if a.Kind == ActionRecheck && (c.Verdict == VerdictViolated || c.Verdict == VerdictError) {
				argv = a.Argv
			}
		}
	}
	switch {
	case r.Summary.Violated > 0 && argv != nil:
		return "Next: fix the violated behavior, then run `" + shellJoin(trimRecheck(argv)) + "`"
	case r.Summary.Error > 0:
		return "Next: the provider failed; run `sure doctor --live`, then retry"
	case r.Summary.Unknown > 0 && adhocUnknown(r):
		return "Next: pass --scope <glob> so your checks read the files the behavior lives in, then rerun"
	case r.Summary.Unknown > 0:
		return "Next: add the files these checks depend on to the contract's scope or context, then rerun `sure`"
	case len(r.UnmappedChanges) > 0:
		return "Next: cover the changed files with a contract, or exclude them in .sure/config.yaml"
	}
	return ""
}

// trimRecheck drops the trust pin and JSON flag that agents need but people
// typing the command do not.
func trimRecheck(argv []string) []string {
	var out []string
	for i := 0; i < len(argv); i++ {
		switch argv[i] {
		case "--json":
			continue
		case "--contracts-ref", "--repo":
			if argv[i] == "--repo" && i+1 < len(argv) && argv[i+1] != "." {
				out = append(out, argv[i], argv[i+1])
			}
			i++
			continue
		}
		out = append(out, argv[i])
	}
	return out
}

func hintFor(code string) string {
	switch code {
	case "not_initialized":
		return "Run `sure init`, write a contract in .sure/contracts/, then commit .sure/."
	case "no_trusted_base", "no_trusted_contracts":
		return "Commit .sure/ so checks have a trusted base, or pass --draft to use the working tree."
	case "missing_api_key":
		return "Export the key in your shell; `sure doctor` shows which variable the provider reads."
	case "invalid_contracts":
		return "Run `sure validate` for the full list."
	}
	return ""
}

// PlanHuman renders a dry run: what would be selected and sent.
func PlanHuman(p Plan, st Style) string {
	var b strings.Builder
	if p.Snapshot == nil {
		for _, e := range p.Errors {
			fmt.Fprintf(&b, "%s %s\n", st.Red("✗"), e.Message)
			if hint := hintFor(e.Code); hint != "" {
				fmt.Fprintf(&b, "  %s\n", st.Dim(hint))
			}
		}
		return b.String()
	}
	r := Run{Snapshot: p.Snapshot, Trust: p.Trust, Provider: p.Provider, Contracts: p.Contracts}
	b.WriteString(st.Bold("sure --dry-run") + st.Dim(" · "+headline(r)) + "\n\n")
	planned := map[string][]PlannedCheck{}
	for _, c := range p.Outcomes {
		id, _, _ := strings.Cut(c.ID, "/")
		planned[id] = append(planned[id], c)
	}
	tw := tabwriter.NewWriter(&b, 0, 2, 3, ' ', 0)
	for _, c := range p.Contracts {
		if !c.Selected {
			continue
		}
		if strings.HasPrefix(c.Decision, "not_sent:") || c.StateBytes == 0 {
			reason := strings.TrimPrefix(c.Decision, "not_sent:")
			for _, m := range c.Missing {
				reason += " " + m.Path
			}
			fmt.Fprintf(tw, "  %s %s\t%s\n", st.Yellow("○"), c.ID, st.Yellow("won't be sent: "+strings.ReplaceAll(reason, "_", " ")))
			continue
		}
		fmt.Fprintf(tw, "  %s %s\t%d checks · %d files · %s · ~%s tokens\n", st.Dim("·"), c.ID,
			len(planned[c.ID]), len(c.Files), kb(c.StateBytes), thousands(c.EstimatedTokens))
	}
	tw.Flush()
	if len(p.UnmappedChanges) > 0 {
		fmt.Fprintf(&b, "\n  %s %d changed %s covered by no contract: %s\n", st.Yellow("⚠"), len(p.UnmappedChanges),
			plural(len(p.UnmappedChanges), "file is", "files are"), list(p.UnmappedChanges, 3))
	}
	fmt.Fprintf(&b, "\n%s · ~%s tokens · no model call made\n",
		fmt.Sprintf("%d %s", p.Summary.Requests, plural(p.Summary.Requests, "request", "requests")), thousands(p.Summary.EstimatedTokens))
	return b.String()
}

func localID(id string) string {
	_, l, ok := strings.Cut(id, "/")
	if !ok {
		return id
	}
	return l
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func list(items []string, max int) string {
	s := append([]string(nil), items...)
	sort.Strings(s)
	if len(s) > max {
		return strings.Join(s[:max], ", ") + fmt.Sprintf(" and %d more", len(s)-max)
	}
	return strings.Join(s, ", ")
}

func kb(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.1f KB", float64(n)/1024)
}

func thousands(n int) string {
	if n < 1000 {
		return fmt.Sprint(n)
	}
	return fmt.Sprintf("%.1fk", float64(n)/1000)
}

func adhocUnknown(r Run) bool {
	adhoc := map[string]bool{}
	for _, c := range r.Contracts {
		adhoc[c.ID] = c.Adhoc
	}
	for _, c := range r.Checks {
		if c.Verdict == VerdictUnknown && adhoc[c.Contract] {
			return true
		}
	}
	return false
}

// shellJoin quotes arguments so the printed command can be pasted as is.
func shellJoin(argv []string) string {
	out := make([]string, len(argv))
	for i, a := range argv {
		if a != "" && !strings.ContainsAny(a, " \t\n'\"$`\\*?[]{}()<>|&;#~") {
			out[i] = a
			continue
		}
		out[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(out, " ")
}
