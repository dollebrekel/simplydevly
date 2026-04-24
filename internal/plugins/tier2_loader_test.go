// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package plugins

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTier2Plugin creates a minimal Tier 2 plugin directory with manifest and main.lua.
func writeTier2Plugin(t *testing.T, dir, name, mainLua string) string {
	t.Helper()
	pluginDir := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))

	manifest := `apiVersion: siply/v1
kind: Plugin
metadata:
  name: ` + name + `
  version: 1.0.0
  siply_min: 0.1.0
  description: Test Lua plugin
  author: test
  license: MIT
  updated: "2026-04-21"
spec:
  tier: 2
  capabilities: {}
`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "manifest.yaml"), []byte(manifest), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "main.lua"), []byte(mainLua), 0o644))
	return pluginDir
}

func newTestTier2Loader(t *testing.T, registryDir string) *Tier2Loader {
	t.Helper()
	registry := NewLocalRegistry(registryDir)
	return NewTier2Loader(registry, nil, nil)
}

func TestTier2Loader_LoadValid(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeTier2Plugin(t, dir, "test-lua", `siply.log("info", "hello from lua")`)

	loader := newTestTier2Loader(t, dir)
	err := loader.Load(context.Background(), "test-lua")
	require.NoError(t, err)

	assert.True(t, loader.IsLoaded("test-lua"))
}

func TestTier2Loader_RejectNonTier2(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "tier1-plugin")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))

	manifest := `apiVersion: siply/v1
kind: Plugin
metadata:
  name: tier1-plugin
  version: 1.0.0
  siply_min: 0.1.0
  description: Tier 1 plugin
  author: test
  license: MIT
  updated: "2026-04-21"
spec:
  tier: 1
  capabilities: {}
`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "manifest.yaml"), []byte(manifest), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "main.lua"), []byte(""), 0o644))

	loader := newTestTier2Loader(t, dir)
	err := loader.Load(context.Background(), "tier1-plugin")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNotTier2))
}

func TestTier2Loader_MissingMainLua(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "no-main")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))

	manifest := `apiVersion: siply/v1
kind: Plugin
metadata:
  name: no-main
  version: 1.0.0
  siply_min: 0.1.0
  description: No main.lua
  author: test
  license: MIT
  updated: "2026-04-21"
spec:
  tier: 2
  capabilities: {}
`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "manifest.yaml"), []byte(manifest), 0o644))

	loader := newTestTier2Loader(t, dir)
	err := loader.Load(context.Background(), "no-main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "main.lua not found")
}

func TestTier2Loader_LuaRuntimeError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeTier2Plugin(t, dir, "bad-lua", `error("intentional crash")`)

	loader := newTestTier2Loader(t, dir)
	err := loader.Load(context.Background(), "bad-lua")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrLuaExecution))
}

func TestTier2Loader_UnloadCleanup(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeTier2Plugin(t, dir, "unload-test", `siply.log("info", "loaded")`)

	loader := newTestTier2Loader(t, dir)
	require.NoError(t, loader.Load(context.Background(), "unload-test"))
	assert.True(t, loader.IsLoaded("unload-test"))

	require.NoError(t, loader.Unload("unload-test"))
	assert.False(t, loader.IsLoaded("unload-test"))
}

func TestTier2Loader_UnloadNotLoaded(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	loader := newTestTier2Loader(t, dir)
	err := loader.Unload("nonexistent")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrPluginNotLoaded))
}

func TestTier2Loader_ReloadOverwrites(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeTier2Plugin(t, dir, "reload-test", `siply.log("info", "v1")`)

	loader := newTestTier2Loader(t, dir)
	require.NoError(t, loader.Load(context.Background(), "reload-test"))

	// Overwrite main.lua and reload.
	mainPath := filepath.Join(dir, "reload-test", "main.lua")
	require.NoError(t, os.WriteFile(mainPath, []byte(`siply.log("info", "v2")`), 0o644))

	require.NoError(t, loader.Load(context.Background(), "reload-test"))
	assert.True(t, loader.IsLoaded("reload-test"))
}

func TestTier2Loader_PluginNotFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	loader := newTestTier2Loader(t, dir)
	err := loader.Load(context.Background(), "does-not-exist")
	require.Error(t, err)
}
