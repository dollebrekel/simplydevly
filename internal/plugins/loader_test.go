// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package plugins_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/plugins"
)

// mockMerger is a test double for ConfigMerger that records calls.
type mockMerger struct {
	mu      sync.RWMutex
	configs map[string]map[string]any
	mergeErr error
	removeErr error
}

func newMockMerger() *mockMerger {
	return &mockMerger{configs: make(map[string]map[string]any)}
}

func (m *mockMerger) MergePluginConfig(pluginName string, pluginConfig map[string]any) error {
	if m.mergeErr != nil {
		return m.mergeErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configs[pluginName] = pluginConfig
	return nil
}

func (m *mockMerger) RemovePluginConfig(pluginName string) error {
	if m.removeErr != nil {
		return m.removeErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.configs, pluginName)
	return nil
}

func (m *mockMerger) get(pluginName string) map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.configs[pluginName]
}

func (m *mockMerger) has(pluginName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.configs[pluginName]
	return ok
}

// setupRegistry creates a LocalRegistry backed by a temp dir with a plugin installed from src.
func setupRegistry(t *testing.T, src string) (*plugins.LocalRegistry, string) {
	t.Helper()
	registryDir := t.TempDir()
	r := plugins.NewLocalRegistry(registryDir)
	ctx := context.Background()
	require.NoError(t, r.Init(ctx))
	require.NoError(t, r.Install(ctx, src))
	return r, registryDir
}

func TestTier1Loader_Load_ValidTier1Plugin(t *testing.T) {
	src := filepath.Join("testdata", "tier1-model-router")
	registry, _ := setupRegistry(t, src)

	merger := newMockMerger()
	loader := plugins.NewTier1Loader(registry, merger)

	err := loader.Load(context.Background(), "model-router")
	require.NoError(t, err)

	// Plugin should be in loaded map.
	assert.True(t, loader.IsLoaded("model-router"))

	// Config should have been merged.
	cfg := merger.get("model-router")
	require.NotNil(t, cfg, "config.yaml content should be merged")

	routing, ok := cfg["routing"].(map[string]any)
	require.True(t, ok, "routing key should be a map")

	rules, ok := routing["rules"].([]any)
	require.True(t, ok, "routing.rules should be a slice")
	assert.Len(t, rules, 2)
}

func TestTier1Loader_Load_ValidTier1Plugin_DataFileOnly(t *testing.T) {
	// tier1-prompt-basic has no config.yaml, only prompts.yaml (data file).
	src := filepath.Join("testdata", "tier1-prompt-basic")
	registry, _ := setupRegistry(t, src)

	merger := newMockMerger()
	loader := plugins.NewTier1Loader(registry, merger)

	err := loader.Load(context.Background(), "prompt-basic")
	require.NoError(t, err)

	assert.True(t, loader.IsLoaded("prompt-basic"))
	// No config.yaml — nothing merged.
	assert.False(t, merger.has("prompt-basic"))
}

func TestTier1Loader_Load_NotTier1ReturnsError(t *testing.T) {
	// Create a temp plugin dir with a Tier 3 manifest.
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "manifest.yaml"), `apiVersion: siply/v1
kind: Plugin
metadata:
  name: tier3-plugin
  version: 0.1.0
  siply_min: 1.0.0
  description: "A Tier 3 test plugin"
  author: siply-dev
  license: Apache-2.0
  updated: "2026-04-10"
spec:
  tier: 3
  capabilities: {}
`)
	registry, _ := setupRegistry(t, src)

	merger := newMockMerger()
	loader := plugins.NewTier1Loader(registry, merger)

	err := loader.Load(context.Background(), "tier3-plugin")
	require.Error(t, err)
	assert.ErrorIs(t, err, plugins.ErrNotTier1)
	assert.False(t, loader.IsLoaded("tier3-plugin"))
}

func TestTier1Loader_Load_TooManyFiles(t *testing.T) {
	src := filepath.Join("testdata", "tier1-too-many-files")
	registry, _ := setupRegistry(t, src)

	merger := newMockMerger()
	loader := plugins.NewTier1Loader(registry, merger)

	err := loader.Load(context.Background(), "too-many-files")
	require.Error(t, err)
	assert.ErrorIs(t, err, plugins.ErrTooManyFiles)
	assert.False(t, loader.IsLoaded("too-many-files"))
}

