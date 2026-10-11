package engine_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/atakankarsli/sure/internal/engine"
	"github.com/atakankarsli/sure/internal/provider"
	"github.com/atakankarsli/sure/internal/provider/fake"
	"github.com/atakankarsli/sure/internal/report"
	"github.com/atakankarsli/sure/internal/testutil"
)

const configYAML = `schema_version: sure.config/v1
source_roots: [src]
exclude: ["**/*.test.*"]
provider:
  name: jev
`

const checkoutMD = `---
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

## CHK-001 — Preserve the cart
Given a non-empty cart, when order submission resolves with an error,
existing cart items and quantities must remain unchanged.

## CHK-002 — Do not show success
When order submission fails, the application must not transition to
the order-success state.

## CHK-003 — Allow another attempt
After a failed submission settles, submitting must become false so
the customer can retry.
`

const cartMD = `---
schema_version: sure.contract/v1
id: cart.lines
severity: error
evidence: code
scope:
  - src/domain/cart.ts
---
# Cart lines

## CART-001 — Keep other lines
Removing a line preserves other lines and their quantities.

## CART-002 — Increment existing SKU
Adding an existing SKU increments its quantity.
`

const finishGood = `import type { Cart } from "../../domain/cart";
import type { OrderResult } from "../../domain/orders";

export function finishCheckout(result: OrderResult, cart: Cart, ui: { submitting: boolean; success: boolean }) {
  if (!result.ok) {
    ui.submitting = false;
    return;
  }
  cart.items = [];
  ui.submitting = false;
  ui.success = true;
}
`

const bugMarker = "if (!result.ok) {\n    cart.items = [];"

var finishBad = strings.Replace(finishGood, "if (!result.ok) {\n", "if (!result.ok) {\n    cart.items = [];\n", 1)

func baseFiles() map[string]string {
	return map[string]string{
		".sure/config.yaml":                    configYAML,
		".sure/contracts/checkout.md":          checkoutMD,
		".sure/contracts/cart.md":              cartMD,
		"src/features/checkout/finish.ts":      finishGood,
		"src/domain/cart.ts":                   "export type Cart = { items: { sku: string; quantity: number }[] };\n",
		"src/domain/orders.ts":                 "export type OrderResult = { ok: true } | { ok: false; error: string };\n",
		"src/features/checkout/finish.test.ts": "expected verdict: violated\n",
	}
}

func committedRepo(t *testing.T, files map[string]string) (string, string) {
	t.Helper()
	dir := testutil.NewRepo(t)
	testutil.WriteFiles(t, dir, files)
	return dir, testutil.Commit(t, dir, "baseline")
}

// decider violates the cart requirement when the bug is present and locates
// the finishCheckout region; everything else is supported.
func decider(q provider.Question, req provider.Request) provider.Answer {
	buggy := strings.Contains(string(req.State), jsonEscape(bugMarker))
	var instr map[string]string
	_ = json.Unmarshal(q.Instructions, &instr)
	cartReq := strings.Contains(instr["requirement"], "cart items")
	if strings.HasSuffix(q.ID, "_locate") {
		if buggy && cartReq {
			var st struct {
				Regions []struct {
					ID         string `json:"id"`
					StartsWith string `json:"starts_with"`
				} `json:"regions"`
			}
			_ = json.Unmarshal(req.State, &st)
			for _, r := range st.Regions {
				if strings.Contains(r.StartsWith, "export function finishCheckout") {
					return fake.Answer(q, r.ID, 0.9)
				}
			}
		}
		return fake.Answer(q, "NONE", 0.9)
	}
	if buggy && cartReq {
		return fake.Answer(q, "violated", 0.95)
	}
	return fake.Answer(q, "supported", 0.96)
}

func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	return strings.Trim(string(b), `"`)
}

type factory struct{ p provider.Provider }

func (f factory) Identity(engine.ProviderSpec) (provider.Identity, error) { return f.p.Identity(), nil }
func (f factory) New(engine.ProviderSpec) (provider.Provider, error)      { return f.p, nil }

