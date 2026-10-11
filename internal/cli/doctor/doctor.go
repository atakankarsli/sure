// Package doctor implements `sure doctor`.
package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/peterbourgon/ff/v3/ffcli"

	"github.com/atakankarsli/sure/internal/cache"
	"github.com/atakankarsli/sure/internal/cli/shared"
	"github.com/atakankarsli/sure/internal/contracts"
	"github.com/atakankarsli/sure/internal/engine"
	"github.com/atakankarsli/sure/internal/gitview"
	"github.com/atakankarsli/sure/internal/provider"
	"github.com/atakankarsli/sure/internal/report"
)

const (
	ok   = "ok"
	warn = "warn"
	bad  = "fail"
)

func Command(deps shared.Deps) *ffcli.Command {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	repo := fs.String("repo", ".", "Repository to inspect")
	prov := fs.String("provider", "", "Provider to inspect: jev or anthropic (default: from config)")
	live := fs.Bool("live", false, "Send one tiny synthetic request (no source) to verify the key and endpoint")
	asJSON := fs.Bool("json", false, "Print one JSON document")
	return &ffcli.Command{
		Name:       "doctor",
		ShortUsage: "sure doctor [--live] [--provider jev|anthropic] [--json]",
		ShortHelp:  "Check setup: Git, contracts, provider key; --live tests the model.",
		LongHelp: `Offline by default. With --live, sends one synthetic question that contains
no source code to the configured provider. Keys are reported as set or unset,
never printed.`,
		FlagSet:   fs,
		UsageFunc: shared.DefaultUsageFunc,
		Exec: func(ctx context.Context, args []string) error {
			d := report.Doctor{SchemaVersion: report.DoctorSchema, Checks: []report.DoctorCheck{}, Errors: []report.Error{}}
			exit := report.ExitClear
			if len(args) > 0 {
				d.Errors = append(d.Errors, report.Error{Code: "invalid_flags", Message: "unexpected argument " + args[0]})
				exit = report.ExitUsage
			} else {
				exit = diagnose(ctx, deps, *repo, *prov, *live, &d)
			}
			d.ExitCode, d.OK = exit, exit == report.ExitClear
			if err := shared.Print(deps.Stdout, *asJSON, d, func() string { return human(d, deps.Style()) }); err != nil {
				return err
			}
			return shared.Exit(exit)
		},
	}
}

