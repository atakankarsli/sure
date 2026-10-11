package contracts

import (
	"context"
	"strings"
	"testing"

	"github.com/atakankarsli/sure/internal/gitview"
	"github.com/atakankarsli/sure/internal/testutil"
)

const checkoutContract = `---
schema_version: sure.contract/v1
id: checkout.failure
severity: error
evidence: code
scope:
  - src/features/checkout/**
context:
  - src/domain/orders.ts
  - src/domain/cart.ts
---
# Checkout failure

Intro text is not a check.

## CHK-001 — Preserve the cart
Given a non-empty cart, when order submission resolves with an error,
existing cart items and quantities must remain unchanged.

## CHK-002 — Do not show success
When order submission fails, the application must not transition to
the order-success state.

` + "```ts\n## not a heading inside a fence\nconst x = 1;\n```" + `

## CHK-003 - Allow another attempt
After a failed submission settles, submitting must become false so
the customer can retry.
`

func TestParseContractValid(t *testing.T) {
	c, issues := ParseContract("c.md", []byte(checkoutContract))
	if len(issues) != 0 {
		t.Fatalf("issues: %v", issues)
	}
	if c.ID != "checkout.failure" || c.Title != "Checkout failure" || c.Severity != "error" || c.Evidence != "code" {
		t.Fatalf("header: %+v", c)
	}
	if len(c.Checks) != 3 {
		t.Fatalf("checks: %+v", c.Checks)
	}
	want := "Given a non-empty cart, when order submission resolves with an error, existing cart items and quantities must remain unchanged."
	if c.Checks[0].Requirement != want {
		t.Fatalf("requirement 1:\n%q\nwant\n%q", c.Checks[0].Requirement, want)
	}
	if !strings.Contains(c.Checks[1].Requirement, "## not a heading inside a fence") ||
		!strings.Contains(c.Checks[1].Requirement, "\n\n```ts\n") {
		t.Fatalf("fenced code not kept verbatim: %q", c.Checks[1].Requirement)
	}
	if c.Checks[2].LocalID != "CHK-003" || c.Checks[2].Title != "Allow another attempt" || c.Checks[2].Line != 29 {
		t.Fatalf("check 3: %+v", c.Checks[2])
	}
	if c.CheckID(c.Checks[0]) != "checkout.failure/CHK-001" {
		t.Fatal(c.CheckID(c.Checks[0]))
	}
}

func TestParseContractNormalizesCRLFAndBOM(t *testing.T) {
	crlf := "\ufeff" + strings.ReplaceAll(checkoutContract, "\n", "\r\n")
	a, iss := ParseContract("a.md", []byte(crlf))
	b, _ := ParseContract("a.md", []byte(checkoutContract))
	if len(iss) != 0 || a.Digest() != b.Digest() {
		t.Fatalf("CRLF/BOM changed meaning: %v", iss)
	}
}

func TestParseContractRejects(t *testing.T) {
	head := "---\nschema_version: sure.contract/v1\nid: a.b\nseverity: error\nevidence: code\nscope:\n  - src/**\n"
	body := "---\n## CHK-001 — T\nText.\n"
	cases := map[string]struct{ doc, code string }{
		"unknown key":       {head + "owner: me\n" + body, "invalid_frontmatter"},
		"duplicate key":     {head + "severity: warning\n" + body, "invalid_frontmatter"},
		"duplicate check":   {head + "---\n## CHK-001 — A\nx\n## CHK-001 — B\ny\n", "duplicate_check"},
		"empty check":       {head + "---\n## CHK-001 — A\n\n## CHK-002 — B\ny\n", "empty_check"},
		"no checks":         {head + "---\n# Title\nonly prose\n", "no_checks"},
		"bad heading":       {head + "---\n## Not a check\nx\n", "invalid_heading"},
		"second h1":         {head + "---\n# A\n## CHK-001 — T\nx\n# B\n", "invalid_heading"},
		"bad glob":          {strings.Replace(head, "src/**", "src/[", 1) + body, "bad_glob"},
		"escaping glob":     {strings.Replace(head, "src/**", "../x/**", 1) + body, "bad_glob"},
		"unsafe context":    {head + "context:\n  - ../secret.ts\n" + body, "unsafe_path"},
		"glob context":      {head + "context:\n  - src/*.ts\n" + body, "bad_context"},
		"bad evidence":      {strings.Replace(head, "evidence: code", "evidence: video", 1) + body, "invalid_evidence_kind"},
		"bad id":            {strings.Replace(head, "id: a.b", "id: Checkout", 1) + body, "invalid_id"},
		"bad severity":      {strings.Replace(head, "severity: error", "severity: low", 1) + body, "invalid_frontmatter"},
		"wrong schema":      {strings.Replace(head, "contract/v1", "contract/v9", 1) + body, "invalid_frontmatter"},
		"no frontmatter":    {"# Title\n## CHK-001 — T\nx\n", "missing_frontmatter"},
		"unclosed fm":       {head + "## CHK-001 — T\n", "missing_frontmatter"},
		"unclosed fence":    {head + "---\n## CHK-001 — T\n```\ncode\n", "unclosed_fence"},
		"empty scope":       {strings.Replace(head, "scope:\n  - src/**\n", "scope: []\n", 1) + body, "empty_scope"},
		"duplicate scope":   {strings.Replace(head, "  - src/**\n", "  - src/**\n  - src/**\n", 1) + body, "duplicate_entry"},
		"empty assumptions": {head + "assumptions:\n  - \"  \"\n" + body, "invalid_frontmatter"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, issues := ParseContract("x.md", []byte(tc.doc))
			for _, is := range issues {
				if is.Code == tc.code {
					return
				}
			}
			t.Fatalf("want issue %s, got %v", tc.code, issues)
		})
	}
}

