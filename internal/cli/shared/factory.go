package shared

import (
	"fmt"

	"github.com/atakankarsli/sure/internal/engine"
	"github.com/atakankarsli/sure/internal/provider"
	"github.com/atakankarsli/sure/internal/provider/anthropic"
	"github.com/atakankarsli/sure/internal/provider/jev"
)

// ProviderFactory builds the real providers. Keys come only from each
// provider's fixed environment variable.
type ProviderFactory struct {
	Getenv func(string) string
	// Configure lets tests point providers at an httptest server.
	ConfigureJev       func(*jev.Config)
	ConfigureAnthropic func(*anthropic.Config)
}

// identityKey stands in for a credential when only the identity is needed;
// a provider built with it is never called.
const identityKey = "identity-only"

func (f ProviderFactory) Identity(spec engine.ProviderSpec) (provider.Identity, error) {
	p, err := f.build(spec, identityKey)
	if err != nil {
		return provider.Identity{}, err
	}
	return p.Identity(), nil
}

func (f ProviderFactory) New(spec engine.ProviderSpec) (provider.Provider, error) {
	env := KeyEnv(spec)
	if env == "" {
		return nil, fmt.Errorf("unknown provider %q", spec.Name)
	}
	key := f.Getenv(env)
	if key == "" {
		return nil, provider.Errorf(provider.ErrMissingKey, "set %s in the environment", env)
	}
	return f.build(spec, key)
}

// KeyEnv names the environment variable that holds the key for a provider.
func KeyEnv(spec engine.ProviderSpec) string {
	switch spec.Name {
	case provider.NameJev:
		if spec.Transport == jev.TransportTypeSafe {
			return jev.KeyEnvTypeSafe
		}
		return jev.KeyEnvGateway
	case provider.NameAnthropic:
		return anthropic.KeyEnv
	}
	return ""
}

func (f ProviderFactory) build(spec engine.ProviderSpec, key string) (provider.Provider, error) {
	switch spec.Name {
	case provider.NameJev:
		cfg := jev.Config{Transport: spec.Transport, Model: spec.Model, BaseURL: spec.BaseURL, APIKey: key}
		if cfg.Transport == "" {
			cfg.Transport = jev.TransportGateway
		}
		if f.ConfigureJev != nil {
			f.ConfigureJev(&cfg)
		}
		return jev.New(cfg)
	case provider.NameAnthropic:
		cfg := anthropic.Config{Model: spec.Model, Effort: spec.Effort, BaseURL: spec.BaseURL, APIKey: key}
		if f.ConfigureAnthropic != nil {
			f.ConfigureAnthropic(&cfg)
		}
		return anthropic.New(cfg)
	default:
		return nil, fmt.Errorf("unknown provider %q; use jev or anthropic", spec.Name)
	}
}