func deps(p provider.Provider, env map[string]string) engine.Deps {
	var mu sync.Mutex
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	return engine.Deps{
		Getenv: func(k string) string { return env[k] },
		Now: func() time.Time {
			mu.Lock()
			defer mu.Unlock()
			now = now.Add(time.Millisecond)
			return now
		},
		NewRunID: func() string { return "run_test" },
		Factory:  factory{p},
		Stderr:   io.Discard,
	}
}

func check(t *testing.T, dir string, p provider.Provider, opts engine.Options) report.Run {
	t.Helper()
	opts.Repo = dir
	if opts.Mode == "" {
		opts.Mode = engine.ModeAll
	}
	opts.NoCache = true
	return engine.Check(context.Background(), opts, deps(p, nil))
}

func verdicts(r report.Run) map[string]string {
	m := map[string]string{}
	for _, c := range r.Checks {
		m[c.ID] = c.Verdict + "/" + c.ReasonCode
	}
	return m
}

func TestAllSupportedIsClear(t *testing.T) {
	dir, head := committedRepo(t, baseFiles())
	f := &fake.Fake{Decide: decider}
	r := check(t, dir, f, engine.Options{})
	if r.ExitCode != 0 || r.Gate != report.GateClear || r.Status != report.StatusCompleted {
		t.Fatalf("got %s/%s/%d errors=%v", r.Status, r.Gate, r.ExitCode, r.Errors)
	}
	s := r.Summary
	if s.Total != 5 || s.Selected != 5 || s.Evaluated != 5 || s.Supported != 5 {
		t.Fatalf("summary %+v", s)
	}
	if f.Calls() != 2 {
		t.Fatalf("want one request per contract, got %d", f.Calls())
	}
	if r.Trust.Mode != "protected" || *r.Trust.ContractsCommit != head || r.Snapshot.ApplicationExecuted {
		t.Fatalf("trust/snapshot %+v %+v", r.Trust, r.Snapshot)
	}
	for _, req := range f.Requests() {
		if strings.Contains(string(req.State), "expected verdict") {
			t.Fatal("test file leaked into model evidence")
		}
	}
	for _, c := range r.Checks {
		if c.MessageOrigin != "template" || c.Probabilities == nil || len(c.Locations) == 0 || c.Locations[0].Role != report.RoleInspected {
			t.Fatalf("check %+v", c)
		}
	}
}

func TestViolationIsLocatedAndExitsOne(t *testing.T) {
	files := baseFiles()
	dir, head := committedRepo(t, files)
	testutil.Write(t, dir, "src/features/checkout/finish.ts", finishBad)

	r := check(t, dir, &fake.Fake{Decide: decider}, engine.Options{})
	if r.ExitCode != report.ExitViolation || r.Gate != report.GateViolated {
		t.Fatalf("got %s/%d %v", r.Gate, r.ExitCode, verdicts(r))
	}
	var cv report.Check
	for _, c := range r.Checks {
		if c.ID == "checkout.failure/CHK-001" {
			cv = c
		}
	}
	if cv.Verdict != report.VerdictViolated || len(cv.Locations) != 1 || cv.Locations[0].Role != report.RoleModelSelected {
		t.Fatalf("violation %+v", cv)
	}
	loc := cv.Locations[0]
	if loc.Path != "src/features/checkout/finish.ts" || loc.StartLine != 4 || !strings.Contains(loc.Excerpt, "cart.items = [];") {
		t.Fatalf("location %+v", loc)
	}
	var argv []string
	for _, a := range cv.NextActions {
		if a.Kind == report.ActionRecheck {
			argv = a.Argv
		}
	}
	want := []string{"sure", "checkout.failure", "--repo", dir, "--contracts-ref", head, "--json"}
	if strings.Join(argv, " ") != strings.Join(want, " ") {
		t.Fatalf("recheck %v", argv)
	}
}

