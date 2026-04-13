// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package integration

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/config"
	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/events"
	"siply.dev/siply/internal/plugins"
)

// TestPromptBasic_Install verifies the prompt-basic plugin installs correctly
// via LocalRegistry as a Tier 1 plugin.
// AC: Story 8.4 — manifest.yaml is valid with tier: 1, no capabilities needed.
func TestPromptBasic_Install(t *testing.T) {
	ctx := context.Background()
	registryDir := t.TempDir()

	bus := events.NewBus()
	require.NoError(t, bus.Init(ctx))
	require.NoError(t, bus.Start(ctx))
	defer func() { _ = bus.Stop(ctx) }()

	var loadedPlugins []string
	unsub := bus.Subscribe(events.EventPluginLoaded, func(_ context.Context, e core.Event) {
		loadedPlugins = append(loadedPlugins, e.Type())
	})
	defer unsub()

	registry := plugins.NewLocalRegistry(registryDir)
	registry.SetEventBus(bus)
	registry.SetSiplyVersion("99.0.0")
	require.NoError(t, registry.Init(ctx))

	// Install prompt-basic.
	pluginSource := filepath.Join(testdataDir(t), "prompt-basic-plugin")
	require.NoError(t, registry.Install(ctx, pluginSource))

	// Verify in registry list.
	metas, err := registry.List(ctx)
	require.NoError(t, err)
	var found bool
	for _, m := range metas {
		if m.Name == "prompt-basic" {
			found = true
			assert.Equal(t, "1.0.0", m.Version)
			assert.Equal(t, 1, m.Tier)
			break
		}
	}
	require.True(t, found, "prompt-basic should be in registry")
}

// TestPromptBasic_Tier1Load verifies the prompt-basic config is merged
// into the project config via Tier1Loader.
// AC: Story 8.4 — templates are YAML config files (Tier 1, no code execution).
func TestPromptBasic_Tier1Load(t *testing.T) {
	ctx := context.Background()
	registryDir := t.TempDir()

	bus := events.NewBus()
	require.NoError(t, bus.Init(ctx))
	require.NoError(t, bus.Start(ctx))
	defer func() { _ = bus.Stop(ctx) }()

	registry := plugins.NewLocalRegistry(registryDir)
	registry.SetEventBus(bus)
	registry.SetSiplyVersion("99.0.0")
	require.NoError(t, registry.Init(ctx))

	// Install prompt-basic.
	pluginSource := filepath.Join(testdataDir(t), "prompt-basic-plugin")
	require.NoError(t, registry.Install(ctx, pluginSource))

	// Setup config loader and Tier1Loader.
	cfgLoader := config.NewLoader(config.LoaderOptions{
		GlobalDir:  t.TempDir(),
		ProjectDir: t.TempDir(),
	})
	require.NoError(t, cfgLoader.Init(ctx))
	merger := config.NewPluginConfigMerger(cfgLoader)

	tier1 := plugins.NewTier1Loader(registry, merger)
	require.NoError(t, tier1.Load(ctx, "prompt-basic"))

	// Verify config was merged.
	cfg := cfgLoader.Config()
	pluginCfg, ok := cfg.Plugins["prompt-basic"]
	require.True(t, ok, "prompt-basic should be in Plugins map after Tier1 load")

	m, ok := pluginCfg.(map[string]any)
	require.True(t, ok)

	// Should contain prompts key.
	prompts, ok := m["prompts"]
	require.True(t, ok, "should have prompts key")

	promptsMap, ok := prompts.(map[string]any)
	require.True(t, ok)

	// Check all 8 prompt templates exist.
	expectedPrompts := []string{
		"code-review", "explain-code", "refactor", "write-tests",
		"debug", "document", "optimize", "security-audit",
	}
	for _, name := range expectedPrompts {
		_, ok := promptsMap[name]
		assert.True(t, ok, "prompt template %q should exist", name)
	}
}

// TestPromptBasic_ManifestValid verifies the manifest has correct structure.
// AC: Story 8.4 — manifest.yaml is valid.
func TestPromptBasic_ManifestValid(t *testing.T) {
	pluginDir := filepath.Join(testdataDir(t), "prompt-basic-plugin")

	manifest, err := plugins.LoadManifestFromDir(pluginDir)
	require.NoError(t, err)

	assert.Equal(t, "siply/v1", manifest.APIVersion)
	assert.Equal(t, "Plugin", manifest.Kind)
	assert.Equal(t, "prompt-basic", manifest.Metadata.Name)
	assert.Equal(t, "1.0.0", manifest.Metadata.Version)
	assert.Equal(t, 1, manifest.Spec.Tier)
	assert.Empty(t, manifest.Spec.Capabilities)
}
