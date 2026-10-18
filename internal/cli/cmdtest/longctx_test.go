package cmdtest

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/atakankarsli/sure/internal/cli/shared"
	"github.com/atakankarsli/sure/internal/provider"
	"github.com/atakankarsli/sure/internal/provider/jev"
	"github.com/atakankarsli/sure/internal/testutil"
)

// typeSafeServer answers in TypeSafe's own wire shape and records the
// long-context header of every request.
type typeSafeServer struct {
	*httptest.Server
	mu      sync.Mutex
	headers []string
}

func newTypeSafeServer(t *testing.T) *typeSafeServer {
	t.Helper()
	s := &typeSafeServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.headers = append(s.headers, r.Header.Get(jev.LongContextHeader))
		s.mu.Unlock()
		b, _ := io.ReadAll(r.Body)
		var req struct {
			Questions map[string]struct {
				Criteria map[string]any `json:"criteria"`
			} `json:"questions"`
		}
		if err := json.Unmarshal(b, &req); err != nil {
			w.WriteHeader(422)
			return
		}
		answers := map[string]any{}
		for id, q := range req.Questions {
			choice := "supported"
			if strings.HasSuffix(id, "_locate") || id == "probe" {
				choice = "NONE"
				if _, ok := q.Criteria[choice]; !ok {
					for k := range q.Criteria {
						choice = k
						break
					}
				}
			}
			probs := map[string]float64{}
			rest := 0.04 / float64(max(len(q.Criteria)-1, 1))
			for k := range q.Criteria {
				probs[k] = rest
			}
			probs[choice] = 0.96
			answers[id] = map[string]any{"type": "choice", "choice": choice, "probabilities": probs, "confidence": 0.9}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"model": "jev-1.13.0", "answers": answers,
			"usage": map[string]int{"input_tokens": len(b) / 4, "output_tokens": 0}})
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *typeSafeServer) factory() shared.ProviderFactory {
	return shared.ProviderFactory{ConfigureJev: func(c *jev.Config) {
		c.BaseURL, c.AllowInsecureBaseURL = s.URL, true
		c.Retry = provider.RetryPolicy{MaxRetries: 1, Gate: &provider.Gate{},
			Sleep: func(context.Context, time.Duration) error { return nil }}
	}}
}

func (s *typeSafeServer) seen() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.headers...)
}

func TestLongContextRaisesTheBudgetAndSendsTheHeader(t *testing.T) {
	dir := repo(t)
	// 100 KiB of scope exceeds the default 64 KiB state budget.
	testutil.Write(t, dir, "src/checkout/big.ts", strings.Repeat("export const pad = 1;\n", 100<<10/22))
	testutil.Commit(t, dir, "big file")
	srv := newTypeSafeServer(t)
	env := map[string]string{"TYPESAFE_API_KEY": testKey, "SURE_CACHE_DIR": t.TempDir()}

	r := Run(env, srv.factory(), "--repo", dir, "--json")
	if r.ExitCode != 3 {
		t.Fatalf("standard budget: exit %d\n%s\n%s", r.ExitCode, r.Stdout, r.Stderr)
	}
	m := validateRun(t, r.Stdout)
	first := m["checks"].([]any)[0].(map[string]any)
	if first["verdict"] != "unknown" || first["reason_code"] != "evidence_budget_exceeded" || len(srv.seen()) != 0 {
		t.Fatalf("standard budget: %v, requests %v", first, srv.seen())
	}

	r = Run(env, srv.factory(), "--repo", dir, "--json", "--long-context")
	if r.ExitCode != 0 {
		t.Fatalf("long context: exit %d\n%s\n%s", r.ExitCode, r.Stdout, r.Stderr)
	}
	m = validateRun(t, r.Stdout)
	prov := m["provider"].(map[string]any)
	if prov["long_context"] != true || prov["transport"] != "typesafe" || m["gate"] != "clear" {
		t.Fatalf("long context run: provider %v gate %v", prov, m["gate"])
	}
	if seen := srv.seen(); len(seen) != 1 || seen[0] != "true" {
		t.Fatalf("long-context header: %q", seen)
	}

	r = Run(env, srv.factory(), "--repo", dir, "--json", "--long-context", "--provider", "anthropic")
	if r.ExitCode != 2 {
		t.Fatalf("anthropic with long context must be a usage error, exit %d\n%s", r.ExitCode, r.Stdout)
	}

	r = Run(env, srv.factory(), "doctor", "--repo", dir, "--live", "--long-context", "--json")
	if r.ExitCode != 0 {
		t.Fatalf("doctor: exit %d\n%s\n%s", r.ExitCode, r.Stdout, r.Stderr)
	}
	if seen := srv.seen(); len(seen) != 2 || seen[1] != "true" {
		t.Fatalf("doctor probe header: %q", seen)
	}
}
