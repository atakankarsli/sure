package evidence

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/atakankarsli/sure/internal/contracts"
	"github.com/atakankarsli/sure/internal/gitview"
	"github.com/atakankarsli/sure/internal/testutil"
)

func config(t *testing.T, extra string) contracts.Config {
	t.Helper()
	cfg, issues := contracts.ParseConfig(contracts.ConfigPath, []byte(
		"schema_version: sure.config/v1\nsource_roots: [src]\nexclude: [\"**/*.test.*\"]\n"+extra))
	if len(issues) > 0 {
		t.Fatal(issues)
	}
	return cfg
}

func repo(t *testing.T, files map[string]string) (string, gitview.Source) {
	t.Helper()
	dir := testutil.NewRepo(t)
	testutil.WriteFiles(t, dir, files)
	r, err := gitview.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return dir, r.Worktree()
}

func TestCaptureAllowlistsBeforeAnythingElse(t *testing.T) {
	dir, src := repo(t, map[string]string{
		"src/a.ts":        "export const a = 1;\n",
		"src/a.test.ts":   "test('leak', () => {})\n",
		"src/.env":        "SECRET=1\n",
		"src/big.ts":      strings.Repeat("x", 2000),
		"oracle/check.ts": "expected verdicts\n",
		"README.md":       "readme\n",
	})
	testutil.Write(t, dir, "src/img.png", "\x89PNG\x00\x01")
	cfg := config(t, "limits:\n  max_file_bytes: 1024\n")
	s, err := Capture(context.Background(), src, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Paths(); !reflect.DeepEqual(got, []string{"src/a.ts"}) {
		t.Fatalf("captured %v", got)
	}
	if s.Skipped["src/big.ts"] != ReasonTooLarge || s.Skipped["src/img.png"] != ReasonBinary {
		t.Fatalf("skipped %v", s.Skipped)
	}
	before := s.Digest()

	testutil.Write(t, dir, "src/new.ts", "untracked\n")
	s2, _ := Capture(context.Background(), src, cfg)
	if s2.Digest() == before {
		t.Fatal("untracked source file did not change the digest")
	}
	testutil.Write(t, dir, "src/a.test.ts", "changed test\n")
	testutil.Write(t, dir, "README.md", "changed\n")
	s3, _ := Capture(context.Background(), src, cfg)
	if s3.Digest() != s2.Digest() {
		t.Fatal("excluded files changed the source digest")
	}
}

func TestCollectOrdersScopeThenContextAndReportsGaps(t *testing.T) {
	s := FromFiles("worktree", "", map[string][]byte{
		"src/features/checkout/finish.ts": []byte("a\n"),
		"src/features/checkout/api.ts":    []byte("b\n"),
		"src/domain/orders.ts":            []byte("c\n"),
		"src/domain/cart.ts":              []byte("d\n"),
	}, map[string]string{"src/features/checkout/logo.png": ReasonBinary})
	cfg := config(t, "")
	c := contracts.Contract{
		ID:      "checkout.failure",
		Scope:   []string{"src/features/checkout/**"},
		Context: []string{"src/domain/orders.ts", "src/domain/cart.ts", "src/domain/missing.ts", "src/features/checkout/api.ts"},
	}
	b := Collect(s, cfg, c)
	var got []string
	for _, f := range b.Files {
		got = append(got, f.ID+" "+f.Role+" "+f.Path)
	}
	want := []string{
		"F001 scope src/features/checkout/api.ts",
		"F002 scope src/features/checkout/finish.ts",
		"F003 context src/domain/cart.ts",
		"F004 context src/domain/orders.ts",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("files:\n%v\nwant\n%v", got, want)
	}
	if !reflect.DeepEqual(b.Missing, []Missing{{Path: "src/domain/missing.ts", Reason: ReasonNotFound}}) {
		t.Fatalf("missing %v", b.Missing)
	}
	if !reflect.DeepEqual(b.Skipped, []Missing{{Path: "src/features/checkout/logo.png", Reason: ReasonBinary}}) {
		t.Fatalf("skipped %v", b.Skipped)
	}

	empty := Collect(s, cfg, contracts.Contract{ID: "x", Scope: []string{"src/nothing/**"}})
	if len(empty.Files) != 0 || len(empty.Missing) != 0 {
		t.Fatalf("empty scope collected %v", empty)
	}
}

func TestRegionsSplitTopLevelBlocks(t *testing.T) {
	src := "import { x } from './x';\n\nexport function a() {\n  return 1;\n\n  // inner blank line stays\n}\n\nexport function b() {\n  return 2;\n}\n"
	b := Collect(FromFiles("worktree", "", map[string][]byte{"src/a.ts": []byte(src)}, nil),
		config(t, ""), contracts.Contract{Scope: []string{"src/**"}})
	var got []string
	for _, r := range b.Regions {
		got = append(got, r.ID+" "+r.Label+" "+strconv.Itoa(r.StartLine)+"-"+strconv.Itoa(r.EndLine))
	}
	want := []string{
		"R001 import { x } from './x'; 1-1",
		"R002 export function a() { 3-7",
		"R003 export function b() { 9-11",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("regions %v", got)
	}
	if ex := Excerpt(b.Files[0], 9, 11); ex != "export function b() {\n  return 2;\n}" {
		t.Fatalf("excerpt %q", ex)
	}
}

func TestVisualManifest(t *testing.T) {
	dir, src := repo(t, map[string]string{"shots/home.png": "pixels"})
	digest := strings.Repeat("a", 64)
	entry := func(artifact, sha, source string) string {
		return `{"schema_version":"sure.visual-manifest/v1","entries":[{"check":"home.look/VIS-001","scenario":"home","artifact":"` +
			artifact + `","artifact_sha256":"` + sha + `","source_digest":"` + source +
			`","producer":{"name":"shots","version":"1"},"viewport":{"width":390,"height":844},"locale":"en","theme":"light","text_size":"default"}]}`
	}
	good := Hash([]byte("pixels"))
	ctx := context.Background()
	assess := func(doc string) string {
		m, err := ParseVisualManifest([]byte(doc))
		if err != nil {
			t.Fatal(err)
		}
		status, _ := AssessVisual(ctx, &m, "home.look/VIS-001", digest, src)
		return status
	}
	if s := assess(entry("shots/home.png", good, digest)); s != VisualUnsupported {
		t.Errorf("valid artifact: %s", s)
	}
	if s := assess(entry("shots/home.png", good, strings.Repeat("b", 64))); s != VisualStale {
		t.Errorf("stale source: %s", s)
	}
	if s := assess(entry("shots/home.png", strings.Repeat("c", 64), digest)); s != VisualHashMismatch {
		t.Errorf("hash mismatch: %s", s)
	}
	if s := assess(entry("shots/none.png", good, digest)); s != VisualMissing {
		t.Errorf("missing artifact: %s", s)
	}
	if err := os.Symlink(filepath.Join(dir, "shots", "home.png"), filepath.Join(dir, "shots", "link.png")); err != nil {
		t.Fatal(err)
	}
	if s := assess(entry("shots/link.png", good, digest)); s != VisualUnsafe {
		t.Errorf("symlinked artifact: %s", s)
	}
	if s, _ := AssessVisual(ctx, nil, "home.look/VIS-001", digest, src); s != VisualMissing {
		t.Errorf("no manifest: %s", s)
	}
	for _, bad := range []string{
		entry("../outside.png", good, digest),
		entry("shots/home.png", "nothex", digest),
		strings.Replace(entry("shots/home.png", good, digest), `"theme"`, `"extra":1,"theme"`, 1),
		strings.Replace(entry("shots/home.png", good, digest), "visual-manifest/v1", "visual-manifest/v2", 1),
	} {
		if _, err := ParseVisualManifest([]byte(bad)); err == nil {
			t.Errorf("accepted bad manifest %s", bad)
		}
	}
}
