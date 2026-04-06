// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/config"
	"siply.dev/siply/internal/core"
)

// mockConfigResolver implements core.ConfigResolver for integration tests.
type lockfileConfigResolver struct {
	cfg *core.Config
}

func (m *lockfileConfigResolver) Init(_ context.Context) error  { return nil }
func (m *lockfileConfigResolver) Start(_ context.Context) error { return nil }
func (m *lockfileConfigResolver) Stop(_ context.Context) error  { return nil }
func (m *lockfileConfigResolver) Health() error                 { return nil }
func (m *lockfileConfigResolver) Config() *core.Config          { return m.cfg }

// TestLockfile_FullFlow tests: set config → generate lockfile → write → re-read via Loader → verify match.
func TestLockfile_FullFlow(t *testing.T) {
	// AC#1, AC#3: generate and read back lockfile through full config lifecycle
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	// Write a project config that the loader will read.
	writeYAML(t, filepath.Join(globalDir, "config.yaml"), `
provider:
  default: anthropic
  model: claude-opus
session:
  retention_count: 50
`)

	// Step 1: Load config via Loader.
	loader := config.NewLoader(config.LoaderOptions{
		GlobalDir:  globalDir,
		ProjectDir: projectDir,
	})
	require.NoError(t, loader.Init(context.Background()))

	originalCfg := loader.Config()
	require.NotNil(t, originalCfg)

	// Step 2: Generate lockfile from current config.
	lf, err := config.GenerateLockfile(context.Background(), config.GenerateOptions{
		ConfigResolver: loader,
	})
	require.NoError(t, err)
	assert.Equal(t, "1", lf.Version)
	assert.Equal(t, "anthropic", lf.Config.Provider.Default)

	// Step 3: Write lockfile to disk.
	lockPath := filepath.Join(projectDir, "config.lock")
	require.NoError(t, config.WriteLockfile(lockPath, lf))

	// Step 4: Create a fresh Loader that reads the lockfile as layer 3.
	loader2 := config.NewLoader(config.LoaderOptions{
		GlobalDir:  globalDir,
		ProjectDir: projectDir,
	})
	require.NoError(t, loader2.Init(context.Background()))

	reloadedCfg := loader2.Config()
	assert.Equal(t, originalCfg.Provider.Default, reloadedCfg.Provider.Default)
	assert.Equal(t, originalCfg.Provider.Model, reloadedCfg.Provider.Model)

	// Step 5: Verify lockfile matches.
	result, err := config.VerifyLockfile(context.Background(), config.VerifyOptions{
		LockfilePath:   lockPath,
		ConfigResolver: loader2,
	})
	require.NoError(t, err)
	assert.True(t, result.Match, "lockfile should match after fresh load, diffs: %v", result.Diffs)
}

// TestLockfile_DetectsMismatchAfterModification verifies that VerifyLockfile
// detects when config has changed after the lockfile was generated.
func TestLockfile_DetectsMismatchAfterModification(t *testing.T) {
	// AC#6: verify detects mismatches
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	writeYAML(t, filepath.Join(globalDir, "config.yaml"), `
provider:
  default: anthropic
  model: claude-opus
`)

	loader := config.NewLoader(config.LoaderOptions{
		GlobalDir:  globalDir,
		ProjectDir: projectDir,
	})
	require.NoError(t, loader.Init(context.Background()))

	lf, err := config.GenerateLockfile(context.Background(), config.GenerateOptions{
		ConfigResolver: loader,
	})
	require.NoError(t, err)

	lockPath := filepath.Join(projectDir, "config.lock")
	require.NoError(t, config.WriteLockfile(lockPath, lf))

	// Now change the config.
	modifiedCfg := &core.Config{
		Provider: core.ProviderConfig{Default: "openai", Model: "gpt-4o"},
	}

	result, err := config.VerifyLockfile(context.Background(), config.VerifyOptions{
		LockfilePath:   lockPath,
		ConfigResolver: &lockfileConfigResolver{cfg: modifiedCfg},
	})
	require.NoError(t, err)
	assert.False(t, result.Match)
	assert.GreaterOrEqual(t, len(result.Diffs), 2) // at least provider.default + provider.model
}

// TestLockfile_Roundtrip verifies generate → marshal → parse produces identical data.
func TestLockfile_Roundtrip(t *testing.T) {
	retention := 42
	cfg := &core.Config{
		Provider: core.ProviderConfig{Default: "ollama", Model: "llama3"},
		Session:  core.SessionConfig{RetentionCount: &retention},
		Plugins:  map[string]any{"test-plugin": map[string]any{"key": "value"}},
	}

	lf, err := config.GenerateLockfile(context.Background(), config.GenerateOptions{
		ConfigResolver: &lockfileConfigResolver{cfg: cfg},
	})
	require.NoError(t, err)

	// Marshal → Parse roundtrip.
	data, err := config.MarshalLockfile(lf)
	require.NoError(t, err)

	parsed, err := config.ParseLockfile(data)
	require.NoError(t, err)

	assert.Equal(t, lf.Version, parsed.Version)
	assert.Equal(t, lf.GeneratedAt, parsed.GeneratedAt)
	assert.Equal(t, lf.Config.Provider.Default, parsed.Config.Provider.Default)
	assert.Equal(t, lf.Config.Provider.Model, parsed.Config.Provider.Model)
	assert.Equal(t, *lf.Config.Session.RetentionCount, *parsed.Config.Session.RetentionCount)
	assert.Len(t, parsed.Plugins, 0) // No plugin registry → empty plugins

	// Write → Read roundtrip via file.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.lock")
	require.NoError(t, config.WriteLockfile(path, lf))

	fileData, err := os.ReadFile(path)
	require.NoError(t, err)
	fileParsed, err := config.ParseLockfile(fileData)
	require.NoError(t, err)
	assert.Equal(t, lf.Version, fileParsed.Version)
	assert.Equal(t, lf.Config.Provider.Default, fileParsed.Config.Provider.Default)
}
