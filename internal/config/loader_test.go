// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/core"
)

func boolPtr(b bool) *bool { return &b }
func intPtr(i int) *int    { return &i }

func TestLoadYAML_GlobalOnly(t *testing.T) {
	// AC#1: loader reads ~/.siply/config.yaml as global config
	dir := t.TempDir()
	copyFixture(t, "testdata/valid_global.yaml", filepath.Join(dir, "config.yaml"))

	l := NewLoader(LoaderOptions{GlobalDir: dir, ProjectDir: t.TempDir()})
	require.NoError(t, l.Init(context.Background()))

	cfg := l.Config()
	require.NotNil(t, cfg)
	assert.Equal(t, "anthropic", cfg.Provider.Default)
	assert.Equal(t, "claude-opus", cfg.Provider.Model)
	assert.Equal(t, intPtr(50), cfg.Session.RetentionCount)
	assert.Equal(t, boolPtr(false), cfg.Telemetry.Enabled)
}

func TestLoadYAML_ProjectOverridesGlobal(t *testing.T) {
	// AC#2, AC#5: project overrides global, keys not removed
	globalDir := t.TempDir()
	projectDir := t.TempDir()
	copyFixture(t, "testdata/valid_global.yaml", filepath.Join(globalDir, "config.yaml"))
	copyFixture(t, "testdata/valid_project.yaml", filepath.Join(projectDir, "config.yaml"))

	l := NewLoader(LoaderOptions{GlobalDir: globalDir, ProjectDir: projectDir})
	require.NoError(t, l.Init(context.Background()))

	cfg := l.Config()
	// Project values override global.
	assert.Equal(t, "openai", cfg.Provider.Default)
	assert.Equal(t, "gpt-4o", cfg.Provider.Model)
	assert.Equal(t, intPtr(100), cfg.Session.RetentionCount)
	// Project set routing.
	assert.Equal(t, boolPtr(true), cfg.Routing.Enabled)
	assert.Equal(t, "openai", cfg.Routing.DefaultProvider)
	// Global telemetry is preserved (not removed by project).
	assert.Equal(t, boolPtr(false), cfg.Telemetry.Enabled)
}

func TestLoadLockfileOverridesBoth(t *testing.T) {
	// AC#3: lockfile overrides both global and project
	globalDir := t.TempDir()
	projectDir := t.TempDir()
	copyFixture(t, "testdata/valid_global.yaml", filepath.Join(globalDir, "config.yaml"))
	copyFixture(t, "testdata/valid_project.yaml", filepath.Join(projectDir, "config.yaml"))
	copyFixture(t, "testdata/valid_lockfile.json", filepath.Join(projectDir, "config.lock"))

	l := NewLoader(LoaderOptions{GlobalDir: globalDir, ProjectDir: projectDir})
	require.NoError(t, l.Init(context.Background()))

	cfg := l.Config()
	// Lockfile overrides provider to openrouter.
	assert.Equal(t, "openrouter", cfg.Provider.Default)
	assert.Equal(t, "anthropic/claude-opus", cfg.Provider.Model)
	assert.Equal(t, intPtr(25), cfg.Session.RetentionCount)
	// Routing from project, partially overridden by lockfile.
	assert.Equal(t, boolPtr(true), cfg.Routing.Enabled)
	assert.Equal(t, "openrouter", cfg.Routing.DefaultProvider)
	// PreprocessProvider from project not overridden by lockfile (zero-value in lockfile).
	assert.Equal(t, "ollama", cfg.Routing.PreprocessProvider)
}

func TestMergeOrder_FullThreeLayers(t *testing.T) {
	// AC#4: merge order global → project → lockfile → runtime
	globalDir := t.TempDir()
	projectDir := t.TempDir()
	copyFixture(t, "testdata/valid_global.yaml", filepath.Join(globalDir, "config.yaml"))
	copyFixture(t, "testdata/valid_project.yaml", filepath.Join(projectDir, "config.yaml"))
	copyFixture(t, "testdata/valid_lockfile.json", filepath.Join(projectDir, "config.lock"))

	overrides := &core.Config{
		Provider: core.ProviderConfig{Default: "ollama"},
	}
	l := NewLoader(LoaderOptions{
		GlobalDir:  globalDir,
		ProjectDir: projectDir,
		Overrides:  overrides,
	})
	require.NoError(t, l.Init(context.Background()))

	cfg := l.Config()
	// Runtime overrides beat everything.
	assert.Equal(t, "ollama", cfg.Provider.Default)
	// Model from lockfile (not overridden by runtime).
	assert.Equal(t, "anthropic/claude-opus", cfg.Provider.Model)
}

