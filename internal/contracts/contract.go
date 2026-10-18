// Package contracts reads Sure's repository config and feature contracts with
// a deliberately restricted grammar, validates them strictly, and compares a
// candidate set against the trusted one.
package contracts

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/atakankarsli/sure/internal/gitview"
)

const (
	ContractSchema   = "sure.contract/v1"
	MaxContractBytes = 256 << 10

	SeverityError   = "error"
	SeverityWarning = "warning"

	EvidenceCode  = "code"
	EvidenceImage = "image"
)

// Issue is one validation problem, located as precisely as the input allows.
type Issue struct {
	File    string `json:"file"`
	Line    int    `json:"line,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (i Issue) String() string {
	if i.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", i.File, i.Line, i.Message)
	}
	return fmt.Sprintf("%s: %s", i.File, i.Message)
}

type Contract struct {
	Path        string   `json:"path"`
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Severity    string   `json:"severity"`
	Evidence    string   `json:"evidence"`
	Scope       []string `json:"scope"`
	Context     []string `json:"context"`
	Assumptions []string `json:"assumptions"`
	Checks      []Check  `json:"checks"`
}

type Check struct {
	LocalID     string `json:"id"`
	Title       string `json:"title"`
	Requirement string `json:"requirement"`
	Line        int    `json:"line"`
}

// CheckID is the canonical "<document-id>/<local-id>" identifier.
func (c Contract) CheckID(ch Check) string { return c.ID + "/" + ch.LocalID }

type frontmatter struct {
	SchemaVersion string   `yaml:"schema_version"`
	ID            string   `yaml:"id"`
	Severity      string   `yaml:"severity"`
	Evidence      string   `yaml:"evidence"`
	Scope         []string `yaml:"scope"`
	Context       []string `yaml:"context"`
	Assumptions   []string `yaml:"assumptions"`
}

var (
	docIDPattern   = regexp.MustCompile(`^[a-z][a-z0-9_-]*(\.[a-z][a-z0-9_-]*)*$`)
	checkHeading   = regexp.MustCompile(`^##[ \t]+([A-Z][A-Z0-9]*-[0-9]+)[ \t]+(?:—|–|-)[ \t]+(\S.*?)[ \t]*#*[ \t]*$`)
	headingPattern = regexp.MustCompile(`^(#{1,6})(?:[ \t]+|$)`)
	fencePattern   = regexp.MustCompile("^[ ]{0,3}(`{3,}|~{3,})")
)

// ParseContract reads one contract file. The grammar:
//
//	--- YAML frontmatter ---
//	# Optional title
//	optional description
//	## <LOCAL-ID> — <title>
//	requirement text …
//
// Every level-2 heading outside fenced code must be a check heading. Fenced
// code is kept verbatim inside the requirement; headings inside it are text.
func ParseContract(file string, data []byte) (Contract, []Issue) {
	var issues []Issue
	add := func(line int, code, format string, args ...any) {
		issues = append(issues, Issue{File: file, Line: line, Code: code, Message: fmt.Sprintf(format, args...)})
	}
	c := Contract{Path: file}
	if len(data) > MaxContractBytes {
		add(0, "contract_too_large", "contract exceeds %d bytes", MaxContractBytes)
		return c, issues
	}
	text := strings.TrimPrefix(string(data), "\ufeff")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")

	if len(lines) == 0 || strings.TrimRight(lines[0], " \t") != "---" {
		add(1, "missing_frontmatter", "contract must start with a '---' frontmatter block")
		return c, issues
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], " \t") == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		add(1, "missing_frontmatter", "frontmatter is not closed with '---'")
		return c, issues
	}
	fmText := strings.Join(lines[1:end], "\n")
	var fm frontmatter
	dec := yaml.NewDecoder(bytes.NewReader([]byte(fmText)))
	dec.KnownFields(true)
	if err := dec.Decode(&fm); err != nil {
		line := yamlLine(err)
		if line > 0 {
			line++
		}
		add(line, "invalid_frontmatter", "%s", cleanYAMLError(err))
		return c, issues
	}
	c.ID, c.Severity, c.Evidence = fm.ID, fm.Severity, fm.Evidence
	c.Scope, c.Context, c.Assumptions = fm.Scope, fm.Context, fm.Assumptions
	if c.Scope == nil {
		c.Scope = []string{}
	}
	if c.Context == nil {
		c.Context = []string{}
	}
	if c.Assumptions == nil {
		c.Assumptions = []string{}
	}

	if fm.SchemaVersion != ContractSchema {
		add(2, "invalid_frontmatter", "schema_version must be %q", ContractSchema)
	}
	if !docIDPattern.MatchString(c.ID) {
		add(2, "invalid_id", "id %q must be lowercase dot-separated words, e.g. checkout.failure", c.ID)
	}
	switch c.Severity {
	case SeverityError, SeverityWarning:
	default:
		add(2, "invalid_frontmatter", "severity must be error or warning")
	}
	switch c.Evidence {
	case EvidenceCode, EvidenceImage:
	default:
		add(2, "invalid_evidence_kind", "evidence must be code or image")
	}
	if len(c.Scope) == 0 {
		add(2, "empty_scope", "scope must list at least one glob")
	}
	seen := map[string]bool{}
	for _, g := range c.Scope {
		if err := validateGlob(g); err != nil {
			add(2, "bad_glob", "scope: %v", err)
		}
		if seen["s:"+g] {
			add(2, "duplicate_entry", "scope lists %q twice", g)
		}
		seen["s:"+g] = true
	}
	for i, p := range c.Context {
		if strings.ContainsAny(p, "*?[{") {
			add(2, "bad_context", "context entry %q must be an exact file path, not a glob", p)
			continue
		}
		cp, err := gitview.CleanRel(p)
		if err != nil {
			add(2, "unsafe_path", "context: %v", err)
			continue
		}
		if seen["c:"+cp] {
			add(2, "duplicate_entry", "context lists %q twice", cp)
		}
		seen["c:"+cp] = true
		c.Context[i] = cp
	}
	for _, a := range c.Assumptions {
		if strings.TrimSpace(a) == "" {
			add(2, "invalid_frontmatter", "assumptions may not be empty")
		}
	}

	parseBody(&c, lines[end+1:], end+2, add)
	if len(c.Checks) == 0 {
		add(end+2, "no_checks", "contract has no '## <ID> — <title>' checks")
	}
	return c, issues
}

