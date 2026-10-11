// Package gitview reads a repository without changing it: the working tree,
// committed trees, and the paths that changed between a base commit and the
// working tree. Git runs with argument arrays, no shell, external diff and
// textconv disabled, fsmonitor off, and bounded output.
package gitview

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

var (
	ErrNotFound = errors.New("file not found")
	ErrSymlink  = errors.New("symbolic links are not read")
	ErrNotFile  = errors.New("not a regular file")
	ErrTooLarge = errors.New("file exceeds the size limit")
	ErrUnsafe   = errors.New("unsafe path")
	ErrNoGit    = errors.New("not a git repository")
)

// MaxGitOutput bounds what any single git command may print.
const MaxGitOutput = 64 << 20

type Repo struct {
	Root   string
	IsGit  bool
	gitBin string

	formatOnce sync.Once
	format     string
}

// Open resolves dir to its repository root. A directory outside Git still
// opens, with IsGit false, so plain trees can be checked in draft mode.
func Open(ctx context.Context, dir string) (*Repo, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("repository %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("repository %s is not a directory", dir)
	}
	r := &Repo{Root: abs}
	bin, err := exec.LookPath("git")
	if err != nil {
		return r, nil
	}
	r.gitBin = bin
	out, err := r.git(ctx, abs, "rev-parse", "--show-toplevel")
	if err != nil {
		return r, nil
	}
	top := strings.TrimSpace(string(out))
	if top == "" {
		return r, nil
	}
	if resolved, err := filepath.EvalSymlinks(top); err == nil {
		top = resolved
	}
	r.Root = top
	r.IsGit = true
	return r, nil
}

func (r *Repo) git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	full := append([]string{
		"-c", "core.fsmonitor=false",
		"-c", "core.quotepath=off",
		"-c", "diff.external=",
		"-c", "core.untrackedCache=false",
		"--no-pager",
	}, args...)
	cmd := exec.CommandContext(ctx, r.gitBin, full...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_LITERAL_PATHSPECS=1",
		"GIT_EXTERNAL_DIFF=",
		"GIT_PAGER=cat",
		"LC_ALL=C",
	)
	var stdout limitedBuffer
	stdout.max = MaxGitOutput
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &limitedWriter{w: &stderr, n: 4096}
	err := cmd.Run()
	if stdout.overflow {
		return nil, fmt.Errorf("git %s: output exceeds %d bytes", args[0], MaxGitOutput)
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("git %s: %s", args[0], firstLine(msg))
	}
	return stdout.Bytes(), nil
}

func (r *Repo) run(ctx context.Context, args ...string) ([]byte, error) {
	if !r.IsGit {
		return nil, ErrNoGit
	}
	return r.git(ctx, r.Root, args...)
}

// ValidateRef rejects values that git could read as an option or that carry
// control characters.
func ValidateRef(ref string) error {
	if ref == "" {
		return errors.New("empty ref")
	}
	if strings.HasPrefix(ref, "-") {
		return fmt.Errorf("ref %q may not start with '-'", ref)
	}
	for _, c := range ref {
		if c < 0x20 || c == 0x7f || c == ' ' {
			return fmt.Errorf("ref %q contains a control or space character", ref)
		}
	}
	return nil
}

// ResolveCommit returns the full commit ID that ref names.
func (r *Repo) ResolveCommit(ctx context.Context, ref string) (string, error) {
	if err := ValidateRef(ref); err != nil {
		return "", err
	}
	out, err := r.run(ctx, "rev-parse", "--verify", "--quiet", "--end-of-options", ref+"^{commit}")
	if err != nil {
		if errors.Is(err, ErrNoGit) || ctx.Err() != nil {
			return "", err
		}
		return "", fmt.Errorf("unknown commit %q", ref)
	}
	return strings.TrimSpace(string(out)), nil
}

// Head returns HEAD's commit, or "" when the branch has no commits yet.
func (r *Repo) Head(ctx context.Context) (string, error) {
	if !r.IsGit {
		return "", nil
	}
	out, err := r.run(ctx, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", nil
	}
	return strings.TrimSpace(string(out)), nil
}

