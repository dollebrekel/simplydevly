// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package plugins

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/core"
)

// testPluginBinaries holds paths to compiled test plugin binaries.
var testPluginBinaries struct {
	once      sync.Once
	testDir   string
	echoPath  string
	crashPath string
	slowPath  string
	buildErr  error
}

// buildTestPlugins compiles the test plugin binaries once per test run.
// Uses os.MkdirTemp instead of t.TempDir() because the binaries are shared
// across tests via sync.Once, and t.TempDir() would be cleaned up after the
// first test completes.
func buildTestPlugins(t *testing.T) {
	t.Helper()

	testPluginBinaries.once.Do(func() {
		dir, err := os.MkdirTemp("", "tier3-test-plugins-*")
		if err != nil {
			testPluginBinaries.buildErr = err
			return
		}
		testPluginBinaries.testDir = dir

		plugins := []struct {
			name string
			src  string
			dest *string
		}{
			{"tier3-test-plugin", "testdata/tier3-test-plugin", &testPluginBinaries.echoPath},
			{"tier3-crash-plugin", "testdata/tier3-crash-plugin", &testPluginBinaries.crashPath},
			{"tier3-slow-plugin", "testdata/tier3-slow-plugin", &testPluginBinaries.slowPath},
		}

		for _, p := range plugins {
			binDir := filepath.Join(testPluginBinaries.testDir, p.name)
			if err := os.MkdirAll(binDir, 0755); err != nil {
				testPluginBinaries.buildErr = err
				return
			}
			binPath := filepath.Join(binDir, p.name)
			cmd := exec.Command("go", "build", "-o", binPath, "./internal/plugins/"+p.src)
			cmd.Dir = findModuleRoot(t)
			cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
			out, err := cmd.CombinedOutput()
			if err != nil {
				testPluginBinaries.buildErr = &buildError{plugin: p.name, output: string(out), err: err}
				return
			}

			// Copy manifest.yaml to the binary directory.
			manifestSrc := filepath.Join(findModuleRoot(t), "internal", "plugins", p.src, "manifest.yaml")
			manifestDst := filepath.Join(binDir, "manifest.yaml")
			data, err := os.ReadFile(manifestSrc)
			if err != nil {
				testPluginBinaries.buildErr = err
				return
			}
			if err := os.WriteFile(manifestDst, data, 0644); err != nil {
				testPluginBinaries.buildErr = err
				return
			}

			*p.dest = binPath
		}
	})

	if testPluginBinaries.buildErr != nil {
		t.Fatalf("build test plugins: %v", testPluginBinaries.buildErr)
	}
	// Note (P16): temp dir is intentionally NOT cleaned per-test — it is shared
	// across all tests via sync.Once. OS temp cleanup handles removal.
}

type buildError struct {
	plugin string
	output string
	err    error
}

func (e *buildError) Error() string {
	return "build " + e.plugin + ": " + e.err.Error() + "\n" + e.output
}

// findModuleRoot locates the Go module root by searching for go.mod.
func findModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find go.mod")
		}
		dir = parent
	}
}

// setupTestRegistry creates a temporary registry with test plugin binaries.
func setupTestRegistry(t *testing.T, pluginName string, binPath string) (*LocalRegistry, string) {
	t.Helper()

	registryDir := t.TempDir()
	registry := NewLocalRegistry(registryDir)
	require.NoError(t, registry.Init(context.Background()))

	// Copy the built plugin directory to the registry.
	pluginDir := filepath.Join(registryDir, pluginName)
	srcDir := filepath.Dir(binPath)
	require.NoError(t, copyDir(srcDir, pluginDir))

	// Re-init to pick up the installed plugin.
	require.NoError(t, registry.Init(context.Background()))

	return registry, registryDir
}

// newTestHostServer creates a HostServer backed by mocks for testing.
func newTestHostServer() *HostServer {
	return NewHostServer(HostServerOptions{
		ToolExecutor:    &mockToolExecutor{},
		CredentialStore: nil,
		ConfigProvider:  nil,
		StatusCollector: nil,
	})
}

type mockToolExecutor struct{}

func (m *mockToolExecutor) Init(_ context.Context) error   { return nil }
func (m *mockToolExecutor) Start(_ context.Context) error  { return nil }
func (m *mockToolExecutor) Stop(_ context.Context) error   { return nil }
func (m *mockToolExecutor) Health() error                  { return nil }
func (m *mockToolExecutor) ListTools() []core.ToolDefinition { return nil }
func (m *mockToolExecutor) GetTool(_ string) (core.ToolDefinition, error) {
	return core.ToolDefinition{}, nil
}
func (m *mockToolExecutor) Execute(_ context.Context, req core.ToolRequest) (core.ToolResponse, error) {
	return core.ToolResponse{Output: "ok"}, nil
}