func TestAcceptancePolicyMapsWeakAnswersToUnknown(t *testing.T) {
	files := baseFiles()
	files[".sure/config.yaml"] = configYAML + "policy:\n  min_selected_probability: 0.5\n"
	dir, _ := committedRepo(t, files)
	weak := func(q provider.Question, req provider.Request) provider.Answer {
		if strings.HasSuffix(q.ID, "_locate") {
			return fake.Answer(q, "NONE", 0.9)
		}
		var instr map[string]string
		_ = json.Unmarshal(q.Instructions, &instr)
		switch {
		case strings.Contains(instr["requirement"], "cart items"):
			return provider.Answer{Choice: "supported", Probabilities: map[string]float64{"supported": 0.55, "violated": 0.45, "unknown": 0}}
		case strings.Contains(instr["requirement"], "order-success"):
			return fake.Answer(q, "unknown", 0.9)
		case strings.Contains(instr["requirement"], "submitting"):
			return fake.Answer(q, "supported", 0.45)
		}
		return fake.Answer(q, "supported", 0.96)
	}
	r := check(t, dir, &fake.Fake{Decide: weak}, engine.Options{})
	v := verdicts(r)
	if v["checkout.failure/CHK-001"] != "unknown/low_confidence" || v["checkout.failure/CHK-002"] != "unknown/model_unknown" ||
		v["checkout.failure/CHK-003"] != "unknown/low_confidence" {
		t.Fatalf("verdicts %v", v)
	}
	if r.ExitCode != report.ExitIncomplete || r.Status != report.StatusIncomplete {
		t.Fatalf("exit %d status %s", r.ExitCode, r.Status)
	}
}

func TestChangedModeSelectionAndUnmapped(t *testing.T) {
	files := baseFiles()
	files["src/catalog/list.ts"] = "export const list = [];\n"
	dir, _ := committedRepo(t, files)
	f := &fake.Fake{Decide: decider}

	r := check(t, dir, f, engine.Options{Mode: engine.ModeChanged})
	if r.Status != report.StatusNoChanges || r.ExitCode != 0 || r.Summary.Evaluated != 0 || f.Calls() != 0 {
		t.Fatalf("no changes: %s exit %d evaluated %d calls %d", r.Status, r.ExitCode, r.Summary.Evaluated, f.Calls())
	}
	for _, c := range r.Checks {
		if c.Verdict != report.VerdictNotChecked {
			t.Fatalf("no-changes run reported %s for %s", c.Verdict, c.ID)
		}
	}

	testutil.Write(t, dir, "src/domain/cart.ts", "export type Cart = { items: { sku: string; quantity: number; note?: string }[] };\n")
	r = check(t, dir, f, engine.Options{Mode: engine.ModeChanged})
	if r.Summary.Selected != 5 || r.ExitCode != 0 {
		t.Fatalf("cart.ts is context of checkout and scope of cart: %+v exit %d", r.Summary, r.ExitCode)
	}

	testutil.Git(t, dir, "checkout", "-q", "--", "src/domain/cart.ts")
	testutil.Write(t, dir, "src/catalog/list.ts", "export const list = [1];\n")
	testutil.Write(t, dir, "src/catalog/new.ts", "export const fresh = 1;\n")
	r = check(t, dir, f, engine.Options{Mode: engine.ModeChanged})
	if r.Summary.Selected != 0 || r.ExitCode != report.ExitIncomplete ||
		strings.Join(r.UnmappedChanges, ",") != "src/catalog/list.ts,src/catalog/new.ts" {
		t.Fatalf("unmapped: %+v %v exit %d", r.Summary, r.UnmappedChanges, r.ExitCode)
	}

	testutil.Write(t, dir, "src/catalog/list.ts", "export const list = [];\n")
	if err := os.Remove(filepath.Join(dir, "src/catalog/new.ts")); err != nil {
		t.Fatal(err)
	}
	testutil.Write(t, dir, "README.md", "docs only\n")
	r = check(t, dir, f, engine.Options{Mode: engine.ModeChanged})
	if r.Status != report.StatusNoApplicable || r.ExitCode != 0 {
		t.Fatalf("non-source change: %s exit %d", r.Status, r.ExitCode)
	}

	testutil.Write(t, dir, "package.json", "{}\n")
	r = check(t, dir, f, engine.Options{Mode: engine.ModeChanged})
	if r.Summary.Selected != 5 || !strings.HasPrefix(r.Contracts[0].SelectedBy, "plan_invalidated:") {
		t.Fatalf("dependency change must select all: %+v %s", r.Summary, r.Contracts[0].SelectedBy)
	}
}

