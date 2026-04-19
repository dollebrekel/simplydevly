// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package profiles

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"siply.dev/siply/internal/core"
)

func TestDiffConfig_DetectsChanges(t *testing.T) {
	current := &core.Config{
		Provider: core.ProviderConfig{Default: "anthropic"},
		TUI:      core.TUIConfig{Profile: "minimal"},
	}
	profile := &core.Config{
		Provider: core.ProviderConfig{Default: "openai", Model: "gpt-4"},
		TUI:      core.TUIConfig{Profile: "standard"},
	}
	changes := DiffConfig(current, profile)
	keys := make(map[string]string)
	for _, c := range changes {
		keys[c.Key] = c.NewValue
	}
	assert.Equal(t, "openai", keys["provider.default"])
	assert.Equal(t, "gpt-4", keys["provider.model"])
	assert.Equal(t, "standard", keys["tui.profile"])
}

func TestDiffConfig_EmptyWhenEqual(t *testing.T) {
	cfg := &core.Config{Provider: core.ProviderConfig{Default: "anthropic"}}
	changes := DiffConfig(cfg, cfg)
	assert.Empty(t, changes)
}

func TestDiffConfig_NilProfile(t *testing.T) {
	changes := DiffConfig(&core.Config{}, nil)
	assert.Nil(t, changes)
}

func TestDiffConfig_NilCurrentTreatedAsEmpty(t *testing.T) {
	profile := &core.Config{Provider: core.ProviderConfig{Default: "openai"}}
	changes := DiffConfig(nil, profile)
	require.Len(t, changes, 1)
	assert.Equal(t, "provider.default", changes[0].Key)
}

func TestApplyProfileConfig_MergesCorrectly(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "config.yaml")

	existing := core.Config{Provider: core.ProviderConfig{Default: "anthropic", Model: "claude-3"}}
	data, err := yaml.Marshal(existing)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(targetPath, data, 0o644))

	profile := &core.Config{Provider: core.ProviderConfig{Default: "openai"}}
	require.NoError(t, ApplyProfileConfig(profile, targetPath))

	result, err := os.ReadFile(targetPath)
	require.NoError(t, err)
	var merged core.Config
	require.NoError(t, yaml.Unmarshal(result, &merged))

	assert.Equal(t, "openai", merged.Provider.Default)
	assert.Equal(t, "claude-3", merged.Provider.Model) // preserved
}

func TestApplyProfileConfig_CreatesFileIfMissing(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "config.yaml")

	profile := &core.Config{TUI: core.TUIConfig{Profile: "standard"}}
	require.NoError(t, ApplyProfileConfig(profile, targetPath))

	_, err := os.Stat(targetPath)
	require.NoError(t, err)
}

func TestApplyProfileConfig_NilProfileIsNoop(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, ApplyProfileConfig(nil, targetPath))
	_, err := os.Stat(targetPath)
	assert.True(t, os.IsNotExist(err))
}

func TestTUIOnlyConfig(t *testing.T) {
	cfg := TUIOnlyConfig("minimal")
	assert.Equal(t, "minimal", cfg.TUI.Profile)
	assert.Empty(t, cfg.Provider.Default)
}
