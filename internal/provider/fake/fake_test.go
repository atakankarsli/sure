package fake

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"

	"github.com/atakankarsli/sure/internal/provider"
)

func request() provider.Request {
	return provider.Request{
		State: json.RawMessage(`{"a":1}`),
		Questions: []provider.Question{{
			ID:           "q1",
			Instructions: json.RawMessage(`"x"`),
			Options:      []provider.Option{{Name: "supported"}, {Name: "violated"}, {Name: "unknown"}},
		}},
	}
}

func TestDefaultAndDecide(t *testing.T) {
	f := &Fake{}
	resp, err := f.Evaluate(context.Background(), request())
	if err != nil {
		t.Fatal(err)
	}
	if a := resp.Answers["q1"]; a.Choice != "supported" || a.Probabilities["supported"] != 0.96 || math.Abs(a.Probabilities["violated"]-0.02) > 1e-9 {
		t.Fatalf("answer = %+v", a)
	}
	if resp.Source != provider.SourceFake || f.Identity().Name != provider.NameFake || f.Calls() != 1 {
		t.Fatalf("resp = %+v", resp)
	}

	f2 := &Fake{Decide: func(q provider.Question, _ provider.Request) provider.Answer { return Answer(q, "violated", 0.9) }}
	resp, err = f2.Evaluate(context.Background(), request())
	if err != nil || resp.Answers["q1"].Choice != "violated" {
		t.Fatalf("resp = %+v err = %v", resp, err)
	}
}

func TestErrAndInvalidDecision(t *testing.T) {
	want := provider.Errorf(provider.ErrTransport, "down")
	f := &Fake{Err: want}
	if _, err := f.Evaluate(context.Background(), request()); !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
	bad := &Fake{Decide: func(provider.Question, provider.Request) provider.Answer {
		return provider.Answer{Choice: "supported", Probabilities: map[string]float64{"supported": 1}}
	}}
	if _, err := bad.Evaluate(context.Background(), request()); provider.KindOf(err) != provider.ErrMalformed {
		t.Fatalf("err = %v", err)
	}
	if len(bad.Requests()) != 1 {
		t.Fatal("request not recorded")
	}
}