func TestRenameSelectsByOldPathAndBaseIsMergeBase(t *testing.T) {
	dir, base := committedRepo(t, baseFiles())
	testutil.Git(t, dir, "checkout", "-q", "-b", "feature")
	testutil.Git(t, dir, "mv", "src/features/checkout/finish.ts", "src/finish.ts")
	testutil.Commit(t, dir, "move")
	testutil.Git(t, dir, "checkout", "-q", "main")
	testutil.Write(t, dir, "src/other.ts", "x\n")
	testutil.Commit(t, dir, "main moves on")
	testutil.Git(t, dir, "checkout", "-q", "feature")

	r := check(t, dir, &fake.Fake{Decide: decider}, engine.Options{Mode: engine.ModeChanged, BaseRef: "main"})
	if *r.Snapshot.BaseCommit != base {
		t.Fatalf("base %s want merge base %s", *r.Snapshot.BaseCommit, base)
	}
	if !strings.HasPrefix(r.Contracts[0].SelectedBy, "changed:") && r.Contracts[1].Selected != true {
		t.Fatalf("checkout contract not selected via old path: %+v", r.Contracts)
	}
	v := verdicts(r)
	if v["checkout.failure/CHK-001"] != "unknown/empty_scope" {
		t.Fatalf("moved file leaves trusted scope empty: %v", v)
	}
	if strings.Join(r.UnmappedChanges, ",") != "src/finish.ts" {
		t.Fatalf("new path should be unmapped: %v", r.UnmappedChanges)
	}
}

func TestMissingContextAndBudgetAreUnknownWithoutCalls(t *testing.T) {
	files := baseFiles()
	dir, _ := committedRepo(t, files)
	if err := os.Remove(filepath.Join(dir, "src/domain/orders.ts")); err != nil {
		t.Fatal(err)
	}
	f := &fake.Fake{Decide: decider}
	r := check(t, dir, f, engine.Options{Mode: engine.ModeContract, Contracts: []string{"checkout.failure"}})
	v := verdicts(r)
	if v["checkout.failure/CHK-001"] != "unknown/missing_context" || f.Calls() != 0 || r.ExitCode != report.ExitIncomplete {
		t.Fatalf("missing context: %v calls %d exit %d", v, f.Calls(), r.ExitCode)
	}
	if v["cart.lines/CART-001"] != "not_checked/not_selected" || r.Summary.NotChecked != 2 {
		t.Fatalf("unselected contract: %v", v)
	}

	files = baseFiles()
	files[".sure/config.yaml"] = configYAML + "limits:\n  max_state_bytes: 1024\n  max_request_bytes: 2048\n"
	files["src/features/checkout/big.ts"] = "// " + strings.Repeat("x", 1500) + "\n"
	dir, _ = committedRepo(t, files)
	f = &fake.Fake{Decide: decider}
	r = check(t, dir, f, engine.Options{Mode: engine.ModeContract, Contracts: []string{"checkout.failure"}})
	if v := verdicts(r); v["checkout.failure/CHK-002"] != "unknown/evidence_budget_exceeded" || f.Calls() != 0 {
		t.Fatalf("budget: %v calls %d", v, f.Calls())
	}
	for _, c := range r.Checks {
		if c.ID == "checkout.failure/CHK-002" && !strings.Contains(c.Message, "src/features/checkout/finish.ts") {
			t.Fatalf("omitted files not listed: %s", c.Message)
		}
	}
}

