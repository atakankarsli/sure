package evidence

import (
	"context"
	"path"
	"regexp"
	"sort"
	"strings"
)

// MaxImportScan bounds how many files Related reads to find importers.
const MaxImportScan = 5000

var (
	jsSpec = regexp.MustCompile(`(?:\bfrom\s*|\bimport\s*\(\s*|\bimport\s+|\brequire\s*\(\s*)["'](\.{1,2}/[^"'\n]+)["']`)
	pyFrom = regexp.MustCompile(`(?m)^\s*from\s+(\.+)([\w.]*)\s+import\b`)

	jsExts  = []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".mts", ".cts", ".vue", ".svelte"}
	jsTries = []string{"", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".mts", ".cts", "/index.ts", "/index.tsx", "/index.js", "/index.jsx"}
)

func isJS(p string) bool {
	for _, e := range jsExts {
		if strings.HasSuffix(p, e) {
			return true
		}
	}
	return false
}

// Imports returns the files a source file imports through relative paths
// (JavaScript, TypeScript and Python), resolved against the snapshot.
func (s *Snapshot) Imports(ctx context.Context, p string) []string {
	if !isJS(p) && !strings.HasSuffix(p, ".py") {
		return nil
	}
	b, reason, err := s.load(ctx, p)
	if err != nil || reason != "" {
		return nil
	}
	dir := path.Dir(p)
	seen := map[string]bool{}
	var out []string
	add := func(x string) {
		if x != p && !seen[x] && s.Has(x) {
			seen[x] = true
			out = append(out, x)
		}
	}
	if isJS(p) {
		for _, m := range jsSpec.FindAllStringSubmatch(string(b), -1) {
			base := path.Join(dir, m[1])
			if strings.HasSuffix(m[1], ".js") {
				// TypeScript source imported with its emitted .js name.
				for _, e := range []string{".ts", ".tsx"} {
					if s.Has(strings.TrimSuffix(base, ".js") + e) {
						add(strings.TrimSuffix(base, ".js") + e)
					}
				}
			}
			for _, t := range jsTries {
				if s.Has(base + t) {
					add(base + t)
					break
				}
			}
		}
		return out
	}
	for _, m := range pyFrom.FindAllStringSubmatch(string(b), -1) {
		d := dir
		for i := 1; i < len(m[1]); i++ {
			d = path.Dir(d)
		}
		mod := strings.ReplaceAll(m[2], ".", "/")
		if mod == "" {
			continue
		}
		base := path.Join(d, mod)
		if s.Has(base + ".py") {
			add(base + ".py")
		} else if s.Has(base + "/__init__.py") {
			add(base + "/__init__.py")
		}
	}
	return out
}

// Related finds the files next to a change: what the changed files import,
// then the files that import them. It adds them in that order while the
// changed and related files together stay within budget bytes.
func Related(ctx context.Context, s *Snapshot, changed []string, budget int) []string {
	isChanged := map[string]bool{}
	used := 0
	for _, p := range changed {
		isChanged[p] = true
		if b, reason, err := s.load(ctx, p); err == nil && reason == "" {
			used += len(b)
		}
	}
	var forward []string
	seen := map[string]bool{}
	for _, p := range changed {
		for _, x := range s.Imports(ctx, p) {
			if !isChanged[x] && !seen[x] {
				seen[x] = true
				forward = append(forward, x)
			}
		}
	}
	sort.Strings(forward)
	var reverse []string
	scanned := 0
	for _, p := range s.paths {
		if isChanged[p] || seen[p] || (!isJS(p) && !strings.HasSuffix(p, ".py")) {
			continue
		}
		if scanned++; scanned > MaxImportScan {
			break
		}
		for _, x := range s.Imports(ctx, p) {
			if isChanged[x] {
				seen[p] = true
				reverse = append(reverse, p)
				break
			}
		}
	}
	var out []string
	for _, p := range append(forward, reverse...) {
		b, reason, err := s.load(ctx, p)
		if err != nil || reason != "" || used+len(b) > budget {
			continue
		}
		used += len(b)
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
