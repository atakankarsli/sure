// Package replay records live decisions and plays them back offline. A player
// answers only requests it has an exact recording for and never reaches a
// network provider.
package replay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/atakankarsli/sure/internal/cache"
	"github.com/atakankarsli/sure/internal/provider"
)

const SchemaVersion = "sure.recording/v1"

type Recording struct {
	SchemaVersion string            `json:"schema_version"`
	Key           string            `json:"key"`
	Identity      provider.Identity `json:"identity"`
	Request       json.RawMessage   `json:"request,omitempty"`
	Response      provider.Response `json:"response"`
}

func Key(id provider.Identity, req provider.Request) string {
	return provider.RequestKey(id, req)
}

type Recorder struct {
	Dir            string
	Inner          provider.Provider
	IncludeRequest bool
}

func (r *Recorder) Identity() provider.Identity { return r.Inner.Identity() }

func (r *Recorder) Evaluate(ctx context.Context, req provider.Request) (provider.Response, error) {
	resp, err := r.Inner.Evaluate(ctx, req)
	if err != nil {
		return provider.Response{}, err
	}
	id := r.Inner.Identity()
	rec := Recording{SchemaVersion: SchemaVersion, Key: Key(id, req), Identity: id, Response: resp}
	if r.IncludeRequest {
		raw, err := requestJSON(req)
		if err != nil {
			return provider.Response{}, provider.Errorf(provider.ErrTransport, "recording: %v", err)
		}
		rec.Request = raw
	}
	if err := r.write(rec); err != nil {
		return provider.Response{}, provider.Errorf(provider.ErrTransport, "recording: %v", err)
	}
	return resp, nil
}

func (r *Recorder) write(rec Recording) error {
	if err := os.MkdirAll(r.Dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return cache.WriteAtomic(filepath.Join(r.Dir, rec.Key+".json"), append(data, '\n'))
}

func requestJSON(req provider.Request) (json.RawMessage, error) {
	type option struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	type question struct {
		ID           string          `json:"id"`
		Instructions json.RawMessage `json:"instructions"`
		Options      []option        `json:"options"`
	}
	qs := make([]question, len(req.Questions))
	for i, q := range req.Questions {
		opts := make([]option, len(q.Options))
		for j, o := range q.Options {
			opts[j] = option{Name: o.Name, Description: o.Description}
		}
		qs[i] = question{ID: q.ID, Instructions: q.Instructions, Options: opts}
	}
	return json.Marshal(struct {
		State     json.RawMessage `json:"state"`
		Questions []question      `json:"questions"`
	}{req.State, qs})
}

// Player replays recordings made by the provider whose identity is Replayed.
type Player struct {
	Dir      string
	Replayed provider.Identity
}

func (p *Player) Identity() provider.Identity { return p.Replayed }

func (p *Player) Evaluate(ctx context.Context, req provider.Request) (provider.Response, error) {
	if ce := provider.ContextError(ctx); ce != nil {
		return provider.Response{}, ce
	}
	key := Key(p.Replayed, req)
	data, err := os.ReadFile(filepath.Join(p.Dir, key+".json"))
	if errors.Is(err, fs.ErrNotExist) {
		return provider.Response{}, provider.Errorf(provider.ErrReplayMiss, "no recording for request %s", key[:12])
	}
	if err != nil {
		return provider.Response{}, provider.Errorf(provider.ErrReplayMiss, "reading recording %s: %v", key[:12], err)
	}
	var rec Recording
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&rec); err != nil {
		return provider.Response{}, provider.Errorf(provider.ErrMalformed, "recording %s: %v", key[:12], err)
	}
	switch {
	case rec.SchemaVersion != SchemaVersion:
		return provider.Response{}, provider.Errorf(provider.ErrMalformed, "recording %s has schema %q", key[:12], rec.SchemaVersion)
	case rec.Key != key:
		return provider.Response{}, provider.Errorf(provider.ErrMalformed, "recording %s does not match its request", key[:12])
	case rec.Identity != p.Replayed:
		return provider.Response{}, provider.Errorf(provider.ErrMalformed, "recording %s was made by a different provider", key[:12])
	}
	if err := provider.ValidateAnswers(req, rec.Response.Answers); err != nil {
		return provider.Response{}, err
	}
	resp := rec.Response
	resp.Source = provider.SourceReplay
	return resp, nil
}