func TestTrustedContractsWinOverWorkingTreeEdits(t *testing.T) {
	dir, _ := committedRepo(t, baseFiles())
	weakened := strings.Replace(checkoutMD, "## CHK-001 — Preserve the cart\nGiven a non-empty cart, when order submission resolves with an error,\nexisting cart items and quantities must remain unchanged.\n", "", 1)
	weakened = strings.Replace(weakened, "src/features/checkout/**", "src/features/checkout/ui/**", 1)
	weakened = strings.Replace(weakened, "severity: error", "severity: warning", 1)
	testutil.Write(t, dir, ".sure/contracts/checkout.md", weakened)
	testutil.Write(t, dir, ".sure/config.yaml", configYAML+"  base_url: https://evil.example\n")
	testutil.Write(t, dir, "src/features/checkout/finish.ts", finishBad)

	f := &fake.Fake{Decide: decider}
	r := check(t, dir, f, engine.Options{})
	if r.ExitCode != report.ExitViolation || verdicts(r)["checkout.failure/CHK-001"] != "violated/source_conflicts_with_requirement" {
		t.Fatalf("trusted check must still fail: %v exit %d", verdicts(r), r.ExitCode)
	}
	kinds := map[string]bool{}
	for _, c := range r.ContractChanges {
		kinds[c.Kind] = true
	}
	if !kinds["config_invalid"] {
		t.Fatalf("untrusted endpoint not reported: %+v", r.ContractChanges)
	}
	if r.Provider.Endpoint != "fake://" {
		t.Fatalf("endpoint redirected to %s", r.Provider.Endpoint)
	}
	for _, req := range f.Requests() {
		if strings.Contains(string(req.State), "checkout.failure") &&
			!strings.Contains(string(req.Questions[0].Instructions), "existing cart items and quantities must remain unchanged") {
			t.Fatal("request did not use the trusted wording")
		}
	}

	testutil.Write(t, dir, ".sure/config.yaml", configYAML)
	testutil.Write(t, dir, "src/features/checkout/finish.ts", finishGood)
	r = check(t, dir, &fake.Fake{Decide: decider}, engine.Options{})
	kinds = map[string]bool{}
	for _, c := range r.ContractChanges {
		kinds[c.Kind+"/"+c.Field] = true
	}
	for _, k := range []string{"check_removed/", "contract_changed/scope", "contract_changed/severity"} {
		if !kinds[k] {
			t.Errorf("missing change %s in %v", k, kinds)
		}
	}
	if r.ExitCode != report.ExitIncomplete || r.Summary.Supported != 5 {
		t.Fatalf("contract changes must make the run incomplete: exit %d %+v", r.ExitCode, r.Summary)
	}
}

func TestDraftAndMissingTrustedBase(t *testing.T) {
	testutil.Isolate(t)
	dir := testutil.NewRepo(t)
	testutil.WriteFiles(t, dir, baseFiles())
	f := &fake.Fake{Decide: decider}
	r := check(t, dir, f, engine.Options{})
	if r.ExitCode != report.ExitUsage || r.Errors[0].Code != "no_trusted_base" {
		t.Fatalf("no commits: %d %v", r.ExitCode, r.Errors)
	}
	r = check(t, dir, f, engine.Options{Draft: true})
	if r.ExitCode != 0 || r.Trust.Mode != "draft" || r.Trust.ContractsCommit != nil {
		t.Fatalf("draft: %d %+v", r.ExitCode, r.Trust)
	}

	plain := t.TempDir()
	testutil.WriteFiles(t, plain, baseFiles())
	r = check(t, plain, f, engine.Options{Draft: true})
	if r.ExitCode != 0 || r.Snapshot.HeadCommit != nil {
		t.Fatalf("plain directory draft: %d %v", r.ExitCode, r.Errors)
	}
	r = check(t, plain, f, engine.Options{Mode: engine.ModeChanged, Draft: true})
	if r.ExitCode != report.ExitUsage {
		t.Fatalf("--changed outside git: %d", r.ExitCode)
	}
	r = check(t, plain, f, engine.Options{Mode: engine.ModeContract, Contracts: []string{"nope"}, Draft: true})
	if r.ExitCode != report.ExitUsage || r.Errors[0].Code != "unknown_contract" {
		t.Fatalf("unknown contract: %d %v", r.ExitCode, r.Errors)
	}
}

type byContract struct {
	errs map[string]error
}

func (b byContract) Identity() provider.Identity { return (&fake.Fake{}).Identity() }

func (b byContract) Evaluate(ctx context.Context, req provider.Request) (provider.Response, error) {
	for id, err := range b.errs {
		if strings.Contains(string(req.State), `"id":"`+id+`"`) {
			return provider.Response{}, err
		}
	}
	return (&fake.Fake{Decide: decider}).Evaluate(ctx, req)
}

