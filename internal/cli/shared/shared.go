// Package shared holds what every Sure command uses: injected dependencies,
// output rendering, exit signalling and the default provider factory.
package shared

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/atakankarsli/sure/internal/engine"
)

// Deps is constructed once per invocation; tests replace any part of it.
type Deps struct {
	Stdout   io.Writer
	Stderr   io.Writer
	Getenv   func(string) string
	Now      func() time.Time
	NewRunID func() string
	Factory  engine.Factory
	Version  string
}

func DefaultDeps(version string) Deps {
	return Deps{
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
		Getenv:   os.Getenv,
		Now:      time.Now,
		NewRunID: NewRunID,
		Factory:  ProviderFactory{Getenv: os.Getenv},
		Version:  version,
	}
}

func (d Deps) Engine() engine.Deps {
	return engine.Deps{Getenv: d.Getenv, Now: d.Now, NewRunID: d.NewRunID, Factory: d.Factory, Stderr: d.Stderr}
}

func NewRunID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "run_" + time.Now().UTC().Format("20060102T150405Z") + "_" + hex.EncodeToString(b)
}

// ExitCode is returned by a command that has already written its output and
// only needs the process to exit with a code.
type ExitCode int

func (e ExitCode) Error() string { return fmt.Sprintf("exit code %d", int(e)) }

// Exit returns nil for 0 so a successful Exec stays a nil error.
func Exit(code int) error {
	if code == 0 {
		return nil
	}
	return ExitCode(code)
}

const (
	FormatJSON     = "json"
	FormatMarkdown = "markdown"
	FormatTable    = "table"
)

// BindOutput registers --output. SURE_DEFAULT_OUTPUT sets the default; the
// flag always wins.
func BindOutput(fs *flag.FlagSet, getenv func(string) string) *string {
	def := FormatMarkdown
	switch v := strings.ToLower(strings.TrimSpace(getenv("SURE_DEFAULT_OUTPUT"))); v {
	case FormatJSON, FormatTable, FormatMarkdown:
		def = v
	case "md":
		def = FormatMarkdown
	}
	return fs.String("output", def, "Output format: markdown, json or table")
}

func NormalizeFormat(f string) (string, error) {
	switch v := strings.ToLower(strings.TrimSpace(f)); v {
	case FormatJSON, FormatMarkdown, FormatTable:
		return v, nil
	case "md":
		return FormatMarkdown, nil
	default:
		return "", fmt.Errorf("--output must be markdown, json or table, not %q", f)
	}
}

// Print writes exactly one document to stdout: indented JSON, or the given
// renderer's text.
func Print(w io.Writer, format string, data any, render func() string) error {
	if format == FormatJSON || render == nil {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return enc.Encode(data)
	}
	s := render()
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	_, err := io.WriteString(w, s)
	return err
}

// StringList is a repeatable flag that also splits on commas.
type StringList []string

func (s *StringList) String() string { return strings.Join(*s, ",") }

func (s *StringList) Set(v string) error {
	for _, part := range strings.Split(v, ",") {
		if p := strings.TrimSpace(part); p != "" {
			*s = append(*s, p)
		}
	}
	return nil
}

// RejectArgs fails a command that takes no positional arguments.
func RejectArgs(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("unexpected argument %q", args[0])
	}
	return nil
}
