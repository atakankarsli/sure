// Package testutil builds throwaway Git repositories for tests.
package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Isolate keeps the developer's global and system Git config out of a test.
func Isolate(t testing.TB) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
}

// NewRepo creates an empty repository on branch main and returns its path.
func NewRepo(t testing.TB) string {
	t.Helper()
	Isolate(t)
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	Git(t, dir, "init", "-q", "-b", "main")
	Git(t, dir, "config", "commit.gpgsign", "false")
	return dir
}

func Git(t testing.TB, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func Write(t testing.TB, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// WriteFiles writes a map of relative path to content.
func WriteFiles(t testing.TB, dir string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		Write(t, dir, rel, content)
	}
}

// Commit stages everything and commits, returning the new commit ID.
func Commit(t testing.TB, dir, msg string) string {
	t.Helper()
	Git(t, dir, "add", "-A")
	Git(t, dir, "commit", "-q", "--allow-empty", "-m", msg)
	return Git(t, dir, "rev-parse", "HEAD")
}