// DefaultBase names the ref changes are compared against: the remote's default
// branch, else a local main or master, else HEAD.
func (r *Repo) DefaultBase(ctx context.Context) string {
	if out, err := r.run(ctx, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil {
		if ref := strings.TrimSpace(string(out)); ref != "" {
			return ref
		}
	}
	for _, ref := range []string{"origin/main", "origin/master", "main", "master"} {
		if _, err := r.ResolveCommit(ctx, ref); err == nil {
			return ref
		}
	}
	return "HEAD"
}

func (r *Repo) MergeBase(ctx context.Context, a, b string) (string, error) {
	out, err := r.run(ctx, "merge-base", "--end-of-options", a, b)
	if err != nil {
		return "", fmt.Errorf("no merge base between %s and %s", short(a), short(b))
	}
	return strings.TrimSpace(string(out)), nil
}

type Change struct {
	Status  string `json:"status"`
	Path    string `json:"path"`
	OldPath string `json:"old_path,omitempty"`
}

// Paths returns every path a change touches, old and new.
func (c Change) Paths() []string {
	if c.OldPath != "" && c.OldPath != c.Path {
		return []string{c.OldPath, c.Path}
	}
	return []string{c.Path}
}

// Changes lists what differs between base and the working tree: committed,
// staged and unstaged edits, deletions, renames, and untracked files that are
// not ignored.
func (r *Repo) Changes(ctx context.Context, base string) ([]Change, error) {
	out, err := r.run(ctx, "diff", "--name-status", "-z", "-M", "--no-ext-diff", "--no-textconv",
		"--ignore-submodules=all", "--end-of-options", base, "--")
	if err != nil {
		return nil, err
	}
	changes, err := parseNameStatus(out)
	if err != nil {
		return nil, err
	}
	untracked, err := r.run(ctx, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	for _, p := range splitZ(untracked) {
		changes = append(changes, Change{Status: "A", Path: p})
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes, nil
}

func parseNameStatus(out []byte) ([]Change, error) {
	fields := splitZ(out)
	var changes []Change
	for i := 0; i < len(fields); {
		status := fields[i]
		i++
		if status == "" {
			continue
		}
		kind := status[:1]
		switch kind {
		case "R", "C":
			if i+1 >= len(fields) {
				return nil, errors.New("git diff: truncated rename record")
			}
			old, nw := fields[i], fields[i+1]
			i += 2
			if kind == "C" {
				changes = append(changes, Change{Status: "A", Path: nw})
				continue
			}
			changes = append(changes, Change{Status: "R", Path: nw, OldPath: old})
		default:
			if i >= len(fields) {
				return nil, errors.New("git diff: truncated record")
			}
			changes = append(changes, Change{Status: kind, Path: fields[i]})
			i++
		}
	}
	return changes, nil
}

func splitZ(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	parts := strings.Split(strings.TrimSuffix(string(b), "\x00"), "\x00")
	out := parts[:0]
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// CleanRel validates a repository-relative slash path.
func CleanRel(p string) (string, error) {
	if p == "" || strings.ContainsRune(p, 0) || strings.Contains(p, "\\") {
		return "", fmt.Errorf("%w: %q", ErrUnsafe, p)
	}
	if path.IsAbs(p) || filepath.IsAbs(p) {
		return "", fmt.Errorf("%w: %q is absolute", ErrUnsafe, p)
	}
	c := path.Clean(p)
	if c == "." || c == ".." || strings.HasPrefix(c, "../") {
		return "", fmt.Errorf("%w: %q leaves the repository", ErrUnsafe, p)
	}
	if c == ".git" || strings.HasPrefix(c, ".git/") {
		return "", fmt.Errorf("%w: %q is Git metadata", ErrUnsafe, p)
	}
	return c, nil
}

// Source is a read-only view of files: the working tree or a commit.
type Source interface {
	List(ctx context.Context) ([]string, error)
	// Inventory lists files with the object ID Git already has for their
	// current content, so unchanged files need not be read to be identified.
	Inventory(ctx context.Context) ([]Entry, error)
	Read(ctx context.Context, rel string, maxBytes int64) ([]byte, error)
	Describe() string
	// ObjectFormat is the hash Git uses for blob IDs ("sha1" or "sha256").
	ObjectFormat(ctx context.Context) string
}

// Entry is one file in an inventory. Blob is empty when the content differs
// from what Git has recorded (or there is no Git), so the caller must hash it.
type Entry struct {
	Path    string
	Blob    string
	Symlink bool
}

// BlobID computes Git's object ID for content, matching IDs from the index.
func BlobID(format string, content []byte) string {
	header := fmt.Sprintf("blob %d\x00", len(content))
	if format == "sha256" {
		h := sha256.New()
		h.Write([]byte(header))
		h.Write(content)
		return hex.EncodeToString(h.Sum(nil))
	}
	h := sha1.New()
	h.Write([]byte(header))
	h.Write(content)
	return hex.EncodeToString(h.Sum(nil))
}

func (r *Repo) objectFormat(ctx context.Context) string {
	if !r.IsGit {
		return "sha1"
	}
	r.formatOnce.Do(func() {
		r.format = "sha1"
		if out, err := r.run(ctx, "rev-parse", "--show-object-format"); err == nil && strings.TrimSpace(string(out)) == "sha256" {
			r.format = "sha256"
		}
	})
	return r.format
}

func (w worktree) ObjectFormat(ctx context.Context) string { return w.r.objectFormat(ctx) }

func (t tree) ObjectFormat(ctx context.Context) string { return t.r.objectFormat(ctx) }

// Inventory reads the index for tracked files and marks the ones whose
// working-tree content differs, and all untracked files, for hashing.
func (w worktree) Inventory(ctx context.Context) ([]Entry, error) {
	if !w.r.IsGit {
		files, err := w.List(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]Entry, len(files))
		for i, f := range files {
			out[i] = Entry{Path: f}
		}
		return out, nil
	}
	staged, err := w.r.run(ctx, "ls-files", "--stage", "-z")
	if err != nil {
		return nil, err
	}
	dirty, err := w.r.run(ctx, "diff-files", "--name-only", "-z", "--no-ext-diff", "--ignore-submodules=all")
	if err != nil {
		return nil, err
	}
	changed := map[string]bool{}
	for _, p := range splitZ(dirty) {
		changed[p] = true
	}
	entries := map[string]Entry{}
	for _, rec := range splitZ(staged) {
		meta, p, ok := strings.Cut(rec, "\t")
		f := strings.Fields(meta)
		if !ok || len(f) != 3 || f[0] == "160000" {
			continue
		}
		e := Entry{Path: p, Symlink: f[0] == "120000"}
		if f[2] == "0" && !changed[p] {
			e.Blob = f[1]
		}
		entries[p] = e
	}
	others, err := w.r.run(ctx, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	for _, p := range splitZ(others) {
		entries[p] = Entry{Path: p}
	}
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// Inventory lists a commit's files with their blob IDs.
func (t tree) Inventory(ctx context.Context) ([]Entry, error) {
	out, err := t.r.run(ctx, "ls-tree", "-r", "-z", "--full-tree", "--end-of-options", t.commit)
	if err != nil {
		return nil, err
	}
	var entries []Entry
	for _, rec := range splitZ(out) {
		meta, p, ok := strings.Cut(rec, "\t")
		f := strings.Fields(meta)
		if !ok || len(f) != 3 || f[1] != "blob" {
			continue
		}
		entries = append(entries, Entry{Path: p, Blob: f[2], Symlink: f[0] == "120000"})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

func (r *Repo) Worktree() Source { return worktree{r} }

func (r *Repo) Tree(commit string) Source { return tree{r, commit} }

type worktree struct{ r *Repo }

func (w worktree) Describe() string { return "worktree" }

// List returns tracked and untracked, non-ignored files. Outside Git it walks
// the directory, skipping .git and symlinked directories.
func (w worktree) List(ctx context.Context) ([]string, error) {
	if w.r.IsGit {
		out, err := w.r.run(ctx, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
		if err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		var files []string
		for _, p := range splitZ(out) {
			if !seen[p] {
				seen[p] = true
				files = append(files, p)
			}
		}
		sort.Strings(files)
		return files, nil
	}
	var files []string
	err := filepath.WalkDir(w.r.Root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if d.Type().IsRegular() {
			rel, err := filepath.Rel(w.r.Root, p)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

// Read returns a regular file's bytes. Symlinks are never followed, and every
// parent directory must be a real directory inside the root.
func (w worktree) Read(_ context.Context, rel string, maxBytes int64) ([]byte, error) {
	c, err := CleanRel(rel)
	if err != nil {
		return nil, err
	}
	cur := w.r.Root
	parts := strings.Split(c, "/")
	for i, part := range parts {
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil, ErrNotFound
			}
			return nil, err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return nil, ErrSymlink
		}
		last := i == len(parts)-1
		if !last && !info.IsDir() {
			return nil, ErrNotFound
		}
		if last {
			if !info.Mode().IsRegular() {
				return nil, ErrNotFile
			}
			if maxBytes > 0 && info.Size() > maxBytes {
				return nil, ErrTooLarge
			}
		}
	}
	f, err := os.Open(cur)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readLimited(f, maxBytes)
}

type tree struct {
	r      *Repo
	commit string
}

func (t tree) Describe() string { return t.commit }

// List returns regular files in the commit; symlinks and submodules are left
// out.
func (t tree) List(ctx context.Context) ([]string, error) {
	out, err := t.r.run(ctx, "ls-tree", "-r", "-z", "--full-tree", "--end-of-options", t.commit)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, rec := range splitZ(out) {
		meta, p, ok := strings.Cut(rec, "\t")
		if !ok {
			continue
		}
		f := strings.Fields(meta)
		if len(f) != 3 || f[1] != "blob" || f[0] == "120000" {
			continue
		}
		files = append(files, p)
	}
	sort.Strings(files)
	return files, nil
}

func (t tree) Read(ctx context.Context, rel string, maxBytes int64) ([]byte, error) {
	c, err := CleanRel(rel)
	if err != nil {
		return nil, err
	}
	spec := t.commit + ":" + c
	info, err := t.r.run(ctx, "ls-tree", "-z", "--full-tree", "--end-of-options", t.commit, "--", c)
	if err != nil {
		return nil, err
	}
	recs := splitZ(info)
	if len(recs) == 0 {
		return nil, ErrNotFound
	}
	meta, _, _ := strings.Cut(recs[0], "\t")
	f := strings.Fields(meta)
	if len(f) != 3 {
		return nil, ErrNotFound
	}
	if f[0] == "120000" {
		return nil, ErrSymlink
	}
	if f[1] != "blob" {
		return nil, ErrNotFile
	}
	if maxBytes > 0 {
		size, err := t.r.run(ctx, "cat-file", "-s", spec)
		if err != nil {
			return nil, err
		}
		var n int64
		if _, err := fmt.Sscan(strings.TrimSpace(string(size)), &n); err == nil && n > maxBytes {
			return nil, ErrTooLarge
		}
	}
	return t.r.run(ctx, "cat-file", "blob", spec)
}

func readLimited(r io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return io.ReadAll(r)
	}
	b, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > maxBytes {
		return nil, ErrTooLarge
	}
	return b, nil
}

type limitedBuffer struct {
	bytes.Buffer
	max      int
	overflow bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.max {
		b.overflow = true
		return 0, errors.New("output limit exceeded")
	}
	return b.Buffer.Write(p)
}

type limitedWriter struct {
	w io.Writer
	n int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.n <= 0 {
		return len(p), nil
	}
	q := p
	if len(q) > l.n {
		q = q[:l.n]
	}
	l.n -= len(q)
	_, _ = l.w.Write(q)
	return len(p), nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
