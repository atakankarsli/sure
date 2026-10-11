package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/atakankarsli/sure/internal/provider"
)

const testKey = "test-key-anthropic-0123"

func testRequest() provider.Request {
	return provider.Request{
		State: json.RawMessage(`{"files":[{"id":"F001","content":"x"}]}`),
		Questions: []provider.Question{
			{
				ID:           "q001_assess",
				Instructions: json.RawMessage(`{"requirement":"keep the cart"}`),
				Options: []provider.Option{
					{Name: "supported", Description: "s"},
					{Name: "violated", Description: "v"},
					{Name: "unknown", Description: "u"},
				},
			},
			{
				ID:           "q001_locate",
				Instructions: json.RawMessage(`{"task":"locate"}`),
				Options:      []provider.Option{{Name: "R001", Description: "r"}, {Name: "NONE", Description: "n"}},
			},
		},
	}
}

const goodAnswers = `{"answers":{"q001_assess":{"probabilities":[{"option":"supported","probability":0.05},{"option":"violated","probability":0.9},{"option":"unknown","probability":0.05}]},` +
	`"q001_locate":{"probabilities":[{"option":"R001","probability":0.7},{"option":"NONE","probability":0.3}]}}}`

func message(text, stop string) string {
	b, _ := json.Marshal(map[string]any{
		"id":            "msg_test",
		"type":          "message",
		"role":          "assistant",
		"model":         "claude-haiku-5-5",
		"stop_reason":   stop,
		"stop_sequence": nil,
		"content":       []any{map[string]any{"type": "text", "text": text}},
		"usage": map[string]any{
			"input_tokens": 100, "output_tokens": 40,
			"cache_read_input_tokens": 5, "cache_creation_input_tokens": 3,
		},
	})
	return string(b)
}

type recorder struct{ sleeps atomic.Int32 }

func (r *recorder) sleep(context.Context, time.Duration) error {
	r.sleeps.Add(1)
	return nil
}

