// Package cmdtest runs the whole CLI in-process against injected writers,
// environment and providers.
package cmdtest

import (
	"bytes"
	"strings"
	"time"

	"github.com/atakankarsli/sure/cmd"
	"github.com/atakankarsli/sure/internal/cli/shared"
)

type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// Run executes one CLI invocation. env replaces the process environment
// entirely; factory may be nil for commands that never build a provider.
func Run(env map[string]string, factory shared.ProviderFactory, args ...string) Result {
	return RunWithStdin(env, factory, "", args...)
}

// RunWithStdin is Run with the given standard input.
func RunWithStdin(env map[string]string, factory shared.ProviderFactory, stdin string, args ...string) Result {
	var out, errb bytes.Buffer
	getenv := func(k string) string { return env[k] }
	factory.Getenv = getenv
	deps := shared.Deps{
		Stdin:    strings.NewReader(stdin),
		Stdout:   &out,
		Stderr:   &errb,
		Getenv:   getenv,
		Now:      time.Now,
		NewRunID: func() string { return "run_cmdtest" },
		Factory:  factory,
		Version:  "test-version",
	}
	code := cmd.Run(args, deps)
	return Result{ExitCode: code, Stdout: out.String(), Stderr: strings.TrimSpace(errb.String())}
}
