package contracts

import "testing"

func TestParseConfigLongContext(t *testing.T) {
	cfg, issues := ParseConfig(ConfigPath, []byte(minimalConfig+"provider:\n  long_context: true\n"))
	if len(issues) != 0 || !cfg.Provider.LongContext || cfg.Provider.Name != "jev" {
		t.Fatalf("long_context on jev: %+v %v", cfg.Provider, issues)
	}
	cfg, issues = ParseConfig(ConfigPath, []byte(minimalConfig+"provider:\n  transport: typesafe\n  long_context: true\n"))
	if len(issues) != 0 || !cfg.Provider.LongContext {
		t.Fatalf("long_context with typesafe: %+v %v", cfg.Provider, issues)
	}
	off, issues := ParseConfig(ConfigPath, []byte(minimalConfig))
	if len(issues) != 0 || off.Provider.LongContext {
		t.Fatalf("long_context must default to off: %+v %v", off.Provider, issues)
	}

	for name, doc := range map[string]string{
		"anthropic": minimalConfig + "provider:\n  name: anthropic\n  long_context: true\n",
		"gateway":   minimalConfig + "provider:\n  transport: gateway\n  long_context: true\n",
	} {
		_, issues := ParseConfig(ConfigPath, []byte(doc))
		found := false
		for _, is := range issues {
			found = found || is.Code == "invalid_config"
		}
		if !found {
			t.Errorf("%s: want invalid_config, got %v", name, issues)
		}
	}
}