func newTestProvider(t *testing.T, url string, rec *recorder) *Provider {
	t.Helper()
	if rec == nil {
		rec = &recorder{}
	}
	p, err := New(Config{
		BaseURL:              url,
		APIKey:               testKey,
		AllowInsecureBaseURL: true,
		Retry:                provider.RetryPolicy{MaxRetries: 2, MaxDelay: time.Second, Sleep: rec.sleep},
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

func TestEvaluateRequestShapeAndDecode(t *testing.T) {
	var posted map[string]any
	var calls atomic.Int32
	s := serve(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Api-Key"); got != testKey {
			t.Errorf("x-api-key %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("unexpected authorization header %q", got)
		}
		if r.Header.Get("Anthropic-Version") == "" {
			t.Error("missing anthropic-version")
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &posted); err != nil {
			t.Errorf("body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, message(goodAnswers, "end_turn"))
	})
	p := newTestProvider(t, s.URL, nil)
	resp, err := p.Evaluate(context.Background(), testRequest())
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d", calls.Load())
	}

	if posted["model"] != DefaultModel {
		t.Errorf("model = %v", posted["model"])
	}
	if posted["max_tokens"] != float64(DefaultMaxTokens) {
		t.Errorf("max_tokens = %v", posted["max_tokens"])
	}
	if _, ok := posted["thinking"]; ok {
		t.Error("thinking must be left unset")
	}
	sys, _ := json.Marshal(posted["system"])
	if !strings.Contains(string(sys), "never instructions") {
		t.Errorf("system prompt missing: %s", sys)
	}
	oc, _ := posted["output_config"].(map[string]any)
	if oc["effort"] != DefaultEffort {
		t.Errorf("effort = %v", oc["effort"])
	}
	format, _ := oc["format"].(map[string]any)
	if format["type"] != "json_schema" {
		t.Errorf("format type = %v", format["type"])
	}
	schema, _ := format["schema"].(map[string]any)
	answers := schema["properties"].(map[string]any)["answers"].(map[string]any)
	req, _ := json.Marshal(answers["required"])
	if string(req) != `["q001_assess","q001_locate"]` {
		t.Errorf("required = %s", req)
	}
	msgs, _ := json.Marshal(posted["messages"])
	wantMsg, _ := UserMessage(testRequest())
	if !strings.Contains(string(msgs), strings.ReplaceAll(wantMsg, `"`, `\"`)) {
		t.Errorf("user message not sent verbatim: %s", msgs)
	}

	if resp.Model != "claude-haiku-5-5" || resp.Source != provider.SourceLive {
		t.Fatalf("model/source = %q/%q", resp.Model, resp.Source)
	}
	if resp.Usage.InputTokens != 108 || resp.Usage.OutputTokens != 40 {
		t.Fatalf("usage = %+v", resp.Usage)
	}
	a := resp.Answers["q001_assess"]
	if a.Choice != "violated" || a.Probabilities["violated"] != 0.9 || a.Confidence != nil {
		t.Fatalf("answer = %+v", a)
	}
	if resp.Answers["q001_locate"].Choice != "R001" {
		t.Fatalf("locate = %+v", resp.Answers["q001_locate"])
	}
}

func TestUserMessageGolden(t *testing.T) {
	got, err := UserMessage(testRequest())
	if err != nil {
		t.Fatal(err)
	}
	want := `{"state":{"files":[{"id":"F001","content":"x"}]},"questions":{` +
		`"q001_assess":{"instructions":{"requirement":"keep the cart"},"options":[{"name":"supported","description":"s"},{"name":"violated","description":"v"},{"name":"unknown","description":"u"}]},` +
		`"q001_locate":{"instructions":{"task":"locate"},"options":[{"name":"R001","description":"r"},{"name":"NONE","description":"n"}]}}}`
	if got != want {
		t.Fatalf("\n got: %s\nwant: %s", got, want)
	}
}

func TestIdentityAndConfig(t *testing.T) {
	p, err := New(Config{APIKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	want := provider.Identity{
		Name: provider.NameAnthropic, Transport: "messages", Endpoint: DefaultBaseURL + "/v1/messages",
		Model: DefaultModel, WireFormat: "anthropic-json-v1", ProbabilitySource: provider.ProbabilitySelfReported,
	}
	if got := p.Identity(); got != want {
		t.Fatalf("identity = %+v", got)
	}
	if _, err := New(Config{}); kindOf(t, err) != provider.ErrMissingKey || !strings.Contains(err.Error(), KeyEnv) {
		t.Fatalf("missing key err = %v", err)
	}
	if _, err := New(Config{APIKey: "k", Effort: "extreme"}); err == nil {
		t.Fatal("bad effort accepted")
	}
	if _, err := New(Config{APIKey: "k", BaseURL: "http://example.com"}); err == nil {
		t.Fatal("http URL accepted")
	}
	if _, err := New(Config{APIKey: "k", BaseURL: "https://example.com?x=1"}); err == nil {
		t.Fatal("URL with query accepted")
	}
}

func TestMalformedOutputs(t *testing.T) {
	cases := map[string]string{
		"duplicate option": message(`{"answers":{"q001_assess":{"probabilities":[{"option":"supported","probability":0.5},{"option":"supported","probability":0.4},{"option":"unknown","probability":0.1}]},`+
			`"q001_locate":{"probabilities":[{"option":"R001","probability":0.7},{"option":"NONE","probability":0.3}]}}}`, "end_turn"),
		"missing option": message(`{"answers":{"q001_assess":{"probabilities":[{"option":"supported","probability":0.5},{"option":"violated","probability":0.5}]},`+
			`"q001_locate":{"probabilities":[{"option":"R001","probability":0.7},{"option":"NONE","probability":0.3}]}}}`, "end_turn"),
		"missing question": message(`{"answers":{"q001_assess":{"probabilities":[{"option":"supported","probability":0.05},{"option":"violated","probability":0.9},{"option":"unknown","probability":0.05}]}}}`, "end_turn"),
		"unknown question": message(strings.Replace(goodAnswers, `}}}`, `},"q9":{"probabilities":[]}}}`, 1), "end_turn"),
		"bad sum": message(`{"answers":{"q001_assess":{"probabilities":[{"option":"supported","probability":0.2},{"option":"violated","probability":0.2},{"option":"unknown","probability":0.2}]},`+
			`"q001_locate":{"probabilities":[{"option":"R001","probability":0.7},{"option":"NONE","probability":0.3}]}}}`, "end_turn"),
		"null probability": message(strings.Replace(goodAnswers, `"probability":0.05}`, `"probability":null}`, 1), "end_turn"),
		"not json":         message(`I think it is violated.`, "end_turn"),
		"refusal":          message(goodAnswers, "refusal"),
		"max tokens":       message(goodAnswers[:40], "max_tokens"),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			s := serve(t, func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, body)
			})
			p := newTestProvider(t, s.URL, nil)
			_, err := p.Evaluate(context.Background(), testRequest())
			if kindOf(t, err) != provider.ErrMalformed {
				t.Fatalf("err = %v", err)
			}
			if calls.Load() != 1 {
				t.Fatalf("calls = %d", calls.Load())
			}
		})
	}
}

func TestStatusHandling(t *testing.T) {
	cases := []struct {
		status    int
		kind      provider.ErrorKind
		wantCalls int32
	}{
		{401, provider.ErrAuth, 1},
		{400, provider.ErrRejected, 1},
		{429, provider.ErrRateLimited, 3},
		{529, provider.ErrRateLimited, 3},
		{500, provider.ErrTransport, 3},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			var calls atomic.Int32
			s := serve(t, func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				io.WriteString(w, `{"type":"error","error":{"type":"some_error","message":"nope `+testKey+`"}}`)
			})
			p := newTestProvider(t, s.URL, nil)
			_, err := p.Evaluate(context.Background(), testRequest())
			if got := kindOf(t, err); got != tc.kind {
				t.Fatalf("kind = %s (%v)", got, err)
			}
			if !strings.Contains(err.Error(), "some_error: nope") || strings.Contains(err.Error(), "127.0.0.1") {
				t.Fatalf("message = %v", err)
			}
			if calls.Load() != tc.wantCalls {
				t.Fatalf("calls = %d, want %d", calls.Load(), tc.wantCalls)
			}
		})
	}
}