func TestUnknownFields_SilentlyIgnored(t *testing.T) {
	// B6: unknown fields in config.yaml are silently ignored for forward compatibility.
	dir := t.TempDir()
	copyFixture(t, "testdata/invalid_unknown_field.yaml", filepath.Join(dir, "config.yaml"))

	l := NewLoader(LoaderOptions{GlobalDir: dir, ProjectDir: t.TempDir()})
	err := l.Init(context.Background())
	require.NoError(t, err, "unknown fields should be silently ignored")

	cfg := l.Config()
	assert.Equal(t, "anthropic", cfg.Provider.Default)
	assert.Equal(t, "claude-opus", cfg.Provider.Model)
}

func TestFileSizeLimit_Rejects(t *testing.T) {
	// AC#7: max 1MB per file
	dir := t.TempDir()
	bigFile := filepath.Join(dir, "config.yaml")
	// Write a file > 1MB.
	data := []byte("provider:\n  default: " + strings.Repeat("x", 1<<20) + "\n")
	require.NoError(t, os.WriteFile(bigFile, data, 0644))

	l := NewLoader(LoaderOptions{GlobalDir: dir, ProjectDir: t.TempDir()})
	err := l.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds 1MB limit")
}

func TestActionableErrorMessages(t *testing.T) {
	// AC#8: actionable error messages
	dir := t.TempDir()
	badYAML := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(badYAML, []byte("provider:\n  default: [broken"), 0644))

	l := NewLoader(LoaderOptions{GlobalDir: dir, ProjectDir: t.TempDir()})
	err := l.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config:")
	assert.Contains(t, err.Error(), "check field names and value types")
}

func TestMissingGlobalFile_UsesDefaults(t *testing.T) {
	// Missing global is not an error — defaults are used.
	l := NewLoader(LoaderOptions{GlobalDir: t.TempDir(), ProjectDir: t.TempDir()})
	require.NoError(t, l.Init(context.Background()))

	cfg := l.Config()
	assert.Equal(t, "anthropic", cfg.Provider.Default)
	assert.Equal(t, intPtr(50), cfg.Session.RetentionCount)
}

func TestMissingProjectFile_GlobalOnly(t *testing.T) {
	// Missing project config is not an error — global-only is used.
	globalDir := t.TempDir()
	copyFixture(t, "testdata/valid_global.yaml", filepath.Join(globalDir, "config.yaml"))

	l := NewLoader(LoaderOptions{GlobalDir: globalDir, ProjectDir: t.TempDir()})
	require.NoError(t, l.Init(context.Background()))

	cfg := l.Config()
	assert.Equal(t, "anthropic", cfg.Provider.Default)
	assert.Equal(t, "claude-opus", cfg.Provider.Model)
}

func TestEmptyFile_NoError(t *testing.T) {
	// Empty YAML file returns empty config, not an error.
	dir := t.TempDir()
	copyFixture(t, "testdata/empty.yaml", filepath.Join(dir, "config.yaml"))

	l := NewLoader(LoaderOptions{GlobalDir: dir, ProjectDir: t.TempDir()})
	require.NoError(t, l.Init(context.Background()))
	require.NotNil(t, l.Config())
}

func TestLifecycleMethods(t *testing.T) {
	// AC: Lifecycle interface compliance
	l := NewLoader(LoaderOptions{GlobalDir: t.TempDir(), ProjectDir: t.TempDir()})
	ctx := context.Background()

	// Health fails before Init.
	require.Error(t, l.Health())

	// Init → Start → Health → Stop
	require.NoError(t, l.Init(ctx))
	require.NoError(t, l.Start(ctx))
	require.NoError(t, l.Health())
	require.NoError(t, l.Stop(ctx))
}

