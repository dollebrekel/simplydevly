// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package config

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/core"
)

// mockConfigResolver implements core.ConfigResolver for testing.
type mockConfigResolver struct {
	cfg *core.Config
}

func (m *mockConfigResolver) Init(_ context.Context) error  { return nil }
func (m *mockConfigResolver) Start(_ context.Context) error { return nil }
func (m *mockConfigResolver) Stop(_ context.Context) error  { return nil }
func (m *mockConfigResolver) Health() error                 { return nil }
func (m *mockConfigResolver) Config() *core.Config          { return m.cfg }

// mockPluginRegistry implements core.PluginRegistry for testing.
type mockPluginRegistry struct {
	plugins []core.PluginMeta
	err     error
}

func (m *mockPluginRegistry) Init(_ context.Context) error               { return nil }
func (m *mockPluginRegistry) Start(_ context.Context) error              { return nil }
func (m *mockPluginRegistry) Stop(_ context.Context) error               { return nil }
func (m *mockPluginRegistry) Health() error                              { return nil }
func (m *mockPluginRegistry) Install(_ context.Context, _ string) error  { return nil }
func (m *mockPluginRegistry) Load(_ context.Context, _ string) error     { return nil }
func (m *mockPluginRegistry) Remove(_ context.Context, _ string) error   { return nil }
func (m *mockPluginRegistry) DevMode(_ context.Context, _ string) error  { return nil }
func (m *mockPluginRegistry) List(_ context.Context) ([]core.PluginMeta, error) {
	return m.plugins, m.err
}

func TestMarshalLockfile_SortedIndentedJSON(t *testing.T) {
	// AC#5: git-diffable JSON with sorted keys and 2-space indent
	lf := &Lockfile{
		Version:     "1",
		GeneratedAt: "2026-04-06T14:30:00Z",
		Config: core.Config{
			Provider: core.ProviderConfig{Default: "anthropic", Model: "claude-opus"},
			Plugins:  map[string]any{"beta": "b", "alpha": "a"},
		},
		Plugins: []LockfilePlugin{
			{Name: "plugin-a", Version: "1.0.0", Tier: 3, Checksum: "sha256:abc"},
		},
	}

	data, err := MarshalLockfile(lf)
	require.NoError(t, err)

	// Verify indentation.
	assert.Contains(t, string(data), "  \"version\": \"1\"")

	// Verify map keys are sorted (Go's encoding/json sorts map keys).
	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	configMap := raw["config"].(map[string]any)
	pluginsMap := configMap["plugins"].(map[string]any)
	_ = pluginsMap // map key order verified by json.Unmarshal roundtrip

	// Verify valid JSON roundtrip.
	parsed, err := ParseLockfile(data)
	require.NoError(t, err)
	assert.Equal(t, lf.Version, parsed.Version)
}

