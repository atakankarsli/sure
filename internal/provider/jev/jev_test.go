package jev

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/atakankarsli/sure/internal/provider"
)

const testKey = "test-key-0123456789"

func testRequest() provider.Request {
	return provider.Request{
		State: json.RawMessage(`{"files": [{"id":"F001","content":"x"}]}`),
		Questions: []provider.Question{
			{
				ID:           "q002_assess",
				Instructions: json.RawMessage(`{"task":"assess","requirement":"cart <kept>"}`),
				Options: []provider.Option{
					{Name: "supported", Description: "s"},
					{Name: "violated", Description: "v"},
					{Name: "unknown", Description: "u"},
				},
			},
			{
				ID:           "q001_locate",
				Instructions: json.RawMessage(`{"task":"locate"}`),
				Options: []provider.Option{
					{Name: "R001", Description: "a.ts lines 1-3"},
					{Name: "NONE", Description: "none"},
				},
			},
		},
	}
}

const goodResponse = `{
  "model": "jev-1.13.0",
  "answers": {
    "q002_assess": {"type":"choice","choice":"violated","probabilities":{"supported":0.02,"violated":0.95,"unknown":0.03},"confidence":0.9},
    "q001_locate": {"type":"choice","choice":"R001","probabilities":{"R001":0.8,"NONE":0.2},"confidence":0.6}
  },
  "usage": {"input_tokens": 120, "output_tokens": 0}
}`

type recorder struct {
	mu     sync.Mutex
	sleeps []time.Duration
}

func (r *recorder) sleep(_ context.Context, d time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sleeps = append(r.sleeps, d)
	return nil
}

func newTestProvider(t *testing.T, transport, model, url string, rec *recorder) *Provider {
	t.Helper()
	if rec == nil {
		rec = &recorder{}
	}
	p, err := New(Config{
		Transport:            transport,
		Model:                model,
		BaseURL:              url,
		APIKey:               testKey,
		AllowInsecureBaseURL: true,
		Retry:                provider.RetryPolicy{MaxRetries: 2, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Second, Sleep: rec.sleep},
	})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func serve(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	return s
}

func reply(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		io.WriteString(w, body)
	}
}

func kindOf(t *testing.T, err error) provider.ErrorKind {
	t.Helper()
	var pe *provider.Error
	if !errors.As(err, &pe) {
		t.Fatalf("error %v (%T) is not a *provider.Error", err, err)
	}
	if strings.Contains(err.Error(), testKey) {
		t.Fatalf("error leaks the key: %v", err)
	}
	return pe.Kind
}

func TestBodyGolden(t *testing.T) {
	req := testRequest()
	want := `{"model":"jev-1.13.0","state":{"files":[{"id":"F001","content":"x"}]},"questions":{` +
		`"q002_assess":{"type":"choice","instructions":{"task":"assess","requirement":"cart <kept>"},"criteria":{"supported":"s","violated":"v","unknown":"u"}},` +
		`"q001_locate":{"type":"choice","instructions":{"task":"locate"},"criteria":{"R001":"a.ts lines 1-3","NONE":"none"}}}}`
	got, err := Body(TransportTypeSafe, "jev-1.13.0", req)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("typesafe body\n got: %s\nwant: %s", got, want)
	}
	gw, err := Body(TransportGateway, "typesafe-ai/jev", req)
	if err != nil {
		t.Fatal(err)
	}
	wantGW := strings.Replace(want, `"jev-1.13.0"`, `"typesafe-ai/jev"`, 1)
	wantGW = strings.TrimSuffix(wantGW, "}") + `,"providerOptions":{"gateway":{"only":["typesafe-ai"]}}}`
	if string(gw) != wantGW {
		t.Fatalf("gateway body\n got: %s\nwant: %s", gw, wantGW)
	}
	if !json.Valid(gw) {
		t.Fatal("gateway body is not valid JSON")
	}
}

