// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package plugins

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestPlugin creates a minimal valid plugin directory for testing.
func createTestPlugin(t *testing.T, dir, name, version string) string {
	t.Helper()
	pluginDir := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(pluginDir, 0755))

	manifest := `apiVersion: siply/v1
kind: Plugin
metadata:
  name: ` + name + `
  version: ` + version + `
  siply_min: "0.1.0"
  description: "Test plugin"
  author: "test"
  license: "Apache-2.0"
  updated: "2026-04-10"
spec:
  tier: 1
  capabilities:
    filesystem: read
`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "manifest.yaml"), []byte(manifest), 0644))
	return pluginDir
}

// createTestPluginWithMin creates a plugin with a specific siply_min.
func createTestPluginWithMin(t *testing.T, dir, name, version, siplyMin string) string {
	t.Helper()
	pluginDir := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(pluginDir, 0755))

	manifest := `apiVersion: siply/v1
kind: Plugin
metadata:
  name: ` + name + `
  version: ` + version + `
  siply_min: "` + siplyMin + `"
  description: "Test plugin"
  author: "test"
  license: "Apache-2.0"
  updated: "2026-04-10"
spec:
  tier: 1
  capabilities:
    filesystem: read
`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "manifest.yaml"), []byte(manifest), 0644))
	return pluginDir
}

func setupTestEnvironment(t *testing.T) (registry *LocalRegistry, backupDir string, registryDir string) {
	t.Helper()
	tmpDir := t.TempDir()
	registryDir = filepath.Join(tmpDir, "plugins")
	backupDir = filepath.Join(tmpDir, "plugins", ".versions")
	require.NoError(t, os.MkdirAll(registryDir, 0755))
	require.NoError(t, os.MkdirAll(backupDir, 0755))

	registry = NewLocalRegistry(registryDir)
	require.NoError(t, registry.Init(context.Background()))
	return registry, backupDir, registryDir
}

func TestNewVersionManager(t *testing.T) {
	registry, backupDir, _ := setupTestEnvironment(t)

	vm := NewVersionManager(registry, backupDir)
	require.NotNil(t, vm)

	// Nil registry returns nil.
	vmNil := NewVersionManager(nil, backupDir)
	assert.Nil(t, vmNil)
}

func TestVersionManager_Check(t *testing.T) {
	registry, backupDir, registryDir := setupTestEnvironment(t)
	ctx := context.Background()

	// Install a plugin.
	sourceDir := t.TempDir()
	createTestPlugin(t, sourceDir, "test-plugin", "1.0.0")
	require.NoError(t, registry.Install(ctx, filepath.Join(sourceDir, "test-plugin")))

	vm := NewVersionManager(registry, backupDir)
	infos, err := vm.Check(ctx)
	require.NoError(t, err)
	require.Len(t, infos, 1)

	assert.Equal(t, "test-plugin", infos[0].Name)
	assert.Equal(t, "1.0.0", infos[0].Current)
	assert.True(t, infos[0].Compatible)
	assert.False(t, infos[0].Pinned)
	_ = registryDir
}

func TestVersionManager_Update(t *testing.T) {
	registry, backupDir, registryDir := setupTestEnvironment(t)
	ctx := context.Background()

	// Install v1.0.0.
	sourceDir := t.TempDir()
	createTestPlugin(t, sourceDir, "test-plugin", "1.0.0")
	require.NoError(t, registry.Install(ctx, filepath.Join(sourceDir, "test-plugin")))

	vm := NewVersionManager(registry, backupDir)

	// Create v2.0.0 source.
	updateSourceDir := t.TempDir()
	createTestPlugin(t, updateSourceDir, "test-plugin", "2.0.0")

	// Update.
	err := vm.Update(ctx, "test-plugin", filepath.Join(updateSourceDir, "test-plugin"))
	require.NoError(t, err)

	// Verify new version is installed.
	manifest, err := LoadManifestFromDir(filepath.Join(registryDir, "test-plugin"))
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", manifest.Metadata.Version)

	// Verify backup exists.
	backupManifest := filepath.Join(backupDir, "test-plugin", "1.0.0", "manifest.yaml")
	_, err = os.Stat(backupManifest)
	assert.NoError(t, err, "backup should exist")

	// Verify previous version tracking.
	assert.Equal(t, "1.0.0", vm.GetPrevious("test-plugin"))
}

func TestVersionManager_Update_PinnedPlugin(t *testing.T) {
	registry, backupDir, _ := setupTestEnvironment(t)
	ctx := context.Background()

	sourceDir := t.TempDir()
	createTestPlugin(t, sourceDir, "test-plugin", "1.0.0")
	require.NoError(t, registry.Install(ctx, filepath.Join(sourceDir, "test-plugin")))

	vm := NewVersionManager(registry, backupDir)
	require.NoError(t, vm.Pin(ctx, "test-plugin", "1.0.0"))

	updateSourceDir := t.TempDir()
	createTestPlugin(t, updateSourceDir, "test-plugin", "2.0.0")

	err := vm.Update(ctx, "test-plugin", filepath.Join(updateSourceDir, "test-plugin"))
	assert.ErrorIs(t, err, ErrPluginPinned)
}

