// Package jev calls the TypeSafe Jev decision model, either through the Vercel
// AI Gateway or directly at TypeSafe.
package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/atakankarsli/sure/internal/provider"
)

const (
	TransportGateway  = "gateway"
	TransportTypeSafe = "typesafe"

	// DefaultGatewayURL is the decision-model route the AI SDK's gateway
	// provider uses (spec v4); the model travels in the ai-model-id header.
	DefaultGatewayURL  = "https://ai-gateway.vercel.sh/v4/ai/decision-model"
	DefaultTypeSafeURL = "https://api.typesafe.ai/v1/systemone"

	DefaultGatewayModel  = "typesafe-ai/jev"
	DefaultTypeSafeModel = "jev-1.13.0"

	KeyEnvGateway  = "AI_GATEWAY_API_KEY"
	KeyEnvTypeSafe = "TYPESAFE_API_KEY"

	// LongContextHeader raises TypeSafe's per-request and per-question limits
	// from 64k/32k to 262k tokens; TypeSafe documents it as slower and less
	// accurate. The gateway is not known to forward it.
	LongContextHeader = "TypeSafe-Danger-Zone-Long-Context"

	maxResponseBytes = 4 << 20
	maxErrorBytes    = 64 << 10
	maxExcerptBytes  = 300
)

var pinnedModel = regexp.MustCompile(`^jev-\d+\.\d+\.\d+$`)

type Config struct {
	Transport            string
	Model                string
	BaseURL              string
	APIKey               string
	HTTPClient           *http.Client
	Retry                provider.RetryPolicy
	AllowInsecureBaseURL bool
	UserAgent            string
	// LongContext sends LongContextHeader; typesafe transport only.
	LongContext bool
}

type Provider struct {
	transport   string
	model       string
	endpoint    string
	wire        string
	key         string
	userAgent   string
	longContext bool
	hc          *http.Client
	retry       provider.RetryPolicy
}

func New(cfg Config) (*Provider, error) {
	var defURL, defModel, keyEnv, wire string
	switch cfg.Transport {
	case TransportGateway:
		defURL, defModel, keyEnv, wire = DefaultGatewayURL, DefaultGatewayModel, KeyEnvGateway, "jev-gateway-v4"
	case TransportTypeSafe:
		defURL, defModel, keyEnv, wire = DefaultTypeSafeURL, DefaultTypeSafeModel, KeyEnvTypeSafe, "jev-typesafe-v1"
	default:
		return nil, fmt.Errorf("unknown jev transport %q (want %s or %s)", cfg.Transport, TransportGateway, TransportTypeSafe)
	}
	if cfg.LongContext {
		if cfg.Transport != TransportTypeSafe {
			return nil, fmt.Errorf("long context needs the typesafe transport (set %s)", KeyEnvTypeSafe)
		}
		wire += "-long"
	}
	key := strings.TrimSpace(cfg.APIKey)
	if key == "" {
		return nil, provider.Errorf(provider.ErrMissingKey, "set %s", keyEnv)
	}
	endpoint := cfg.BaseURL
	if endpoint == "" {
		endpoint = defURL
	}
	if err := checkURL(endpoint, cfg.AllowInsecureBaseURL); err != nil {
		return nil, err
	}
	model := cfg.Model
	if model == "" {
		model = defModel
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = provider.NewHTTPClient(90 * time.Second)
	}
	retry := cfg.Retry
	if isZeroPolicy(retry) {
		retry = provider.DefaultRetryPolicy()
	}
	ua := cfg.UserAgent
	if ua == "" {
		ua = "sure"
	}
	return &Provider{
		transport:   cfg.Transport,
		model:       model,
		endpoint:    endpoint,
		wire:        wire,
		key:         key,
		userAgent:   ua,
		longContext: cfg.LongContext,
		hc:          hc,
		retry:       retry,
	}, nil
}

func checkURL(raw string, allowInsecure bool) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid provider URL: %v", err)
	}
	switch {
	case u.Scheme != "https" && !(allowInsecure && u.Scheme == "http"):
		return fmt.Errorf("provider URL must use https")
	case u.Host == "":
		return fmt.Errorf("provider URL has no host")
	case u.User != nil || u.RawQuery != "" || u.Fragment != "":
		return fmt.Errorf("provider URL must not contain credentials, a query or a fragment")
	}
	return nil
}

