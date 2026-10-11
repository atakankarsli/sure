package provider

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"testing"
	"time"
)

func question(id string, opts ...string) Question {
	q := Question{ID: id, Instructions: json.RawMessage(`{"task":"t"}`)}
	for _, o := range opts {
		q.Options = append(q.Options, Option{Name: o, Description: o})
	}
	return q
}

func TestValidateAnswers(t *testing.T) {
	req := Request{State: json.RawMessage(`{}`), Questions: []Question{question("q1", "a", "b", "c")}}
	good := Answer{Choice: "a", Probabilities: map[string]float64{"a": 0.9, "b": 0.05, "c": 0.05}}
	if err := ValidateAnswers(req, map[string]Answer{"q1": good}); err != nil {
		t.Fatal(err)
	}
	conf := 1.5
	cases := map[string]map[string]Answer{
		"missing answer": {},
		"extra answer":   {"q1": good, "q2": good},
		"missing option": {"q1": {Choice: "a", Probabilities: map[string]float64{"a": 0.95, "b": 0.05}}},
		"unknown option": {"q1": {Choice: "a", Probabilities: map[string]float64{"a": 0.9, "b": 0.05, "z": 0.05}}},
		"out of range":   {"q1": {Choice: "a", Probabilities: map[string]float64{"a": 1.5, "b": -0.25, "c": -0.25}}},
		"nan":            {"q1": {Choice: "a", Probabilities: map[string]float64{"a": math.NaN(), "b": 0.5, "c": 0.5}}},
		"bad sum":        {"q1": {Choice: "a", Probabilities: map[string]float64{"a": 0.5, "b": 0.1, "c": 0.1}}},
		"not argmax":     {"q1": {Choice: "b", Probabilities: map[string]float64{"a": 0.9, "b": 0.05, "c": 0.05}}},
		"choice not opt": {"q1": {Choice: "z", Probabilities: map[string]float64{"a": 0.9, "b": 0.05, "c": 0.05}}},
		"bad confidence": {"q1": {Choice: "a", Probabilities: good.Probabilities, Confidence: &conf}},
	}
	for name, answers := range cases {
		err := ValidateAnswers(req, answers)
		if KindOf(err) != ErrMalformed {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func TestRequestKeyChangesWithEveryInput(t *testing.T) {
	id := Identity{Name: "jev", Transport: "gateway", Endpoint: "https://x", Model: "m", WireFormat: "w"}
	req := Request{State: json.RawMessage(`{"a":1}`), Questions: []Question{question("q1", "a", "b")}}
	base := RequestKey(id, req, "v1")
	if RequestKey(id, req, "v1") != base {
		t.Fatal("key is not deterministic")
	}
	id2 := id
	id2.Endpoint = "https://y"
	req2 := req
	req2.State = json.RawMessage(`{"a":2}`)
	req3 := Request{State: req.State, Questions: []Question{question("q1", "a", "c")}}
	for name, k := range map[string]string{
		"endpoint": RequestKey(id2, req, "v1"),
		"state":    RequestKey(id, req2, "v1"),
		"options":  RequestKey(id, req3, "v1"),
		"extra":    RequestKey(id, req, "v2"),
	} {
		if k == base {
			t.Errorf("%s change kept the same key", name)
		}
	}
}

func TestDoRetriesOnlyTransientFailures(t *testing.T) {
	var slept []time.Duration
	pol := RetryPolicy{MaxRetries: 2, BaseDelay: time.Second, MaxDelay: 10 * time.Second, Gate: &Gate{},
		Sleep: func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil }}

	calls := 0
	_, err := Do(context.Background(), pol, func(context.Context) (int, error) {
		calls++
		return 0, &Error{Kind: ErrRateLimited, Status: 429, RetryAfter: 3 * time.Second}
	})
	if KindOf(err) != ErrRateLimited || calls != 3 {
		t.Fatalf("rate limit: kind %v calls %d", KindOf(err), calls)
	}
	if slept[0] != 3*time.Second {
		t.Fatalf("Retry-After not honored: %v", slept)
	}

	for _, kind := range []ErrorKind{ErrAuth, ErrRejected, ErrMalformed} {
		calls = 0
		_, err = Do(context.Background(), pol, func(context.Context) (int, error) {
			calls++
			return 0, Errorf(kind, "no")
		})
		if calls != 1 || KindOf(err) != kind {
			t.Errorf("%s retried %d times", kind, calls)
		}
	}

	calls = 0
	v, err := Do(context.Background(), pol, func(context.Context) (int, error) {
		calls++
		if calls < 2 {
			return 0, Errorf(ErrTransport, "blip")
		}
		return 7, nil
	})
	if err != nil || v != 7 {
		t.Fatalf("recovery: %v %v", v, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = Do(ctx, DefaultRetryPolicy(), func(context.Context) (int, error) { return 0, Errorf(ErrTransport, "x") })
	if KindOf(err) != ErrCanceled {
		t.Fatalf("canceled context: %v", err)
	}
}

func TestParseRetryAfterAndClassify(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	h := http.Header{}
	h.Set("Retry-After", "2.5")
	if d := ParseRetryAfter(h, now); d != 2500*time.Millisecond {
		t.Fatalf("seconds: %v", d)
	}
	h.Set("Retry-After", now.Add(4*time.Second).Format(http.TimeFormat))
	if d := ParseRetryAfter(h, now); d != 4*time.Second {
		t.Fatalf("date: %v", d)
	}
	h.Set("Retry-After", "soon")
	if d := ParseRetryAfter(h, now); d != 0 {
		t.Fatalf("garbage: %v", d)
	}
	want := map[int]ErrorKind{401: ErrAuth, 403: ErrAuth, 422: ErrRejected, 400: ErrRejected, 429: ErrRateLimited,
		529: ErrRateLimited, 500: ErrTransport, 503: ErrTransport, 408: ErrTransport}
	for status, kind := range want {
		if got := ClassifyStatus(status); got != kind {
			t.Errorf("%d: %s, want %s", status, got, kind)
		}
	}
}