const minimalConfig = "schema_version: sure.config/v1\nsource_roots: [src]\n"

func TestParseConfigDefaults(t *testing.T) {
	cfg, issues := ParseConfig(ConfigPath, []byte(minimalConfig))
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	if cfg.Contracts != DefaultDir || cfg.Provider.Name != "jev" || cfg.Provider.Transport != "gateway" ||
		cfg.Limits.Concurrency != 4 || cfg.Policy.MinSelectedProbability != 0.90 || cfg.Policy.MinProbabilityMargin != 0.20 {
		t.Fatalf("defaults: %+v", cfg)
	}
	zero, issues := ParseConfig(ConfigPath, []byte(minimalConfig+"policy:\n  min_probability_margin: 0\n"))
	if len(issues) != 0 || zero.Policy.MinProbabilityMargin != 0 {
		t.Fatalf("explicit zero margin lost: %+v %v", zero.Policy, issues)
	}
}

func TestParseConfigRejects(t *testing.T) {
	cases := map[string]struct{ doc, code string }{
		"endpoint":        {minimalConfig + "provider:\n  name: jev\n  base_url: https://evil.example\n", "untrusted_provider_setting"},
		"key env":         {minimalConfig + "provider:\n  key_env: AWS_SECRET_ACCESS_KEY\n", "untrusted_provider_setting"},
		"unknown key":     {minimalConfig + "telemetry: true\n", "invalid_config"},
		"duplicate key":   {minimalConfig + "source_roots: [lib]\n", "invalid_config"},
		"no roots":        {"schema_version: sure.config/v1\n", "invalid_config"},
		"unsafe root":     {"schema_version: sure.config/v1\nsource_roots: [../x]\n", "unsafe_path"},
		"bad provider":    {minimalConfig + "provider:\n  name: openai\n", "invalid_config"},
		"bad transport":   {minimalConfig + "provider:\n  transport: proxy\n", "invalid_config"},
		"jev effort":      {minimalConfig + "provider:\n  effort: low\n", "invalid_config"},
		"bad effort":      {minimalConfig + "provider:\n  name: anthropic\n  effort: turbo\n", "invalid_config"},
		"bad model":       {minimalConfig + "provider:\n  model: \"x y\"\n", "invalid_config"},
		"concurrency":     {minimalConfig + "limits:\n  concurrency: 0\n", "invalid_config"},
		"selected prob":   {minimalConfig + "policy:\n  min_selected_probability: 0.3\n", "invalid_config"},
		"request < state": {minimalConfig + "limits:\n  max_state_bytes: 50000\n  max_request_bytes: 40000\n", "invalid_config"},
		"bad exclude":     {minimalConfig + "exclude: [\"/abs/**\"]\n", "bad_glob"},
		"wrong schema":    {"schema_version: sure.config/v2\nsource_roots: [src]\n", "invalid_config"},
		"not yaml":        {"schema_version: [\n", "invalid_yaml"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, issues := ParseConfig(ConfigPath, []byte(tc.doc))
			for _, is := range issues {
				if is.Code == tc.code {
					return
				}
			}
			t.Fatalf("want %s, got %v", tc.code, issues)
		})
	}
}