func TestTier3Loader_Load_ValidPlugin(t *testing.T) {
	buildTestPlugins(t)

	registry, _ := setupTestRegistry(t, "tier3-test-plugin", testPluginBinaries.echoPath)
	hs := newTestHostServer()
	loader := NewTier3Loader(registry, hs)

	err := loader.Load(context.Background(), "tier3-test-plugin")
	require.NoError(t, err)

	// Plugin should be loaded but NOT running (lazy).
	assert.True(t, loader.IsLoaded("tier3-test-plugin"))
	assert.False(t, loader.IsRunning("tier3-test-plugin"))
}

func TestTier3Loader_Load_NonTier3Plugin(t *testing.T) {
	registryDir := t.TempDir()
	registry := NewLocalRegistry(registryDir)
	require.NoError(t, registry.Init(context.Background()))

	// Create a Tier 1 plugin manifest.
	pluginDir := filepath.Join(registryDir, "tier1-plugin")
	require.NoError(t, os.MkdirAll(pluginDir, 0755))

	manifest := `apiVersion: siply/v1
kind: Plugin
metadata:
  name: tier1-plugin
  version: 1.0.0
  siply_min: 0.1.0
  description: Tier 1 test
  author: test
  license: Apache-2.0
  updated: "2026-04-10"
spec:
  tier: 1
  capabilities:
    filesystem: read
`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "manifest.yaml"), []byte(manifest), 0644))
	require.NoError(t, registry.Init(context.Background()))

	hs := newTestHostServer()
	loader := NewTier3Loader(registry, hs)

	err := loader.Load(context.Background(), "tier1-plugin")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotTier3)
}

func TestTier3Loader_Load_MissingBinary(t *testing.T) {
	registryDir := t.TempDir()
	registry := NewLocalRegistry(registryDir)
	require.NoError(t, registry.Init(context.Background()))

	// Create a Tier 3 plugin directory WITHOUT a binary.
	pluginDir := filepath.Join(registryDir, "no-binary-plugin")
	require.NoError(t, os.MkdirAll(pluginDir, 0755))

	manifest := `apiVersion: siply/v1
kind: Plugin
metadata:
  name: no-binary-plugin
  version: 1.0.0
  siply_min: 0.1.0
  description: No binary test
  author: test
  license: Apache-2.0
  updated: "2026-04-10"
spec:
  tier: 3
  capabilities:
    filesystem: read
`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "manifest.yaml"), []byte(manifest), 0644))
	require.NoError(t, registry.Init(context.Background()))

	hs := newTestHostServer()
	loader := NewTier3Loader(registry, hs)

	err := loader.Load(context.Background(), "no-binary-plugin")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrBinaryNotFound)
}

func TestTier3Loader_SpawnAndExecute(t *testing.T) {
	buildTestPlugins(t)

	registry, _ := setupTestRegistry(t, "tier3-test-plugin", testPluginBinaries.echoPath)
	hs := newTestHostServer()
	loader := NewTier3Loader(registry, hs)

	ctx := context.Background()
	require.NoError(t, loader.Load(ctx, "tier3-test-plugin"))

	// Execute triggers lazy spawn.
	payload := []byte(`{"hello":"world"}`)
	result, err := loader.Execute(ctx, "tier3-test-plugin", "echo", payload)
	require.NoError(t, err)
	assert.Equal(t, payload, result)

	// Plugin should now be running.
	assert.True(t, loader.IsRunning("tier3-test-plugin"))

	// Clean up.
	require.NoError(t, loader.Unload(ctx, "tier3-test-plugin"))
}

func TestTier3Loader_CrashIsolation(t *testing.T) {
	buildTestPlugins(t)

	registry, _ := setupTestRegistry(t, "tier3-crash-plugin", testPluginBinaries.crashPath)
	hs := newTestHostServer()
	loader := NewTier3Loader(registry, hs)

	ctx := context.Background()
	require.NoError(t, loader.Load(ctx, "tier3-crash-plugin"))

	// Execute should fail with ErrPluginCrashed, but core should survive (NFR23).
	_, err := loader.Execute(ctx, "tier3-crash-plugin", "crash", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPluginCrashed)

	// Core process is still running — we're here, which proves crash isolation.
}

func TestTier3Loader_OperationTimeout(t *testing.T) {
	buildTestPlugins(t)

	registry, _ := setupTestRegistry(t, "tier3-slow-plugin", testPluginBinaries.slowPath)
	hs := newTestHostServer()
	// Use very short timeout for the test.
	loader := NewTier3Loader(registry, hs, WithOperationTimeout(500*time.Millisecond))

	ctx := context.Background()
	require.NoError(t, loader.Load(ctx, "tier3-slow-plugin"))

	// Execute should timeout (NFR24).
	_, err := loader.Execute(ctx, "tier3-slow-plugin", "slow", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPluginTimeout)
}

func TestTier3Loader_GracefulUnload(t *testing.T) {
	buildTestPlugins(t)

	registry, _ := setupTestRegistry(t, "tier3-test-plugin", testPluginBinaries.echoPath)
	hs := newTestHostServer()
	loader := NewTier3Loader(registry, hs)

	ctx := context.Background()
	require.NoError(t, loader.Load(ctx, "tier3-test-plugin"))

	// Spawn first.
	_, err := loader.Execute(ctx, "tier3-test-plugin", "echo", []byte("test"))
	require.NoError(t, err)
	assert.True(t, loader.IsRunning("tier3-test-plugin"))

	// Graceful unload.
	err = loader.Unload(ctx, "tier3-test-plugin")
	require.NoError(t, err)
	assert.False(t, loader.IsLoaded("tier3-test-plugin"))
}

