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

func TestNewLocalRegistry(t *testing.T) {
	r := NewLocalRegistry("/tmp/test-plugins")
	require.NotNil(t, r)
	assert.Equal(t, "/tmp/test-plugins", r.registryDir)
	assert.NotNil(t, r.devPaths)
	assert.NotNil(t, r.plugins)
}

func TestInit_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	r := NewLocalRegistry(dir)

	err := r.Init(context.Background())
	require.NoError(t, err)

	list, err := r.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestInit_NonExistentDir(t *testing.T) {
	r := NewLocalRegistry("/nonexistent/path/that/doesnt/exist")
	err := r.Init(context.Background())
	require.NoError(t, err) // non-existent dir is not an error, just empty
}

func TestInit_WithValidPlugins(t *testing.T) {
	dir := t.TempDir()
	setupPlugin(t, dir, "memory-default", "testdata/valid_manifest.yaml")

	r := NewLocalRegistry(dir)
	err := r.Init(context.Background())
	require.NoError(t, err)

	list, err := r.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "memory-default", list[0].Name)
}

func TestInit_MixedValidInvalid(t *testing.T) {
	dir := t.TempDir()
	setupPlugin(t, dir, "memory-default", "testdata/valid_manifest.yaml")
	setupPlugin(t, dir, "bad-plugin", "testdata/invalid_bad_tier.yaml")

	r := NewLocalRegistry(dir)
	err := r.Init(context.Background())
	require.NoError(t, err)

	list, err := r.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, list, 1) // only valid plugin loaded
	assert.Equal(t, "memory-default", list[0].Name)
}

func TestInstall_Success(t *testing.T) {
	registryDir := t.TempDir()
	sourceDir := t.TempDir()
	copyFixture(t, "testdata/valid_manifest.yaml", filepath.Join(sourceDir, "manifest.yaml"))

	r := NewLocalRegistry(registryDir)
	require.NoError(t, r.Init(context.Background()))

	err := r.Install(context.Background(), sourceDir)
	require.NoError(t, err)

	// Verify plugin in registry
	list, err := r.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "memory-default", list[0].Name)

	// Verify files copied
	_, err = os.Stat(filepath.Join(registryDir, "memory-default", "manifest.yaml"))
	assert.NoError(t, err)
}

func TestInstall_AlreadyExists(t *testing.T) {
	registryDir := t.TempDir()
	sourceDir := t.TempDir()
	copyFixture(t, "testdata/valid_manifest.yaml", filepath.Join(sourceDir, "manifest.yaml"))

	r := NewLocalRegistry(registryDir)
	require.NoError(t, r.Init(context.Background()))

	require.NoError(t, r.Install(context.Background(), sourceDir))
	err := r.Install(context.Background(), sourceDir)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAlreadyInstalled)
}

func TestInstall_InvalidManifest(t *testing.T) {
	registryDir := t.TempDir()
	sourceDir := t.TempDir()
	copyFixture(t, "testdata/invalid_bad_tier.yaml", filepath.Join(sourceDir, "manifest.yaml"))

	r := NewLocalRegistry(registryDir)
	require.NoError(t, r.Init(context.Background()))

	err := r.Install(context.Background(), sourceDir)
	require.Error(t, err)
}

