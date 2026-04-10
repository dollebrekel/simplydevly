// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package core

// ConfigResolver provides layered configuration resolution.
// Merge order: global (~/.siply/config.yaml) → project (.siply/config.yaml)
// → lockfile (.siply/config.lock) → runtime flags.
// Each layer can only override, not remove keys from the parent layer.
type ConfigResolver interface {
	Lifecycle

	// Config returns the fully resolved configuration.
	Config() *Config
}

// Config is the root configuration structure merged from all layers.
type Config struct {
	Provider  ProviderConfig  `yaml:"provider" json:"provider"`
	Routing   RoutingConfig   `yaml:"routing" json:"routing"`
	Session   SessionConfig   `yaml:"session" json:"session"`
	Telemetry TelemetryConfig `yaml:"telemetry" json:"telemetry"`
	TUI       TUIConfig       `yaml:"tui,omitempty" json:"tui,omitzero"`
	// Plugins holds plugin-specific configuration keyed by plugin name.
	// Each plugin owns its own namespace; values are opaque to the loader.
	// Layer merge (global→project→lockfile) is shallow per plugin name.
	// Runtime Tier 1 plugin loading uses deep merge via PluginConfigMerger.
	Plugins map[string]any `yaml:"plugins" json:"plugins"`
}

// TUIConfig holds TUI presentation settings.
type TUIConfig struct {
	Profile string `yaml:"profile,omitempty" json:"profile,omitempty"` // "minimal", "standard", or "" (first-run needed)
}

// ProviderConfig holds AI provider settings.
type ProviderConfig struct {
	Default string `yaml:"default" json:"default"`
	Model   string `yaml:"model" json:"model"`
}

// RoutingConfig holds smart routing configuration.
type RoutingConfig struct {
	Enabled            *bool  `yaml:"enabled" json:"enabled"`
	DefaultProvider    string `yaml:"default_provider" json:"default_provider"`
	PreprocessProvider string `yaml:"preprocess_provider" json:"preprocess_provider"`
	PreprocessModel    string `yaml:"preprocess_model" json:"preprocess_model"`
}

// SessionConfig holds session management settings.
type SessionConfig struct {
	RetentionCount *int `yaml:"retention_count" json:"retention_count"`
}

// TelemetryConfig holds telemetry settings.
type TelemetryConfig struct {
	Enabled *bool `yaml:"enabled" json:"enabled"`
}
