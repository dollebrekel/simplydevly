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
	Agent     AgentSettings   `yaml:"agent,omitempty" json:"agent,omitzero"`
	Sandbox    SandboxConfig    `yaml:"sandbox,omitempty" json:"sandbox,omitzero"`
	Checkpoint CheckpointConfig `yaml:"checkpoint,omitempty" json:"checkpoint,omitzero"`
	// Plugins holds plugin-specific configuration keyed by plugin name.
	// Each plugin owns its own namespace; values are opaque to the loader.
	// Layer merge (global→project→lockfile) is shallow per plugin name.
	// Runtime Tier 1 plugin loading uses deep merge via PluginConfigMerger.
	Plugins map[string]any `yaml:"plugins" json:"plugins"`
}

// AgentSettings holds agent configuration selection.
type AgentSettings struct {
	Config string `yaml:"config" json:"config"` // name of the active agent config
}

// TUIConfig holds TUI presentation settings.
type TUIConfig struct {
	Profile string `yaml:"profile,omitempty" json:"profile,omitempty"` // "minimal", "standard", or "" (first-run needed)
}

// ProviderConfig holds AI provider settings.
type ProviderConfig struct {
	Default    string `yaml:"default" json:"default"`
	Model      string `yaml:"model" json:"model"`
	LocalModel string `yaml:"local_model,omitempty" json:"local_model,omitempty"`
	LocalURL   string `yaml:"local_url,omitempty" json:"local_url,omitempty"`
	// Deprecated aliases for backward compatibility (offline_model → local_model).
	OfflineModel string `yaml:"offline_model,omitempty" json:"offline_model,omitempty"`
	OfflineURL   string `yaml:"offline_url,omitempty" json:"offline_url,omitempty"`
}

// RoutingConfig holds smart routing configuration.
type RoutingConfig struct {
	Enabled            *bool                      `yaml:"enabled" json:"enabled"`
	DefaultProvider    string                     `yaml:"default_provider" json:"default_provider"`
	PreprocessProvider string                     `yaml:"preprocess_provider" json:"preprocess_provider"`
	PreprocessModel    string                     `yaml:"preprocess_model" json:"preprocess_model"`
	Pricing            map[string]ProviderPricing `yaml:"pricing,omitempty" json:"pricing,omitempty"`
	Rules              []RoutingRule              `yaml:"rules,omitempty" json:"rules,omitempty"`
	PreferCheapest     *bool                      `yaml:"prefer_cheapest,omitempty" json:"prefer_cheapest,omitempty"`
}

// ProviderPricing holds per-provider token cost information for cost-based routing.
type ProviderPricing struct {
	InputPer1M  float64 `yaml:"input_per_1m" json:"input_per_1m"`
	OutputPer1M float64 `yaml:"output_per_1m" json:"output_per_1m"`
}

// RoutingRule maps a task category to a provider and optional model override.
type RoutingRule struct {
	Category string `yaml:"category" json:"category"`
	Provider string `yaml:"provider" json:"provider"`
	Model    string `yaml:"model,omitempty" json:"model,omitempty"`
}

// SessionConfig holds session management settings.
type SessionConfig struct {
	RetentionCount *int `yaml:"retention_count" json:"retention_count"`
}

// TelemetryConfig holds telemetry settings.
type TelemetryConfig struct {
	Enabled *bool `yaml:"enabled" json:"enabled"`
}

// SandboxConfig holds OS-level sandbox settings for bash command isolation.
type SandboxConfig struct {
	Enabled           *bool    `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	FailIfUnavailable *bool    `yaml:"fail_if_unavailable,omitempty" json:"fail_if_unavailable,omitempty"`
	ExtraReadPaths    []string `yaml:"extra_read_paths,omitempty" json:"extra_read_paths,omitempty"`
	ExtraWritePaths   []string `yaml:"extra_write_paths,omitempty" json:"extra_write_paths,omitempty"`
	AllowNetwork      *bool    `yaml:"allow_network,omitempty" json:"allow_network,omitempty"`
	MemoryLimitMB     *int     `yaml:"memory_limit_mb,omitempty" json:"memory_limit_mb,omitempty"`
	MaxProcesses      *int     `yaml:"max_processes,omitempty" json:"max_processes,omitempty"`
}