func TestVersionManager_Update_Incompatible(t *testing.T) {
	registry, backupDir, _ := setupTestEnvironment(t)
	ctx := context.Background()

	sourceDir := t.TempDir()
	createTestPlugin(t, sourceDir, "test-plugin", "1.0.0")
	require.NoError(t, registry.Install(ctx, filepath.Join(sourceDir, "test-plugin")))

	vm := NewVersionManager(registry, backupDir)
	vm.SetSiplyVersion("1.0.0") // Set a real version so compatibility check actually works.

	// Create v2.0.0 source with high siply_min.
	updateSourceDir := t.TempDir()
	createTestPluginWithMin(t, updateSourceDir, "test-plugin", "2.0.0", "99.0.0")

	err := vm.Update(ctx, "test-plugin", filepath.Join(updateSourceDir, "test-plugin"))
	assert.ErrorIs(t, err, ErrIncompatible)
}

func TestVersionManager_Update_AlreadyLatest(t *testing.T) {
	registry, backupDir, _ := setupTestEnvironment(t)
	ctx := context.Background()

	sourceDir := t.TempDir()
	createTestPlugin(t, sourceDir, "test-plugin", "1.0.0")
	require.NoError(t, registry.Install(ctx, filepath.Join(sourceDir, "test-plugin")))

	vm := NewVersionManager(registry, backupDir)

	// Try to "update" with same version.
	sameSourceDir := t.TempDir()
	createTestPlugin(t, sameSourceDir, "test-plugin", "1.0.0")

	err := vm.Update(ctx, "test-plugin", filepath.Join(sameSourceDir, "test-plugin"))
	assert.ErrorIs(t, err, ErrAlreadyLatest)
}

func TestVersionManager_UpdateAll(t *testing.T) {
	registry, backupDir, _ := setupTestEnvironment(t)
	ctx := context.Background()

	// Install two plugins.
	sourceDir := t.TempDir()
	createTestPlugin(t, sourceDir, "plugin-a", "1.0.0")
	createTestPlugin(t, sourceDir, "plugin-b", "1.0.0")
	require.NoError(t, registry.Install(ctx, filepath.Join(sourceDir, "plugin-a")))
	require.NoError(t, registry.Install(ctx, filepath.Join(sourceDir, "plugin-b")))

	vm := NewVersionManager(registry, backupDir)

	// Pin plugin-b.
	require.NoError(t, vm.Pin(ctx, "plugin-b", "1.0.0"))

	// Create update sources.
	updateDir := t.TempDir()
	createTestPlugin(t, updateDir, "plugin-a", "2.0.0")
	createTestPlugin(t, updateDir, "plugin-b", "2.0.0")

	sources := map[string]string{
		"plugin-a": filepath.Join(updateDir, "plugin-a"),
		"plugin-b": filepath.Join(updateDir, "plugin-b"),
	}

	results, err := vm.UpdateAll(ctx, sources)
	require.NoError(t, err)
	require.Len(t, results, 2)

	resultMap := make(map[string]UpdateResult)
	for _, r := range results {
		resultMap[r.Name] = r
	}

	assert.Equal(t, "updated", resultMap["plugin-a"].Status)
	assert.Equal(t, "skipped", resultMap["plugin-b"].Status)
	assert.ErrorIs(t, resultMap["plugin-b"].Error, ErrPluginPinned)
}

func TestVersionManager_Rollback(t *testing.T) {
	registry, backupDir, registryDir := setupTestEnvironment(t)
	ctx := context.Background()

	// Install v1.0.0.
	sourceDir := t.TempDir()
	createTestPlugin(t, sourceDir, "test-plugin", "1.0.0")
	require.NoError(t, registry.Install(ctx, filepath.Join(sourceDir, "test-plugin")))

	vm := NewVersionManager(registry, backupDir)

	// Update to v2.0.0.
	updateSourceDir := t.TempDir()
	createTestPlugin(t, updateSourceDir, "test-plugin", "2.0.0")
	require.NoError(t, vm.Update(ctx, "test-plugin", filepath.Join(updateSourceDir, "test-plugin")))

	// Rollback.
	err := vm.Rollback(ctx, "test-plugin")
	require.NoError(t, err)

	// Verify v1.0.0 is restored.
	manifest, err := LoadManifestFromDir(filepath.Join(registryDir, "test-plugin"))
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", manifest.Metadata.Version)
}

func TestVersionManager_Rollback_NoPrevious(t *testing.T) {
	registry, backupDir, _ := setupTestEnvironment(t)
	ctx := context.Background()

	sourceDir := t.TempDir()
	createTestPlugin(t, sourceDir, "test-plugin", "1.0.0")
	require.NoError(t, registry.Install(ctx, filepath.Join(sourceDir, "test-plugin")))

	vm := NewVersionManager(registry, backupDir)

	err := vm.Rollback(ctx, "test-plugin")
	assert.ErrorIs(t, err, ErrNoPreviousVersion)
}

