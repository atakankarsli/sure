package contracts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/atakankarsli/sure/internal/gitview"
)

// ErrNotInitialized means the source has no .sure/config.yaml.
var ErrNotInitialized = errors.New("no " + ConfigPath + " found; run 'sure init'")

// Set is a config plus every contract it governs, read from one source (the
// working tree or a trusted commit).
type Set struct {
	Config    Config
	Contracts []Contract
	Issues    []Issue
}

func (s *Set) Valid() bool { return len(s.Issues) == 0 }

func (s *Set) Contract(id string) (Contract, bool) {
	for _, c := range s.Contracts {
		if c.ID == id {
			return c, true
		}
	}
	return Contract{}, false
}

func (s *Set) CheckCount() int {
	n := 0
	for _, c := range s.Contracts {
		n += len(c.Checks)
	}
	return n
}

// Load reads and validates the config and contracts from src. A missing
// config returns ErrNotInitialized; every other problem is reported in
// Set.Issues.
func Load(ctx context.Context, src gitview.Source) (*Set, error) {
	data, err := src.Read(ctx, ConfigPath, MaxConfigBytes)
	if err != nil {
		if errors.Is(err, gitview.ErrNotFound) {
			return nil, ErrNotInitialized
		}
		if errors.Is(err, gitview.ErrTooLarge) {
			return &Set{Issues: []Issue{{File: ConfigPath, Code: "config_too_large", Message: "config is too large"}}}, nil
		}
		return nil, fmt.Errorf("read %s: %w", ConfigPath, err)
	}
	cfg, issues := ParseConfig(ConfigPath, data)
	set := &Set{Config: cfg, Issues: issues}
	if len(issues) > 0 {
		return set, nil
	}
	files, err := src.List(ctx)
	if err != nil {
		return nil, err
	}
	prefix := cfg.Contracts + "/"
	ids := map[string]string{}
	for _, f := range files {
		if !strings.HasPrefix(f, prefix) || !strings.HasSuffix(f, ".md") {
			continue
		}
		b, err := src.Read(ctx, f, MaxContractBytes)
		if err != nil {
			set.Issues = append(set.Issues, Issue{File: f, Code: "unreadable_contract", Message: err.Error()})
			continue
		}
		c, iss := ParseContract(f, b)
		set.Issues = append(set.Issues, iss...)
		if len(iss) > 0 {
			continue
		}
		if prev, ok := ids[c.ID]; ok {
			set.Issues = append(set.Issues, Issue{File: f, Line: 2, Code: "duplicate_id",
				Message: fmt.Sprintf("contract id %q is also used by %s", c.ID, prev)})
			continue
		}
		ids[c.ID] = f
		set.Issues = append(set.Issues, crossCheck(cfg, c)...)
		set.Contracts = append(set.Contracts, c)
	}
	sort.Slice(set.Contracts, func(i, j int) bool { return set.Contracts[i].ID < set.Contracts[j].ID })
	return set, nil
}

// crossCheck rejects contracts that contradict the config: context files the
// config would never allow as evidence, or scopes that can match nothing.
func crossCheck(cfg Config, c Contract) []Issue {
	var issues []Issue
	for _, p := range c.Context {
		if !cfg.InSourceRoots(p) {
			issues = append(issues, Issue{File: c.Path, Line: 2, Code: "contradictory_config",
				Message: fmt.Sprintf("context file %s is outside source_roots", p)})
		} else if cfg.Excluded(p) {
			issues = append(issues, Issue{File: c.Path, Line: 2, Code: "contradictory_config",
				Message: fmt.Sprintf("context file %s is excluded by config", p)})
		}
	}
	for _, g := range c.Scope {
		if !scopeCanReachRoots(cfg, g) {
			issues = append(issues, Issue{File: c.Path, Line: 2, Code: "contradictory_config",
				Message: fmt.Sprintf("scope %q lies outside source_roots", g)})
		}
	}
	return issues
}

func scopeCanReachRoots(cfg Config, glob string) bool {
	base, _ := doublestar.SplitPattern(glob)
	if base == "." {
		return true
	}
	for _, r := range cfg.SourceRoots {
		if r == "." || base == r || strings.HasPrefix(base, r+"/") || strings.HasPrefix(r, base+"/") {
			return true
		}
	}
	return false
}

// Covers reports whether a path triggers the contract: it matches a scope
// glob or is an explicit context file.
func (c Contract) Covers(p string) bool {
	for _, g := range c.Scope {
		if doublestar.MatchUnvalidated(g, p) {
			return true
		}
	}
	for _, x := range c.Context {
		if x == p {
			return true
		}
	}
	return false
}

// Digest is a stable hash of the contract's meaning, independent of
// formatting outside the normalized fields.
func (c Contract) Digest() string { return hashJSON(c) }

func (cfg Config) Digest() string { return hashJSON(cfg) }

// Digest covers the config and every contract, in ID order.
func (s *Set) Digest() string {
	parts := []string{"config:" + s.Config.Digest()}
	for _, c := range s.Contracts {
		parts = append(parts, c.ID+":"+c.Digest())
	}
	return hashString(strings.Join(parts, "\n"))
}

func hashJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return hashString(string(b))
}

func hashString(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
