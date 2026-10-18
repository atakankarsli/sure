package engine_test

import (
	"context"
	"strings"
	"testing"

	"github.com/atakankarsli/sure/internal/engine"
	"github.com/atakankarsli/sure/internal/provider"
	"github.com/atakankarsli/sure/internal/provider/fake"
)

type specFactory struct {
	inner factory
	got   *engine.ProviderSpec
}

func (s specFactory) Identity(sp engine.ProviderSpec) (provider.Identity, error) {
	*s.got = sp
	return s.inner.Identity(sp)
}

func (s specFactory) New(sp engine.ProviderSpec) (provider.Provider, error) { return s.inner.New(sp) }

func TestLongContextRaisesTheBudgetAndReachesTheFactory(t *testing.T) {
	files := baseFiles()
	files["src/features/checkout/big.ts"] = "// " + strings.Repeat("x", 100<<10) + "\n"
	dir, _ := committedRepo(t, files)
	opts := engine.Options{Mode: engine.ModeContract, Contracts: []string{"checkout.failure"}}

	f := &fake.Fake{Decide: decider}
	r := check(t, dir, f, opts)
	if v := verdicts(r); v["checkout.failure/CHK-001"] != "unknown/evidence_budget_exceeded" || f.Calls() != 0 {
		t.Fatalf("standard budget: %v calls %d", v, f.Calls())
	}

	f = &fake.Fake{Decide: decider}
	var got engine.ProviderSpec
	d := deps(f, nil)
	d.Factory = specFactory{inner: factory{f}, got: &got}
	opts.Repo, opts.NoCache, opts.LongContext = dir, true, true
	r = engine.Check(context.Background(), opts, d)
	if v := verdicts(r); !strings.HasPrefix(v["checkout.failure/CHK-001"], "supported/") || f.Calls() != 1 || r.ExitCode != 0 {
		t.Fatalf("long context: %v calls %d exit %d", v, f.Calls(), r.ExitCode)
	}
	if !got.LongContext {
		t.Fatalf("factory did not see long context: %+v", got)
	}
}