func TestOperationalFailuresAndPrecedence(t *testing.T) {
	dir, _ := committedRepo(t, baseFiles())
	testutil.Write(t, dir, "src/features/checkout/finish.ts", finishBad)
	cases := []struct {
		errs map[string]error
		exit int
	}{
		{map[string]error{"cart.lines": provider.Errorf(provider.ErrRateLimited, "busy")}, report.ExitRateLimit},
		{map[string]error{"cart.lines": provider.Errorf(provider.ErrTransport, "down")}, report.ExitNetwork},
		{map[string]error{"cart.lines": provider.Errorf(provider.ErrAuth, "bad key")}, report.ExitNetwork},
		{map[string]error{"cart.lines": provider.Errorf(provider.ErrDeadline, "late")}, report.ExitTimeout},
		{map[string]error{
			"cart.lines":       provider.Errorf(provider.ErrRateLimited, "busy"),
			"checkout.failure": provider.Errorf(provider.ErrMalformed, "bad decision"),
		}, report.ExitMalformed},
		{map[string]error{
			"cart.lines":       provider.Errorf(provider.ErrMalformed, "bad decision"),
			"checkout.failure": provider.Errorf(provider.ErrCanceled, "canceled"),
		}, report.ExitCanceled},
	}
	for _, tc := range cases {
		r := check(t, dir, byContract{tc.errs}, engine.Options{})
		if r.ExitCode != tc.exit || r.Gate != report.GateError || r.Status != report.StatusFailed {
			t.Errorf("%v: exit %d gate %s", tc.errs, r.ExitCode, r.Gate)
		}
		if r.Summary.Error == 0 || len(r.Errors) != len(tc.errs) {
			t.Errorf("%v: summary %+v errors %v", tc.errs, r.Summary, r.Errors)
		}
	}
}

func TestDeadlineCancelsRun(t *testing.T) {
	dir, _ := committedRepo(t, baseFiles())
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)
	r := engine.Check(ctx, engine.Options{Repo: dir, Mode: engine.ModeAll, NoCache: true}, deps(&fake.Fake{Decide: decider}, nil))
	if r.ExitCode != report.ExitTimeout {
		t.Fatalf("deadline exit %d %v", r.ExitCode, r.Errors)
	}
}

func TestVisualContractNeverPasses(t *testing.T) {
	files := baseFiles()
	files[".sure/contracts/home.md"] = `---
schema_version: sure.contract/v1
id: home.look
severity: error
evidence: image
scope:
  - src/features/checkout/**
---
# Home look

## VIS-001 — Large buttons
Primary buttons are at least 56 points tall.
`
	dir, _ := committedRepo(t, files)
	f := &fake.Fake{Decide: decider}
	r := check(t, dir, f, engine.Options{Mode: engine.ModeContract, Contracts: []string{"home.look"}})
	if v := verdicts(r); v["home.look/VIS-001"] != "unknown/missing_visual_evidence" || f.Calls() != 0 {
		t.Fatalf("visual %v calls %d", v, f.Calls())
	}
}

func TestUncommittedBytesAreWhatTheModelSees(t *testing.T) {
	dir, _ := committedRepo(t, baseFiles())
	testutil.Write(t, dir, "src/domain/orders.ts", "export type OrderResult = { ok: boolean }; // edited, unstaged\n")
	f := &fake.Fake{Decide: decider}
	check(t, dir, f, engine.Options{Mode: engine.ModeContract, Contracts: []string{"checkout.failure"}})
	if !strings.Contains(string(f.Requests()[0].State), "edited, unstaged") {
		t.Fatal("request did not carry the working-tree bytes")
	}
}

