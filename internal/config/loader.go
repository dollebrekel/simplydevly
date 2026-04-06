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
}

// Loader implements core.ConfigResolver with three-layer merge:
// global → project → lockfile → runtime overrides.
type Loader struct {
	opts   LoaderOptions
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

	// Layer 3: Lockfile (optional).
	lockPath := filepath.Join(projectDir, "config.lock")
	lock, err := loadLockfile(lockPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("config: loading lockfile: %w", err)
	}
	if lock != nil {
		merged = merge(merged, lock)
		slog.Info("config loaded", "layer", "lockfile", "path", lockPath)
	}

	// Layer 4: Runtime overrides.
	if l.opts.Overrides != nil {
		merged = merge(merged, l.opts.Overrides)
	}

	l.config = merged
	return nil
}

func (l *Loader) Start(_ context.Context) error { return nil }
func (l *Loader) Stop(_ context.Context) error  { return nil }

// Health returns an error if Init has not been called.
func (l *Loader) Health() error {
	if l.config == nil {
		return fmt.Errorf("config: not loaded")
	}
	return nil
}

// Config returns the fully resolved configuration.
func (l *Loader) Config() *core.Config {
	return l.config
}

// defaults returns the base configuration with sensible default values.
func defaults() *core.Config {
	return &core.Config{
		Provider: core.ProviderConfig{
			Default: "anthropic",
		},
		Session: core.SessionConfig{
			RetentionCount: 50,
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
	dec.KnownFields(true)

	var cfg core.Config
	if err := dec.Decode(&cfg); err != nil {
		return nil, formatYAMLError(path, err)
	}

	// Ensure no second document in the stream.
	var discard any
	if dec.Decode(&discard) != io.EOF {
		return nil, fmt.Errorf("config: %s contains multiple YAML documents; only one is allowed", path)
	}

	return &cfg, nil
}

// loadLockfile reads a JSON lockfile.
func loadLockfile(path string) (*core.Config, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxConfigFileSize {
		return nil, fmt.Errorf("config: file exceeds 1MB limit: %s (%d bytes)", path, info.Size())
	}
	if info.Size() == 0 {
		return &core.Config{}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg core.Config
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("config: parsing lockfile %s: %w (check JSON syntax and field names)", path, err)
	}
	return &cfg, nil
}

// merge applies overrides from upper onto base. Upper values replace base
// values when non-zero. This is override-only — base keys are never removed.
func merge(base, upper *core.Config) *core.Config {
	out := *base

	// Provider
	if upper.Provider.Default != "" {
		out.Provider.Default = upper.Provider.Default
	}
	if upper.Provider.Model != "" {
		out.Provider.Model = upper.Provider.Model
	}

	// Routing
	if upper.Routing.Enabled != nil {
		out.Routing.Enabled = upper.Routing.Enabled
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
	if upper.Session.RetentionCount > 0 {
		out.Session.RetentionCount = upper.Session.RetentionCount
	}

	// Telemetry
	if upper.Telemetry.Enabled != nil {
		out.Telemetry.Enabled = upper.Telemetry.Enabled
	}

	// Plugins — merge maps (upper keys override, base keys preserved).
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

