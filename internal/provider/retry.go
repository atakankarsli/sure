package provider

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RetryPolicy bounds retries of transient failures. Every worker of one run
// shares the same Gate, so a rate-limit response pauses all of them.
type RetryPolicy struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	Gate       *Gate
	Sleep      func(context.Context, time.Duration) error
	Jitter     func() float64
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries: 4,
		BaseDelay:  time.Second,
		MaxDelay:   30 * time.Second,
		Gate:       &Gate{},
		Sleep:      SleepContext,
		Jitter:     rand.Float64,
	}
}

func SleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Gate holds a shared "do not send before" time.
type Gate struct {
	mu    sync.Mutex
	until time.Time
}

func (g *Gate) pauseFor(d time.Duration) {
	if g == nil || d <= 0 {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if t := time.Now().Add(d); t.After(g.until) {
		g.until = t
	}
}

func (g *Gate) wait(ctx context.Context, sleep func(context.Context, time.Duration) error) error {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	d := time.Until(g.until)
	g.mu.Unlock()
	if d <= 0 {
		return ctx.Err()
	}
	return sleep(ctx, d)
}

// Retryable reports whether a failure may succeed on a later attempt. Invalid
// requests, bad credentials and malformed decisions are never retried.
func Retryable(err error) bool {
	var pe *Error
	if !errors.As(err, &pe) {
		return false
	}
	return pe.Kind == ErrTransport || pe.Kind == ErrRateLimited
}

// Do runs attempt until it succeeds, fails permanently, retries run out or ctx
// ends. The last provider error is returned unchanged so rate-limit exhaustion
// stays distinguishable from other transport failures.
func Do[T any](ctx context.Context, p RetryPolicy, attempt func(context.Context) (T, error)) (T, error) {
	var zero T
	sleep := p.Sleep
	if sleep == nil {
		sleep = SleepContext
	}
	for n := 0; ; n++ {
		if err := p.Gate.wait(ctx, sleep); err != nil {
			return zero, ContextError(ctx)
		}
		out, err := attempt(ctx)
		if err == nil {
			return out, nil
		}
		if ce := ContextError(ctx); ce != nil {
			return zero, ce
		}
		if !Retryable(err) || n >= p.MaxRetries {
			return zero, err
		}
		delay := p.backoff(n)
		var pe *Error
		if errors.As(err, &pe) && pe.RetryAfter > 0 {
			delay = min(pe.RetryAfter, p.MaxDelay)
		}
		if errors.As(err, &pe) && pe.Kind == ErrRateLimited {
			p.Gate.pauseFor(delay)
		}
		if err := sleep(ctx, delay); err != nil {
			return zero, ContextError(ctx)
		}
	}
}

func (p RetryPolicy) backoff(n int) time.Duration {
	d := p.BaseDelay << n
	if d <= 0 || d > p.MaxDelay {
		d = p.MaxDelay
	}
	j := 1.0
	if p.Jitter != nil {
		j = 0.5 + p.Jitter()/2
	}
	return time.Duration(float64(d) * j)
}

// ParseRetryAfter reads a Retry-After header in seconds or HTTP-date form.
func ParseRetryAfter(h http.Header, now time.Time) time.Duration {
	v := strings.TrimSpace(h.Get("Retry-After"))
	if v == "" {
		return 0
	}
	if s, err := strconv.ParseFloat(v, 64); err == nil && s >= 0 {
		return time.Duration(s * float64(time.Second))
	}
	if t, err := http.ParseTime(v); err == nil && t.After(now) {
		return t.Sub(now)
	}
	return 0
}

// ClassifyStatus maps an HTTP status to a provider error kind. 529 is the
// overload status used by both TypeSafe and Anthropic.
func ClassifyStatus(status int) ErrorKind {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return ErrAuth
	case status == http.StatusTooManyRequests || status == 529:
		return ErrRateLimited
	case status == http.StatusRequestTimeout || status >= 500:
		return ErrTransport
	default:
		return ErrRejected
	}
}

// NewHTTPClient returns a client that never follows redirects, so a response
// cannot move a request carrying source or a key to another location.
func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
