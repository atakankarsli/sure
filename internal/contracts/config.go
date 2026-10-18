package contracts

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"gopkg.in/yaml.v3"

	"github.com/atakankarsli/sure/internal/gitview"
)

const (
	ConfigSchema   = "sure.config/v1"
	ConfigPath     = ".sure/config.yaml"
	DefaultDir     = ".sure/contracts"
	MaxConfigBytes = 64 << 10
)

// Defaults are proposed, uncalibrated starting points, not provider
// guarantees.
const (
	DefaultConcurrency        = 4
	DefaultMaxFileBytes       = 256 << 10
	DefaultMaxStateBytes      = 80 << 10
	DefaultMaxRequestBytes    = 120 << 10
	DefaultMaxEstimatedTokens = 30000
	DefaultPolicyVersion      = "experimental-v1"
	DefaultMinSelected        = 0.90
	DefaultMinMargin          = 0.20
	DefaultMinLocation        = 0.50
	DefaultProviderName       = "jev"
	AnthropicTransport        = "messages"
)

// BuiltinExclude is always applied on top of the configured exclusions.
var BuiltinExclude = []string{
	".git/**",
	".sure/**",
	"**/.env",
	"**/.env.*",
	"**/node_modules/**",
	"**/*.pem",
	"**/*.key",
	"**/*.p12",
	"**/id_rsa*",
	"**/id_ed25519*",
	"**/.npmrc",
	"**/.netrc",
}

type Config struct {
	SchemaVersion string         `json:"schema_version"`
	Contracts     string         `json:"contracts"`
	SourceRoots   []string       `json:"source_roots"`
	Exclude       []string       `json:"exclude"`
	Provider      ProviderConfig `json:"provider"`
	Limits        Limits         `json:"limits"`
	Policy        Policy         `json:"policy"`
	Visual        VisualConfig   `json:"visual"`
}

type ProviderConfig struct {
	Name        string `json:"name"`
	Transport   string `json:"transport"`
	Model       string `json:"model"`
	Effort      string `json:"effort,omitempty"`
	LongContext bool   `json:"long_context,omitempty"`
}

type Limits struct {
	Concurrency        int   `json:"concurrency"`
	MaxFileBytes       int64 `json:"max_file_bytes"`
	MaxStateBytes      int   `json:"max_state_bytes"`
	MaxRequestBytes    int   `json:"max_request_bytes"`
	MaxEstimatedTokens int   `json:"max_estimated_tokens"`
}

type Policy struct {
	Version                string  `json:"version"`
	MinSelectedProbability float64 `json:"min_selected_probability"`
	MinProbabilityMargin   float64 `json:"min_probability_margin"`
	MinLocationProbability float64 `json:"min_location_probability"`
}

type VisualConfig struct {
	Manifest string `json:"manifest,omitempty"`
}

type rawConfig struct {
	SchemaVersion string   `yaml:"schema_version"`
	Contracts     string   `yaml:"contracts"`
	SourceRoots   []string `yaml:"source_roots"`
	Exclude       []string `yaml:"exclude"`
	Provider      struct {
		Name        string `yaml:"name"`
		Transport   string `yaml:"transport"`
		Model       string `yaml:"model"`
		Effort      string `yaml:"effort"`
		LongContext *bool  `yaml:"long_context"`
	} `yaml:"provider"`
	Limits struct {
		Concurrency        *int   `yaml:"concurrency"`
		MaxFileBytes       *int64 `yaml:"max_file_bytes"`
		MaxStateBytes      *int   `yaml:"max_state_bytes"`
		MaxRequestBytes    *int   `yaml:"max_request_bytes"`
		MaxEstimatedTokens *int   `yaml:"max_estimated_tokens"`
	} `yaml:"limits"`
	Policy struct {
		Version                string   `yaml:"version"`
		MinSelectedProbability *float64 `yaml:"min_selected_probability"`
		MinProbabilityMargin   *float64 `yaml:"min_probability_margin"`
		MinLocationProbability *float64 `yaml:"min_location_probability"`
	} `yaml:"policy"`
	Visual struct {
		Manifest string `yaml:"manifest"`
	} `yaml:"visual"`
}