func TestTier1Loader_Load_FileTooLarge(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "manifest.yaml"), `apiVersion: siply/v1
kind: Plugin
metadata:
  name: large-file-plugin
  version: 0.1.0
  siply_min: 1.0.0
  description: "Test fixture: large config file"
  author: siply-dev
  license: Apache-2.0
  updated: "2026-04-10"
spec:
  tier: 1
  capabilities: {}
`)
	// Create a config.yaml exceeding 1MB.
	largeData := make([]byte, (1<<20)+1)
	for i := range largeData {
		largeData[i] = 'x'
	}
	// Make it valid YAML: "key: <big string>"
	content := append([]byte("key: "), largeData...)
	require.NoError(t, os.WriteFile(filepath.Join(src, "config.yaml"), content, 0644))

	registry, _ := setupRegistry(t, src)

	merger := newMockMerger()
	loader := plugins.NewTier1Loader(registry, merger)

	err := loader.Load(context.Background(), "large-file-plugin")
	require.Error(t, err)
	assert.ErrorIs(t, err, plugins.ErrFileTooLarge)
}

func TestTier1Loader_Load_RejectsCustomTypes(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "manifest.yaml"), `apiVersion: siply/v1
kind: Plugin
metadata:
  name: custom-type-plugin
  version: 0.1.0
  siply_min: 1.0.0
  description: "Test fixture: custom type in config"
  author: siply-dev
  license: Apache-2.0
  updated: "2026-04-10"
spec:
  tier: 1
  capabilities: {}
`)
	// Custom YAML type — rejected by strict parser.
	writeFile(t, filepath.Join(src, "config.yaml"), "key: !!python/object:module.Class {}")

	registry, _ := setupRegistry(t, src)

	merger := newMockMerger()
	loader := plugins.NewTier1Loader(registry, merger)

	err := loader.Load(context.Background(), "custom-type-plugin")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "custom YAML type")
}

func TestTier1Loader_Load_RejectsAliases(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "manifest.yaml"), `apiVersion: siply/v1
kind: Plugin
metadata:
  name: alias-plugin
  version: 0.1.0
  siply_min: 1.0.0
  description: "Test fixture: YAML alias in config"
  author: siply-dev
  license: Apache-2.0
  updated: "2026-04-10"
spec:
  tier: 1
  capabilities: {}
`)
	// YAML aliases — rejected by strict parser.
	writeFile(t, filepath.Join(src, "config.yaml"), `base: &anchor
  key: value
merged:
  <<: *anchor
`)

	registry, _ := setupRegistry(t, src)

	merger := newMockMerger()
	loader := plugins.NewTier1Loader(registry, merger)

	err := loader.Load(context.Background(), "alias-plugin")
	require.Error(t, err)
	// Either the alias or merge key check triggers.
	assert.Contains(t, err.Error(), "not allowed")
}

func TestTier1Loader_Unload_RemovesConfig(t *testing.T) {
	src := filepath.Join("testdata", "tier1-model-router")
	registry, _ := setupRegistry(t, src)

	merger := newMockMerger()
	loader := plugins.NewTier1Loader(registry, merger)
	ctx := context.Background()

	require.NoError(t, loader.Load(ctx, "model-router"))
	assert.True(t, loader.IsLoaded("model-router"))
	assert.True(t, merger.has("model-router"))

	require.NoError(t, loader.Unload(ctx, "model-router"))
	assert.False(t, loader.IsLoaded("model-router"))
	assert.False(t, merger.has("model-router"))
}

