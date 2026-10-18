// Package cache stores validated provider decisions on disk, addressed by the
// exact request that produced them.
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	"github.com/atakankarsli/sure/internal/provider"
)

const SchemaVersion = "sure.decision-cache/v1"

var (
	keyPattern       = regexp.MustCompile(`^[0-9a-f]{64}$`)
	namespacePattern = regexp.MustCompile(`^[0-9a-f]{1,64}$`)
)

// DefaultDir is SURE_CACHE_DIR when set, otherwise the user cache directory.
func DefaultDir(getenv func(string) string) (string, error) {
	if d := getenv("SURE_CACHE_DIR"); d != "" {
		if !filepath.IsAbs(d) {
			return "", fmt.Errorf("SURE_CACHE_DIR must be an absolute path")
		}
		return filepath.Clean(d), nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locating the user cache directory: %w", err)
	}
	return filepath.Join(base, "sure"), nil
}

// Namespace separates caches of different repositories and trust contexts.
func Namespace(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		fmt.Fprintf(h, "%d:%s\n", len(p), p)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

type Cache struct {
	dir string
}

func Open(baseDir, namespace string) (*Cache, error) {
	if !namespacePattern.MatchString(namespace) {
		return nil, fmt.Errorf("invalid cache namespace %q", namespace)
	}
	dir := filepath.Join(baseDir, "decisions", namespace)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating cache directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("securing cache directory: %w", err)
	}
	return &Cache{dir: dir}, nil
}

func (c *Cache) Dir() string { return c.dir }

type entry struct {
	SchemaVersion string            `json:"schema_version"`
	Key           string            `json:"key"`
	Response      provider.Response `json:"response"`
}

func (c *Cache) path(key string) (string, error) {
	if !keyPattern.MatchString(key) {
		return "", fmt.Errorf("invalid cache key %q", key)
	}
	return filepath.Join(c.dir, key+".json"), nil
}

// Get returns a stored decision. A corrupt or mismatched entry is removed and
// reported as a miss.
func (c *Cache) Get(key string) (provider.Response, bool, error) {
	p, err := c.path(key)
	if err != nil {
		return provider.Response{}, false, err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return provider.Response{}, false, nil
	}
	if err != nil {
		return provider.Response{}, false, err
	}
	var e entry
	if json.Unmarshal(data, &e) != nil || e.SchemaVersion != SchemaVersion || e.Key != key || e.Response.Answers == nil {
		_ = os.Remove(p)
		return provider.Response{}, false, nil
	}
	return e.Response, true, nil
}

func (c *Cache) Put(key string, r provider.Response) error {
	p, err := c.path(key)
	if err != nil {
		return err
	}
	data, err := json.Marshal(entry{SchemaVersion: SchemaVersion, Key: key, Response: r})
	if err != nil {
		return err
	}
	return WriteAtomic(p, data)
}

// WriteAtomic writes data to a 0600 temporary file next to path and renames it
// into place, so readers never see a partial file.
func WriteAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	fail := func(err error) error {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Chmod(0o600); err != nil {
		return fail(err)
	}
	if _, err := f.Write(data); err != nil {
		return fail(err)
	}
	if err := f.Sync(); err != nil {
		return fail(err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// Wrapped serves decisions from the cache and stores successful live ones.
// Failures are never cached.
type Wrapped struct {
	inner      provider.Provider
	cache      *Cache
	extra      []string
	OnPutError func(error)
}

func Wrap(p provider.Provider, c *Cache, extra ...string) *Wrapped {
	return &Wrapped{inner: p, cache: c, extra: extra}
}

func (w *Wrapped) Identity() provider.Identity { return w.inner.Identity() }

func (w *Wrapped) Key(req provider.Request) string {
	return provider.RequestKey(w.inner.Identity(), req, w.extra...)
}

func (w *Wrapped) Evaluate(ctx context.Context, req provider.Request) (provider.Response, error) {
	key := w.Key(req)
	if resp, ok, err := w.cache.Get(key); err == nil && ok && provider.ValidateAnswers(req, resp.Answers) == nil {
		resp.Source = provider.SourceCache
		return resp, nil
	}
	resp, err := w.inner.Evaluate(ctx, req)
	if err != nil {
		return provider.Response{}, err
	}
	if err := w.cache.Put(key, resp); err != nil && w.OnPutError != nil {
		w.OnPutError(err)
	}
	return resp, nil
}