func diagnose(ctx context.Context, deps shared.Deps, dir, provName string, live bool, d *report.Doctor) int {
	exit := report.ExitClear
	add := func(name, status, detail string) {
		d.Checks = append(d.Checks, report.DoctorCheck{Name: name, Status: status, Detail: detail})
		if status == bad && exit == report.ExitClear {
			exit = report.ExitUsage
		}
	}

	if out, err := exec.CommandContext(ctx, "git", "--version").Output(); err != nil {
		add("git", warn, "git is not available; only --draft checks of plain directories work")
	} else {
		add("git", ok, strings.TrimSpace(string(out)))
	}

	repo, err := gitview.Open(ctx, dir)
	if err != nil {
		add("repository", bad, err.Error())
		return exit
	}
	head, _ := repo.Head(ctx)
	switch {
	case !repo.IsGit:
		add("repository", warn, repo.Root+" is not a Git repository; protected checks need one")
	case head == "":
		add("repository", warn, repo.Root+" has no commits yet")
	default:
		add("repository", ok, fmt.Sprintf("%s at %s", repo.Root, head[:12]))
	}

	var cfg *contracts.Config
	set, err := contracts.Load(ctx, repo.Worktree())
	switch {
	case errors.Is(err, contracts.ErrNotInitialized):
		add("config", bad, err.Error())
	case err != nil:
		add("config", bad, err.Error())
	case !set.Valid():
		add("config", bad, fmt.Sprintf("%d validation issues; run 'sure contracts validate'", len(set.Issues)))
	default:
		cfg = &set.Config
		add("config", ok, fmt.Sprintf("%d contracts, %d checks in the working tree", len(set.Contracts), set.CheckCount()))
	}

	if repo.IsGit && head != "" {
		trusted, err := contracts.Load(ctx, repo.Tree(head))
		switch {
		case errors.Is(err, contracts.ErrNotInitialized):
			add("trusted_contracts", warn, "HEAD has no committed .sure/config.yaml; commit .sure/ or check with --draft")
		case err != nil:
			add("trusted_contracts", bad, err.Error())
		case !trusted.Valid():
			add("trusted_contracts", bad, "contracts committed at HEAD are invalid")
		default:
			add("trusted_contracts", ok, fmt.Sprintf("%d contracts, %d checks at HEAD", len(trusted.Contracts), trusted.CheckCount()))
		}
	}

	spec := engine.ProviderSpec{Name: contracts.DefaultProviderName, BaseURL: deps.Getenv("SURE_PROVIDER_BASE_URL")}
	if cfg != nil {
		spec.Name, spec.Transport, spec.Model, spec.Effort = cfg.Provider.Name, cfg.Provider.Transport, cfg.Provider.Model, cfg.Provider.Effort
	}
	if provName != "" && provName != spec.Name {
		spec = engine.ProviderSpec{Name: provName, BaseURL: spec.BaseURL}
	}
	id, err := deps.Factory.Identity(spec)
	if err != nil {
		add("provider", bad, err.Error())
		return exit
	}
	add("provider", ok, fmt.Sprintf("%s/%s model %s at %s (%s probabilities)", id.Name, id.Transport, id.Model, id.Endpoint, id.ProbabilitySource))
	keyEnv := shared.KeyEnvFor(id)
	if deps.Getenv(keyEnv) == "" {
		add("api_key", bad, keyEnv+" is not set")
	} else {
		add("api_key", ok, keyEnv+" is set")
	}

	if dirPath, err := cache.DefaultDir(deps.Getenv); err != nil {
		add("cache", warn, err.Error())
	} else if c, err := cache.Open(dirPath, cache.Namespace(repo.Root)); err != nil {
		add("cache", warn, err.Error())
	} else {
		add("cache", ok, c.Dir())
	}

	if live {
		if code := probe(ctx, deps, spec, add); code != report.ExitClear && (exit == report.ExitClear || exit == report.ExitUsage) {
			exit = code
		}
	}
	return exit
}

// probe asks one synthetic question that carries no repository content.
func probe(ctx context.Context, deps shared.Deps, spec engine.ProviderSpec, add func(string, string, string)) int {
	p, err := deps.Factory.New(spec)
	if err != nil {
		add("live_probe", bad, err.Error())
		return report.ExitUsage
	}
	state, _ := json.Marshal(map[string]string{"note": "Sure connectivity probe. It contains no source code."})
	instr, _ := json.Marshal(map[string]string{"task": "Is the number 7 greater than the number 3?"})
	req := provider.Request{State: state, Questions: []provider.Question{{ID: "probe", Instructions: instr,
		Options: []provider.Option{{Name: "yes", Description: "7 is greater than 3."}, {Name: "no", Description: "7 is not greater than 3."}}}}}
	pctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	resp, err := p.Evaluate(pctx, req)
	if err != nil {
		add("live_probe", bad, err.Error())
		return engine.ExitFor(provider.KindOf(err))
	}
	a := resp.Answers["probe"]
	add("live_probe", ok, fmt.Sprintf("model %s answered %q (p=%.2f), %d input tokens", resp.Model, a.Choice, a.Probabilities[a.Choice], resp.Usage.InputTokens))
	return report.ExitClear
}

func human(d report.Doctor, st report.Style) string {
	var b strings.Builder
	for _, c := range d.Checks {
		mark := st.Green("✓")
		switch c.Status {
		case warn:
			mark = st.Yellow("○")
		case bad:
			mark = st.Red("✗")
		}
		fmt.Fprintf(&b, "%s %-17s %s\n", mark, c.Name, st.Dim(c.Detail))
	}
	for _, e := range d.Errors {
		fmt.Fprintf(&b, "%s %s\n", st.Red("✗"), e.Message)
	}
	return b.String()
}