func TestTier1Loader_Unload_NotLoadedReturnsError(t *testing.T) {
	registryDir := t.TempDir()
	registry := plugins.NewLocalRegistry(registryDir)
	require.NoError(t, registry.Init(context.Background()))

	merger := newMockMerger()
	loader := plugins.NewTier1Loader(registry, merger)

	err := loader.Unload(context.Background(), "nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, plugins.ErrPluginNotLoaded)
}

func TestTier1Loader_IsLoaded(t *testing.T) {
	src := filepath.Join("testdata", "tier1-model-router")
	registry, _ := setupRegistry(t, src)

	merger := newMockMerger()
	loader := plugins.NewTier1Loader(registry, merger)
	ctx := context.Background()

	assert.False(t, loader.IsLoaded("model-router"))
	require.NoError(t, loader.Load(ctx, "model-router"))
	assert.True(t, loader.IsLoaded("model-router"))
}

func TestTier1Loader_List_ReturnsAllLoaded(t *testing.T) {
	src1 := filepath.Join("testdata", "tier1-model-router")
	registry, _ := setupRegistry(t, src1)

	// Install a second plugin.
	src2 := filepath.Join("testdata", "tier1-prompt-basic")
	require.NoError(t, registry.Install(context.Background(), src2))

	merger := newMockMerger()
	loader := plugins.NewTier1Loader(registry, merger)
	ctx := context.Background()

	require.NoError(t, loader.Load(ctx, "model-router"))
	require.NoError(t, loader.Load(ctx, "prompt-basic"))

	listed := loader.List(ctx)
	assert.Len(t, listed, 2)

	names := make(map[string]bool)
	for _, p := range listed {
		names[p.Manifest.Metadata.Name] = true
	}
	assert.True(t, names["model-router"])
	assert.True(t, names["prompt-basic"])
}

func TestTier1Loader_Load_PluginNotFound(t *testing.T) {
	registryDir := t.TempDir()
	registry := plugins.NewLocalRegistry(registryDir)
	require.NoError(t, registry.Init(context.Background()))

	merger := newMockMerger()
	loader := plugins.NewTier1Loader(registry, merger)

	err := loader.Load(context.Background(), "nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, plugins.ErrNotFound)
}

func TestTier1Loader_Concurrent_LoadUnloadList(t *testing.T) {
	src := filepath.Join("testdata", "tier1-model-router")
	registry, _ := setupRegistry(t, src)

	merger := newMockMerger()
	loader := plugins.NewTier1Loader(registry, merger)
	ctx := context.Background()

	const goroutines = 20
	var wg sync.WaitGroup

	// Concurrent Load + Unload + List + IsLoaded.
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			switch n % 4 {
			case 0:
				_ = loader.Load(ctx, "model-router")
			case 1:
				_ = loader.Unload(ctx, "model-router")
			case 2:
				_ = loader.List(ctx)
			case 3:
				_ = loader.IsLoaded("model-router")
			}
		}(i)
	}
	wg.Wait()
	// No race conditions — the test passes if -race finds no issues.
}

// writeFile is a test helper to create a file with the given content.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
}

// TestParsePluginYAML_Standalone tests the YAML parsing helpers via exported API.
// Since parsePluginYAML is unexported, we test it indirectly through Load.
func TestParsePluginYAML_MergeKeyInDataFile(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "manifest.yaml"), `apiVersion: siply/v1
kind: Plugin
metadata:
  name: merge-key-data-plugin
  version: 0.1.0
  siply_min: 1.0.0
  description: "Test fixture: merge key in data file"
  author: siply-dev
  license: Apache-2.0
  updated: "2026-04-10"
spec:
  tier: 1
  capabilities: {}
`)
	// Merge key in a data file (prompts.yaml) — also rejected.
	writeFile(t, filepath.Join(src, "prompts.yaml"), `base: &anchor
  key: value
merged:
  <<: *anchor
`)

	registry, _ := setupRegistry(t, src)
	merger := newMockMerger()
	loader := plugins.NewTier1Loader(registry, merger)

	err := loader.Load(context.Background(), "merge-key-data-plugin")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

// Verify sentinel error variables are non-nil and have distinct messages.
func TestSentinelErrors(t *testing.T) {
	errors := map[string]error{
		"ErrNotTier1":        plugins.ErrNotTier1,
		"ErrTooManyFiles":    plugins.ErrTooManyFiles,
		"ErrFileTooLarge":    plugins.ErrFileTooLarge,
		"ErrPluginNotLoaded": plugins.ErrPluginNotLoaded,
	}
	messages := make(map[string]bool)
	for name, err := range errors {
		require.NotNil(t, err, "sentinel error %s should not be nil", name)
		msg := err.Error()
		assert.False(t, messages[msg], "sentinel error %s has duplicate message %q", name, msg)
		messages[msg] = true
	}
}