func TestMerge_BooleanFalseOverridesTrue(t *testing.T) {
	// Verify that an upper layer can explicitly set a boolean to false,
	// overriding a true value from a lower layer.
	base := &core.Config{
		Routing:   core.RoutingConfig{Enabled: boolPtr(true)},
		Telemetry: core.TelemetryConfig{Enabled: boolPtr(true)},
	}
	upper := &core.Config{
		Routing:   core.RoutingConfig{Enabled: boolPtr(false)},
		Telemetry: core.TelemetryConfig{Enabled: boolPtr(false)},
	}
	result := merge(base, upper)
	require.NotNil(t, result.Routing.Enabled)
	assert.False(t, *result.Routing.Enabled)
	require.NotNil(t, result.Telemetry.Enabled)
	assert.False(t, *result.Telemetry.Enabled)
}

func TestMerge_RetentionCountZeroOverridesNonZero(t *testing.T) {
	// F4: verify that an upper layer can explicitly set retention_count to 0.
	base := &core.Config{
		Session: core.SessionConfig{RetentionCount: intPtr(50)},
	}
	upper := &core.Config{
		Session: core.SessionConfig{RetentionCount: intPtr(0)},
	}
	result := merge(base, upper)
	require.NotNil(t, result.Session.RetentionCount)
	assert.Equal(t, 0, *result.Session.RetentionCount)
}

func TestMerge_PointerFieldsNotAliased(t *testing.T) {
	// F1+F2: verify that merge does not alias pointer fields between layers.
	base := &core.Config{
		Routing:   core.RoutingConfig{Enabled: boolPtr(true)},
		Telemetry: core.TelemetryConfig{Enabled: boolPtr(false)},
		Session:   core.SessionConfig{RetentionCount: intPtr(50)},
		Plugins:   map[string]any{"a": "base"},
	}
	upper := &core.Config{}
	result := merge(base, upper)

	// Mutating result should not affect base.
	*result.Routing.Enabled = false
	*result.Session.RetentionCount = 999
	result.Plugins["a"] = "mutated"

	assert.True(t, *base.Routing.Enabled)
	assert.Equal(t, 50, *base.Session.RetentionCount)
	assert.Equal(t, "base", base.Plugins["a"])
}

func TestLoadLockfile_TrailingData(t *testing.T) {
	// F8: reject lockfiles with trailing data after the JSON object.
	dir := t.TempDir()
	badJSON := filepath.Join(dir, "config.lock")
	require.NoError(t, os.WriteFile(badJSON, []byte(`{"provider":{"default":"x"}}{"extra":true}`), 0644))

	l := NewLoader(LoaderOptions{GlobalDir: t.TempDir(), ProjectDir: dir})
	err := l.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trailing data")
}

func TestPluginNamespaceIsolation(t *testing.T) {
	// Plugins are merged per-key, not leaked across namespaces.
	base := &core.Config{
		Plugins: map[string]any{
			"pluginA": map[string]any{"key": "baseA"},
			"pluginB": map[string]any{"key": "baseB"},
		},
	}
	upper := &core.Config{
		Plugins: map[string]any{
			"pluginA": map[string]any{"key": "overrideA"},
		},
	}
	result := merge(base, upper)
	// pluginA overridden.
	assert.Equal(t, map[string]any{"key": "overrideA"}, result.Plugins["pluginA"])
	// pluginB preserved from base.
	assert.Equal(t, map[string]any{"key": "baseB"}, result.Plugins["pluginB"])
}

func TestLoadLockfile_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	badJSON := filepath.Join(dir, "config.lock")
	require.NoError(t, os.WriteFile(badJSON, []byte("{broken"), 0644))

	globalDir := t.TempDir()
	l := NewLoader(LoaderOptions{GlobalDir: globalDir, ProjectDir: dir})
	err := l.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config: parsing lockfile")
}

func TestLoadLockfile_UnknownFields(t *testing.T) {
	dir := t.TempDir()
	badJSON := filepath.Join(dir, "config.lock")
	require.NoError(t, os.WriteFile(badJSON, []byte(`{"unknown_field": true}`), 0644))

	globalDir := t.TempDir()
	l := NewLoader(LoaderOptions{GlobalDir: globalDir, ProjectDir: dir})
	err := l.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config: parsing lockfile")
}

