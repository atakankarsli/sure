package cmdtest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/atakankarsli/sure/internal/cli/shared"
	"github.com/atakankarsli/sure/internal/provider"
	"github.com/atakankarsli/sure/internal/provider/jev"
	"github.com/atakankarsli/sure/internal/testutil"
	"github.com/atakankarsli/sure/schemas"
)

const testKey = "test-gateway-key"

const configYAML = `schema_version: sure.config/v1
source_roots: [src]
exclude: ["**/*.test.*"]
`

const contractMD = `---
schema_version: sure.contract/v1
id: checkout.failure
severity: error
evidence: code
scope:
  - src/checkout/**
context:
  - src/cart.ts
---
# Checkout failure

## CHK-001 — Preserve the cart
When order submission fails, existing cart items must remain unchanged.

## CHK-002 — Allow another attempt
After a failed submission settles, submitting must become false.
`

const goodFinish = "export function finish(r, cart, ui) {\n  if (!r.ok) {\n    ui.submitting = false;\n    return;\n  }\n  cart.items = [];\n}\n"
const badFinish = "export function finish(r, cart, ui) {\n  if (!r.ok) {\n    cart.items = []; // BUG\n    ui.submitting = false;\n    return;\n  }\n  cart.items = [];\n}\n"

func repo(t *testing.T) string {
	t.Helper()
	dir := testutil.NewRepo(t)
	testutil.WriteFiles(t, dir, map[string]string{
		".sure/config.yaml":           configYAML,
		".sure/contracts/checkout.md": contractMD,
		"src/checkout/finish.ts":      goodFinish,
		"src/checkout/finish.test.ts": "SECRET-ORACLE expected: violated\n",
		"src/cart.ts":                 "export type Cart = { items: string[] };\n",
	})
	testutil.Commit(t, dir, "baseline")
	return dir
}

// jevServer imitates the gateway's /v1/evaluate: it violates the cart check
// when the state contains the BUG marker.
type jevServer struct {
	*httptest.Server
	mu     sync.Mutex
	bodies []string
	auth   []string
	status int
}

func newJevServer(t *testing.T) *jevServer {
	s := &jevServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.bodies = append(s.bodies, string(b))
		s.auth = append(s.auth, r.Header.Get("Authorization"))
		status := s.status
		s.mu.Unlock()
		if status != 0 {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":{"message":"busy","type":"rate_limited"}}`))
			return
		}
		var req struct {
			State     json.RawMessage `json:"stateParts"`
			Questions map[string]struct {
				Instructions map[string]string `json:"instructions"`
				Criteria     map[string]any    `json:"criteria"`
			} `json:"questions"`
		}
		if err := json.Unmarshal(b, &req); err != nil {
			w.WriteHeader(422)
			return
		}
		buggy := strings.Contains(string(req.State), "BUG")
		answers := map[string]any{}
		for id, q := range req.Questions {
			opts := make([]string, 0, len(q.Criteria))
			for k := range q.Criteria {
				opts = append(opts, k)
			}
			choice := "supported"
			if strings.HasSuffix(id, "_locate") {
				choice = "NONE"
				if buggy {
					choice = "R001"
				}
			} else if buggy && strings.Contains(q.Instructions["requirement"], "cart items") {
				choice = "violated"
			}
			if _, ok := q.Criteria[choice]; !ok {
				sort.Strings(opts)
				choice = opts[len(opts)-1]
			}
			probs := map[string]float64{}
			rest := 0.04 / float64(len(opts)-1)
			for _, o := range opts {
				probs[o] = rest
			}
			probs[choice] = 0.96
			answers[id] = map[string]any{"type": "choice", "choice": choice, "probabilities": probs, "confidence": 0.9}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"model": "jev-1.13.0", "answers": answers,
			"usage": map[string]int{"inputTokens": len(b) / 4, "outputTokens": 0}})
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *jevServer) factory() shared.ProviderFactory {
	return shared.ProviderFactory{ConfigureJev: func(c *jev.Config) {
		c.BaseURL, c.AllowInsecureBaseURL = s.URL+"/v4/ai/decision-model", true
		c.Retry = provider.RetryPolicy{MaxRetries: 1, Gate: &provider.Gate{},
			Sleep: func(context.Context, time.Duration) error { return nil }}
	}}
}