func TestBodyRejectsBadRequests(t *testing.T) {
	cases := map[string]func(*provider.Request){
		"no questions":     func(r *provider.Request) { r.Questions = nil },
		"empty state":      func(r *provider.Request) { r.State = nil },
		"duplicate id":     func(r *provider.Request) { r.Questions[1].ID = r.Questions[0].ID },
		"no options":       func(r *provider.Request) { r.Questions[0].Options = nil },
		"duplicate option": func(r *provider.Request) { r.Questions[1].Options[1].Name = "R001" },
		"bad instructions": func(r *provider.Request) { r.Questions[0].Instructions = json.RawMessage(`{`) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			req := testRequest()
			mutate(&req)
			if _, err := Body(TransportGateway, "m", req); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestEvaluateSuccessAndHeaders(t *testing.T) {
	var gotBody []byte
	s := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+testKey {
			t.Errorf("authorization %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("content-type %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("accept %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != "sure" {
			t.Errorf("user-agent %q", got)
		}
		gotBody, _ = io.ReadAll(r.Body)
		io.WriteString(w, goodResponse)
	})
	p := newTestProvider(t, TransportTypeSafe, "", s.URL, nil)
	resp, err := p.Evaluate(context.Background(), testRequest())
	if err != nil {
		t.Fatal(err)
	}
	want, _ := Body(TransportTypeSafe, DefaultTypeSafeModel, testRequest())
	if string(gotBody) != string(want) {
		t.Fatalf("posted body differs:\n%s\n%s", gotBody, want)
	}
	if resp.Model != "jev-1.13.0" || resp.Source != provider.SourceLive {
		t.Fatalf("model/source = %q/%q", resp.Model, resp.Source)
	}
	if resp.Usage.InputTokens != 120 || resp.Usage.OutputTokens != 0 {
		t.Fatalf("usage = %+v", resp.Usage)
	}
	a := resp.Answers["q002_assess"]
	if a.Choice != "violated" || a.Probabilities["violated"] != 0.95 || a.Confidence == nil || *a.Confidence != 0.9 {
		t.Fatalf("answer = %+v", a)
	}
}

func TestIdentity(t *testing.T) {
	p := newTestProvider(t, TransportGateway, "", "http://127.0.0.1:1/v1/evaluate", nil)
	id := p.Identity()
	want := provider.Identity{
		Name: provider.NameJev, Transport: TransportGateway, Endpoint: "http://127.0.0.1:1/v1/evaluate",
		Model: DefaultGatewayModel, WireFormat: "jev-gateway-v1", ProbabilitySource: provider.ProbabilityModel,
	}
	if id != want {
		t.Fatalf("identity = %+v", id)
	}
	p2, err := New(Config{Transport: TransportTypeSafe, APIKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	if id := p2.Identity(); id.Endpoint != DefaultTypeSafeURL || id.Model != DefaultTypeSafeModel || id.WireFormat != "jev-typesafe-v1" {
		t.Fatalf("typesafe identity = %+v", id)
	}
}

func TestNewValidation(t *testing.T) {
	_, err := New(Config{Transport: "openai", APIKey: "k"})
	if err == nil {
		t.Fatal("unknown transport accepted")
	}
	_, err = New(Config{Transport: TransportGateway})
	if kindOf(t, err) != provider.ErrMissingKey || !strings.Contains(err.Error(), KeyEnvGateway) {
		t.Fatalf("missing key error = %v", err)
	}
	_, err = New(Config{Transport: TransportTypeSafe, APIKey: "  "})
	if kindOf(t, err) != provider.ErrMissingKey || !strings.Contains(err.Error(), KeyEnvTypeSafe) {
		t.Fatalf("missing key error = %v", err)
	}
	for _, u := range []string{
		"http://example.com/v1/evaluate",
		"https://user:pw@example.com/x",
		"https://example.com/x?a=1",
		"https://example.com/x#f",
		"https:///nohost",
		"::",
	} {
		if _, err := New(Config{Transport: TransportGateway, APIKey: "k", BaseURL: u}); err == nil {
			t.Errorf("URL %q accepted", u)
		}
	}
	if _, err := New(Config{Transport: TransportGateway, APIKey: "k", BaseURL: "http://127.0.0.1/x", AllowInsecureBaseURL: true}); err != nil {
		t.Fatalf("insecure URL with AllowInsecureBaseURL: %v", err)
	}
}

func TestRetryAfterThenSuccess(t *testing.T) {
	var calls atomic.Int32
	s := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, `{"error":{"message":"slow down","type":"rate_limited"}}`)
			return
		}
		io.WriteString(w, goodResponse)
	})
	rec := &recorder{}
	p := newTestProvider(t, TransportTypeSafe, "", s.URL, rec)
	if _, err := p.Evaluate(context.Background(), testRequest()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d", calls.Load())
	}
	if len(rec.sleeps) != 1 || rec.sleeps[0] != 2*time.Second {
		t.Fatalf("sleeps = %v, want [2s]", rec.sleeps)
	}
}

func TestRetryStatuses(t *testing.T) {
	cases := []struct {
		status    int
		wantKind  provider.ErrorKind
		wantCalls int32
	}{
		{529, provider.ErrRateLimited, 3},
		{429, provider.ErrRateLimited, 3},
		{500, provider.ErrTransport, 3},
		{503, provider.ErrTransport, 3},
		{401, provider.ErrAuth, 1},
		{403, provider.ErrAuth, 1},
		{422, provider.ErrRejected, 1},
		{400, provider.ErrRejected, 1},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			var calls atomic.Int32
			s := serve(t, func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(tc.status)
				io.WriteString(w, `{"detail":{"message":"field questions.q1 is bad"}}`)
			})
			p := newTestProvider(t, TransportTypeSafe, "", s.URL, nil)
			_, err := p.Evaluate(context.Background(), testRequest())
			if got := kindOf(t, err); got != tc.wantKind {
				t.Fatalf("kind = %s, want %s (%v)", got, tc.wantKind, err)
			}
			var pe *provider.Error
			errors.As(err, &pe)
			if pe.Status != tc.status {
				t.Fatalf("status = %d", pe.Status)
			}
			if !strings.Contains(pe.Message, "field questions.q1 is bad") {
				t.Fatalf("message = %q", pe.Message)
			}
			if calls.Load() != tc.wantCalls {
				t.Fatalf("calls = %d, want %d", calls.Load(), tc.wantCalls)
			}
		})
	}
}

