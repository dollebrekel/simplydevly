// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package config

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"

	"siply.dev/siply/internal/core"
)

const maxConfigFileSize = 1 << 20 // 1MB

// LoaderOptions configures the Loader.
type LoaderOptions struct {
	// GlobalDir is the path to the global config directory (default: ~/.siply).
	GlobalDir string
	// ProjectDir is the path to the project config directory (default: .siply in cwd).
	ProjectDir string
	// Overrides are runtime flag overrides applied as the fourth layer.
	Overrides *core.Config
	// SkipLockfile skips loading the lockfile layer. Use this when generating
	// a new lockfile to prevent the old lockfile from overriding config changes.
	SkipLockfile bool
}

// Loader implements core.ConfigResolver with four-layer merge:
// global → project → lockfile → runtime overrides.
type Loader struct {
	opts   LoaderOptions
	mu     sync.RWMutex
	config *core.Config
}

// NewLoader creates a new config Loader.
func NewLoader(opts LoaderOptions) *Loader {
	return &Loader{opts: opts}
}

// Init loads and merges configuration from all layers.
func (l *Loader) Init(_ context.Context) error {
	globalDir := l.opts.GlobalDir
	if globalDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("config: cannot determine home directory: %w", err)
		}
		globalDir = filepath.Join(home, ".siply")
	}

	projectDir := l.opts.ProjectDir
	if projectDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("config: cannot determine working directory: %w", err)
		}
		projectDir = filepath.Join(cwd, ".siply")
	}

	// Layer 1: Global config (optional — missing is not an error).
	globalPath := filepath.Join(globalDir, "config.yaml")
	global, err := loadYAML(globalPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("config: loading global config: %w", err)
	}
	if global == nil {
		global = defaults()
		slog.Debug("config: no global config found, using defaults", "path", globalPath)
	} else {
		slog.Info("config loaded", "layer", "global", "path", globalPath)
	}

	// Layer 2: Project config (optional — missing means global-only).
	projectPath := filepath.Join(projectDir, "config.yaml")
	project, err := loadYAML(projectPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("config: loading project config: %w", err)
	}
	merged := global
	if project != nil {
		merged = merge(global, project)
		slog.Info("config loaded", "layer", "project", "path", projectPath)
	}

	// Layer 3: Lockfile (optional — skipped during lockfile generation).
	if !l.opts.SkipLockfile {
		lockPath := filepath.Join(projectDir, "config.lock")
		lock, err := loadLockfile(lockPath)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("config: loading lockfile: %w", err)
		}
		if lock != nil {
			merged = merge(merged, lock)
			slog.Info("config loaded", "layer", "lockfile", "path", lockPath)
		}
	}

	// Layer 4: Runtime overrides.
	if l.opts.Overrides != nil {
		merged = merge(merged, l.opts.Overrides)
	}

	l.mu.Lock()
	l.config = merged
	l.mu.Unlock()
	return nil
}

func (l *Loader) Start(_ context.Context) error { return nil }
func (l *Loader) Stop(_ context.Context) error  { return nil }

// Health returns an error if Init has not been called.
func (l *Loader) Health() error {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.config == nil {
		return fmt.Errorf("config: not loaded")
	}
	return nil
}

// Config returns a defensive copy of the fully resolved configuration.
// Callers may modify the returned value without affecting the Loader's state.
func (l *Loader) Config() *core.Config {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.config == nil {
		return nil
	}
	// Shallow copy of the struct.
	c := *l.config
	// Deep-copy pointer fields to prevent caller mutation of internal state.
	if l.config.Routing.Enabled != nil {
		v := *l.config.Routing.Enabled
		c.Routing.Enabled = &v
	}
	if l.config.Session.RetentionCount != nil {
		v := *l.config.Session.RetentionCount
		c.Session.RetentionCount = &v
	}
	if l.config.Telemetry.Enabled != nil {
		v := *l.config.Telemetry.Enabled
		c.Telemetry.Enabled = &v
	}
	if l.config.Plugins != nil {
		c.Plugins = make(map[string]any, len(l.config.Plugins))
		maps.Copy(c.Plugins, l.config.Plugins)
	}
	return &c
}

// defaults returns the base configuration with sensible default values.
func defaults() *core.Config {
	defaultRetention := 50
	return &core.Config{
		Provider: core.ProviderConfig{
			Default: "anthropic",
		},
		Session: core.SessionConfig{
			RetentionCount: &defaultRetention,
		},
	}
}

// loadYAML reads a YAML config file in strict mode.
// Returns (nil, nil-ish os.ErrNotExist) when the file does not exist.
func loadYAML(path string) (*core.Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("config: stat %s: %w", path, err)
	}
	if info.Size() > maxConfigFileSize {
		return nil, fmt.Errorf("config: file exceeds 1MB limit: %s (%d bytes)", path, info.Size())
	}
	if info.Size() == 0 {
		return &core.Config{}, nil
	}

	dec := yaml.NewDecoder(f)

	var cfg core.Config
	if err := dec.Decode(&cfg); err != nil {
		return nil, formatYAMLError(path, err)
	}
	slog.Debug("config: ignoring unknown fields if present", "path", path)

	// Ensure no second document in the stream.
	var discard any
	if dec.Decode(&discard) != io.EOF {
		return nil, fmt.Errorf("config: %s contains multiple YAML documents; only one is allowed", path)
	}

	return &cfg, nil
}