func env(t *testing.T, extra ...string) map[string]string {
	m := map[string]string{"AI_GATEWAY_API_KEY": testKey, "SURE_CACHE_DIR": t.TempDir()}
	for i := 0; i+1 < len(extra); i += 2 {
		m[extra[i]] = extra[i+1]
	}
	return m
}

var (
	schemaOnce sync.Once
	runSchema  *jsonschema.Schema
	schemaErr  error
)

func validateRun(t *testing.T, stdout string) map[string]any {
	t.Helper()
	schemaOnce.Do(func() {
		b, err := schemas.FS.ReadFile("run-result.schema.json")
		if err != nil {
			schemaErr = err
			return
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
		if err != nil {
			schemaErr = err
			return
		}
		c := jsonschema.NewCompiler()
		if schemaErr = c.AddResource("urn:sure:run-result:v1", doc); schemaErr == nil {
			runSchema, schemaErr = c.Compile("urn:sure:run-result:v1")
		}
	})
	if schemaErr != nil {
		t.Fatal(schemaErr)
	}
	m := singleJSON(t, stdout)
	inst, _ := jsonschema.UnmarshalJSON(strings.NewReader(stdout))
	if err := runSchema.Validate(inst); err != nil {
		t.Fatalf("output does not match run-result schema: %v\n%s", err, stdout)
	}
	return m
}

// singleJSON asserts stdout is exactly one JSON document and nothing else.
func singleJSON(t *testing.T, stdout string) map[string]any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(stdout))
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%q", err, stdout)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		t.Fatalf("stdout has more than one JSON document: %q", stdout)
	}
	if strings.Contains(stdout, "\x1b[") {
		t.Fatal("stdout contains ANSI escapes")
	}
	return m
}

func TestHelpVersionAndUsageErrors(t *testing.T) {
	testutil.Isolate(t)
	r := Run(nil, shared.ProviderFactory{}, "--version")
	if r.ExitCode != 0 || strings.TrimSpace(r.Stdout) != "test-version" {
		t.Fatalf("--version: %+v", r)
	}
	for _, args := range [][]string{{"--help"}, {"check", "--help"}, {"validate", "-h"}, {"init", "--help"}} {
		r = Run(nil, shared.ProviderFactory{}, args...)
		if r.ExitCode != 0 || !strings.Contains(r.Stdout, "USAGE") {
			t.Fatalf("%v: %+v", args, r)
		}
	}
	r = Run(nil, shared.ProviderFactory{}, "--help")
	for _, want := range []string{"sure init", "sure validate", "sure doctor", "--json", "--dry-run"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("help lacks %q", want)
		}
	}
	empty := t.TempDir()
	r = Run(nil, shared.ProviderFactory{}, "--repo", empty)
	if r.ExitCode != 2 || !strings.Contains(r.Stdout, "sure init") {
		t.Fatalf("bare sure without .sure/: %+v", r)
	}
	r = Run(nil, shared.ProviderFactory{}, "--repo", empty, "--no-such-flag")
	if r.ExitCode != 2 || r.Stdout != "" {
		t.Fatalf("bad flag must not print to stdout: %+v", r)
	}
}