func TestConfigAllowed(t *testing.T) {
	cfg, _ := ParseConfig(ConfigPath, []byte(minimalConfig+"exclude: [\"**/*.test.*\"]\n"))
	cases := map[string]bool{
		"src/a.ts":                  true,
		"src/a.test.ts":             false,
		"lib/a.ts":                  false,
		"src/.env":                  false,
		"src/.env.local":            false,
		"src/node_modules/x/i.js":   false,
		"src/keys/server.pem":       false,
		"srcx/a.ts":                 false,
		"src/deep/nested/module.ts": true,
	}
	for p, want := range cases {
		if got := cfg.Allowed(p); got != want {
			t.Errorf("Allowed(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestLoadAndCrossCheck(t *testing.T) {
	dir := testutil.NewRepo(t)
	testutil.WriteFiles(t, dir, map[string]string{
		ConfigPath:                                minimalConfig + "exclude: [\"src/generated/**\"]\n",
		".sure/contracts/checkout.md":             checkoutContract,
		".sure/contracts/dup.md":                  strings.Replace(checkoutContract, "Checkout failure", "Again", 1),
		".sure/contracts/notes.txt":               "ignored",
		".sure/contracts/nested/excluded.md":      strings.Replace(strings.Replace(checkoutContract, "checkout.failure", "gen.x", 1), "src/domain/cart.ts", "src/generated/c.ts", 1),
		".sure/contracts/nested/outside-scope.md": strings.Replace(strings.Replace(checkoutContract, "checkout.failure", "lib.x", 1), "src/features/checkout/**", "lib/**", 1),
	})
	r, err := gitview.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	set, err := Load(context.Background(), r.Worktree())
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]bool{}
	for _, is := range set.Issues {
		codes[is.Code] = true
	}
	for _, want := range []string{"duplicate_id", "contradictory_config"} {
		if !codes[want] {
			t.Errorf("missing issue %s in %v", want, set.Issues)
		}
	}
	if len(set.Contracts) != 3 || set.Contracts[0].ID != "checkout.failure" {
		t.Fatalf("contracts: %d", len(set.Contracts))
	}
	if !set.Contracts[0].Covers("src/features/checkout/finish.ts") || !set.Contracts[0].Covers("src/domain/cart.ts") ||
		set.Contracts[0].Covers("src/domain/other.ts") {
		t.Fatal("Covers mismatch")
	}

	empty := t.TempDir()
	r2, _ := gitview.Open(context.Background(), empty)
	if _, err := Load(context.Background(), r2.Worktree()); err != ErrNotInitialized {
		t.Fatalf("want ErrNotInitialized, got %v", err)
	}
}

func mustSet(t *testing.T, config string, docs ...string) *Set {
	t.Helper()
	cfg, issues := ParseConfig(ConfigPath, []byte(config))
	s := &Set{Config: cfg, Issues: issues}
	for i, d := range docs {
		c, iss := ParseContract(".sure/contracts/c"+string(rune('a'+i))+".md", []byte(d))
		s.Issues = append(s.Issues, iss...)
		s.Contracts = append(s.Contracts, c)
	}
	return s
}

func TestDiffTrustBoundaryCases(t *testing.T) {
	trusted := mustSet(t, minimalConfig, checkoutContract)
	kinds := func(ch []Change) map[string]bool {
		m := map[string]bool{}
		for _, c := range ch {
			m[c.Kind+"/"+c.Field] = true
		}
		return m
	}
	if d := Diff(trusted, mustSet(t, minimalConfig, checkoutContract), nil); len(d) != 0 {
		t.Fatalf("identical sets differ: %v", d)
	}

	removed := strings.Replace(checkoutContract, "## CHK-002 — Do not show success\nWhen order submission fails, the application must not transition to\nthe order-success state.\n", "", 1)
	if d := kinds(Diff(trusted, mustSet(t, minimalConfig, removed), nil)); !d["check_removed/"] {
		t.Errorf("C037 deleted check not reported: %v", d)
	}
	narrowed := strings.Replace(checkoutContract, "src/features/checkout/**", "src/features/checkout/ui/**", 1)
	if d := kinds(Diff(trusted, mustSet(t, minimalConfig, narrowed), nil)); !d["contract_changed/scope"] {
		t.Errorf("C038 narrowed scope not reported: %v", d)
	}
	lowered := strings.Replace(checkoutContract, "severity: error", "severity: warning", 1)
	if d := kinds(Diff(trusted, mustSet(t, minimalConfig+"policy:\n  min_selected_probability: 0.6\n", lowered), nil)); !d["contract_changed/severity"] || !d["config_changed/policy.min_selected_probability"] {
		t.Errorf("C039 lowered gate not reported: %v", d)
	}
	redirect := mustSet(t, minimalConfig+"provider:\n  base_url: https://evil.example\n", checkoutContract)
	if d := kinds(Diff(trusted, redirect, nil)); !d["config_invalid/"+ConfigPath] {
		t.Errorf("C040 endpoint injection not reported: %v", d)
	}
	reworded := strings.Replace(checkoutContract, "must remain unchanged", "should mostly remain", 1)
	if d := kinds(Diff(trusted, mustSet(t, minimalConfig, reworded), nil)); !d["check_changed/requirement"] {
		t.Errorf("reworded requirement not reported: %v", d)
	}
	added := checkoutContract + "\n## CHK-004 — New\nSomething new.\n"
	if d := kinds(Diff(trusted, mustSet(t, minimalConfig, added), nil)); !d["check_added/"] {
		t.Errorf("added check not reported: %v", d)
	}
	if d := kinds(Diff(trusted, mustSet(t, minimalConfig), nil)); !d["contract_removed/"] {
		t.Errorf("removed contract not reported: %v", d)
	}
	if d := Diff(trusted, nil, ErrNotInitialized); len(d) != 1 || d[0].Kind != ChangeConfigInvalid {
		t.Errorf("missing candidate config: %v", d)
	}
}
