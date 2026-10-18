package contracts

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// Change is one difference between the trusted contracts and the working
// tree's. Every change needs human review; none of them alters what is
// evaluated.
type Change struct {
	Kind     string `json:"kind"`
	Contract string `json:"contract,omitempty"`
	Check    string `json:"check,omitempty"`
	Field    string `json:"field,omitempty"`
	Detail   string `json:"detail"`
}

const (
	ChangeConfig          = "config_changed"
	ChangeConfigInvalid   = "config_invalid"
	ChangeContractAdded   = "contract_added"
	ChangeContractRemoved = "contract_removed"
	ChangeContractInvalid = "contract_invalid"
	ChangeContractField   = "contract_changed"
	ChangeCheckAdded      = "check_added"
	ChangeCheckRemoved    = "check_removed"
	ChangeCheckChanged    = "check_changed"
)

// Diff lists how candidate differs from trusted. A nil candidate (the working
// tree has no config) is reported as a config change.
func Diff(trusted, candidate *Set, candidateErr error) []Change {
	var out []Change
	if candidateErr != nil {
		return []Change{{Kind: ChangeConfigInvalid, Field: ConfigPath, Detail: candidateErr.Error()}}
	}
	if !candidate.Valid() {
		for _, is := range candidate.Issues {
			kind := ChangeContractInvalid
			if is.File == ConfigPath {
				kind = ChangeConfigInvalid
			}
			out = append(out, Change{Kind: kind, Field: is.File, Detail: is.String()})
		}
	}
	if candidate.Config.SchemaVersion != "" {
		for _, f := range configFieldDiff(trusted.Config, candidate.Config) {
			out = append(out, Change{Kind: ChangeConfig, Field: f, Detail: "config field " + f + " differs from the trusted base"})
		}
	}

	cand := map[string]Contract{}
	for _, c := range candidate.Contracts {
		cand[c.ID] = c
	}
	trustedIDs := map[string]bool{}
	for _, t := range trusted.Contracts {
		trustedIDs[t.ID] = true
		c, ok := cand[t.ID]
		if !ok {
			if candidate.Valid() {
				out = append(out, Change{Kind: ChangeContractRemoved, Contract: t.ID,
					Detail: "contract " + t.ID + " is missing from the working tree; the trusted version is still evaluated"})
			}
			continue
		}
		out = append(out, contractDiff(t, c)...)
	}
	for _, c := range candidate.Contracts {
		if !trustedIDs[c.ID] {
			out = append(out, Change{Kind: ChangeContractAdded, Contract: c.ID,
				Detail: fmt.Sprintf("new contract %s (%d checks) is not evaluated until it is part of the trusted base", c.ID, len(c.Checks))})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Contract != b.Contract {
			return a.Contract < b.Contract
		}
		if a.Check != b.Check {
			return a.Check < b.Check
		}
		return a.Kind+a.Field < b.Kind+b.Field
	})
	return out
}

func contractDiff(t, c Contract) []Change {
	var out []Change
	field := func(name string, a, b any) {
		if !reflect.DeepEqual(a, b) {
			out = append(out, Change{Kind: ChangeContractField, Contract: t.ID, Field: name,
				Detail: fmt.Sprintf("%s changed for %s; the trusted value is used", name, t.ID)})
		}
	}
	field("severity", t.Severity, c.Severity)
	field("evidence", t.Evidence, c.Evidence)
	field("scope", t.Scope, c.Scope)
	field("context", t.Context, c.Context)
	field("assumptions", t.Assumptions, c.Assumptions)

	cc := map[string]Check{}
	for _, ch := range c.Checks {
		cc[ch.LocalID] = ch
	}
	seen := map[string]bool{}
	for _, tc := range t.Checks {
		seen[tc.LocalID] = true
		id := t.CheckID(tc)
		got, ok := cc[tc.LocalID]
		switch {
		case !ok:
			out = append(out, Change{Kind: ChangeCheckRemoved, Contract: t.ID, Check: id,
				Detail: id + " was removed from the working tree; the trusted check is still evaluated"})
		case got.Requirement != tc.Requirement:
			out = append(out, Change{Kind: ChangeCheckChanged, Contract: t.ID, Check: id, Field: "requirement",
				Detail: id + " requirement text changed; the trusted wording is evaluated"})
		case got.Title != tc.Title:
			out = append(out, Change{Kind: ChangeCheckChanged, Contract: t.ID, Check: id, Field: "title",
				Detail: id + " title changed"})
		}
	}
	for _, ch := range c.Checks {
		if !seen[ch.LocalID] {
			id := c.CheckID(ch)
			out = append(out, Change{Kind: ChangeCheckAdded, Contract: t.ID, Check: id,
				Detail: id + " is new and is not evaluated until it is part of the trusted base"})
		}
	}
	return out
}

func configFieldDiff(a, b Config) []string {
	var fields []string
	av, bv := reflect.ValueOf(a), reflect.ValueOf(b)
	for i := 0; i < av.NumField(); i++ {
		if !reflect.DeepEqual(av.Field(i).Interface(), bv.Field(i).Interface()) {
			sub := av.Field(i)
			if sub.Kind() == reflect.Struct {
				for j := 0; j < sub.NumField(); j++ {
					if !reflect.DeepEqual(sub.Field(j).Interface(), bv.Field(i).Field(j).Interface()) {
						fields = append(fields, jsonName(av.Type().Field(i).Tag.Get("json"))+"."+
							jsonName(sub.Type().Field(j).Tag.Get("json")))
					}
				}
				continue
			}
			fields = append(fields, jsonName(av.Type().Field(i).Tag.Get("json")))
		}
	}
	return fields
}

func jsonName(tag string) string {
	name, _, _ := strings.Cut(tag, ",")
	return name
}