func TestSkipLockfile_IgnoresExistingLockfile(t *testing.T) {
	// Bug fix: when generating a new lockfile, the existing lockfile must NOT
	// be loaded as layer 3 — otherwise config changes are overwritten.
	globalDir := t.TempDir()
	projectDir := t.TempDir()
	copyFixture(t, "testdata/valid_global.yaml", filepath.Join(globalDir, "config.yaml"))
	copyFixture(t, "testdata/valid_project.yaml", filepath.Join(projectDir, "config.yaml"))
	copyFixture(t, "testdata/valid_lockfile.json", filepath.Join(projectDir, "config.lock"))

	// Without SkipLockfile: lockfile overrides project config.
	withLock := NewLoader(LoaderOptions{GlobalDir: globalDir, ProjectDir: projectDir})
	require.NoError(t, withLock.Init(context.Background()))
	assert.Equal(t, "openrouter", withLock.Config().Provider.Default)

	// With SkipLockfile: only global + project, no lockfile layer.
	withoutLock := NewLoader(LoaderOptions{
		GlobalDir:    globalDir,
		ProjectDir:   projectDir,
		SkipLockfile: true,
	})
	require.NoError(t, withoutLock.Init(context.Background()))
	// Project says "openai", not lockfile's "openrouter".
	assert.Equal(t, "openai", withoutLock.Config().Provider.Default)
}

func TestTUIProfile_MinimalParsesCorrectly(t *testing.T) {
	dir := t.TempDir()
	copyFixture(t, "testdata/valid_global_with_tui.yaml", filepath.Join(dir, "config.yaml"))

	l := NewLoader(LoaderOptions{GlobalDir: dir, ProjectDir: t.TempDir()})
	require.NoError(t, l.Init(context.Background()))

	cfg := l.Config()
	assert.Equal(t, "minimal", cfg.TUI.Profile)
}

func TestTUIProfile_StandardParsesCorrectly(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("tui:\n  profile: standard\n"), 0644))

	l := NewLoader(LoaderOptions{GlobalDir: dir, ProjectDir: t.TempDir()})
	require.NoError(t, l.Init(context.Background()))

	cfg := l.Config()
	assert.Equal(t, "standard", cfg.TUI.Profile)
}

func TestTUIProfile_EmptyDefaultsToEmpty(t *testing.T) {
	// When no tui section exists, profile should be empty string.
	dir := t.TempDir()
	copyFixture(t, "testdata/valid_global.yaml", filepath.Join(dir, "config.yaml"))

	l := NewLoader(LoaderOptions{GlobalDir: dir, ProjectDir: t.TempDir()})
	require.NoError(t, l.Init(context.Background()))

	cfg := l.Config()
	assert.Equal(t, "", cfg.TUI.Profile)
}

func TestMerge_TUIProfile_NonEmptyOverridesEmpty(t *testing.T) {
	base := &core.Config{}
	upper := &core.Config{TUI: core.TUIConfig{Profile: "minimal"}}
	result := merge(base, upper)
	assert.Equal(t, "minimal", result.TUI.Profile)
}

func TestMerge_TUIProfile_UpperEmptyPreservesBase(t *testing.T) {
	base := &core.Config{TUI: core.TUIConfig{Profile: "standard"}}
	upper := &core.Config{}
	result := merge(base, upper)
	assert.Equal(t, "standard", result.TUI.Profile)
}

func TestMerge_TUIProfile_NonEmptyOverridesNonEmpty(t *testing.T) {
	base := &core.Config{TUI: core.TUIConfig{Profile: "minimal"}}
	upper := &core.Config{TUI: core.TUIConfig{Profile: "standard"}}
	result := merge(base, upper)
	assert.Equal(t, "standard", result.TUI.Profile)
}