func isZeroPolicy(p provider.RetryPolicy) bool {
	return p.MaxRetries == 0 && p.BaseDelay == 0 && p.MaxDelay == 0 &&
		p.Gate == nil && p.Sleep == nil && p.Jitter == nil
}

func (p *Provider) Identity() provider.Identity {
	return provider.Identity{
		Name:              provider.NameJev,
		Transport:         p.transport,
		Endpoint:          p.endpoint,
		Model:             p.model,
		WireFormat:        p.wire,
		ProbabilitySource: provider.ProbabilityModel,
		LongContext:       p.longContext,
	}
}

// Body renders the exact wire body for a request. Questions keep request order
// and criteria keep option order, because Jev can be sensitive to option order.
// An empty option description is sent as null.
func Body(transport, model string, req provider.Request) ([]byte, error) {
	if len(req.Questions) == 0 {
		return nil, errors.New("request has no questions")
	}
	var b bytes.Buffer
	if transport == TransportGateway {
		b.WriteString(`{"stateParts":[{"type":"json","value":`)
		if err := compact(&b, req.State); err != nil {
			return nil, fmt.Errorf("state: %w", err)
		}
		b.WriteString(`}],"questions":`)
	} else {
		b.WriteString(`{"model":`)
		writeString(&b, model)
		b.WriteString(`,"state":`)
		if err := compact(&b, req.State); err != nil {
			return nil, fmt.Errorf("state: %w", err)
		}
		b.WriteString(`,"questions":`)
	}
	b.WriteByte('{')
	seen := map[string]bool{}
	for i, q := range req.Questions {
		if q.ID == "" || seen[q.ID] {
			return nil, fmt.Errorf("empty or duplicate question id %q", q.ID)
		}
		seen[q.ID] = true
		if len(q.Options) == 0 {
			return nil, fmt.Errorf("question %q has no options", q.ID)
		}
		if i > 0 {
			b.WriteByte(',')
		}
		writeString(&b, q.ID)
		b.WriteString(`:{"type":"choice","instructions":`)
		if err := compact(&b, q.Instructions); err != nil {
			return nil, fmt.Errorf("question %q instructions: %w", q.ID, err)
		}
		b.WriteString(`,"criteria":{`)
		opts := map[string]bool{}
		for j, o := range q.Options {
			if o.Name == "" || opts[o.Name] {
				return nil, fmt.Errorf("question %q has an empty or duplicate option %q", q.ID, o.Name)
			}
			opts[o.Name] = true
			if j > 0 {
				b.WriteByte(',')
			}
			writeString(&b, o.Name)
			b.WriteByte(':')
			if o.Description == "" {
				b.WriteString("null")
			} else {
				writeString(&b, o.Description)
			}
		}
		b.WriteString(`}}`)
	}
	b.WriteByte('}')
	if transport == TransportGateway {
		b.WriteString(`,"providerOptions":{"gateway":{"only":["typesafe-ai"]}}`)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

func writeString(b *bytes.Buffer, s string) {
	enc := json.NewEncoder(b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	b.Truncate(b.Len() - 1)
}

func compact(b *bytes.Buffer, raw json.RawMessage) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return errors.New("empty JSON")
	}
	return json.Compact(b, raw)
}

func (p *Provider) Evaluate(ctx context.Context, req provider.Request) (provider.Response, error) {
	if ce := provider.ContextError(ctx); ce != nil {
		return provider.Response{}, ce
	}
	body, err := Body(p.transport, p.model, req)
	if err != nil {
		return provider.Response{}, provider.Errorf(provider.ErrRejected, "invalid request: %v", err)
	}
	return provider.Do(ctx, p.retry, func(ctx context.Context) (provider.Response, error) {
		return p.send(ctx, req, body)
	})
}

func (p *Provider) send(ctx context.Context, req provider.Request, body []byte) (provider.Response, error) {
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return provider.Response{}, provider.Errorf(provider.ErrRejected, "building request: %v", err)
	}
	hreq.Header.Set("Authorization", "Bearer "+p.key)
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("Accept", "application/json")
	hreq.Header.Set("User-Agent", p.userAgent)
	if p.longContext {
		hreq.Header.Set(LongContextHeader, "true")
	}
	if p.transport == TransportGateway {
		hreq.Header.Set("ai-gateway-protocol-version", "0.0.1")
		hreq.Header.Set("ai-gateway-auth-method", "api-key")
		hreq.Header.Set("ai-decision-model-specification-version", "4")
		hreq.Header.Set("ai-model-id", p.model)
	}

	resp, err := p.hc.Do(hreq)
	if err != nil {
		if ce := provider.ContextError(ctx); ce != nil {
			return provider.Response{}, ce
		}
		return provider.Response{}, p.redact(provider.Errorf(provider.ErrTransport, "%v", err))
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		return provider.Response{}, &provider.Error{Kind: provider.ErrRejected, Status: resp.StatusCode, Message: "redirect refused"}
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBytes))
		return provider.Response{}, p.redact(&provider.Error{
			Kind:       provider.ClassifyStatus(resp.StatusCode),
			Status:     resp.StatusCode,
			Message:    errorExcerpt(raw, resp.StatusCode),
			RetryAfter: provider.ParseRetryAfter(resp.Header, time.Now()),
		})
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		if ce := provider.ContextError(ctx); ce != nil {
			return provider.Response{}, ce
		}
		return provider.Response{}, provider.Errorf(provider.ErrTransport, "reading response: %v", err)
	}
	if len(raw) > maxResponseBytes {
		return provider.Response{}, provider.Errorf(provider.ErrMalformed, "response larger than %d bytes", maxResponseBytes)
	}
	out, err := p.decode(req, raw)
	if err != nil {
		return provider.Response{}, p.redact(err)
	}
	return out, nil
}

