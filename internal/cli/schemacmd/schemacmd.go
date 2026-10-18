// Package schemacmd implements `sure schema`.
package schemacmd

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/peterbourgon/ff/v3/ffcli"

	"github.com/atakankarsli/sure/internal/cli/shared"
	"github.com/atakankarsli/sure/schemas"
)

func Command(deps shared.Deps) *ffcli.Command {
	fs := flag.NewFlagSet("schema", flag.ContinueOnError)
	return &ffcli.Command{
		Name:       "schema",
		ShortUsage: "sure schema [name]",
		ShortHelp:  "Print a published JSON schema (default: run-result).",
		LongHelp:   "Available: " + strings.Join(names(), ", "),
		FlagSet:    fs,
		UsageFunc:  shared.DefaultUsageFunc,
		Exec: func(_ context.Context, args []string) error {
			name := "run-result"
			switch len(args) {
			case 0:
			case 1:
				name = args[0]
			default:
				return fmt.Errorf("unexpected argument %q", args[1])
			}
			b, err := schemas.FS.ReadFile(name + ".schema.json")
			if err != nil {
				return fmt.Errorf("unknown schema %q; available: %s", name, strings.Join(names(), ", "))
			}
			_, err = deps.Stdout.Write(b)
			return err
		},
	}
}

func names() []string {
	entries, _ := fs.Glob(schemas.FS, "*.schema.json")
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, strings.TrimSuffix(e, ".schema.json"))
	}
	sort.Strings(out)
	return out
}
