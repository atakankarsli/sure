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

// Snapshot holds the bytes of every allowlisted file, read once, so the
// model, digests and locations all describe the same content.
type Snapshot struct {
	Kind    string
	Ref     string
	Files   map[string][]byte
	Skipped map[string]string
	paths   []string
	digest  string
}

// Capture reads every file the config allows from src.
func Capture(ctx context.Context, src gitview.Source, cfg contracts.Config) (*Snapshot, error) {
	all, err := src.List(ctx)
	if err != nil {
		return nil, err
	}
	s := &Snapshot{Kind: "worktree", Files: map[string][]byte{}, Skipped: map[string]string{}}
	if d := src.Describe(); d != "worktree" {
		s.Kind, s.Ref = "commit", d
	}
	total := 0
	for _, p := range all {
		if !cfg.Allowed(p) {
			continue
		}
		if len(s.Files)+len(s.Skipped) >= MaxSnapshotFiles {
			return nil, fmt.Errorf("source_roots select more than %d files; narrow them", MaxSnapshotFiles)
		}
		b, err := src.Read(ctx, p, cfg.Limits.MaxFileBytes)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			s.Skipped[p] = readReason(err)
			continue
		}
		if !isText(b) {
			s.Skipped[p] = ReasonBinary
			continue
		}
		total += len(b)
		if total > MaxSnapshotBytes {
			return nil, fmt.Errorf("source_roots hold more than %d bytes of text; narrow them", MaxSnapshotBytes)
		}
		s.Files[p] = b
	}
	s.finish()
	return s, nil
}

// FromFiles builds a snapshot from known content; used for tests and for
// base-commit reassessment.
func FromFiles(kind, ref string, files map[string][]byte, skipped map[string]string) *Snapshot {
	s := &Snapshot{Kind: kind, Ref: ref, Files: files, Skipped: skipped}
	if s.Skipped == nil {
		s.Skipped = map[string]string{}
	}
	s.finish()
	return s
}

func (s *Snapshot) finish() {
	s.paths = s.paths[:0]
	for p := range s.Files {
		s.paths = append(s.paths, p)
	}
	sort.Strings(s.paths)
	h := sha256.New()
	keys := make([]string, 0, len(s.Files)+len(s.Skipped))
	for p := range s.Files {
		keys = append(keys, p)
	}
	for p := range s.Skipped {
		keys = append(keys, p)
	}
	sort.Strings(keys)
	for _, p := range keys {
		if b, ok := s.Files[p]; ok {
			fmt.Fprintf(h, "%s\x00%s\n", p, Hash(b))
		} else {
			fmt.Fprintf(h, "%s\x00skipped:%s\n", p, s.Skipped[p])
		}
	}
	s.digest = hex.EncodeToString(h.Sum(nil))
}

// Digest identifies the captured source, including skipped entries.
func (s *Snapshot) Digest() string { return s.digest }

// Paths returns the readable files in sorted order.
func (s *Snapshot) Paths() []string { return s.paths }

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
func Collect(s *Snapshot, cfg contracts.Config, c contracts.Contract) Bundle {
	b := Bundle{Files: []File{}, Missing: []Missing{}, Skipped: []Missing{}}
	inScope := func(p string) bool {
		for _, g := range c.Scope {
			if matchGlob(g, p) {
				return true
			}
		}
		return false
	}
	var paths []string
	for _, p := range s.paths {
		if inScope(p) {
			paths = append(paths, p)
		}
	}
	var skipped []string
	for p := range s.Skipped {
		if inScope(p) {
			skipped = append(skipped, p)
		}
	}
	sort.Strings(skipped)
	for _, p := range skipped {
		b.Skipped = append(b.Skipped, Missing{Path: p, Reason: s.Skipped[p]})
	}
	roles := map[string]string{}
	for _, p := range paths {
		roles[p] = RoleScope
	}
	var context []string
	for _, p := range c.Context {
		if _, ok := roles[p]; ok {
			continue
		}
		if _, ok := s.Files[p]; ok {
			roles[p] = RoleContext
			context = append(context, p)
			continue
		}
		reason := ReasonNotFound
		if r, ok := s.Skipped[p]; ok {
			reason = r
		} else if !cfg.Allowed(p) {
			reason = ReasonExcluded
		}
		b.Missing = append(b.Missing, Missing{Path: p, Reason: reason})
	}
	sort.Strings(context)
	for i, p := range append(paths, context...) {
		content := s.Files[p]
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
	return b
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