func TestBareSureEndToEndOverHTTP(t *testing.T) {
	dir := repo(t)
	srv := newJevServer(t)

	r := Run(env(t), srv.factory(), "--repo", dir, "--json")
	if r.ExitCode != 0 {
		t.Fatalf("clean run exit %d\nstdout %s\nstderr %s", r.ExitCode, r.Stdout, r.Stderr)
	}
	m := validateRun(t, r.Stdout)
	conf := m["confidence"].(map[string]any)
	if m["gate"] != "clear" || conf["percent"] != float64(96) || m["snapshot"].(map[string]any)["selection"] != "all_no_changes" {
		t.Fatalf("clean run: gate %v confidence %v snapshot %v", m["gate"], conf, m["snapshot"])
	}
	if len(srv.bodies) != 1 || srv.auth[0] != "Bearer "+testKey {
		t.Fatalf("requests %d auth %q", len(srv.bodies), srv.auth)
	}
	if strings.Contains(srv.bodies[0], "SECRET-ORACLE") || !strings.Contains(srv.bodies[0], `"only":["typesafe-ai"]`) {
		t.Fatalf("request leaked a test file or lost upstream pinning: %s", srv.bodies[0])
	}

	testutil.Write(t, dir, "src/checkout/finish.ts", badFinish)
	r = Run(env(t), srv.factory(), "--repo", dir, "--json")
	if r.ExitCode != 1 {
		t.Fatalf("violation exit %d\n%s\n%s", r.ExitCode, r.Stdout, r.Stderr)
	}
	m = validateRun(t, r.Stdout)
	first := m["checks"].([]any)[0].(map[string]any)
	if first["verdict"] != "violated" || first["locations"].([]any)[0].(map[string]any)["role"] != "model_selected" ||
		m["confidence"].(map[string]any)["percent"] != float64(2) || m["snapshot"].(map[string]any)["selection"] != "changed" {
		t.Fatalf("violation: %v confidence %v", first, m["confidence"])
	}

	r = Run(env(t), srv.factory(), "--repo", dir)
	if r.ExitCode != 1 || !strings.Contains(r.Stdout, "2% sure") || !strings.Contains(r.Stdout, "✗ checkout.failure") ||
		!strings.Contains(r.Stdout, "→ src/checkout/finish.ts") || strings.Contains(r.Stdout, "\x1b[") {
		t.Fatalf("human output:\n%s", r.Stdout)
	}
	r = Run(env(t), srv.factory(), "checkout", "--repo", dir, "--json")
	if r.ExitCode != 1 || validateRun(t, r.Stdout)["snapshot"].(map[string]any)["selection"] != "contract" {
		t.Fatalf("prefix selection: %+v", r)
	}
	r = Run(env(t), srv.factory(), "check", "--repo", dir, "--json")
	if r.ExitCode != 1 {
		t.Fatalf("check alias: %+v", r)
	}
}

func TestErrorsStayInsideTheJSONEnvelope(t *testing.T) {
	dir := repo(t)
	srv := newJevServer(t)
	cases := []struct {
		name string
		env  map[string]string
		args []string
		exit int
		code string
	}{
		{"all and names", env(t), []string{"--all", "checkout"}, 2, "invalid_flags"},
		{"bad provider", env(t), []string{"--provider", "openai"}, 2, "invalid_flags"},
		{"replay and provider", env(t), []string{"--replay", t.TempDir(), "--provider", "jev"}, 2, "invalid_flags"},
		{"missing key", map[string]string{"SURE_CACHE_DIR": t.TempDir()}, nil, 2, "missing_api_key"},
		{"unknown contract", env(t), []string{"nope"}, 2, "unknown_contract"},
		{"bad base", env(t), []string{"--base", "--upload-pack=x"}, 2, "invalid_base"},
	}
	for _, tc := range cases {
		args := append([]string{"--repo", dir, "--json"}, tc.args...)
		r := Run(tc.env, srv.factory(), args...)
		if r.ExitCode != tc.exit {
			t.Errorf("%s: exit %d want %d\n%s", tc.name, r.ExitCode, tc.exit, r.Stdout)
			continue
		}
		m := validateRun(t, r.Stdout)
		errs := m["errors"].([]any)
		if len(errs) == 0 || errs[0].(map[string]any)["code"] != tc.code {
			t.Errorf("%s: errors %v", tc.name, errs)
		}
	}
	if len(srv.bodies) != 0 {
		t.Fatalf("invalid invocations reached the provider %d times", len(srv.bodies))
	}

	srv.status = http.StatusTooManyRequests
	r := Run(env(t), srv.factory(), "--repo", dir, "--json", "--no-cache")
	if r.ExitCode != 11 {
		t.Fatalf("rate limit exit %d\n%s", r.ExitCode, r.Stdout)
	}
	validateRun(t, r.Stdout)
	if strings.Contains(r.Stdout+r.Stderr, testKey) {
		t.Fatal("the API key appeared in output")
	}
}

func TestReplayNeedsNoKeyAndNoNetwork(t *testing.T) {
	dir := repo(t)
	srv := newJevServer(t)
	rec := t.TempDir()
	if r := Run(env(t), srv.factory(), "--repo", dir, "--json", "--record", rec); r.ExitCode != 0 {
		t.Fatalf("record: %+v", r)
	}
	calls := len(srv.bodies)
	r := Run(map[string]string{"SURE_CACHE_DIR": t.TempDir()}, srv.factory(), "--repo", dir, "--json", "--replay", rec)
	if r.ExitCode != 0 || len(srv.bodies) != calls || validateRun(t, r.Stdout)["provider"].(map[string]any)["mode"] != "replay" {
		t.Fatalf("replay: exit %d calls %d->%d\n%s", r.ExitCode, calls, len(srv.bodies), r.Stdout)
	}
}