// loadLockfile reads a JSON lockfile and returns the config portion.
// Supports both new format (with "version" and "plugins" fields) and
// legacy format (raw core.Config JSON without wrapper).
func loadLockfile(path string) (*core.Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("config: stat %s: %w", path, err)
	}
	if info.Size() > maxConfigFileSize {
		return nil, fmt.Errorf("config: file exceeds 1MB limit: %s (%d bytes)", path, info.Size())
	}
	if info.Size() == 0 {
		return &core.Config{}, nil
	}

	data, err := io.ReadAll(io.LimitReader(f, maxConfigFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("config: reading lockfile %s: %w", path, err)
	}

	// Try new Lockfile format first (has "version" field).
	var probe struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &probe); err == nil && probe.Version != "" {
		lf, err := ParseLockfile(data)
		if err != nil {
			return nil, fmt.Errorf("lockfile: parsing %s: %w", path, err)
		}
		return &lf.Config, nil
	}

	// Legacy format: raw core.Config JSON.
	var cfg core.Config
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("config: parsing lockfile %s: %w (check JSON syntax and field names)", path, err)
	}

	// Ensure no trailing data after the JSON object.
	if dec.More() {
		return nil, fmt.Errorf("config: %s contains trailing data after JSON object; only one object is allowed", path)
	}

	return &cfg, nil
}

// merge applies overrides from upper onto base. Upper values replace base
// values when non-zero. This is override-only — base keys are never removed.
func merge(base, upper *core.Config) *core.Config {
	out := *base

	// Deep-copy pointer fields from base to prevent aliasing between layers.
	if base.Routing.Enabled != nil {
		v := *base.Routing.Enabled
		out.Routing.Enabled = &v
	}
	if base.Telemetry.Enabled != nil {
		v := *base.Telemetry.Enabled
		out.Telemetry.Enabled = &v
	}
	if base.Session.RetentionCount != nil {
		v := *base.Session.RetentionCount
		out.Session.RetentionCount = &v
	}

	// Provider
	if upper.Provider.Default != "" {
		out.Provider.Default = upper.Provider.Default
	}
	if upper.Provider.Model != "" {
		out.Provider.Model = upper.Provider.Model
	}

	// Routing
	if upper.Routing.Enabled != nil {
		v := *upper.Routing.Enabled
		out.Routing.Enabled = &v
	}
	if upper.Routing.DefaultProvider != "" {
		out.Routing.DefaultProvider = upper.Routing.DefaultProvider
	}
	if upper.Routing.PreprocessProvider != "" {
		out.Routing.PreprocessProvider = upper.Routing.PreprocessProvider
	}
	if upper.Routing.PreprocessModel != "" {
		out.Routing.PreprocessModel = upper.Routing.PreprocessModel
	}

	// Session
	if upper.Session.RetentionCount != nil {
		v := *upper.Session.RetentionCount
		out.Session.RetentionCount = &v
	}

	// Telemetry
	if upper.Telemetry.Enabled != nil {
		v := *upper.Telemetry.Enabled
		out.Telemetry.Enabled = &v
	}

	// TUI
	if upper.TUI.Profile != "" {
		out.TUI.Profile = upper.TUI.Profile
	}

	// Plugins — shallow merge at the plugin-name level (upper keys override,
	// base keys preserved). Deep-copy the base map to prevent aliasing.
	//
	// NOTE: This is intentionally a shallow merge per plugin namespace for the
	// global→project→lockfile config layers. If upper defines pluginA: {key1: "new"},
	// the ENTIRE pluginA value replaces the base — intra-plugin keys from the base
	// are NOT preserved at this layer.
	//
	// Deep merge for Tier 1 plugin configs is implemented in Story 6-2 via
	// PluginConfigMerger (internal/config/plugin_merge.go). Tier 1 plugins loaded
	// at runtime use deep merge through ConfigMerger.MergePluginConfig.
	// Resolved: Story 6-2 added deep merge via PluginConfigMerger (internal/config/plugin_merge.go).
	if base.Plugins != nil {
		out.Plugins = make(map[string]any, len(base.Plugins))
		maps.Copy(out.Plugins, base.Plugins)
	}
	if len(upper.Plugins) > 0 {
		if out.Plugins == nil {
			out.Plugins = make(map[string]any)
		}
		maps.Copy(out.Plugins, upper.Plugins)
	}

	return &out
}

// formatYAMLError produces an actionable error message from a yaml.v3 error.
func formatYAMLError(path string, err error) error {
	return fmt.Errorf("config: invalid YAML in %s: %w (check field names and value types against schema)", path, err)
}