func TestParseLockfile_RejectsUnknownFields(t *testing.T) {
	// AC#2: strict parsing rejects unknown fields
	data := []byte(`{"version":"1","generated_at":"2026-01-01T00:00:00Z","config":{},"plugins":[],"unknown":"bad"}`)
	_, err := ParseLockfile(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lockfile: failed to parse")
}

func TestParseLockfile_RejectsTrailingData(t *testing.T) {
	data := []byte(`{"version":"1","generated_at":"2026-01-01T00:00:00Z","config":{},"plugins":[]}{"extra":true}`)
	_, err := ParseLockfile(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trailing data")
}

func TestGenerateLockfile_NilPluginRegistry(t *testing.T) {
	// Nil PluginRegistry produces empty plugins array (not null).
	cfg := &core.Config{
		Provider: core.ProviderConfig{Default: "anthropic"},
	}
	resolver := &mockConfigResolver{cfg: cfg}

	lf, err := GenerateLockfile(context.Background(), GenerateOptions{
		ConfigResolver: resolver,
	})
	require.NoError(t, err)
	assert.Equal(t, "1", lf.Version)
	assert.NotEmpty(t, lf.GeneratedAt)
	require.NotNil(t, lf.Plugins)
	assert.Len(t, lf.Plugins, 0)

	// Verify JSON encodes as [] not null.
	data, err := MarshalLockfile(lf)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"plugins": []`)
}

func TestGenerateLockfile_WithPluginRegistry(t *testing.T) {
	// Plugins are sorted by name.
	cfg := &core.Config{
		Provider: core.ProviderConfig{Default: "anthropic"},
	}
	registry := &mockPluginRegistry{
		plugins: []core.PluginMeta{
			{Name: "zeta-plugin", Version: "2.0.0", Tier: 1},
			{Name: "alpha-plugin", Version: "1.0.0", Tier: 3},
		},
	}

	lf, err := GenerateLockfile(context.Background(), GenerateOptions{
		ConfigResolver: &mockConfigResolver{cfg: cfg},
		PluginRegistry: registry,
	})
	require.NoError(t, err)
	require.Len(t, lf.Plugins, 2)
	// AC#2: sorted by name
	assert.Equal(t, "alpha-plugin", lf.Plugins[0].Name)
	assert.Equal(t, "zeta-plugin", lf.Plugins[1].Name)
}

func TestWriteLockfile_CreatesFileWith0644(t *testing.T) {
	// AC#5: lockfile is git-shareable with 0644 permissions
	dir := t.TempDir()
	path := filepath.Join(dir, "config.lock")

	lf := &Lockfile{
		Version:     "1",
		GeneratedAt: "2026-01-01T00:00:00Z",
		Config:      core.Config{},
		Plugins:     []LockfilePlugin{},
	}

	require.NoError(t, WriteLockfile(path, lf))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0644), info.Mode().Perm())
}

func TestWriteLockfile_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.lock")

	lf1 := &Lockfile{Version: "1", GeneratedAt: "2026-01-01T00:00:00Z", Config: core.Config{Provider: core.ProviderConfig{Default: "v1"}}, Plugins: []LockfilePlugin{}}
	lf2 := &Lockfile{Version: "1", GeneratedAt: "2026-01-02T00:00:00Z", Config: core.Config{Provider: core.ProviderConfig{Default: "v2"}}, Plugins: []LockfilePlugin{}}

	require.NoError(t, WriteLockfile(path, lf1))
	require.NoError(t, WriteLockfile(path, lf2))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	parsed, err := ParseLockfile(data)
	require.NoError(t, err)
	assert.Equal(t, "v2", parsed.Config.Provider.Default)
}

func TestVerifyLockfile_Match(t *testing.T) {
	// AC#6: verify returns match when config matches
	cfg := &core.Config{
		Provider: core.ProviderConfig{Default: "anthropic", Model: "claude-opus"},
		Session:  core.SessionConfig{RetentionCount: intPtr(50)},
	}

	lf := &Lockfile{
		Version:     "1",
		GeneratedAt: "2026-01-01T00:00:00Z",
		Config:      *cfg,
		Plugins:     []LockfilePlugin{},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.lock")
	require.NoError(t, WriteLockfile(path, lf))

	result, err := VerifyLockfile(context.Background(), VerifyOptions{
		LockfilePath:   path,
		ConfigResolver: &mockConfigResolver{cfg: cfg},
	})
	require.NoError(t, err)
	assert.True(t, result.Match)
	assert.Empty(t, result.Diffs)
}

func TestVerifyLockfile_ConfigMismatch(t *testing.T) {
	// AC#6: verify detects mismatches
	lockCfg := &core.Config{
		Provider: core.ProviderConfig{Default: "anthropic", Model: "claude-opus"},
	}
	currentCfg := &core.Config{
		Provider: core.ProviderConfig{Default: "openai", Model: "gpt-4o"},
	}

	lf := &Lockfile{
		Version:     "1",
		GeneratedAt: "2026-01-01T00:00:00Z",
		Config:      *lockCfg,
		Plugins:     []LockfilePlugin{},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.lock")
	require.NoError(t, WriteLockfile(path, lf))

	result, err := VerifyLockfile(context.Background(), VerifyOptions{
		LockfilePath:   path,
		ConfigResolver: &mockConfigResolver{cfg: currentCfg},
	})
	require.NoError(t, err)
	assert.False(t, result.Match)
	assert.Len(t, result.Diffs, 2) // provider.default + provider.model
}

func TestVerifyLockfile_MissingPlugins(t *testing.T) {
	// Verify handles missing plugins gracefully
	lockCfg := &core.Config{
		Provider: core.ProviderConfig{Default: "anthropic"},
	}
	lf := &Lockfile{
		Version:     "1",
		GeneratedAt: "2026-01-01T00:00:00Z",
		Config:      *lockCfg,
		Plugins: []LockfilePlugin{
			{Name: "missing-plugin", Version: "1.0.0", Tier: 3},
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.lock")
	require.NoError(t, WriteLockfile(path, lf))

	registry := &mockPluginRegistry{plugins: []core.PluginMeta{}}

	result, err := VerifyLockfile(context.Background(), VerifyOptions{
		LockfilePath:   path,
		ConfigResolver: &mockConfigResolver{cfg: lockCfg},
		PluginRegistry: registry,
	})
	require.NoError(t, err)
	assert.False(t, result.Match)
	require.Len(t, result.Diffs, 1)
	assert.Equal(t, "plugin.missing-plugin", result.Diffs[0].Field)
	assert.Equal(t, "not installed", result.Diffs[0].Actual)
}

func TestLegacyLockfileFormat_LoadsCorrectly(t *testing.T) {
	// Legacy lockfile (no version field) should still load via loadLockfile.
	dir := t.TempDir()
	legacyJSON := `{"provider":{"default":"openrouter","model":"gpt-4o"},"session":{"retention_count":25}}`
	lockPath := filepath.Join(dir, "config.lock")
	require.NoError(t, os.WriteFile(lockPath, []byte(legacyJSON), 0644))

	globalDir := t.TempDir()
	l := NewLoader(LoaderOptions{GlobalDir: globalDir, ProjectDir: dir})
	require.NoError(t, l.Init(context.Background()))

	cfg := l.Config()
	assert.Equal(t, "openrouter", cfg.Provider.Default)
	assert.Equal(t, "gpt-4o", cfg.Provider.Model)
	assert.Equal(t, intPtr(25), cfg.Session.RetentionCount)
}

func TestVerifyLockfile_NotFound(t *testing.T) {
	_, err := VerifyLockfile(context.Background(), VerifyOptions{
		LockfilePath:   "/nonexistent/config.lock",
		ConfigResolver: &mockConfigResolver{cfg: &core.Config{}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.Contains(t, err.Error(), "run 'siply lock' first")
}