func TestErrorExcerpt(t *testing.T) {
	cases := map[string]string{
		`{"error":{"message":"AI Gateway requires a valid credit card","type":"customer_verification_required"}}`: "customer_verification_required: AI Gateway requires a valid credit card",
		`{"detail":"Cannot authenticate with the server"}`:                                                        "Cannot authenticate with the server",
		`{"detail":[{"loc":["body","questions"],"msg":"bad"}]}`:                                                   `[{"loc":["body","questions"],"msg":"bad"}]`,
		`not json`: "Unauthorized",
	}
	for body, want := range cases {
		if got := errorExcerpt([]byte(body), 401); got != want {
			t.Errorf("excerpt(%s) = %q, want %q", body, got, want)
		}
	}
	long := `{"detail":"` + strings.Repeat("é", 400) + `"}`
	if got := errorExcerpt([]byte(long), 500); len(got) > maxExcerptBytes+len("…") {
		t.Fatalf("excerpt not truncated: %d bytes", len(got))
	}
}

func TestRedirectNotFollowed(t *testing.T) {
	var followed atomic.Bool
	target := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		followed.Store(true)
		io.WriteString(w, goodResponse)
	})
	s := serve(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	})
	p, err := New(Config{Transport: TransportTypeSafe, BaseURL: s.URL, APIKey: testKey, AllowInsecureBaseURL: true,
		HTTPClient: provider.NewHTTPClient(5 * time.Second),
		Retry:      provider.RetryPolicy{MaxRetries: 2, Sleep: (&recorder{}).sleep}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Evaluate(context.Background(), testRequest())
	if kindOf(t, err) != provider.ErrRejected || followed.Load() {
		t.Fatalf("err = %v, followed = %v", err, followed.Load())
	}
}

func TestMalformedResponses(t *testing.T) {
	assess := func(s string) string {
		return `{"model":"jev-1.13.0","answers":{"q002_assess":` + s +
			`,"q001_locate":{"type":"choice","choice":"R001","probabilities":{"R001":0.8,"NONE":0.2},"confidence":0.6}},"usage":{"input_tokens":1,"output_tokens":0}}`
	}
	cases := map[string]string{
		"missing answer": `{"model":"jev-1.13.0","answers":{"q001_locate":{"type":"choice","choice":"R001","probabilities":{"R001":0.8,"NONE":0.2},"confidence":0.6}},"usage":{"input_tokens":1,"output_tokens":0}}`,
		"extra answer": `{"model":"jev-1.13.0","answers":{"q002_assess":{"type":"choice","choice":"violated","probabilities":{"supported":0.02,"violated":0.95,"unknown":0.03},"confidence":0.9},` +
			`"q001_locate":{"type":"choice","choice":"R001","probabilities":{"R001":0.8,"NONE":0.2},"confidence":0.6},"q999":{"type":"choice","choice":"a","probabilities":{"a":1},"confidence":1}},"usage":{"input_tokens":1,"output_tokens":0}}`,
		"missing confidence":   assess(`{"type":"choice","choice":"violated","probabilities":{"supported":0.02,"violated":0.95,"unknown":0.03}}`),
		"null probability":     assess(`{"type":"choice","choice":"violated","probabilities":{"supported":null,"violated":0.95,"unknown":0.05},"confidence":0.9}`),
		"missing probability":  assess(`{"type":"choice","choice":"violated","probabilities":{"violated":0.95,"unknown":0.05},"confidence":0.9}`),
		"string probability":   assess(`{"type":"choice","choice":"violated","probabilities":{"supported":"0.02","violated":0.95,"unknown":0.03},"confidence":0.9}`),
		"probability 1.5":      assess(`{"type":"choice","choice":"violated","probabilities":{"supported":0,"violated":1.5,"unknown":0},"confidence":0.9}`),
		"sums to 0.5":          assess(`{"type":"choice","choice":"violated","probabilities":{"supported":0.1,"violated":0.3,"unknown":0.1},"confidence":0.9}`),
		"choice not argmax":    assess(`{"type":"choice","choice":"supported","probabilities":{"supported":0.02,"violated":0.95,"unknown":0.03},"confidence":0.9}`),
		"choice not an option": assess(`{"type":"choice","choice":"maybe","probabilities":{"supported":0.02,"violated":0.95,"unknown":0.03},"confidence":0.9}`),
		"wrong type":           assess(`{"type":"noul","noul":0.9}`),
		"missing choice":       assess(`{"type":"choice","probabilities":{"supported":0.02,"violated":0.95,"unknown":0.03},"confidence":0.9}`),
		"null answer":          assess(`null`),
		"confidence 2":         assess(`{"type":"choice","choice":"violated","probabilities":{"supported":0.02,"violated":0.95,"unknown":0.03},"confidence":2}`),
		"negative usage":       strings.Replace(goodResponse, `"input_tokens": 120`, `"input_tokens": -1`, 1),
		"fractional usage":     strings.Replace(goodResponse, `"input_tokens": 120`, `"input_tokens": 1.5`, 1),
		"missing usage":        strings.Replace(goodResponse, `"usage": {"input_tokens": 120, "output_tokens": 0}`, `"x": 1`, 1),
		"missing output usage": strings.Replace(goodResponse, `, "output_tokens": 0`, ``, 1),
		"missing model":        strings.Replace(goodResponse, `"model": "jev-1.13.0",`, ``, 1),
		"empty model":          strings.Replace(goodResponse, `"model": "jev-1.13.0"`, `"model": ""`, 1),
		"pinned mismatch":      strings.Replace(goodResponse, `"model": "jev-1.13.0"`, `"model": "jev-1.14.0"`, 1),
		"missing answers":      `{"model":"jev-1.13.0","usage":{"input_tokens":1,"output_tokens":0}}`,
		"not json":             `<html>oops</html>`,
		"trailing data":        goodResponse + `{}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			s := serve(t, func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				io.WriteString(w, body)
			})
			p := newTestProvider(t, TransportTypeSafe, "", s.URL, nil)
			_, err := p.Evaluate(context.Background(), testRequest())
			if got := kindOf(t, err); got != provider.ErrMalformed {
				t.Fatalf("kind = %s (%v)", got, err)
			}
			if calls.Load() != 1 {
				t.Fatalf("malformed response retried: %d calls", calls.Load())
			}
		})
	}
}

func TestAliasAcceptsAnyReturnedModel(t *testing.T) {
	body := strings.Replace(goodResponse, `"model": "jev-1.13.0"`, `"model": "jev-1.14.2"`, 1)
	s := serve(t, reply(200, body))
	for _, m := range []string{"jev-latest", "typesafe-ai/jev"} {
		p := newTestProvider(t, TransportGateway, m, s.URL, nil)
		resp, err := p.Evaluate(context.Background(), testRequest())
		if err != nil {
			t.Fatalf("%s: %v", m, err)
		}
		if resp.Model != "jev-1.14.2" {
			t.Fatalf("model = %q", resp.Model)
		}
	}
}

func TestOversizedResponse(t *testing.T) {
	s := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"model":"`))
		w.Write([]byte(strings.Repeat("a", maxResponseBytes)))
		w.Write([]byte(`"}`))
	})
	p := newTestProvider(t, TransportTypeSafe, "", s.URL, nil)
	_, err := p.Evaluate(context.Background(), testRequest())
	if kindOf(t, err) != provider.ErrMalformed {
		t.Fatalf("err = %v", err)
	}
}