func parseBody(c *Contract, lines []string, firstLine int, add func(int, string, string, ...any)) {
	var cur *Check
	var buf []string
	fence := ""
	flush := func() {
		if cur == nil {
			return
		}
		cur.Requirement = normalizeRequirement(buf)
		if cur.Requirement == "" {
			add(cur.Line, "empty_check", "check %s has no requirement text", cur.LocalID)
		}
		c.Checks = append(c.Checks, *cur)
		cur, buf = nil, nil
	}
	ids := map[string]int{}
	for i, line := range lines {
		n := firstLine + i
		if m := fencePattern.FindStringSubmatch(line); m != nil {
			marker := m[1]
			switch {
			case fence == "":
				fence = marker
			case marker[0] == fence[0] && len(marker) >= len(fence) && strings.TrimSpace(strings.TrimLeft(line, " ")[len(marker):]) == "":
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
		h := headingPattern.FindStringSubmatch(line)
		switch {
		case h != nil && len(h[1]) == 1:
			if cur != nil || len(c.Checks) > 0 || c.Title != "" {
				add(n, "invalid_heading", "only one level-1 heading is allowed, before the checks")
				continue
			}
			c.Title = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(line[1:]), "#"))
		case h != nil && len(h[1]) == 2:
			flush()
			m := checkHeading.FindStringSubmatch(line)
			if m == nil {
				add(n, "invalid_heading", "level-2 headings must be checks: '## CHK-001 — Title'")
				continue
			}
			if prev, ok := ids[m[1]]; ok {
				add(n, "duplicate_check", "check %s is already defined on line %d", m[1], prev)
			}
			ids[m[1]] = n
			cur = &Check{LocalID: m[1], Title: m[2], Line: n}
		default:
			if cur != nil {
				buf = append(buf, line)
			}
		}
	}
	if fence != "" {
		add(firstLine+len(lines)-1, "unclosed_fence", "a fenced code block is not closed")
	}
	flush()
	if c.Title == "" {
		c.Title = c.ID
	}
}

// normalizeRequirement joins soft-wrapped lines of a paragraph with single
// spaces, separates paragraphs with a blank line and keeps fenced code
// verbatim.
func normalizeRequirement(lines []string) string {
	var out []string
	var para []string
	fence := ""
	var block []string
	endPara := func() {
		if len(para) > 0 {
			out = append(out, strings.Join(para, " "))
			para = nil
		}
	}
	for _, raw := range lines {
		line := strings.TrimRight(raw, " \t")
		if m := fencePattern.FindStringSubmatch(line); m != nil {
			if fence == "" {
				endPara()
				fence = m[1]
				block = []string{line}
				continue
			}
			if m[1][0] == fence[0] && len(m[1]) >= len(fence) {
				block = append(block, line)
				out = append(out, strings.Join(block, "\n"))
				fence, block = "", nil
				continue
			}
		}
		if fence != "" {
			block = append(block, line)
			continue
		}
		t := strings.TrimSpace(line)
		if t == "" {
			endPara()
			continue
		}
		para = append(para, t)
	}
	if fence != "" {
		out = append(out, strings.Join(block, "\n"))
	}
	endPara()
	return strings.Join(out, "\n\n")
}
