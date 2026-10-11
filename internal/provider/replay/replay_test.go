package replay

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/atakankarsli/sure/internal/provider"
	"github.com/atakankarsli/sure/internal/provider/fake"
)

func request(state string) provider.Request {
	return provider.Request{
		State: json.RawMessage(state),
		Questions: []provider.Question{{
			ID:           "q1",
			Instructions: json.RawMessage(`{"requirement":"r"}`),
			Options:      []provider.Option{{Name: "supported"}, {Name: "violated"}, {Name: "unknown"}},
		}},
	}
}

type noNetwork struct{}

func (noNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	panic("replay reached the network")
}

func TestRecordThenReplay(t *testing.T) {
	old := http.DefaultTransport
	http.DefaultTransport = noNetwork{}
	t.Cleanup(func() { http.DefaultTransport = old })

	dir := filepath.Join(t.TempDir(), "rec")
	live := &fake.Fake{Decide: func(q provider.Question, _ provider.Request) provider.Answer { return fake.Answer(q, "violated", 0.93) }}
	rec := &Recorder{Dir: dir, Inner: live, IncludeRequest: true}
	req := request(`{"src":"x"}`)
	want, err := rec.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Identity() != live.Identity() {
		t.Fatal("recorder identity differs")
	}

	key := Key(live.Identity(), req)
	path := filepath.Join(dir, key+".json")
	fi, err := os.Stat(path)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("recording: %v mode %v", err, fi)
	}
	raw, _ := os.ReadFile(path)
	var stored Recording
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	var body struct {
		State     map[string]string `json:"state"`
		Questions []struct {
			ID string `json:"id"`
		} `json:"questions"`
	}
	if err := json.Unmarshal(stored.Request, &body); err != nil || body.State["src"] != "x" || body.Questions[0].ID != "q1" {
		t.Fatalf("IncludeRequest did not store the request: %s", stored.Request)
	}

	p := &Player{Dir: dir, Replayed: live.Identity()}
	got, err := p.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != provider.SourceReplay || got.Answers["q1"].Choice != "violated" || got.Model != want.Model {
		t.Fatalf("replayed %+v", got)
	}
	if p.Identity() != live.Identity() {
		t.Fatal("player identity differs")
	}

	if _, err := p.Evaluate(context.Background(), request(`{"src":"y"}`)); provider.KindOf(err) != provider.ErrReplayMiss {
		t.Fatalf("miss err = %v", err)
	}
	other := live.Identity()
	other.Model = "another"
	if _, err := (&Player{Dir: dir, Replayed: other}).Evaluate(context.Background(), req); provider.KindOf(err) != provider.ErrReplayMiss {
		t.Fatalf("different identity err = %v", err)
	}
}

func TestRecorderOmitsRequestByDefaultAndSkipsFailures(t *testing.T) {
	dir := t.TempDir()
	live := &fake.Fake{}
	if _, err := (&Recorder{Dir: dir, Inner: live}).Evaluate(context.Background(), request(`{"secret":"src"}`)); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("entries = %v", entries)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if strings.Contains(string(raw), "secret") {
		t.Fatal("request stored without IncludeRequest")
	}

	failDir := t.TempDir()
	failing := &fake.Fake{Err: provider.Errorf(provider.ErrTransport, "down")}
	if _, err := (&Recorder{Dir: failDir, Inner: failing}).Evaluate(context.Background(), request(`{}`)); err == nil {
		t.Fatal("expected error")
	}
	if entries, _ := os.ReadDir(failDir); len(entries) != 0 {
		t.Fatal("failure recorded")
	}
}

func TestTamperedRecordings(t *testing.T) {
	dir := t.TempDir()
	live := &fake.Fake{}
	req := request(`{"a":1}`)
	if _, err := (&Recorder{Dir: dir, Inner: live}).Evaluate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	key := Key(live.Identity(), req)
	path := filepath.Join(dir, key+".json")
	orig, _ := os.ReadFile(path)
	p := &Player{Dir: dir, Replayed: live.Identity()}

	cases := map[string]func(string) string{
		"key":           func(s string) string { return strings.Replace(s, key, strings.Repeat("0", 64), 1) },
		"schema":        func(s string) string { return strings.Replace(s, SchemaVersion, "sure.recording/v0", 1) },
		"identity":      func(s string) string { return strings.Replace(s, `"model": "fake-model"`, `"model": "x"`, 1) },
		"probabilities": func(s string) string { return strings.Replace(s, `0.96`, `0.5`, 1) },
		"unknown field": func(s string) string {
			return strings.Replace(s, `"schema_version"`, `"extra": 1, "schema_version"`, 1)
		},
		"not json": func(string) string { return "{" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(mutate(string(orig))), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := p.Evaluate(context.Background(), req)
			if k := provider.KindOf(err); k != provider.ErrMalformed {
				t.Fatalf("kind = %s err = %v", k, err)
			}
		})
	}
}
