package contracts

import (
	"strings"
	"testing"

	"github.com/bmatcuk/doublestar/v4"
)

func TestParseAdhocLightList(t *testing.T) {
	doc := "Intro text is ignored.\n\n- Failed payment keeps the cart\n- Success clears the cart\n"
	_, checks, issues := ParseAdhoc("x.md", []byte(doc))
	if len(issues) != 0 || len(checks) != 2 || checks[1].Requirement != "Success clears the cart" {
		t.Fatalf("bullets: %+v %v", checks, issues)
	}

	doc = "# Checks\n\n## Keep the cart\nWhen payment fails,\nthe cart keeps its items.\n- detail bullet stays in the text\n\n## PAY-007 — Retry\n\n### Totals use server prices\n"
	_, checks, issues = ParseAdhoc("x.md", []byte(doc))
	if len(issues) != 0 || len(checks) != 4 {
		t.Fatalf("headings: %+v %v", checks, issues)
	}
	if checks[1].Requirement != "When payment fails, the cart keeps its items. - detail bullet stays in the text" {
		t.Fatalf("requirement %q", checks[1].Requirement)
	}
	if checks[2].LocalID != "PAY-007" || checks[2].Requirement != "Retry" || checks[3].Requirement != "Totals use server prices" {
		t.Fatalf("ids and bare headings: %+v", checks[2:])
	}

	if _, _, issues := ParseAdhoc("x.md", []byte("just prose\n")); len(issues) != 1 || issues[0].Code != "no_checks" {
		t.Fatalf("empty: %v", issues)
	}
	full, light, issues := ParseAdhoc("c.md", []byte(checkoutContract))
	if len(issues) != 0 || len(light) != 0 || len(full) != 1 || full[0].ID != "checkout.failure" {
		t.Fatalf("full contract: %v %v", full, issues)
	}
}

func TestAdhocContract(t *testing.T) {
	c, issues := AdhocContract([]Check{Expectation("  keeps   the cart "), {LocalID: "PAY-1", Title: "t", Requirement: "r"}}, nil)
	if len(issues) != 0 || c.ID != AdhocID || c.Checks[0].LocalID != "ADHOC-001" || c.Checks[0].Requirement != "keeps the cart" || c.Checks[1].LocalID != "PAY-1" {
		t.Fatalf("%+v %v", c, issues)
	}
	if _, issues := AdhocContract([]Check{Expectation("x")}, []string{"../up/**"}); len(issues) != 1 {
		t.Fatalf("bad scope accepted: %v", issues)
	}
	for _, p := range []string{"src/a[1].ts", "src/{x}.ts", "src/what?.ts"} {
		if ok, _ := doublestar.Match(EscapeGlob(p), p); !ok {
			t.Errorf("%s does not match its escaped glob %s", p, EscapeGlob(p))
		}
		if ok, _ := doublestar.Match(EscapeGlob(p), strings.Replace(p, "src/", "src/z", 1)); ok {
			t.Errorf("escaped %s matched another path", p)
		}
	}
	if len(DefaultConfig().Exclude) == 0 {
		t.Fatal("default config has no exclusions")
	}
}