func TestCacheReuseAndInvalidation(t *testing.T) {
	dir, _ := committedRepo(t, baseFiles())
	f := &fake.Fake{Decide: decider}
	env := map[string]string{"SURE_CACHE_DIR": t.TempDir()}
	run := func() report.Run {
		return engine.Check(context.Background(), engine.Options{Repo: dir, Mode: engine.ModeAll}, deps(f, env))
	}
	first := run()
	second := run()
	if f.Calls() != 2 || first.Provider.Mode != "fake" || second.Provider.Mode != "cache" || second.Provider.Usage.CacheHits != 2 {
		t.Fatalf("cache reuse: calls %d modes %s/%s hits %d", f.Calls(), first.Provider.Mode, second.Provider.Mode, second.Provider.Usage.CacheHits)
	}
	testutil.Write(t, dir, "src/domain/orders.ts", "export type OrderResult = { ok: true };\n")
	third := run()
	if f.Calls() != 3 || third.Provider.Mode != "mixed" {
		t.Fatalf("context change must invalidate only the dependent contract: calls %d mode %s", f.Calls(), third.Provider.Mode)
	}
}

func TestRecordThenReplay(t *testing.T) {
	dir, _ := committedRepo(t, baseFiles())
	testutil.Write(t, dir, "src/features/checkout/finish.ts", finishBad)
	rec := t.TempDir()
	f := &fake.Fake{Decide: decider}
	live := check(t, dir, f, engine.Options{RecordDir: rec})
	replayed := check(t, dir, f, engine.Options{Provider: provider.NameReplay, ReplayDir: rec})
	if f.Calls() != 2 || replayed.Provider.Mode != "replay" || replayed.ExitCode != live.ExitCode {
		t.Fatalf("replay: calls %d mode %s exit %d/%d", f.Calls(), replayed.Provider.Mode, live.ExitCode, replayed.ExitCode)
	}
	testutil.Write(t, dir, "src/features/checkout/finish.ts", finishGood)
	miss := check(t, dir, f, engine.Options{Provider: provider.NameReplay, ReplayDir: rec})
	if miss.ExitCode != report.ExitNetwork || f.Calls() != 2 {
		t.Fatalf("replay miss must fail without a live call: exit %d calls %d", miss.ExitCode, f.Calls())
	}
}

func TestAssessBaseLabelsIntroducedRegression(t *testing.T) {
	dir, _ := committedRepo(t, baseFiles())
	testutil.Write(t, dir, "src/features/checkout/finish.ts", finishBad)
	r := check(t, dir, &fake.Fake{Decide: decider}, engine.Options{Mode: engine.ModeChanged, AssessBase: true})
	for _, c := range r.Checks {
		want := report.RegressionNotAssessed
		if c.ID == "checkout.failure/CHK-001" {
			want = report.RegressionIntroduced
		}
		if c.Regression != want {
			t.Errorf("%s regression %s, want %s", c.ID, c.Regression, want)
		}
	}
}

func TestCheckNeverExecutesTheTarget(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "executed")
	files := baseFiles()
	files["package.json"] = `{"scripts":{"preinstall":"touch ` + marker + `","test":"touch ` + marker + `","build":"touch ` + marker + `"}}`
	files["Makefile"] = "all:\n\ttouch " + marker + "\n"
	files["src/features/checkout/run.sh"] = "#!/bin/sh\ntouch " + marker + "\n"
	dir, _ := committedRepo(t, files)
	testutil.Git(t, dir, "config", "core.fsmonitor", filepath.Join(dir, "src/features/checkout/run.sh"))
	if err := os.Chmod(filepath.Join(dir, "src/features/checkout/run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Write(t, dir, "src/features/checkout/finish.ts", finishBad)
	check(t, dir, &fake.Fake{Decide: decider}, engine.Options{Mode: engine.ModeChanged, AssessBase: true})
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("sure check executed something from the target repository")
	}
}

func TestPlanDoesNotCallProvider(t *testing.T) {
	dir, _ := committedRepo(t, baseFiles())
	f := &fake.Fake{Decide: decider}
	p := engine.Plan(context.Background(), engine.Options{Repo: dir, Mode: engine.ModeAll}, deps(f, nil))
	if f.Calls() != 0 || p.Summary.Requests != 2 || p.Summary.WouldEvaluate != 5 || p.ExitCode != 0 {
		t.Fatalf("plan %+v calls %d", p.Summary, f.Calls())
	}
}