func TestCancellationAndDeadline(t *testing.T) {
	release := make(chan struct{})
	s := serve(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	t.Cleanup(func() { close(release) })
	p := newTestProvider(t, TransportTypeSafe, "", s.URL, nil)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	_, err := p.Evaluate(ctx, testRequest())
	if kindOf(t, err) != provider.ErrCanceled {
		t.Fatalf("cancel err = %v", err)
	}

	dctx, dcancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer dcancel()
	_, err = p.Evaluate(dctx, testRequest())
	if kindOf(t, err) != provider.ErrDeadline {
		t.Fatalf("deadline err = %v", err)
	}

	done, cancelDone := context.WithCancel(context.Background())
	cancelDone()
	_, err = p.Evaluate(done, testRequest())
	if kindOf(t, err) != provider.ErrCanceled {
		t.Fatalf("pre-canceled err = %v", err)
	}
}

func TestNetworkErrorRetried(t *testing.T) {
	s := httptest.NewServer(http.NotFoundHandler())
	url := s.URL
	s.Close()
	rec := &recorder{}
	p := newTestProvider(t, TransportTypeSafe, "", url, rec)
	_, err := p.Evaluate(context.Background(), testRequest())
	if kindOf(t, err) != provider.ErrTransport {
		t.Fatalf("err = %v", err)
	}
	if len(rec.sleeps) != 2 {
		t.Fatalf("expected 2 retries, slept %v", rec.sleeps)
	}
}

func TestKeyRedactedFromErrorBody(t *testing.T) {
	s := serve(t, reply(401, `{"detail":"bad key `+testKey+`"}`))
	p := newTestProvider(t, TransportTypeSafe, "", s.URL, nil)
	_, err := p.Evaluate(context.Background(), testRequest())
	if kindOf(t, err) != provider.ErrAuth || !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("err = %v", err)
	}
}
