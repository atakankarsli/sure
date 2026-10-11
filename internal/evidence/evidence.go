// Package evidence captures the allowlisted source bytes once per run and
// assembles, per contract, the complete files that its scope and context
// select. It never crops a file: a bundle is either whole or reported as over
// budget.
package evidence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/atakankarsli/sure/internal/contracts"
	"github.com/atakankarsli/sure/internal/gitview"
)

const (
	CollectorVersion = "files-v1"
	MaxSnapshotBytes = 128 << 20
	MaxSnapshotFiles = 20000
	// MaxRegions bounds the options of a localization question.
	MaxRegions = 120
)

const (
	ReasonNotFound = "not_found"
	ReasonExcluded = "excluded"
	ReasonBinary   = "binary"
	ReasonTooLarge = "too_large"
	ReasonSymlink  = "symlink"
	ReasonNotFile  = "not_a_file"
	ReasonUnread   = "unreadable"
)

// Snapshot identifies every allowlisted file once, at the start of a run, and
// reads content only for files that evidence needs. Unchanged tracked files
// are identified by the blob ID Git already has; changed and untracked files
// are read and hashed up front. Every file is read at most once, so the model,
// digests and locations describe the same bytes.
type Snapshot struct {
	Kind string
	Ref  string

	src     gitview.Source
	maxFile int64
	entries map[string]gitview.Entry
	paths   []string
	digest  string

	mu      sync.Mutex
	content map[string][]byte
	skipped map[string]string
	loaded  int
}

// Capture inventories every file the config allows from src.
func Capture(ctx context.Context, src gitview.Source, cfg contracts.Config) (*Snapshot, error) {
	inv, err := src.Inventory(ctx)
	if err != nil {
		return nil, err
	}
	s := &Snapshot{Kind: "worktree", src: src, maxFile: cfg.Limits.MaxFileBytes,
		entries: map[string]gitview.Entry{}, content: map[string][]byte{}, skipped: map[string]string{}}
	if d := src.Describe(); d != "worktree" {
		s.Kind, s.Ref = "commit", d
	}
	for _, e := range inv {
		if !cfg.Allowed(e.Path) {
			continue
		}
		if len(s.entries) >= MaxSnapshotFiles {
			return nil, fmt.Errorf("source_roots select more than %d files; narrow them", MaxSnapshotFiles)
		}
		s.entries[e.Path] = e
	}
	format := src.ObjectFormat(ctx)
	h := sha256.New()
	for _, p := range sortedKeys(s.entries) {
		e := s.entries[p]
		id := e.Blob
		if id == "" {
			b, reason, err := s.load(ctx, p)
			if err != nil {
				return nil, err
			}
			switch reason {
			case ReasonNotFound:
				delete(s.entries, p)
				continue
			case "":
				id = gitview.BlobID(format, b)
			default:
				id = "skipped:" + reason
			}
		} else if e.Symlink {
			id = "skipped:" + ReasonSymlink
		}
		s.paths = append(s.paths, p)
		fmt.Fprintf(h, "%s\x00%s\n", p, id)
	}
	s.digest = hex.EncodeToString(h.Sum(nil))
	return s, nil
}