// Verify all sentinel errors embed "plugins:" prefix per project conventions.
func TestSentinelErrors_HavePluginsPrefix(t *testing.T) {
	for _, err := range []error{
		plugins.ErrNotTier1,
		plugins.ErrTooManyFiles,
		plugins.ErrFileTooLarge,
		plugins.ErrPluginNotLoaded,
	} {
		assert.Contains(t, err.Error(), "plugins:", "error %q should have plugins: prefix", err)
	}
}

// TestNewTier1Loader_NilGuards verifies constructor nil guards.
func TestNewTier1Loader_NilRegistry(t *testing.T) {
	merger := newMockMerger()
	loader := plugins.NewTier1Loader(nil, merger)

	err := loader.Load(context.Background(), "any")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "registry is nil")
}

func TestNewTier1Loader_NilMerger(t *testing.T) {
	registryDir := t.TempDir()
	registry := plugins.NewLocalRegistry(registryDir)
	require.NoError(t, registry.Init(context.Background()))

	loader := plugins.NewTier1Loader(registry, nil)
	err := loader.Load(context.Background(), "any")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "configMerger is nil")
}

// TestInstallAndLoad verifies the install→load→unload→remove lifecycle.
func TestInstallAndLoad_Lifecycle(t *testing.T) {
	src := filepath.Join("testdata", "tier1-model-router")

	registryDir := t.TempDir()
	registry := plugins.NewLocalRegistry(registryDir)
	ctx := context.Background()
	require.NoError(t, registry.Init(ctx))

	merger := newMockMerger()
	loader := plugins.NewTier1Loader(registry, merger)

	// Install.
	require.NoError(t, registry.Install(ctx, src))

	// Load.
	require.NoError(t, loader.Load(ctx, "model-router"))
	assert.True(t, loader.IsLoaded("model-router"))
	assert.True(t, merger.has("model-router"))

	// Unload.
	require.NoError(t, loader.Unload(ctx, "model-router"))
	assert.False(t, loader.IsLoaded("model-router"))
	assert.False(t, merger.has("model-router"))

	// Remove from registry.
	require.NoError(t, registry.Remove(ctx, "model-router"))

	// Verify gone from registry.
	metas, err := registry.List(ctx)
	require.NoError(t, err)
	for _, m := range metas {
		assert.NotEqual(t, "model-router", m.Name)
	}
}

// Verify that LoadManifestFromDir is exported and usable from tests.
func TestLoadManifestFromDir_Tier1(t *testing.T) {
	m, err := plugins.LoadManifestFromDir(filepath.Join("testdata", "tier1-model-router"))
	require.NoError(t, err)
	assert.Equal(t, "model-router", m.Metadata.Name)
	assert.Equal(t, 1, m.Spec.Tier)
}

// Large parallel test to surface races with -race flag.
func TestConcurrent_MultiplePlugins(t *testing.T) {
	src1 := filepath.Join("testdata", "tier1-model-router")
	registry, _ := setupRegistry(t, src1)

	src2 := filepath.Join("testdata", "tier1-prompt-basic")
	require.NoError(t, registry.Install(context.Background(), src2))

	merger := newMockMerger()
	loader := plugins.NewTier1Loader(registry, merger)
	ctx := context.Background()

	var wg sync.WaitGroup
	names := []string{"model-router", "prompt-basic"}

	for _, name := range names {
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(n string, idx int) {
				defer wg.Done()
				switch idx % 3 {
				case 0:
					_ = loader.Load(ctx, n)
				case 1:
					_ = loader.IsLoaded(n)
				case 2:
					_ = loader.List(ctx)
				}
			}(name, i)
		}
	}

	// Also sprinkle some Unloads.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			_ = loader.Unload(ctx, n)
		}(fmt.Sprintf("plugin-%d", i))
	}

	wg.Wait()
}
