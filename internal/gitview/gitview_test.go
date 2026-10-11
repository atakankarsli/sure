package gitview

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/atakankarsli/sure/internal/testutil"
)

func TestChangesCoverCommittedStagedUnstagedUntrackedRenameDelete(t *testing.T) {
	dir := testutil.NewRepo(t)
	testutil.WriteFiles(t, dir, map[string]string{
		"src/a.ts":    "export const a = 1;\n",
		"src/b.ts":    "export const b = 1;\n",
		"src/c.ts":    "export const c = 'a fairly long line so rename detection has content';\n",
		"src/h.ts":    "export const h = 1;\n",
		".gitignore":  "ignored.ts\n",
		"src/keep.ts": "export const keep = 1;\n",
	})
	base := testutil.Commit(t, dir, "base")
	testutil.Write(t, dir, "src/h.ts", "export const h = 2;\n")
	testutil.Commit(t, dir, "branch work")

	testutil.Write(t, dir, "src/a.ts", "export const a = 2;\n")
	testutil.Write(t, dir, "src/e.ts", "export const e = 1;\n")
	testutil.Git(t, dir, "add", "src/e.ts")
	testutil.Git(t, dir, "rm", "-q", "src/b.ts")
	testutil.Git(t, dir, "mv", "src/c.ts", "src/c2.ts")
	testutil.Write(t, dir, "src/f.ts", "export const f = 1;\n")
	testutil.Write(t, dir, "ignored.ts", "x\n")

	r := open(t, dir)
	got, err := r.Changes(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	want := []Change{
		{Status: "M", Path: "src/a.ts"},
		{Status: "D", Path: "src/b.ts"},
		{Status: "R", Path: "src/c2.ts", OldPath: "src/c.ts"},
		{Status: "A", Path: "src/e.ts"},
		{Status: "A", Path: "src/f.ts"},
		{Status: "M", Path: "src/h.ts"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("changes:\n got %+v\nwant %+v", got, want)
	}
	if p := (Change{Status: "R", Path: "n", OldPath: "o"}).Paths(); !reflect.DeepEqual(p, []string{"o", "n"}) {
		t.Fatalf("rename paths %v", p)
	}
}

func TestExternalDiffTextconvAndFsmonitorNeverRun(t *testing.T) {
	dir := testutil.NewRepo(t)
	marker := filepath.Join(t.TempDir(), "ran")
	script := filepath.Join(t.TempDir(), "hook.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Write(t, dir, "src/a.ts", "a\n")
	testutil.Write(t, dir, ".gitattributes", "*.ts diff=evil\n")
	base := testutil.Commit(t, dir, "base")
	testutil.Git(t, dir, "config", "diff.external", script)
	testutil.Git(t, dir, "config", "diff.evil.textconv", script)
	testutil.Git(t, dir, "config", "diff.evil.command", script)
	testutil.Git(t, dir, "config", "core.fsmonitor", script)
	testutil.Write(t, dir, "src/a.ts", "b\n")

	r := open(t, dir)
	if _, err := r.Changes(context.Background(), base); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Worktree().List(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a repository-configured program ran (marker present, err=%v)", err)
	}
}

func TestResolveCommitAndMergeBase(t *testing.T) {
	dir := testutil.NewRepo(t)
	testutil.Write(t, dir, "a", "1")
	first := testutil.Commit(t, dir, "one")
	testutil.Git(t, dir, "checkout", "-q", "-b", "feature")
	testutil.Write(t, dir, "a", "2")
	testutil.Commit(t, dir, "two")
	r := open(t, dir)
	ctx := context.Background()

	got, err := r.ResolveCommit(ctx, "main")
	if err != nil || got != first {
		t.Fatalf("resolve main = %q, %v", got, err)
	}
	head, _ := r.Head(ctx)
	mb, err := r.MergeBase(ctx, got, head)
	if err != nil || mb != first {
		t.Fatalf("merge base = %q, %v", mb, err)
	}
	for _, bad := range []string{"-x", "--output=/tmp/x", "a b", "nope"} {
		if _, err := r.ResolveCommit(ctx, bad); err == nil {
			t.Errorf("ResolveCommit(%q) succeeded", bad)
		}
	}
}

func TestWorktreeReadRefusesUnsafePaths(t *testing.T) {
	dir := testutil.NewRepo(t)
	outside := t.TempDir()
	testutil.Write(t, outside, "secret.txt", "secret")
	testutil.Write(t, dir, "src/ok.ts", "ok")
	testutil.Write(t, dir, "src/big.ts", "0123456789")
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(dir, "src", "link.ts")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "src", "linkdir")); err != nil {
		t.Fatal(err)
	}
	w := open(t, dir).Worktree()
	ctx := context.Background()

	if b, err := w.Read(ctx, "src/ok.ts", 100); err != nil || string(b) != "ok" {
		t.Fatalf("read ok: %q %v", b, err)
	}
	cases := map[string]error{
		"src/link.ts":            ErrSymlink,
		"src/linkdir/secret.txt": ErrSymlink,
		"../x":                   ErrUnsafe,
		"/etc/passwd":            ErrUnsafe,
		".git/config":            ErrUnsafe,
		"src/missing.ts":         ErrNotFound,
		"src/big.ts":             ErrTooLarge,
		"src":                    ErrNotFile,
	}
	for p, want := range cases {
		if _, err := w.Read(ctx, p, 5); !errors.Is(err, want) {
			t.Errorf("Read(%q) = %v, want %v", p, err, want)
		}
	}
}

func TestTreeReadsCommittedBytes(t *testing.T) {
	dir := testutil.NewRepo(t)
	testutil.Write(t, dir, ".sure/config.yaml", "committed")
	if err := os.Symlink("config.yaml", filepath.Join(dir, ".sure", "alias.yaml")); err != nil {
		t.Fatal(err)
	}
	c := testutil.Commit(t, dir, "base")
	testutil.Write(t, dir, ".sure/config.yaml", "edited")
	tr := open(t, dir).Tree(c)
	ctx := context.Background()

	files, err := tr.List(ctx)
	if err != nil || !reflect.DeepEqual(files, []string{".sure/config.yaml"}) {
		t.Fatalf("tree list = %v, %v", files, err)
	}
	b, err := tr.Read(ctx, ".sure/config.yaml", 100)
	if err != nil || string(b) != "committed" {
		t.Fatalf("tree read = %q, %v", b, err)
	}
	if _, err := tr.Read(ctx, ".sure/alias.yaml", 100); !errors.Is(err, ErrSymlink) {
		t.Fatalf("symlink read err = %v", err)
	}
	if _, err := tr.Read(ctx, ".sure/none.yaml", 100); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing read err = %v", err)
	}
	if _, err := tr.Read(ctx, ".sure/config.yaml", 3); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("size limit err = %v", err)
	}
}

func TestPlainDirectoryOpensWithoutGit(t *testing.T) {
	testutil.Isolate(t)
	dir := t.TempDir()
	testutil.Write(t, dir, "src/a.ts", "a")
	testutil.Write(t, dir, ".git-not/x", "b")
	r := open(t, dir)
	if r.IsGit {
		t.Fatal("plain directory reported as git")
	}
	files, err := r.Worktree().List(context.Background())
	if err != nil || !reflect.DeepEqual(files, []string{".git-not/x", "src/a.ts"}) {
		t.Fatalf("walk = %v, %v", files, err)
	}
	if _, err := r.Changes(context.Background(), "HEAD"); !errors.Is(err, ErrNoGit) {
		t.Fatalf("changes outside git: %v", err)
	}
}

func open(t *testing.T, dir string) *Repo {
	t.Helper()
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
