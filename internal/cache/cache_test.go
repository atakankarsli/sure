package cache

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
			Options:      []provider.Option{{Name: "supported", Description: "s"}, {Name: "violated", Description: "v"}, {Name: "unknown", Description: "u"}},
		}},
	}
}

func open(t *testing.T) *Cache {
	t.Helper()
	c, err := Open(t.TempDir(), Namespace("repo", "trust"))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}

func TestPutGetRoundTripAndPermissions(t *testing.T) {
	c := open(t)
	f := &fake.Fake{}
	resp, err := f.Evaluate(context.Background(), request(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	key := provider.RequestKey(f.Identity(), request(`{"a":1}`))
	if _, ok, err := c.Get(key); ok || err != nil {
		t.Fatalf("empty cache hit=%v err=%v", ok, err)
	}
	if err := c.Put(key, resp); err != nil {
		t.Fatal(err)
	}
	got, ok, err := c.Get(key)
	if err != nil || !ok {
		t.Fatalf("hit=%v err=%v", ok, err)
	}
	if got.Model != resp.Model || got.Answers["q1"].Choice != "supported" || got.Usage != resp.Usage {
		t.Fatalf("got %+v", got)
	}
	if m := mode(t, c.Dir()); m != 0o700 {
		t.Fatalf("dir mode %o", m)
	}
	if m := mode(t, filepath.Join(c.Dir(), key+".json")); m != 0o600 {
		t.Fatalf("file mode %o", m)
	}
	entries, _ := os.ReadDir(c.Dir())
	if len(entries) != 1 {
		t.Fatalf("leftover temp files: %v", entries)
	}
}

func TestInvalidKeysAndNamespaces(t *testing.T) {
	c := open(t)
	for _, k := range []string{"", "../x", "ABC", "zz"} {
		if err := c.Put(k, provider.Response{}); err == nil {
			t.Errorf("put accepted key %q", k)
		}
		if _, _, err := c.Get(k); err == nil {
			t.Errorf("get accepted key %q", k)
		}
	}
	if _, err := Open(t.TempDir(), "../escape"); err == nil {
		t.Fatal("namespace with separators accepted")
	}
}

func TestCorruptEntryIsMissAndRemoved(t *testing.T) {
	c := open(t)
	key := provider.RequestKey(provider.Identity{Name: "x"}, request(`{}`))
	p := filepath.Join(c.Dir(), key+".json")
	for _, body := range []string{`{not json`, `{"schema_version":"other","key":"` + key + `","response":{"answers":{}}}`, `{"schema_version":"` + SchemaVersion + `","key":"bad","response":{"answers":{}}}`} {
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, ok, err := c.Get(key); ok || err != nil {
			t.Fatalf("corrupt entry: hit=%v err=%v", ok, err)
		}
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatal("corrupt entry not removed")
		}
	}
}

func TestWrapHitMissAndNoErrorCaching(t *testing.T) {
	c := open(t)
	f := &fake.Fake{}
	w := Wrap(f, c, "prompt-v1")
	req := request(`{"a":1}`)

	r1, err := w.Evaluate(context.Background(), req)
	if err != nil || r1.Source != provider.SourceFake {
		t.Fatalf("first: %+v %v", r1, err)
	}
	r2, err := w.Evaluate(context.Background(), req)
	if err != nil || r2.Source != provider.SourceCache {
		t.Fatalf("second: %+v %v", r2, err)
	}
	if f.Calls() != 1 {
		t.Fatalf("calls = %d", f.Calls())
	}
	if w.Identity() != f.Identity() {
		t.Fatal("identity not passed through")
	}

	failing := &fake.Fake{Err: provider.Errorf(provider.ErrTransport, "down")}
	wf := Wrap(failing, c)
	other := request(`{"b":2}`)
	for range 2 {
		if _, err := wf.Evaluate(context.Background(), other); err == nil {
			t.Fatal("expected error")
		}
	}
	if failing.Calls() != 2 {
		t.Fatalf("error was cached: calls = %d", failing.Calls())
	}
	if _, ok, _ := c.Get(wf.Key(other)); ok {
		t.Fatal("failure stored")
	}
}

func TestStaleEntryThatNoLongerValidatesIsMiss(t *testing.T) {
	c := open(t)
	f := &fake.Fake{}
	w := Wrap(f, c)
	req := request(`{"a":1}`)
	bad := provider.Response{Model: "m", Answers: map[string]provider.Answer{"q1": {Choice: "supported", Probabilities: map[string]float64{"supported": 1}}}}
	if err := c.Put(w.Key(req), bad); err != nil {
		t.Fatal(err)
	}
	resp, err := w.Evaluate(context.Background(), req)
	if err != nil || resp.Source != provider.SourceFake || f.Calls() != 1 {
		t.Fatalf("resp=%+v err=%v calls=%d", resp, err, f.Calls())
	}
}

func TestKeyChangesWithEveryInput(t *testing.T) {
	id := provider.Identity{Name: "jev", Transport: "gateway", Endpoint: "e", Model: "m", WireFormat: "w"}
	base := provider.RequestKey(id, request(`{"a":1}`), "x")
	variants := map[string]string{}
	variants["state"] = provider.RequestKey(id, request(`{"a":2}`), "x")
	q := request(`{"a":1}`)
	q.Questions[0].Instructions = json.RawMessage(`{"requirement":"r2"}`)
	variants["instructions"] = provider.RequestKey(id, q, "x")
	o := request(`{"a":1}`)
	o.Questions[0].Options[2].Description = "u2"
	variants["option"] = provider.RequestKey(id, o, "x")
	id2 := id
	id2.Model = "m2"
	variants["model"] = provider.RequestKey(id2, request(`{"a":1}`), "x")
	id3 := id
	id3.Endpoint = "e2"
	variants["endpoint"] = provider.RequestKey(id3, request(`{"a":1}`), "x")
	variants["extra"] = provider.RequestKey(id, request(`{"a":1}`), "y")
	for name, k := range variants {
		if k == base {
			t.Errorf("key unchanged when %s changes", name)
		}
	}
}

func TestDefaultDirAndNamespace(t *testing.T) {
	env := map[string]string{"SURE_CACHE_DIR": "/tmp/sure-cache"}
	d, err := DefaultDir(func(k string) string { return env[k] })
	if err != nil || d != "/tmp/sure-cache" {
		t.Fatalf("d=%q err=%v", d, err)
	}
	env["SURE_CACHE_DIR"] = "relative/dir"
	if _, err := DefaultDir(func(k string) string { return env[k] }); err == nil {
		t.Fatal("relative SURE_CACHE_DIR accepted")
	}
	d, err = DefaultDir(func(string) string { return "" })
	if err != nil || filepath.Base(d) != "sure" {
		t.Fatalf("d=%q err=%v", d, err)
	}
	if Namespace("a", "b") == Namespace("ab") || len(Namespace("a")) != 16 {
		t.Fatal("namespace is ambiguous or wrong length")
	}
}