// FromFiles builds a snapshot from known content; used for tests.
func FromFiles(kind, ref string, files map[string][]byte, skipped map[string]string) *Snapshot {
	s := &Snapshot{Kind: kind, Ref: ref, entries: map[string]gitview.Entry{}, content: map[string][]byte{}, skipped: map[string]string{}}
	h := sha256.New()
	all := map[string]bool{}
	for p, b := range files {
		all[p] = true
		s.content[p] = b
	}
	for p, r := range skipped {
		all[p] = true
		s.skipped[p] = r
	}
	for _, p := range sortedKeys(all) {
		s.entries[p] = gitview.Entry{Path: p}
		s.paths = append(s.paths, p)
		if b, ok := files[p]; ok {
			fmt.Fprintf(h, "%s\x00%s\n", p, gitview.BlobID("sha1", b))
		} else {
			fmt.Fprintf(h, "%s\x00skipped:%s\n", p, skipped[p])
		}
	}
	s.digest = hex.EncodeToString(h.Sum(nil))
	return s
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// load returns a file's bytes, or the reason it cannot be evidence. Content
// is cached so a file is read at most once per run.
func (s *Snapshot) load(ctx context.Context, p string) ([]byte, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.content[p]; ok {
		return b, "", nil
	}
	if r, ok := s.skipped[p]; ok {
		return nil, r, nil
	}
	e, ok := s.entries[p]
	switch {
	case !ok:
		return nil, ReasonNotFound, nil
	case e.Symlink:
		s.skipped[p] = ReasonSymlink
		return nil, ReasonSymlink, nil
	case s.src == nil:
		return nil, ReasonNotFound, nil
	}
	b, err := s.src.Read(ctx, p, s.maxFile)
	if err != nil {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		r := readReason(err)
		s.skipped[p] = r
		return nil, r, nil
	}
	if !isText(b) {
		s.skipped[p] = ReasonBinary
		return nil, ReasonBinary, nil
	}
	s.loaded += len(b)
	if s.loaded > MaxSnapshotBytes {
		return nil, "", fmt.Errorf("evidence needs more than %d bytes of source; narrow source_roots or scopes", MaxSnapshotBytes)
	}
	s.content[p] = b
	return b, "", nil
}

// Digest identifies the allowlisted source, including unreadable entries.
func (s *Snapshot) Digest() string { return s.digest }

// Paths returns every allowlisted file in sorted order, readable or not.
func (s *Snapshot) Paths() []string { return s.paths }

// Has reports whether a path is an allowlisted file of the snapshot.
func (s *Snapshot) Has(p string) bool {
	_, ok := s.entries[p]
	return ok
}

// LoadedBytes is how much source content the run has read so far.
func (s *Snapshot) LoadedBytes() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loaded
}

func Hash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func readReason(err error) string {
	switch {
	case errors.Is(err, gitview.ErrNotFound):
		return ReasonNotFound
	case errors.Is(err, gitview.ErrTooLarge):
		return ReasonTooLarge
	case errors.Is(err, gitview.ErrSymlink):
		return ReasonSymlink
	case errors.Is(err, gitview.ErrNotFile):
		return ReasonNotFile
	default:
		return ReasonUnread
	}
}

func isText(b []byte) bool {
	head := b
	if len(head) > 8000 {
		head = head[:8000]
	}
	return !bytes.ContainsRune(head, 0) && utf8.Valid(b)
}

type File struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Role    string `json:"role"`
	SHA256  string `json:"sha256"`
	Bytes   int    `json:"bytes"`
	Lines   int    `json:"lines"`
	Content string `json:"-"`
}

type Missing struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// Region is a candidate source range for localization.
type Region struct {
	ID        string `json:"id"`
	FileID    string `json:"file_id"`
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Label     string `json:"label"`
}

// Bundle is the evidence for one contract.
type Bundle struct {
	Files   []File    `json:"files"`
	Missing []Missing `json:"missing"`
	Skipped []Missing `json:"skipped"`
	Regions []Region  `json:"-"`
}

func (b Bundle) Bytes() int {
	n := 0
	for _, f := range b.Files {
		n += f.Bytes
	}
	return n
}

const (
	RoleScope   = "scope"
	RoleContext = "context"
)