var (
	modelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/:-]{0,99}$`)
	// Settings that would let a repository redirect source or credentials.
	forbiddenProviderKeys = map[string]bool{
		"base_url": true, "baseurl": true, "endpoint": true, "url": true, "host": true,
		"key_env": true, "api_key": true, "apikey": true, "key": true, "token": true,
	}
)

// ParseConfig strictly decodes and validates a config file. Every problem is
// returned as an Issue; Config is only meaningful when issues is empty.
func ParseConfig(file string, data []byte) (Config, []Issue) {
	var issues []Issue
	add := func(line int, code, format string, args ...any) {
		issues = append(issues, Issue{File: file, Line: line, Code: code, Message: fmt.Sprintf(format, args...)})
	}
	if len(data) > MaxConfigBytes {
		add(0, "config_too_large", "config exceeds %d bytes", MaxConfigBytes)
		return Config{}, issues
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		add(yamlLine(err), "invalid_yaml", "%s", cleanYAMLError(err))
		return Config{}, issues
	}
	for _, f := range forbiddenProviderSettings(&root) {
		add(f.line, "untrusted_provider_setting",
			"provider.%s is not allowed: endpoints and key variables cannot be set by repository config", f.key)
	}
	var raw rawConfig
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&raw); err != nil {
		if len(issues) == 0 {
			add(yamlLine(err), "invalid_config", "%s", cleanYAMLError(err))
		}
		return Config{}, issues
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err == nil {
		add(0, "invalid_config", "config must contain a single YAML document")
	}

	cfg := Config{
		SchemaVersion: raw.SchemaVersion,
		Contracts:     raw.Contracts,
		SourceRoots:   raw.SourceRoots,
		Exclude:       raw.Exclude,
		Provider: ProviderConfig{
			Name:        raw.Provider.Name,
			Transport:   raw.Provider.Transport,
			Model:       raw.Provider.Model,
			Effort:      raw.Provider.Effort,
			LongContext: raw.Provider.LongContext != nil && *raw.Provider.LongContext,
		},
		Policy: Policy{Version: raw.Policy.Version},
		Visual: VisualConfig{Manifest: raw.Visual.Manifest},
	}
	if cfg.SchemaVersion != ConfigSchema {
		add(0, "invalid_config", "schema_version must be %q", ConfigSchema)
	}
	if cfg.Contracts == "" {
		cfg.Contracts = DefaultDir
	} else if c, err := gitview.CleanRel(cfg.Contracts); err != nil {
		add(0, "unsafe_path", "contracts: %v", err)
	} else {
		cfg.Contracts = c
	}
	if len(cfg.SourceRoots) == 0 {
		add(0, "invalid_config", "source_roots must list at least one directory")
	}
	for i, r := range cfg.SourceRoots {
		if r == "." {
			continue
		}
		c, err := gitview.CleanRel(strings.TrimSuffix(r, "/"))
		if err != nil {
			add(0, "unsafe_path", "source_roots: %v", err)
			continue
		}
		cfg.SourceRoots[i] = c
	}
	for _, p := range cfg.Exclude {
		if err := validateGlob(p); err != nil {
			add(0, "bad_glob", "exclude: %v", err)
		}
	}

	switch cfg.Provider.Name {
	case "":
		cfg.Provider.Name = DefaultProviderName
		fallthrough
	case "jev":
		// An empty transport is chosen at run time from whichever key is set.
		switch cfg.Provider.Transport {
		case "", "gateway", "typesafe":
		default:
			add(0, "invalid_config", "provider.transport %q is not gateway or typesafe", cfg.Provider.Transport)
		}
		if cfg.Provider.Effort != "" {
			add(0, "invalid_config", "provider.effort applies only to the anthropic provider")
		}
		if cfg.Provider.LongContext && cfg.Provider.Transport == "gateway" {
			add(0, "invalid_config", "provider.long_context needs the typesafe transport")
		}
	case "anthropic":
		switch cfg.Provider.Transport {
		case "", AnthropicTransport:
			cfg.Provider.Transport = AnthropicTransport
		default:
			add(0, "invalid_config", "provider.transport for anthropic must be %q", AnthropicTransport)
		}
		if cfg.Provider.LongContext {
			add(0, "invalid_config", "provider.long_context applies only to the jev provider")
		}
		switch cfg.Provider.Effort {
		case "", "low", "medium", "high", "xhigh", "max":
		default:
			add(0, "invalid_config", "provider.effort %q is not low, medium, high, xhigh or max", cfg.Provider.Effort)
		}
	default:
		add(0, "invalid_config", "provider.name %q is not jev or anthropic", cfg.Provider.Name)
	}
	if cfg.Provider.Model != "" && !modelPattern.MatchString(cfg.Provider.Model) {
		add(0, "invalid_config", "provider.model %q has unexpected characters", cfg.Provider.Model)
	}

	l := raw.Limits
	cfg.Limits.Concurrency = intIn(l.Concurrency, DefaultConcurrency, 1, 16, "limits.concurrency", add)
	cfg.Limits.MaxFileBytes = int64(intIn(l.MaxFileBytes, DefaultMaxFileBytes, 1<<10, 4<<20, "limits.max_file_bytes", add))
	cfg.Limits.MaxStateBytes = intIn(l.MaxStateBytes, DefaultMaxStateBytes, 1<<10, 1<<20, "limits.max_state_bytes", add)
	cfg.Limits.MaxRequestBytes = intIn(l.MaxRequestBytes, DefaultMaxRequestBytes, 2<<10, 2<<20, "limits.max_request_bytes", add)
	cfg.Limits.MaxEstimatedTokens = intIn(l.MaxEstimatedTokens, DefaultMaxEstimatedTokens, 1000, 1000000, "limits.max_estimated_tokens", add)
	if cfg.Limits.MaxRequestBytes < cfg.Limits.MaxStateBytes {
		add(0, "invalid_config", "limits.max_request_bytes must be at least limits.max_state_bytes")
	}

	p := raw.Policy
	if cfg.Policy.Version == "" {
		cfg.Policy.Version = DefaultPolicyVersion
	}
	cfg.Policy.MinSelectedProbability = floatIn(p.MinSelectedProbability, DefaultMinSelected, 0.5, 1, "policy.min_selected_probability", add)
	cfg.Policy.MinProbabilityMargin = floatIn(p.MinProbabilityMargin, DefaultMinMargin, 0, 1, "policy.min_probability_margin", add)
	cfg.Policy.MinLocationProbability = floatIn(p.MinLocationProbability, DefaultMinLocation, 0, 1, "policy.min_location_probability", add)

	if cfg.Visual.Manifest != "" {
		if c, err := gitview.CleanRel(cfg.Visual.Manifest); err != nil {
			add(0, "unsafe_path", "visual.manifest: %v", err)
		} else {
			cfg.Visual.Manifest = c
		}
	}
	return cfg, issues
}

type forbidden struct {
	key  string
	line int
}

func forbiddenProviderSettings(root *yaml.Node) []forbidden {
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return nil
	}
	var out []forbidden
	top := root.Content[0]
	for i := 0; i+1 < len(top.Content); i += 2 {
		if top.Content[i].Value != "provider" || top.Content[i+1].Kind != yaml.MappingNode {
			continue
		}
		prov := top.Content[i+1]
		for j := 0; j+1 < len(prov.Content); j += 2 {
			k := prov.Content[j]
			if forbiddenProviderKeys[strings.ToLower(k.Value)] {
				out = append(out, forbidden{key: k.Value, line: k.Line})
			}
		}
	}
	return out
}

func intIn[T int | int64](v *T, def T, lo, hi T, name string, add func(int, string, string, ...any)) int {
	if v == nil {
		return int(def)
	}
	if *v < lo || *v > hi {
		add(0, "invalid_config", "%s must be between %d and %d", name, lo, hi)
		return int(def)
	}
	return int(*v)
}

func floatIn(v *float64, def, lo, hi float64, name string, add func(int, string, string, ...any)) float64 {
	if v == nil {
		return def
	}
	if *v < lo || *v > hi {
		add(0, "invalid_config", "%s must be between %g and %g", name, lo, hi)
		return def
	}
	return *v
}

func validateGlob(p string) error {
	if p == "" {
		return errors.New("empty pattern")
	}
	if strings.HasPrefix(p, "/") || strings.Contains(p, "\\") || strings.ContainsRune(p, 0) {
		return fmt.Errorf("pattern %q must be a relative slash path", p)
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return fmt.Errorf("pattern %q leaves the repository", p)
		}
	}
	if !doublestar.ValidatePattern(p) {
		return fmt.Errorf("pattern %q is not a valid glob", p)
	}
	return nil
}

// InSourceRoots reports whether a repository path lies under a source root.
func (c Config) InSourceRoots(p string) bool {
	for _, r := range c.SourceRoots {
		if r == "." || p == r || strings.HasPrefix(p, r+"/") {
			return true
		}
	}
	return false
}

// Excluded reports whether a path is removed from evidence by the built-in or
// configured exclusions.
func (c Config) Excluded(p string) bool {
	for _, pat := range BuiltinExclude {
		if doublestar.MatchUnvalidated(pat, p) {
			return true
		}
	}
	for _, pat := range c.Exclude {
		if doublestar.MatchUnvalidated(pat, p) {
			return true
		}
	}
	return false
}

// Allowed reports whether a path may become evidence.
func (c Config) Allowed(p string) bool {
	return c.InSourceRoots(p) && !c.Excluded(p)
}

func yamlLine(err error) int {
	var te *yaml.TypeError
	msg := err.Error()
	if errors.As(err, &te) && len(te.Errors) > 0 {
		msg = te.Errors[0]
	}
	var line int
	if i := strings.Index(msg, "line "); i >= 0 {
		fmt.Sscanf(msg[i+5:], "%d", &line)
	}
	return line
}

func cleanYAMLError(err error) string {
	var te *yaml.TypeError
	if errors.As(err, &te) {
		return strings.Join(te.Errors, "; ")
	}
	return strings.TrimPrefix(err.Error(), "yaml: ")
}