func TestAutoModeSelectionAndConfidence(t *testing.T) {
	dir, _ := committedRepo(t, baseFiles())
	f := &fake.Fake{Decide: decider}
	auto := engine.Options{Mode: engine.ModeAuto}

	r := check(t, dir, f, auto)
	if r.Snapshot.Selection != "all_no_changes" || r.Summary.Selected != 5 || *r.Snapshot.BaseRef != "main" {
		t.Fatalf("no changes: selection %s selected %d base %v", r.Snapshot.Selection, r.Summary.Selected, r.Snapshot.BaseRef)
	}
	if r.Confidence == nil || r.Confidence.Percent != 96 || r.Contracts[0].Confidence == nil {
		t.Fatalf("clean confidence %+v", r.Confidence)
	}

	testutil.Write(t, dir, "src/features/checkout/finish.ts", finishBad)
	r = check(t, dir, f, auto)
	if r.Snapshot.Selection != "changed" || r.Summary.Selected != 3 || r.Summary.NotChecked != 2 {
		t.Fatalf("changed: %s %+v", r.Snapshot.Selection, r.Summary)
	}
	if r.Confidence.Percent != 2 || r.ExitCode != report.ExitViolation {
		t.Fatalf("violation confidence %+v exit %d", r.Confidence, r.ExitCode)
	}
	for _, c := range r.Contracts {
		if c.ID == "cart.lines" && c.Confidence != nil {
			t.Fatal("an unselected contract got a confidence")
		}
	}

	testutil.Write(t, dir, "src/features/checkout/finish.ts", finishGood)
	if err := os.Remove(filepath.Join(dir, "src/domain/orders.ts")); err != nil {
		t.Fatal(err)
	}
	r = check(t, dir, f, engine.Options{Mode: engine.ModeContract, Contracts: []string{"checkout"}})
	if r.Confidence == nil || r.Confidence.Percent != 0 || r.Confidence.Undecided != 3 || r.Snapshot.Selection != "contract" {
		t.Fatalf("undecided checks must count as 0: %+v", r.Confidence)
	}

	r = check(t, dir, byContract{map[string]error{"cart.lines": provider.Errorf(provider.ErrTransport, "down")}}, engine.Options{Mode: engine.ModeAll})
	if r.Confidence != nil {
		t.Fatalf("a provider failure must leave no confidence: %+v", r.Confidence)
	}
}

func TestContractPrefixSelection(t *testing.T) {
	dir, _ := committedRepo(t, baseFiles())
	r := check(t, dir, &fake.Fake{Decide: decider}, engine.Options{Mode: engine.ModeContract, Contracts: []string{"cart", "checkout.failure/CHK-002"}})
	if r.Summary.Selected != 5 {
		t.Fatalf("prefix and check-id selection: %+v", r.Summary)
	}
	r = check(t, dir, &fake.Fake{Decide: decider}, engine.Options{Mode: engine.ModeContract, Contracts: []string{"car"}})
	if r.ExitCode != report.ExitUsage || !strings.Contains(r.Errors[0].Message, "known: cart.lines, checkout.failure") {
		t.Fatalf("partial word must not match: %d %v", r.ExitCode, r.Errors)
	}
}

func TestCommittedBranchEditsToContractsAreNotTrusted(t *testing.T) {
	dir, base := committedRepo(t, baseFiles())
	testutil.Git(t, dir, "checkout", "-q", "-b", "feature")
	testutil.Write(t, dir, ".sure/contracts/checkout.md", strings.Replace(checkoutMD, "severity: error", "severity: warning", 1))
	testutil.Write(t, dir, "src/features/checkout/finish.ts", finishBad)
	testutil.Commit(t, dir, "weaken and break")

	for _, opts := range []engine.Options{
		{Mode: engine.ModeContract, Contracts: []string{"checkout.failure"}},
		{Mode: engine.ModeAll},
		{Mode: engine.ModeAuto},
	} {
		r := check(t, dir, &fake.Fake{Decide: decider}, opts)
		if *r.Trust.ContractsCommit != base || r.ExitCode != report.ExitViolation {
			t.Fatalf("%s: trusted %s (want base %s), exit %d", opts.Mode, *r.Trust.ContractsCommit, base, r.ExitCode)
		}
	}
}