// Collect selects every allowed file matching the contract's scope plus its
// explicit context files. A context file that cannot be supplied is Missing;
// a scope match that cannot be read is Skipped.
func Collect(ctx context.Context, s *Snapshot, cfg contracts.Config, c contracts.Contract) (Bundle, error) {
	b := Bundle{Files: []File{}, Missing: []Missing{}, Skipped: []Missing{}}
	inScope := func(p string) bool {
		for _, g := range c.Scope {
			if matchGlob(g, p) {
				return true
			}
		}
		return false
	}
	roles := map[string]string{}
	var paths []string
	for _, p := range s.paths {
		if !inScope(p) {
			continue
		}
		_, reason, err := s.load(ctx, p)
		if err != nil {
			return b, err
		}
		if reason != "" {
			b.Skipped = append(b.Skipped, Missing{Path: p, Reason: reason})
			continue
		}
		roles[p] = RoleScope
		paths = append(paths, p)
	}
	var context []string
	for _, p := range c.Context {
		if _, ok := roles[p]; ok {
			continue
		}
		if !cfg.Allowed(p) {
			b.Missing = append(b.Missing, Missing{Path: p, Reason: ReasonExcluded})
			continue
		}
		_, reason, err := s.load(ctx, p)
		if err != nil {
			return b, err
		}
		if reason != "" {
			b.Missing = append(b.Missing, Missing{Path: p, Reason: reason})
			continue
		}
		roles[p] = RoleContext
		context = append(context, p)
	}
	sort.Strings(context)
	for i, p := range append(paths, context...) {
		content, _, _ := s.load(ctx, p)
		b.Files = append(b.Files, File{
			ID:      fmt.Sprintf("F%03d", i+1),
			Path:    p,
			Role:    roles[p],
			SHA256:  Hash(content),
			Bytes:   len(content),
			Lines:   lineCount(content),
			Content: string(content),
		})
	}
	b.Regions = regions(b.Files)
	return b, nil
}

func lineCount(b []byte) int {
	if len(b) == 0 {
		return 0
	}
	n := bytes.Count(b, []byte("\n"))
	if b[len(b)-1] != '\n' {
		n++
	}
	return n
}

// regions splits files at top-level blocks: a non-blank line with no
// indentation that follows a blank line starts a new region. Files fall back
// to one region each when there would be too many options.
func regions(files []File) []Region {
	var out []Region
	for _, f := range files {
		out = append(out, fileRegions(f)...)
	}
	if len(out) > MaxRegions {
		out = out[:0]
		for _, f := range files {
			if f.Lines == 0 {
				continue
			}
			out = append(out, Region{FileID: f.ID, Path: f.Path, StartLine: 1, EndLine: f.Lines, Label: "whole file"})
		}
	}
	if len(out) > MaxRegions {
		return nil
	}
	for i := range out {
		out[i].ID = fmt.Sprintf("R%03d", i+1)
	}
	return out
}

func fileRegions(f File) []Region {
	lines := strings.Split(strings.TrimSuffix(f.Content, "\n"), "\n")
	if f.Lines == 0 {
		return nil
	}
	var out []Region
	start := -1
	prevBlank := true
	end := func(last int) {
		if start < 0 {
			return
		}
		for last > start && strings.TrimSpace(lines[last]) == "" {
			last--
		}
		out = append(out, Region{FileID: f.ID, Path: f.Path, StartLine: start + 1, EndLine: last + 1, Label: label(lines[start])})
		start = -1
	}
	for i, l := range lines {
		blank := strings.TrimSpace(l) == ""
		topLevel := !blank && l[0] != ' ' && l[0] != '\t'
		switch {
		case start < 0 && !blank:
			start = i
		case topLevel && prevBlank && start >= 0:
			end(i - 1)
			start = i
		}
		prevBlank = blank
	}
	end(len(lines) - 1)
	return out
}

func label(line string) string {
	l := strings.TrimSpace(line)
	if r := []rune(l); len(r) > 80 {
		l = string(r[:80]) + "…"
	}
	return l
}

// Excerpt returns lines [start, end] of a file, capped for reports.
func Excerpt(f File, start, end int) string {
	lines := strings.Split(f.Content, "\n")
	if start < 1 || start > len(lines) {
		return ""
	}
	if end > len(lines) {
		end = len(lines)
	}
	if end-start > 19 {
		end = start + 19
	}
	out := strings.Join(lines[start-1:end], "\n")
	if len(out) > 2000 {
		out = strings.ToValidUTF8(out[:2000], "")
	}
	return out
}

func matchGlob(pattern, p string) bool { return doublestar.MatchUnvalidated(pattern, p) }