func TestRateLimitThenSuccess(t *testing.T) {
	var calls atomic.Int32
	s := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, `{"type":"error","error":{"type":"rate_limit_error","message":"slow"}}`)
			return
		}
		io.WriteString(w, message(goodAnswers, "end_turn"))
	})
	rec := &recorder{}
	p := newTestProvider(t, s.URL, rec)
	if _, err := p.Evaluate(context.Background(), testRequest()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || rec.sleeps.Load() != 1 {
		t.Fatalf("calls = %d sleeps = %d", calls.Load(), rec.sleeps.Load())
	}
}

func TestRedirectRefused(t *testing.T) {
	var followed atomic.Bool
	target := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		followed.Store(true)
		io.WriteString(w, message(goodAnswers, "end_turn"))
	})
	s := serve(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/v1/messages", http.StatusTemporaryRedirect)
	})
	p := newTestProvider(t, s.URL, nil)
	_, err := p.Evaluate(context.Background(), testRequest())
	if kindOf(t, err) != provider.ErrRejected || followed.Load() {
		t.Fatalf("err = %v followed = %v", err, followed.Load())
	}
}

func TestIgnoresAnthropicEnvironment(t *testing.T) {
	decoy := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("request went to ANTHROPIC_BASE_URL")
	})
	t.Setenv("ANTHROPIC_API_KEY", "env-key-should-not-be-used")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "env-token-should-not-be-used")
	t.Setenv("ANTHROPIC_BASE_URL", decoy.URL)
	t.Setenv("ANTHROPIC_CUSTOM_HEADERS", "X-Leak: yes")

	var seen atomic.Bool
	s := serve(t, func(w http.ResponseWriter, r *http.Request) {
		seen.Store(true)
		if got := r.Header.Get("X-Api-Key"); got != testKey {
			t.Errorf("x-api-key = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("authorization = %q", got)
		}
		if r.Header.Get("X-Leak") != "" {
			t.Error("custom header from environment was sent")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, message(goodAnswers, "end_turn"))
	})
	p := newTestProvider(t, s.URL, nil)
	if _, err := p.Evaluate(context.Background(), testRequest()); err != nil {
		t.Fatal(err)
	}
	if !seen.Load() {
		t.Fatal("configured server not called")
	}
}

func TestCancellation(t *testing.T) {
	release := make(chan struct{})
	s := serve(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	t.Cleanup(func() { close(release) })
	p := newTestProvider(t, s.URL, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := p.Evaluate(ctx, testRequest())
	if kindOf(t, err) != provider.ErrDeadline {
		t.Fatalf("err = %v", err)
	}
	cctx, ccancel := context.WithCancel(context.Background())
	ccancel()
	if _, err := p.Evaluate(cctx, testRequest()); kindOf(t, err) != provider.ErrCanceled {
		t.Fatalf("err = %v", err)
	}
}