func TestInitValidateDryRunDoctorSchema(t *testing.T) {
	testutil.Isolate(t)
	dir := testutil.NewRepo(t)
	testutil.Write(t, dir, "src/a.ts", "export const a = 1;\n")
	testutil.Write(t, dir, ".sure/contract-template.md", "mine\n")

	r := Run(nil, shared.ProviderFactory{}, "init", "--repo", dir, "--json")
	m := singleJSON(t, r.Stdout)
	if r.ExitCode != 0 || len(m["created"].([]any)) != 3 || m["existing"].([]any)[0] != ".sure/contract-template.md" {
		t.Fatalf("init: %+v", r)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, ".sure/contract-template.md")); string(b) != "mine\n" {
		t.Fatal("init overwrote an existing file")
	}
	cfg, _ := os.ReadFile(filepath.Join(dir, ".sure/config.yaml"))
	if !strings.Contains(string(cfg), "  - src\n") {
		t.Fatalf("init did not detect src/: %s", cfg)
	}
	r = Run(nil, shared.ProviderFactory{}, "init", "--repo", dir)
	if r.ExitCode != 0 || !strings.Contains(r.Stdout, "(kept)") {
		t.Fatalf("second init: %+v", r)
	}

	testutil.Write(t, dir, ".sure/contracts/a.md", contractMD)
	testutil.Write(t, dir, "src/cart.ts", "x\n")
	testutil.Write(t, dir, "src/checkout/finish.ts", goodFinish)
	r = Run(nil, shared.ProviderFactory{}, "validate", "--repo", dir, "--json")
	if r.ExitCode != 0 || singleJSON(t, r.Stdout)["valid"] != true {
		t.Fatalf("validate: %+v", r)
	}
	testutil.Write(t, dir, ".sure/contracts/b.md", strings.Replace(contractMD, "severity: error", "severity: high", 1))
	r = Run(nil, shared.ProviderFactory{}, "validate", "--repo", dir)
	if r.ExitCode != 2 || !strings.Contains(r.Stdout, "severity must be error or warning") {
		t.Fatalf("invalid contract: %+v", r)
	}
	if err := os.Remove(filepath.Join(dir, ".sure/contracts/b.md")); err != nil {
		t.Fatal(err)
	}

	r = Run(nil, shared.ProviderFactory{}, "--repo", dir, "--dry-run", "--json")
	if r.ExitCode != 2 || !strings.Contains(r.Stdout, "no_trusted_base") {
		t.Fatalf("dry run before any commit: %+v", r)
	}
	r = Run(nil, shared.ProviderFactory{}, "--repo", dir, "--dry-run", "--draft", "--json")
	p := singleJSON(t, r.Stdout)
	if r.ExitCode != 0 || p["summary"].(map[string]any)["requests"] != float64(1) {
		t.Fatalf("draft dry run: %+v", r)
	}
	r = Run(nil, shared.ProviderFactory{}, "--repo", dir, "--dry-run", "--draft")
	if !strings.Contains(r.Stdout, "no model call made") {
		t.Fatalf("dry run human: %s", r.Stdout)
	}

	srv := newJevServer(t)
	r = Run(map[string]string{"SURE_CACHE_DIR": t.TempDir()}, srv.factory(), "doctor", "--repo", dir, "--json")
	d := singleJSON(t, r.Stdout)
	if r.ExitCode != 2 || d["ok"] != false || !strings.Contains(r.Stdout, "AI_GATEWAY_API_KEY is not set") {
		t.Fatalf("doctor without key: %+v", r)
	}
	r = Run(env(t), srv.factory(), "doctor", "--repo", dir, "--live")
	if r.ExitCode != 0 || !strings.Contains(r.Stdout, "live_probe") || len(srv.bodies) != 1 || strings.Contains(srv.bodies[0], "src/") {
		t.Fatalf("doctor --live: %+v bodies %v", r, srv.bodies)
	}

	r = Run(nil, shared.ProviderFactory{}, "schema")
	if r.ExitCode != 0 || !strings.Contains(r.Stdout, "urn:sure:run-result:v1") {
		t.Fatalf("schema: %+v", r)
	}
}
