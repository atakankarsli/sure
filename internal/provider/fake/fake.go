// Package fake is a deterministic in-memory provider for tests.
package fake

import (
	"context"
	"sync"

	"github.com/atakankarsli/sure/internal/provider"
)

var defaultID = provider.Identity{
	Name:              provider.NameFake,
	Transport:         "fake",
	Endpoint:          "fake://",
	Model:             "fake-model",
	WireFormat:        "fake-v1",
	ProbabilitySource: provider.ProbabilityModel,
}

type Fake struct {
	ID     provider.Identity
	Decide func(q provider.Question, req provider.Request) provider.Answer
	Err    error

	mu       sync.Mutex
	requests []provider.Request
}

func (f *Fake) Identity() provider.Identity {
	if f.ID == (provider.Identity{}) {
		return defaultID
	}
	return f.ID
}

func (f *Fake) Evaluate(ctx context.Context, req provider.Request) (provider.Response, error) {
	f.mu.Lock()
	f.requests = append(f.requests, req)
	f.mu.Unlock()
	if ce := provider.ContextError(ctx); ce != nil {
		return provider.Response{}, ce
	}
	if f.Err != nil {
		return provider.Response{}, f.Err
	}
	answers := make(map[string]provider.Answer, len(req.Questions))
	for _, q := range req.Questions {
		if f.Decide != nil {
			answers[q.ID] = f.Decide(q, req)
			continue
		}
		if len(q.Options) > 0 {
			answers[q.ID] = Answer(q, q.Options[0].Name, 0.96)
		}
	}
	if err := provider.ValidateAnswers(req, answers); err != nil {
		return provider.Response{}, err
	}
	return provider.Response{
		Model:   f.Identity().Model,
		Answers: answers,
		Usage:   provider.Usage{InputTokens: len(req.State) / 3},
		Source:  provider.SourceFake,
	}, nil
}

func (f *Fake) Requests() []provider.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]provider.Request(nil), f.requests...)
}

func (f *Fake) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

// Answer gives choice probability p and splits the rest evenly over the other
// options.
func Answer(q provider.Question, choice string, p float64) provider.Answer {
	probs := make(map[string]float64, len(q.Options))
	rest := 0.0
	if len(q.Options) > 1 {
		rest = (1 - p) / float64(len(q.Options)-1)
	}
	for _, o := range q.Options {
		probs[o.Name] = rest
	}
	probs[choice] = p
	return provider.Answer{Choice: choice, Probabilities: probs}
}