func TestLoad_FromRegistry(t *testing.T) {
	dir := t.TempDir()
	setupPlugin(t, dir, "memory-default", "testdata/valid_manifest.yaml")

	r := NewLocalRegistry(dir)
	require.NoError(t, r.Init(context.Background()))

	// Clear plugins to test Load
	r.mu.Lock()
	r.plugins = make(map[string]*Manifest)
	r.mu.Unlock()

	err := r.Load(context.Background(), "memory-default")
	require.NoError(t, err)

	list, err := r.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestLoad_FromDevPaths(t *testing.T) {
	registryDir := t.TempDir()
	devDir := t.TempDir()
	copyFixture(t, "testdata/valid_manifest.yaml", filepath.Join(devDir, "manifest.yaml"))

	r := NewLocalRegistry(registryDir)
	require.NoError(t, r.Init(context.Background()))

	// Set up dev path manually
	r.mu.Lock()
	r.devPaths["memory-default"] = devDir
	r.mu.Unlock()

	err := r.Load(context.Background(), "memory-default")
	require.NoError(t, err)

	list, err := r.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestLoad_NotFound(t *testing.T) {
	dir := t.TempDir()
	r := NewLocalRegistry(dir)
	require.NoError(t, r.Init(context.Background()))

	err := r.Load(context.Background(), "nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestList_ReturnsCorrectPluginMeta(t *testing.T) {
	dir := t.TempDir()
	setupPlugin(t, dir, "memory-default", "testdata/valid_manifest.yaml")

	r := NewLocalRegistry(dir)
	require.NoError(t, r.Init(context.Background()))

	list, err := r.List(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 1)

	meta := list[0]
	assert.Equal(t, "memory-default", meta.Name)
	assert.Equal(t, "1.0.0", meta.Version)
	assert.Equal(t, 3, meta.Tier)
	assert.NotEmpty(t, meta.Capabilities)
}

func TestRemove_Success(t *testing.T) {
	registryDir := t.TempDir()
	sourceDir := t.TempDir()
	copyFixture(t, "testdata/valid_manifest.yaml", filepath.Join(sourceDir, "manifest.yaml"))

	r := NewLocalRegistry(registryDir)
	require.NoError(t, r.Init(context.Background()))
	require.NoError(t, r.Install(context.Background(), sourceDir))

	err := r.Remove(context.Background(), "memory-default")
	require.NoError(t, err)

	list, err := r.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, list)

	// Verify directory removed
	_, err = os.Stat(filepath.Join(registryDir, "memory-default"))
	assert.True(t, os.IsNotExist(err))
}

func TestRemove_NotFound(t *testing.T) {
	dir := t.TempDir()
	r := NewLocalRegistry(dir)
	require.NoError(t, r.Init(context.Background()))

	err := r.Remove(context.Background(), "nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestRemove_DevModeRejected(t *testing.T) {
	registryDir := t.TempDir()
	devDir := t.TempDir()
	copyFixture(t, "testdata/valid_manifest.yaml", filepath.Join(devDir, "manifest.yaml"))

	r := NewLocalRegistry(registryDir)
	require.NoError(t, r.Init(context.Background()))
	require.NoError(t, r.DevMode(context.Background(), devDir))

	err := r.Remove(context.Background(), "memory-default")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDevModeRemove)
}

func TestDevMode_Success(t *testing.T) {
	registryDir := t.TempDir()
	devDir := t.TempDir()
	copyFixture(t, "testdata/valid_manifest.yaml", filepath.Join(devDir, "manifest.yaml"))

	r := NewLocalRegistry(registryDir)
	require.NoError(t, r.Init(context.Background()))

	err := r.DevMode(context.Background(), devDir)
	require.NoError(t, err)

	list, err := r.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "memory-default", list[0].Name)
}

func TestDevMode_InvalidManifest(t *testing.T) {
	registryDir := t.TempDir()
	devDir := t.TempDir()
	copyFixture(t, "testdata/invalid_bad_tier.yaml", filepath.Join(devDir, "manifest.yaml"))

	r := NewLocalRegistry(registryDir)
	require.NoError(t, r.Init(context.Background()))

	err := r.DevMode(context.Background(), devDir)
	require.Error(t, err)
}

func TestHealth(t *testing.T) {
	t.Run("accessible dir", func(t *testing.T) {
		dir := t.TempDir()
		r := NewLocalRegistry(dir)
		assert.NoError(t, r.Health())
	})

	t.Run("inaccessible dir", func(t *testing.T) {
		r := NewLocalRegistry("/nonexistent/path")
		assert.Error(t, r.Health())
	})

	t.Run("empty registryDir", func(t *testing.T) {
		r := NewLocalRegistry("")
		assert.Error(t, r.Health())
	})
}

func TestStop_ClearsState(t *testing.T) {
	dir := t.TempDir()
	setupPlugin(t, dir, "memory-default", "testdata/valid_manifest.yaml")

	r := NewLocalRegistry(dir)
	require.NoError(t, r.Init(context.Background()))

	list, err := r.List(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 1)

	require.NoError(t, r.Stop(context.Background()))

	list, err = r.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestConcurrentAccess(t *testing.T) {
	registryDir := t.TempDir()
	r := NewLocalRegistry(registryDir)
	require.NoError(t, r.Init(context.Background()))

	// Pre-install a plugin via direct setup
	setupPlugin(t, registryDir, "memory-default", "testdata/valid_manifest.yaml")
	require.NoError(t, r.Init(context.Background())) // re-init to pick it up

	ctx := context.Background()
	var wg sync.WaitGroup

	// Parallel reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = r.List(ctx)
		}()
	}

	// Parallel load attempts
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.Load(ctx, "memory-default")
		}()
	}

	// Prepare fixtures before spawning goroutines (avoid require in goroutines).
	installSources := make([]string, 5)
	for i := range installSources {
		installSources[i] = t.TempDir()
		copyFixture(t, "testdata/valid_manifest.yaml", filepath.Join(installSources[i], "manifest.yaml"))
	}

	// Parallel install attempts (may fail with ErrAlreadyInstalled; tests race safety)
	installErrs := make([]error, len(installSources))
	for i, src := range installSources {
		wg.Add(1)
		go func(idx int, source string) {
			defer wg.Done()
			installErrs[idx] = r.Install(ctx, source)
		}(i, src)
	}

	wg.Wait()
}

// setupPlugin creates a plugin subdirectory with a manifest in the registry dir.
func setupPlugin(t *testing.T, registryDir, name, fixtureFile string) {
	t.Helper()
	pluginDir := filepath.Join(registryDir, name)
	require.NoError(t, os.MkdirAll(pluginDir, 0755))
	copyFixture(t, fixtureFile, filepath.Join(pluginDir, "manifest.yaml"))
}
