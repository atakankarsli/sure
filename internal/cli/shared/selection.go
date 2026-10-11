package shared

import (
	"errors"
	"flag"

	"github.com/atakankarsli/sure/internal/engine"
	"github.com/atakankarsli/sure/internal/provider"
)

// Selection holds the flags shared by plan and check.
type Selection struct {
	Repo         *string
	All          *bool
	Changed      *bool
	Contracts    StringList
	Base         *string
	ContractsRef *string
	Draft        *bool
	Provider     *string
	Recordings   *string
}

func BindSelection(fs *flag.FlagSet) *Selection {
	s := &Selection{
		Repo:         fs.String("repo", ".", "Repository to check"),
		All:          fs.Bool("all", false, "Select every trusted contract"),
		Changed:      fs.Bool("changed", false, "Select contracts whose scope or context intersects changed paths"),
		Base:         fs.String("base", "", "With --changed: compare against the merge base of this ref and HEAD (default HEAD)"),
		ContractsRef: fs.String("contracts-ref", "", "Read trusted contracts from this ref (default: the comparison base, else HEAD)"),
		Draft:        fs.Bool("draft", false, "Use the working tree's contracts; the run is labelled trust_mode=draft and is not a protected gate"),
		Provider:     fs.String("provider", "", "Decision provider: jev, anthropic or replay (default: from config)"),
		Recordings:   fs.String("recordings", "", "With --provider replay: directory of recorded decisions"),
	}
	fs.Var(&s.Contracts, "contract", "Select a contract by id (repeatable or comma-separated)")
	return s
}

// Options validates flag combinations and converts them to engine options.
func (s *Selection) Options() (engine.Options, error) {
	o := engine.Options{
		Repo:         *s.Repo,
		Contracts:    s.Contracts,
		BaseRef:      *s.Base,
		ContractsRef: *s.ContractsRef,
		Draft:        *s.Draft,
		Provider:     *s.Provider,
		ReplayDir:    *s.Recordings,
	}
	n := 0
	if *s.All {
		n++
		o.Mode = engine.ModeAll
	}
	if *s.Changed {
		n++
		o.Mode = engine.ModeChanged
	}
	if len(s.Contracts) > 0 {
		n++
		o.Mode = engine.ModeContract
	}
	switch {
	case n != 1:
		return o, errors.New("choose exactly one of --all, --changed or --contract")
	case o.BaseRef != "" && o.Mode != engine.ModeChanged:
		return o, errors.New("--base only applies with --changed")
	case o.Draft && o.ContractsRef != "":
		return o, errors.New("--draft and --contracts-ref cannot be combined")
	}
	switch o.Provider {
	case "", provider.NameJev, provider.NameAnthropic, provider.NameReplay:
	default:
		return o, errors.New("--provider must be jev, anthropic or replay")
	}
	if o.ReplayDir != "" && o.Provider != provider.NameReplay {
		return o, errors.New("--recordings only applies with --provider replay")
	}
	return o, nil
}