func TestConfig_DefensiveCopy(t *testing.T) {
	// B4: Verify that modifying the returned config does not mutate internal state.
	dir := t.TempDir()
	copyFixture(t, "testdata/valid_global.yaml", filepath.Join(dir, "config.yaml"))

	l := NewLoader(LoaderOptions{GlobalDir: dir, ProjectDir: t.TempDir()})
	require.NoError(t, l.Init(context.Background()))

	cfg1 := l.Config()
	require.NotNil(t, cfg1)

	// Mutate the returned copy.
	cfg1.Provider.Default = "mutated"
	if cfg1.Routing.Enabled != nil {
		*cfg1.Routing.Enabled = !*cfg1.Routing.Enabled
	}
	if cfg1.Session.RetentionCount != nil {
		*cfg1.Session.RetentionCount = 999999
	}
	if cfg1.Plugins != nil {
		cfg1.Plugins["injected"] = "evil"
	}

	// Get a fresh copy and verify internal state is unchanged.
	cfg2 := l.Config()
	assert.NotEqual(t, "mutated", cfg2.Provider.Default)
	if cfg2.Session.RetentionCount != nil {
		assert.NotEqual(t, 999999, *cfg2.Session.RetentionCount)
	}
	if cfg2.Plugins != nil {
		_, found := cfg2.Plugins["injected"]
		assert.False(t, found, "injected key should not appear in internal config")
	}
}

func TestConfig_ConcurrentInitAndConfig(t *testing.T) {
	// B7: Concurrent calls to Init and Config must not race.
	dir := t.TempDir()
	copyFixture(t, "testdata/valid_global.yaml", filepath.Join(dir, "config.yaml"))

	l := NewLoader(LoaderOptions{GlobalDir: dir, ProjectDir: t.TempDir()})

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 100 {
			_ = l.Init(context.Background())
		}
	}()
	for range 100 {
		_ = l.Config()
		_ = l.Health()
	}
	<-done
}

// copyFixture copies a testdata fixture to a destination path.
func copyFixture(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	require.NoError(t, err, "reading fixture %s", src)
	require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0755))
	require.NoError(t, os.WriteFile(dst, data, 0644))
}

func TestKeybindingsLoading_GlobalAndProject(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()
	copyFixture(t, "testdata/valid_global.yaml", filepath.Join(globalDir, "config.yaml"))

	require.NoError(t, os.WriteFile(filepath.Join(globalDir, "keybindings.yaml"), []byte(`keybindings:
  - key: "ctrl+t"
    action: "toggle-tree"
    force: true
`), 0644))

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "keybindings.yaml"), []byte(`keybindings:
  - key: "ctrl+g"
    action: "go-to-definition"
`), 0644))

	l := NewLoader(LoaderOptions{GlobalDir: globalDir, ProjectDir: projectDir})
	require.NoError(t, l.Init(context.Background()))

	gkb := l.GlobalKeybindings()
	require.NotNil(t, gkb)
	require.Len(t, gkb.Keybindings, 1)
	assert.Equal(t, "ctrl+t", gkb.Keybindings[0].Key)
	assert.True(t, gkb.Keybindings[0].Force)

	pkb := l.ProjectKeybindings()
	require.NotNil(t, pkb)
	require.Len(t, pkb.Keybindings, 1)
	assert.Equal(t, "ctrl+g", pkb.Keybindings[0].Key)
}

func TestKeybindingsLoading_MissingFilesGraceful(t *testing.T) {
	l := NewLoader(LoaderOptions{GlobalDir: t.TempDir(), ProjectDir: t.TempDir()})
	require.NoError(t, l.Init(context.Background()))

	assert.Nil(t, l.GlobalKeybindings())
	assert.Nil(t, l.ProjectKeybindings())
}

func TestKeybindingsLoading_InvalidFileWarnsButContinues(t *testing.T) {
	globalDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(globalDir, "config.yaml"), []byte("provider:\n  default: anthropic\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(globalDir, "keybindings.yaml"), []byte("keybindings:\n  - [broken"), 0644))

	l := NewLoader(LoaderOptions{GlobalDir: globalDir, ProjectDir: t.TempDir()})
	err := l.Init(context.Background())
	require.NoError(t, err, "invalid keybindings should warn but not fail init")
	assert.Nil(t, l.GlobalKeybindings(), "invalid keybindings should not be stored")
}

func TestMergeConfig_ExportedWrapper(t *testing.T) {
	base := &core.Config{
		Provider: core.ProviderConfig{Default: "anthropic", Model: "claude-3"},
	}
	upper := &core.Config{
		Provider: core.ProviderConfig{Default: "openai"},
	}
	result := MergeConfig(base, upper)
	assert.Equal(t, "openai", result.Provider.Default)
	assert.Equal(t, "claude-3", result.Provider.Model)
}