type wireResponse struct {
	Model   *string                `json:"model"`
	Answers map[string]*wireAnswer `json:"answers"`
	Usage   *wireUsage             `json:"usage"`
}

type wireAnswer struct {
	Type          *string             `json:"type"`
	Choice        *string             `json:"choice"`
	Probabilities map[string]*float64 `json:"probabilities"`
	Confidence    *float64            `json:"confidence"`
}

type wireUsage struct {
	InputTokens  *int64 `json:"input_tokens"`
	OutputTokens *int64 `json:"output_tokens"`
}

func (p *Provider) decode(req provider.Request, raw []byte) (provider.Response, error) {
	if p.transport == TransportGateway {
		return p.decodeGateway(req, raw)
	}
	malformed := func(format string, args ...any) (provider.Response, error) {
		return provider.Response{}, provider.Errorf(provider.ErrMalformed, format, args...)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	var w wireResponse
	if err := dec.Decode(&w); err != nil {
		return malformed("decoding response: %v", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return malformed("unexpected data after response object")
	}
	if w.Model == nil || strings.TrimSpace(*w.Model) == "" {
		return malformed("response has no model")
	}
	if pinnedModel.MatchString(p.model) && *w.Model != p.model {
		return malformed("requested model %s but %s answered", p.model, *w.Model)
	}
	if w.Answers == nil {
		return malformed("response has no answers")
	}
	if w.Usage == nil || w.Usage.InputTokens == nil || w.Usage.OutputTokens == nil {
		return malformed("response has no usage")
	}
	if *w.Usage.InputTokens < 0 || *w.Usage.OutputTokens < 0 {
		return malformed("response has negative usage")
	}

	answers := make(map[string]provider.Answer, len(w.Answers))
	for id, wa := range w.Answers {
		switch {
		case wa == nil:
			return malformed("answer %q is null", id)
		case wa.Type == nil || *wa.Type != "choice":
			return malformed("answer %q is not a choice answer", id)
		case wa.Choice == nil:
			return malformed("answer %q has no choice", id)
		case wa.Probabilities == nil:
			return malformed("answer %q has no probabilities", id)
		case wa.Confidence == nil:
			return malformed("answer %q has no confidence", id)
		}
		probs := make(map[string]float64, len(wa.Probabilities))
		for opt, v := range wa.Probabilities {
			if v == nil {
				return malformed("answer %q has a null probability for %q", id, opt)
			}
			probs[opt] = *v
		}
		conf := *wa.Confidence
		answers[id] = provider.Answer{Choice: *wa.Choice, Probabilities: probs, Confidence: &conf}
	}
	if err := provider.ValidateAnswers(req, answers); err != nil {
		return provider.Response{}, err
	}
	return provider.Response{
		Model:   *w.Model,
		Answers: answers,
		Usage:   provider.Usage{InputTokens: int(*w.Usage.InputTokens), OutputTokens: int(*w.Usage.OutputTokens)},
		Source:  provider.SourceLive,
	}, nil
}

// gatewayResponse follows the AI SDK decision model v4 result that the
// gateway returns: distributions and usage are optional there, but Sure needs
// a distribution for every choice it accepts.
type gatewayResponse struct {
	Answers map[string]*gatewayAnswer `json:"answers"`
	Model   *string                   `json:"model"`
	Usage   *struct {
		InputTokens  *float64 `json:"inputTokens"`
		OutputTokens *float64 `json:"outputTokens"`
	} `json:"usage"`
}

type gatewayAnswer struct {
	Type          *string             `json:"type"`
	Choice        *string             `json:"choice"`
	Probabilities map[string]*float64 `json:"probabilities"`
}

func (p *Provider) decodeGateway(req provider.Request, raw []byte) (provider.Response, error) {
	malformed := func(format string, args ...any) (provider.Response, error) {
		return provider.Response{}, provider.Errorf(provider.ErrMalformed, format, args...)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	var w gatewayResponse
	if err := dec.Decode(&w); err != nil {
		return malformed("decoding response: %v", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return malformed("unexpected data after response object")
	}
	if w.Answers == nil {
		return malformed("response has no answers")
	}
	model := p.model
	if w.Model != nil && strings.TrimSpace(*w.Model) != "" {
		model = *w.Model
	}
	var usage provider.Usage
	if w.Usage != nil {
		for _, v := range []*float64{w.Usage.InputTokens, w.Usage.OutputTokens} {
			if v != nil && (*v < 0 || *v != float64(int64(*v))) {
				return malformed("response has invalid usage")
			}
		}
		if w.Usage.InputTokens != nil {
			usage.InputTokens = int(*w.Usage.InputTokens)
		}
		if w.Usage.OutputTokens != nil {
			usage.OutputTokens = int(*w.Usage.OutputTokens)
		}
	}
	answers := make(map[string]provider.Answer, len(w.Answers))
	for id, wa := range w.Answers {
		switch {
		case wa == nil || wa.Type == nil:
			return malformed("answer %q has no type", id)
		case *wa.Type == "refusal":
			answers[id] = provider.Answer{Refused: true}
			continue
		case *wa.Type != "choice":
			return malformed("answer %q is a %s answer, not a choice", id, *wa.Type)
		case wa.Choice == nil:
			return malformed("answer %q has no choice", id)
		case wa.Probabilities == nil:
			return malformed("answer %q has no probability distribution", id)
		}
		probs := make(map[string]float64, len(wa.Probabilities))
		for opt, v := range wa.Probabilities {
			if v == nil {
				return malformed("answer %q has a null probability for %q", id, opt)
			}
			probs[opt] = *v
		}
		answers[id] = provider.Answer{Choice: *wa.Choice, Probabilities: probs}
	}
	if err := provider.ValidateAnswers(req, answers); err != nil {
		return provider.Response{}, err
	}
	return provider.Response{Model: model, Answers: answers, Usage: usage, Source: provider.SourceLive}, nil
}

// errorExcerpt pulls a short message out of the gateway ({"error":{...}}) or
// TypeSafe ({"detail":...}) error bodies.
func errorExcerpt(raw []byte, status int) string {
	var body struct {
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
		Detail  json.RawMessage `json:"detail"`
		Message string          `json:"message"`
	}
	msg := ""
	if json.Unmarshal(raw, &body) == nil {
		switch {
		case body.Error != nil:
			msg = strings.TrimSpace(strings.Join(nonEmpty(body.Error.Type, body.Error.Message), ": "))
		case len(body.Detail) > 0:
			msg = detailText(body.Detail)
		default:
			msg = body.Message
		}
	}
	if msg == "" {
		msg = http.StatusText(status)
	}
	return truncate(msg, maxExcerptBytes)
}

func detailText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var obj struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &obj) == nil && obj.Message != "" {
		return obj.Message
	}
	var b bytes.Buffer
	if json.Compact(&b, raw) == nil {
		return b.String()
	}
	return ""
}

func nonEmpty(parts ...string) []string {
	out := parts[:0]
	for _, s := range parts {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !isRuneStart(s[n]) {
		n--
	}
	return s[:n] + "…"
}

func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }

func (p *Provider) redact(err error) error {
	var pe *provider.Error
	if errors.As(err, &pe) && p.key != "" && strings.Contains(pe.Message, p.key) {
		pe.Message = strings.ReplaceAll(pe.Message, p.key, "[redacted]")
	}
	return err
}
