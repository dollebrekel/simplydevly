package credential

import (
	"context"
	"fmt"
	"os"

	"siply.dev/siply/internal/core"
)

// envKeyMap maps provider names to environment variable names.
var envKeyMap = map[string]string{
	"anthropic":  "ANTHROPIC_API_KEY",
	"openai":     "OPENAI_API_KEY",
	"openrouter": "OPENROUTER_API_KEY",
}

// EnvStore implements core.CredentialStore by reading API keys from
// environment variables. This is the minimal bootstrap for Story 2.6;
// full credential management comes in Story 3.2.
type EnvStore struct{}

func (s *EnvStore) Init(_ context.Context) error  { return nil }
func (s *EnvStore) Start(_ context.Context) error { return nil }
func (s *EnvStore) Stop(_ context.Context) error  { return nil }
func (s *EnvStore) Health() error                 { return nil }

// GetProvider reads the API key from the corresponding environment variable.
func (s *EnvStore) GetProvider(_ context.Context, provider string) (core.Credential, error) {
	envVar, ok := envKeyMap[provider]
	if !ok {
		// Ollama needs no API key.
		if provider == "ollama" {
			return core.Credential{Value: "unused"}, nil
		}
		return core.Credential{}, fmt.Errorf("credential: unknown provider %q", provider)
	}
	val := os.Getenv(envVar)
	if val == "" {
		return core.Credential{}, fmt.Errorf("credential: %s environment variable is not set", envVar)
	}
	return core.Credential{Value: val}, nil
}

// SetProvider is a no-op for env-based credentials.
func (s *EnvStore) SetProvider(_ context.Context, _ string, _ core.Credential) error {
	return fmt.Errorf("credential: env store is read-only")
}

// GetPluginCredential is not supported in the env store.
func (s *EnvStore) GetPluginCredential(_ context.Context, _ string, _ string) (core.Credential, error) {
	return core.Credential{}, fmt.Errorf("credential: plugin credentials not supported in env store")
}

// SetPluginCredential is not supported in the env store.
func (s *EnvStore) SetPluginCredential(_ context.Context, _ string, _ string, _ core.Credential) error {
	return fmt.Errorf("credential: plugin credentials not supported in env store")
}
