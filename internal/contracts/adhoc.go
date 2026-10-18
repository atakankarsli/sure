package contracts

import (
	"fmt"
	"regexp"
	"strings"
)

// AdhocID names the contract that holds checks an agent passes for one run.
// Ad-hoc checks are never trusted: they are the agent's own expectations.
const AdhocID = "adhoc"

// DefaultExclude is what `sure init` writes and what a repository without
// .sure/ uses for ad-hoc checks.
var DefaultExclude = []string{
	"**/*.test.*", "**/*.spec.*", "**/__tests__/**", "**/*_test.go", "**/test/**", "**/tests/**",
	"**/*Tests/**", "**/testdata/**", "**/dist/**", "**/build/**", "**/.next/**", "**/coverage/**", "**/vendor/**",
}

// DefaultConfig is the configuration of a repository without .sure/.
func DefaultConfig() Config {
	var b strings.Builder
	b.WriteString("schema_version: " + ConfigSchema + "\nsource_roots: [\".\"]\nexclude:\n")
	for _, p := range DefaultExclude {
		fmt.Fprintf(&b, "  - %q\n", p)
	}
	cfg, issues := ParseConfig(ConfigPath, []byte(b.String()))
	if len(issues) > 0 {
		panic(issues[0].String())
	}
	return cfg
}

var (
	anyHeading = regexp.MustCompile(`^#{1,3}[ \t]+(.+?)[ \t]*#*[ \t]*$`)
	bullet     = regexp.MustCompile(`^(?:[-*+]|\d+[.)])[ \t]+(\S.*)$`)
)

// ParseAdhoc reads checks for one run. A file with frontmatter is a full
// contract. Anything else is a light list: each heading starts a check whose
// following text is the requirement (the heading alone when there is none),
// and each top-level bullet outside a heading section is a check.
func ParseAdhoc(name string, data []byte) ([]Contract, []Check, []Issue) {
	text := strings.ReplaceAll(strings.TrimPrefix(string(data), "\ufeff"), "\r\n", "\n")
	if strings.HasPrefix(text, "---\n") {
		c, issues := ParseContract(name, []byte(text))
		return []Contract{c}, nil, issues
	}
	var checks []Check
	var issues []Issue
	var cur *Check
	var buf []string
	fence := ""
	flush := func() {
		if cur == nil {
			return
		}
		cur.Requirement = normalizeRequirement(buf)
		if cur.Requirement == "" {
			cur.Requirement = cur.Title
		}
		checks = append(checks, *cur)
		cur, buf = nil, nil
	}
	for i, line := range strings.Split(text, "\n") {
		n := i + 1
		if m := fencePattern.FindStringSubmatch(line); m != nil {
			if fence == "" {
				fence = m[1]
			} else if m[1][0] == fence[0] && len(m[1]) >= len(fence) {
				fence = ""
			}
			if cur != nil {
				buf = append(buf, line)
			}
			continue
		}
		if fence != "" {
			if cur != nil {
				buf = append(buf, line)
			}
			continue
		}
		if m := anyHeading.FindStringSubmatch(line); m != nil {
			flush()
			title, id := m[1], ""
			if cm := checkHeading.FindStringSubmatch("## " + title); cm != nil {
				id, title = cm[1], cm[2]
			}
			cur = &Check{LocalID: id, Title: title, Line: n}
			continue
		}
		if cur == nil {
			if m := bullet.FindStringSubmatch(line); m != nil && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				t := strings.TrimSpace(m[1])
				checks = append(checks, Check{Title: t, Requirement: t, Line: n})
			}
			continue
		}
		buf = append(buf, line)
	}
	flush()
	if len(checks) == 0 {
		issues = append(issues, Issue{File: name, Code: "no_checks", Message: "no checks found: use '## Title' sections or '- ' bullets"})
	}
	return nil, checks, issues
}

// Expectation turns a sentence into an ad-hoc check.
func Expectation(text string) Check {
	t := strings.Join(strings.Fields(text), " ")
	return Check{Title: t, Requirement: t}
}

// AdhocContract wraps loose checks into the run's ad-hoc contract. Checks
// without an ID get ADHOC-001, ADHOC-002, …; scope may be empty, meaning the
// changed files.
func AdhocContract(checks []Check, scope []string) (Contract, []Issue) {
	c := Contract{
		Path:        "(ad-hoc)",
		ID:          AdhocID,
		Title:       "Checks for this run",
		Severity:    SeverityError,
		Evidence:    EvidenceCode,
		Scope:       append([]string{}, scope...),
		Context:     []string{},
		Assumptions: []string{},
	}
	var issues []Issue
	seen := map[string]bool{}
	for i, ch := range checks {
		if ch.LocalID == "" {
			ch.LocalID = fmt.Sprintf("ADHOC-%03d", i+1)
		}
		if seen[ch.LocalID] {
			issues = append(issues, Issue{File: c.Path, Code: "duplicate_check", Message: "check " + ch.LocalID + " is given twice"})
		}
		seen[ch.LocalID] = true
		if strings.TrimSpace(ch.Requirement) == "" {
			issues = append(issues, Issue{File: c.Path, Code: "empty_check", Message: "an ad-hoc check has no text"})
		}
		c.Checks = append(c.Checks, ch)
	}
	for _, g := range scope {
		if err := validateGlob(g); err != nil {
			issues = append(issues, Issue{File: c.Path, Code: "bad_glob", Message: "--scope: " + err.Error()})
		}
	}
	return c, issues
}

// EscapeGlob quotes a literal path so it matches only itself.
func EscapeGlob(p string) string {
	var b strings.Builder
	for _, r := range p {
		if strings.ContainsRune(`*?[]{}\`, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