func TestVersionManager_Pin(t *testing.T) {
	registry, backupDir, _ := setupTestEnvironment(t)
	ctx := context.Background()

	sourceDir := t.TempDir()
	createTestPlugin(t, sourceDir, "test-plugin", "1.0.0")
	require.NoError(t, registry.Install(ctx, filepath.Join(sourceDir, "test-plugin")))

	vm := NewVersionManager(registry, backupDir)

	err := vm.Pin(ctx, "test-plugin", "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", vm.GetPinned("test-plugin"))
}

func TestVersionManager_Unpin(t *testing.T) {
	registry, backupDir, _ := setupTestEnvironment(t)
	ctx := context.Background()

	sourceDir := t.TempDir()
	createTestPlugin(t, sourceDir, "test-plugin", "1.0.0")
	require.NoError(t, registry.Install(ctx, filepath.Join(sourceDir, "test-plugin")))

	vm := NewVersionManager(registry, backupDir)
	require.NoError(t, vm.Pin(ctx, "test-plugin", "1.0.0"))
	assert.Equal(t, "1.0.0", vm.GetPinned("test-plugin"))

	require.NoError(t, vm.Unpin(ctx, "test-plugin"))
	assert.Equal(t, "", vm.GetPinned("test-plugin"))
}

func TestVersionManager_ConcurrentOperations(t *testing.T) {
	registry, backupDir, _ := setupTestEnvironment(t)
	ctx := context.Background()

	// Install two plugins so concurrent operations target different plugins.
	sourceDir := t.TempDir()
	createTestPlugin(t, sourceDir, "plugin-x", "1.0.0")
	createTestPlugin(t, sourceDir, "plugin-y", "1.0.0")
	require.NoError(t, registry.Install(ctx, filepath.Join(sourceDir, "plugin-x")))
	require.NoError(t, registry.Install(ctx, filepath.Join(sourceDir, "plugin-y")))

	vm := NewVersionManager(registry, backupDir)

	// Run concurrent operations — the point is race detection (-race flag).
	var wg sync.WaitGroup

	wg.Add(5)
	go func() {
		defer wg.Done()
		_, _ = vm.Check(ctx)
	}()
	go func() {
		defer wg.Done()
		_ = vm.Pin(ctx, "plugin-x", "1.0.0")
	}()
	go func() {
		defer wg.Done()
		_ = vm.Pin(ctx, "plugin-y", "1.0.0")
	}()
	go func() {
		defer wg.Done()
		_ = vm.Unpin(ctx, "plugin-x")
	}()
	go func() {
		defer wg.Done()
		_, _ = vm.Check(ctx)
	}()
	wg.Wait()
}

func TestVersionManager_IsCompatiblePlugin(t *testing.T) {
	registry, backupDir, _ := setupTestEnvironment(t)
	ctx := context.Background()

	sourceDir := t.TempDir()
	createTestPlugin(t, sourceDir, "test-plugin", "1.0.0")
	require.NoError(t, registry.Install(ctx, filepath.Join(sourceDir, "test-plugin")))

	vm := NewVersionManager(registry, backupDir)

	compatible, reason := vm.IsCompatiblePlugin(ctx, "test-plugin")
	assert.True(t, compatible)
	assert.Empty(t, reason)
}

func TestVersionManager_LoadPinState(t *testing.T) {
	registry, backupDir, _ := setupTestEnvironment(t)

	vm := NewVersionManager(registry, backupDir)

	pinned := map[string]string{"plugin-a": "1.0.0"}
	previous := map[string]string{"plugin-b": "0.9.0"}
	vm.LoadPinState(pinned, previous)

	assert.Equal(t, "1.0.0", vm.GetPinned("plugin-a"))
	assert.Equal(t, "0.9.0", vm.GetPrevious("plugin-b"))
}

func TestVersionManager_Update_EmptyName(t *testing.T) {
	registry, backupDir, _ := setupTestEnvironment(t)
	vm := NewVersionManager(registry, backupDir)
	err := vm.Update(context.Background(), "", "/some/path")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "plugin name is empty")
}

func TestVersionManager_Update_EmptySource(t *testing.T) {
	registry, backupDir, _ := setupTestEnvironment(t)
	vm := NewVersionManager(registry, backupDir)
	err := vm.Update(context.Background(), "test", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "source path is empty")
}

func TestVersionManager_Update_PathTraversal(t *testing.T) {
	registry, backupDir, _ := setupTestEnvironment(t)
	vm := NewVersionManager(registry, backupDir)
	err := vm.Update(context.Background(), "../etc", "/some/path")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid plugin name")
}

func TestVersionManager_Pin_WrongVersion(t *testing.T) {
	registry, backupDir, _ := setupTestEnvironment(t)
	ctx := context.Background()

	sourceDir := t.TempDir()
	createTestPlugin(t, sourceDir, "test-plugin", "1.0.0")
	require.NoError(t, registry.Install(ctx, filepath.Join(sourceDir, "test-plugin")))

	vm := NewVersionManager(registry, backupDir)
	err := vm.Pin(ctx, "test-plugin", "2.0.0")
	assert.ErrorIs(t, err, ErrVersionNotFound)
}
