// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package integration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/config"
	"siply.dev/siply/internal/core"
)

// TestConfigLayering_GlobalToProjectOverride verifies the full three-layer
// merge from real temp files: global → project → lockfile.
func TestConfigLayering_GlobalToProjectOverride(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	// Layer 1: Global config.
	writeYAML(t, filepath.Join(globalDir, "config.yaml"), `
provider:
  default: anthropic
  model: claude-opus
session:
  retention_count: 50
telemetry:
  enabled: false
`)

	// Layer 2: Project config overrides provider and session.
	writeYAML(t, filepath.Join(projectDir, "config.yaml"), `
provider:
  default: openai
routing:
  enabled: true
  default_provider: openai
session:
  retention_count: 200
`)

	// Layer 3: Lockfile pins provider to a specific value.
	lockData := core.Config{
		Provider: core.ProviderConfig{
			Default: "openrouter",
			Model:   "anthropic/claude-opus-4",
		},
	}
	writeLockfile(t, filepath.Join(projectDir, "config.lock"), lockData)

	// Load and verify.
	l := config.NewLoader(config.LoaderOptions{
		GlobalDir:  globalDir,
		ProjectDir: projectDir,
	})
	require.NoError(t, l.Init(context.Background()))

	cfg := l.Config()

	// Lockfile wins for provider.
	assert.Equal(t, "openrouter", cfg.Provider.Default)
	assert.Equal(t, "anthropic/claude-opus-4", cfg.Provider.Model)

	// Project wins for routing (lockfile didn't set it).
	require.NotNil(t, cfg.Routing.Enabled)
	assert.True(t, *cfg.Routing.Enabled)
	assert.Equal(t, "openai", cfg.Routing.DefaultProvider)

	// Project wins for session (lockfile didn't set retention).
	assert.Equal(t, 200, cfg.Session.RetentionCount)

	// Global telemetry preserved (override-only: never removed).
	require.NotNil(t, cfg.Telemetry.Enabled)
	assert.False(t, *cfg.Telemetry.Enabled)
}

// TestConfigLayering_MissingLayers verifies graceful handling when layers are
// absent: only global → defaults used.
func TestConfigLayering_MissingLayers(t *testing.T) {
	l := config.NewLoader(config.LoaderOptions{
		GlobalDir:  t.TempDir(), // empty — no config.yaml
		ProjectDir: t.TempDir(), // empty — no config.yaml or config.lock
	})
	require.NoError(t, l.Init(context.Background()))

	cfg := l.Config()
	// Should get defaults.
	assert.Equal(t, "anthropic", cfg.Provider.Default)
	assert.Equal(t, 50, cfg.Session.RetentionCount)
}

// TestConfigLayering_OverrideOnlySemantics verifies that upper layers cannot
// remove keys from lower layers.
func TestConfigLayering_OverrideOnlySemantics(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	// Global sets model.
	writeYAML(t, filepath.Join(globalDir, "config.yaml"), `
provider:
  default: anthropic
  model: claude-opus
`)

	// Project sets only default — model should be preserved from global.
	writeYAML(t, filepath.Join(projectDir, "config.yaml"), `
provider:
  default: openai
`)

	l := config.NewLoader(config.LoaderOptions{
		GlobalDir:  globalDir,
		ProjectDir: projectDir,
	})
	require.NoError(t, l.Init(context.Background()))

	cfg := l.Config()
	assert.Equal(t, "openai", cfg.Provider.Default)
	assert.Equal(t, "claude-opus", cfg.Provider.Model) // preserved from global
}

func writeYAML(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
}

func writeLockfile(t *testing.T, path string, cfg core.Config) {
	t.Helper()
	data, err := json.MarshalIndent(cfg, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0644))
}
