// Package shared holds what every Sure command uses: injected dependencies,
// output, exit signalling and the default provider factory.
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
	"github.com/atakankarsli/sure/internal/report"
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
	// Color enables ANSI styling of human output.
	Color bool
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
		Color:    colorTerminal(os.Stdout),
	}
}

func colorTerminal(f *os.File) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func (d Deps) Style() report.Style { return report.Style{Color: d.Color} }

func (d Deps) Engine(verbose bool) engine.Deps {
	e := engine.Deps{Getenv: d.Getenv, Now: d.Now, NewRunID: d.NewRunID, Factory: d.Factory, Stderr: d.Stderr}
	if verbose {
		e.Progress = d.Stderr
	}
	return e
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

// Print writes exactly one document to stdout: indented JSON, or the human
// rendering.
func Print(w io.Writer, asJSON bool, data any, human func() string) error {
	if asJSON || human == nil {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return enc.Encode(data)
	}
	s := human()
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	_, err := io.WriteString(w, s)
	return err
}

// ParseInterspersed lets flags follow positional arguments, so
// `sure checkout --json` works like `sure --json checkout`. Everything after
// "--" is positional.
func ParseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			break
		}
		if args[0] == "--" {
			pos = append(pos, args[1:]...)
			break
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
	return pos, nil
}