func TestTier3Loader_ForcedUnload(t *testing.T) {
	buildTestPlugins(t)

	registry, _ := setupTestRegistry(t, "tier3-slow-plugin", testPluginBinaries.slowPath)
	hs := newTestHostServer()
	loader := NewTier3Loader(registry, hs)

	ctx := context.Background()
	require.NoError(t, loader.Load(ctx, "tier3-slow-plugin"))

	// Spawn the slow plugin.
	require.NoError(t, loader.Spawn(ctx, "tier3-slow-plugin"))
	assert.True(t, loader.IsRunning("tier3-slow-plugin"))

	// Unload — Shutdown will timeout, so forced kill happens.
	err := loader.Unload(ctx, "tier3-slow-plugin")
	require.NoError(t, err)
	assert.False(t, loader.IsLoaded("tier3-slow-plugin"))
}

func TestTier3Loader_LazySpawnOnFirstExecute(t *testing.T) {
	buildTestPlugins(t)

	registry, _ := setupTestRegistry(t, "tier3-test-plugin", testPluginBinaries.echoPath)
	hs := newTestHostServer()
	loader := NewTier3Loader(registry, hs)

	ctx := context.Background()
	require.NoError(t, loader.Load(ctx, "tier3-test-plugin"))

	// Not running yet.
	assert.False(t, loader.IsRunning("tier3-test-plugin"))

	// First execute should trigger spawn.
	result, err := loader.Execute(ctx, "tier3-test-plugin", "echo", []byte("lazy"))
	require.NoError(t, err)
	assert.Equal(t, []byte("lazy"), result)

	// Now running.
	assert.True(t, loader.IsRunning("tier3-test-plugin"))

	require.NoError(t, loader.Unload(ctx, "tier3-test-plugin"))
}

func TestTier3Loader_ConcurrentExecuteAndUnload(t *testing.T) {
	buildTestPlugins(t)

	registry, _ := setupTestRegistry(t, "tier3-test-plugin", testPluginBinaries.echoPath)
	hs := newTestHostServer()
	loader := NewTier3Loader(registry, hs)

	ctx := context.Background()
	require.NoError(t, loader.Load(ctx, "tier3-test-plugin"))

	// Spawn first.
	_, err := loader.Execute(ctx, "tier3-test-plugin", "echo", []byte("warmup"))
	require.NoError(t, err)

	// Concurrent Execute + Unload — race detection via -race flag.
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _ = loader.Execute(ctx, "tier3-test-plugin", "echo", []byte("concurrent"))
	}()

	go func() {
		defer wg.Done()
		_ = loader.Unload(ctx, "tier3-test-plugin")
	}()

	wg.Wait()
	// Test passes if no race detected.
}

func TestTier3Loader_List_ReturnsConsistentSnapshot(t *testing.T) {
	buildTestPlugins(t)

	registry, _ := setupTestRegistry(t, "tier3-test-plugin", testPluginBinaries.echoPath)
	hs := newTestHostServer()
	loader := NewTier3Loader(registry, hs)

	ctx := context.Background()
	require.NoError(t, loader.Load(ctx, "tier3-test-plugin"))

	// List before spawn — should show loaded but not running.
	infos := loader.List(ctx)
	require.Len(t, infos, 1)
	assert.Equal(t, "tier3-test-plugin", infos[0].Name)
	assert.Equal(t, 3, infos[0].Tier)
	assert.False(t, infos[0].Running)
	assert.False(t, infos[0].Crashed)

	// Spawn and list again — should show running.
	_, err := loader.Execute(ctx, "tier3-test-plugin", "echo", []byte("test"))
	require.NoError(t, err)

	infos = loader.List(ctx)
	require.Len(t, infos, 1)
	assert.True(t, infos[0].Running)
	assert.False(t, infos[0].Crashed)

	require.NoError(t, loader.Unload(ctx, "tier3-test-plugin"))
}

func TestTier3Loader_IsRunningReflectsProcessState(t *testing.T) {
	buildTestPlugins(t)

	registry, _ := setupTestRegistry(t, "tier3-test-plugin", testPluginBinaries.echoPath)
	hs := newTestHostServer()
	loader := NewTier3Loader(registry, hs)

	ctx := context.Background()
	require.NoError(t, loader.Load(ctx, "tier3-test-plugin"))

	// Not running before spawn.
	assert.False(t, loader.IsRunning("tier3-test-plugin"))

	// Running after spawn.
	_, err := loader.Execute(ctx, "tier3-test-plugin", "echo", []byte("test"))
	require.NoError(t, err)
	assert.True(t, loader.IsRunning("tier3-test-plugin"))

	// Not running after unload.
	require.NoError(t, loader.Unload(ctx, "tier3-test-plugin"))
	assert.False(t, loader.IsRunning("tier3-test-plugin"))
}

