// Package provider defines the decision-provider interface shared by the Jev,
// Claude, replay and fake implementations, plus response validation that every
// implementation applies before an answer reaches the engine.
package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"
)

const (
	NameJev       = "jev"
	NameAnthropic = "anthropic"
	NameReplay    = "replay"
	NameFake      = "fake"

	ProbabilityModel        = "model_distribution"
	ProbabilitySelfReported = "self_reported"

	SourceLive   = "live"
	SourceCache  = "cache"
	SourceReplay = "replay"
	SourceFake   = "fake"
)

// Option is one labelled answer of a choice question. Order is significant:
// providers present options in exactly this order.
type Option struct {
	Name        string
	Description string
}

// Question is a single choice question. ID is a correlation key only; the
// requirement text must be inside Instructions.
type Question struct {
	ID           string
	Instructions json.RawMessage
	Options      []Option
}

func (q Question) OptionNames() []string {
	names := make([]string, len(q.Options))
	for i, o := range q.Options {
		names[i] = o.Name
	}
	return names
}

// Request is the provider-neutral decision request. State must be canonical
// JSON so identical evidence produces identical bytes.
type Request struct {
	State     json.RawMessage
	Questions []Question
}

// Answer carries a complete probability distribution over the question's
// options. Choice is the provider's selected option.
type Answer struct {
	Choice        string             `json:"choice"`
	Probabilities map[string]float64 `json:"probabilities"`
	Confidence    *float64           `json:"confidence,omitempty"`
	// Refused means the model declined this question; it carries no choice.
	Refused bool `json:"refused,omitempty"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Response is a validated decision for every question of a request.
type Response struct {
	Model   string            `json:"model"`
	Answers map[string]Answer `json:"answers"`
	Usage   Usage             `json:"usage"`
	Source  string            `json:"-"`
}

// Identity describes where decisions come from. Everything here is part of the
// cache key, so a change of endpoint, model or wire format never reuses a
// decision made under different conditions.
type Identity struct {
	Name              string `json:"name"`
	Transport         string `json:"transport"`
	Endpoint          string `json:"endpoint"`
	Model             string `json:"model"`
	WireFormat        string `json:"wire_format"`
	ProbabilitySource string `json:"probability_source"`
	LongContext       bool   `json:"long_context,omitempty"`
}

type Provider interface {
	Identity() Identity
	Evaluate(ctx context.Context, req Request) (Response, error)
}

type ErrorKind string

const (
	ErrTransport   ErrorKind = "provider_unavailable"
	ErrAuth        ErrorKind = "provider_auth"
	ErrRejected    ErrorKind = "provider_rejected_request"
	ErrRateLimited ErrorKind = "provider_rate_limited"
	ErrDeadline    ErrorKind = "deadline_exceeded"
	ErrMalformed   ErrorKind = "malformed_decision"
	ErrCanceled    ErrorKind = "canceled"
	ErrReplayMiss  ErrorKind = "replay_miss"
	ErrMissingKey  ErrorKind = "missing_api_key"
)

// Error is the only error type providers return. Message never contains a
// credential or request body.
type Error struct {
	Kind       ErrorKind
	Status     int
	Message    string
	RetryAfter time.Duration
}

func (e *Error) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("%s (HTTP %d): %s", e.Kind, e.Status, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Kind, e.Message)
}

func Errorf(kind ErrorKind, format string, args ...any) *Error {
	return &Error{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

// KindOf classifies any error returned while evaluating, including context
// errors that escaped a provider.
func KindOf(err error) ErrorKind {
	var pe *Error
	switch {
	case err == nil:
		return ""
	case errors.As(err, &pe):
		return pe.Kind
	case errors.Is(err, context.Canceled):
		return ErrCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return ErrDeadline
	default:
		return ErrTransport
	}
}

// ContextError converts a finished context into the matching provider error.
func ContextError(ctx context.Context) *Error {
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return Errorf(ErrDeadline, "run deadline exceeded")
	case ctx.Err() != nil:
		return Errorf(ErrCanceled, "canceled")
	}
	return nil
}

// SumTolerance is how far a distribution may drift from 1.0 before it is
// rejected as implausible.
const SumTolerance = 0.02

// ValidateAnswers checks that answers cover exactly the request's questions and
// that each is a complete, finite distribution over the question's options whose
// choice is a highest-probability option.
func ValidateAnswers(req Request, answers map[string]Answer) error {
	want := make(map[string]Question, len(req.Questions))
	for _, q := range req.Questions {
		want[q.ID] = q
	}
	for id := range answers {
		if _, ok := want[id]; !ok {
			return Errorf(ErrMalformed, "answer for unknown question %q", id)
		}
	}
	ids := make([]string, 0, len(want))
	for id := range want {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		a, ok := answers[id]
		if !ok {
			return Errorf(ErrMalformed, "missing answer for question %q", id)
		}
		if err := validateAnswer(want[id], a); err != nil {
			return Errorf(ErrMalformed, "question %q: %s", id, err)
		}
	}
	return nil
}

func validateAnswer(q Question, a Answer) error {
	if a.Refused {
		if a.Choice != "" || len(a.Probabilities) > 0 {
			return fmt.Errorf("a refusal carries a choice")
		}
		return nil
	}
	if len(a.Probabilities) != len(q.Options) {
		return fmt.Errorf("expected %d probabilities, got %d", len(q.Options), len(a.Probabilities))
	}
	sum := 0.0
	best := math.Inf(-1)
	for _, o := range q.Options {
		p, ok := a.Probabilities[o.Name]
		if !ok {
			return fmt.Errorf("missing probability for option %q", o.Name)
		}
		if math.IsNaN(p) || math.IsInf(p, 0) || p < 0 || p > 1 {
			return fmt.Errorf("probability for %q out of range: %v", o.Name, p)
		}
		sum += p
		best = math.Max(best, p)
	}
	if math.Abs(sum-1) > SumTolerance {
		return fmt.Errorf("probabilities sum to %.4f", sum)
	}
	chosen, ok := a.Probabilities[a.Choice]
	if !ok {
		return fmt.Errorf("choice %q is not an option", a.Choice)
	}
	if chosen < best-1e-9 {
		return fmt.Errorf("choice %q is not a highest-probability option", a.Choice)
	}
	if a.Confidence != nil {
		c := *a.Confidence
		if math.IsNaN(c) || c < 0 || c > 1 {
			return fmt.Errorf("confidence out of range: %v", c)
		}
	}
	return nil
}

// ArgMax returns the first option, in question order, with the highest
// probability.
func ArgMax(q Question, probs map[string]float64) string {
	best, bestP := "", math.Inf(-1)
	for _, o := range q.Options {
		if p := probs[o.Name]; p > bestP {
			best, bestP = o.Name, p
		}
	}
	return best
}

// RequestKey is the content address of a decision: provider identity, the exact
// state and questions, and any extra version strings the caller supplies.
// Cache and replay both key on it.
func RequestKey(id Identity, req Request, extra ...string) string {
	h := sha256.New()
	write := func(s string) {
		fmt.Fprintf(h, "%d:%s\n", len(s), s)
	}
	write("sure.request-key/v1")
	for _, s := range []string{id.Name, id.Transport, id.Endpoint, id.Model, id.WireFormat} {
		write(s)
	}
	write(string(req.State))
	for _, q := range req.Questions {
		write(q.ID)
		write(string(q.Instructions))
		for _, o := range q.Options {
			write(o.Name)
			write(o.Description)
		}
	}
	for _, s := range extra {
		write(s)
	}
	return hex.EncodeToString(h.Sum(nil))
}
