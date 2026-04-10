// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package config_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/config"
)

// newTestLoader creates a Loader in an isolated temp directory.
func newTestLoader(t *testing.T) *config.Loader {
	t.Helper()
	opts := config.LoaderOptions{
		GlobalDir:  t.TempDir(),
		ProjectDir: t.TempDir(),
	}
	loader := config.NewLoader(opts)
	require.NoError(t, loader.Init(context.Background()))
	return loader
}

// TestMergePluginConfig_NestedMapsAreMergedRecursively verifies that nested maps
// are deep-merged rather than replaced (go-best-practices: deep-copy-before-merge).
func TestMergePluginConfig_NestedMapsAreMergedRecursively(t *testing.T) {
	loader := newTestLoader(t)
	merger := config.NewPluginConfigMerger(loader)

	base := map[string]any{
		"routing": map[string]any{
			"provider": "anthropic",
			"model":    "claude-opus",
		},
	}
	require.NoError(t, merger.MergePluginConfig("plugin-a", base))

	// Deep merge: override only the "model" key inside "routing".
	upper := map[string]any{
		"routing": map[string]any{
			"model": "claude-haiku",
		},
	}
	require.NoError(t, merger.MergePluginConfig("plugin-a", upper))

	cfg := loader.Config()
	require.NotNil(t, cfg)

	pluginCfg, ok := cfg.Plugins["plugin-a"].(map[string]any)
	require.True(t, ok, "plugin-a config should be a map")

	routing, ok := pluginCfg["routing"].(map[string]any)
	require.True(t, ok)

	// "provider" from base must be preserved.
	assert.Equal(t, "anthropic", routing["provider"], "base key 'provider' should be preserved")
	// "model" from upper must override.
	assert.Equal(t, "claude-haiku", routing["model"], "upper key 'model' should override")
}

// TestMergePluginConfig_ScalarValuesAreReplaced verifies that scalars and lists
// from upper replace those in base (no list appending).
func TestMergePluginConfig_ScalarValuesAreReplaced(t *testing.T) {
	loader := newTestLoader(t)
	merger := config.NewPluginConfigMerger(loader)

	base := map[string]any{
		"timeout": 30,
		"retries": 3,
		"tags":    []any{"a", "b"},
	}
	require.NoError(t, merger.MergePluginConfig("plugin-a", base))

	upper := map[string]any{
		"timeout": 60,
		"tags":    []any{"c"},
	}
	require.NoError(t, merger.MergePluginConfig("plugin-a", upper))

	cfg := loader.Config()
	pluginCfg, ok := cfg.Plugins["plugin-a"].(map[string]any)
	require.True(t, ok)

	// Scalar: upper replaces base.
	assert.Equal(t, 60, pluginCfg["timeout"], "scalar should be replaced by upper")
	// Scalar not in upper: base preserved.
	assert.Equal(t, 3, pluginCfg["retries"], "base scalar not in upper should be preserved")
	// List: upper replaces (no append).
	tags, ok := pluginCfg["tags"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"c"}, tags, "list should be replaced, not appended")
}

// TestMergePluginConfig_NewKeysAdded verifies that new keys from upper are added
// while base keys not present in upper are preserved.
func TestMergePluginConfig_NewKeysAdded(t *testing.T) {
	loader := newTestLoader(t)
	merger := config.NewPluginConfigMerger(loader)

	base := map[string]any{"key1": "val1"}
	require.NoError(t, merger.MergePluginConfig("plugin-a", base))

	upper := map[string]any{"key2": "val2"}
	require.NoError(t, merger.MergePluginConfig("plugin-a", upper))

	cfg := loader.Config()
	pluginCfg, ok := cfg.Plugins["plugin-a"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, "val1", pluginCfg["key1"], "base key should be preserved")
	assert.Equal(t, "val2", pluginCfg["key2"], "new key from upper should be added")
}

// TestMergePluginConfig_AliasingPrevention verifies that mutating the input map
// after MergePluginConfig does not affect the internal config state
// (go-best-practices: deep-copy-before-merge).
func TestMergePluginConfig_AliasingPrevention(t *testing.T) {
	loader := newTestLoader(t)
	merger := config.NewPluginConfigMerger(loader)

	input := map[string]any{
		"settings": map[string]any{
			"key": "original",
		},
	}
	require.NoError(t, merger.MergePluginConfig("plugin-a", input))

	// Mutate the input AFTER merging — should not affect stored config.
	inputSettings := input["settings"].(map[string]any)
	inputSettings["key"] = "mutated"

	cfg := loader.Config()
	pluginCfg, ok := cfg.Plugins["plugin-a"].(map[string]any)
	require.True(t, ok)

	settings, ok := pluginCfg["settings"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "original", settings["key"], "stored config should not be affected by external mutation")
}

// TestRemovePluginConfig_KeyDeleted verifies that RemovePluginConfig removes
// the plugin's entire namespace from Plugins.
func TestRemovePluginConfig_KeyDeleted(t *testing.T) {
	loader := newTestLoader(t)
	merger := config.NewPluginConfigMerger(loader)

	require.NoError(t, merger.MergePluginConfig("plugin-a", map[string]any{"key": "val"}))
	require.NoError(t, merger.MergePluginConfig("plugin-b", map[string]any{"key": "val"}))

	cfg := loader.Config()
	assert.Contains(t, cfg.Plugins, "plugin-a")
	assert.Contains(t, cfg.Plugins, "plugin-b")

	require.NoError(t, merger.RemovePluginConfig("plugin-a"))

	cfg = loader.Config()
	assert.NotContains(t, cfg.Plugins, "plugin-a", "plugin-a should be deleted")
	assert.Contains(t, cfg.Plugins, "plugin-b", "plugin-b should remain untouched")
}

// TestRemovePluginConfig_NonExistentIsNoOp verifies that removing a plugin
// that was never merged does not return an error.
func TestRemovePluginConfig_NonExistentIsNoOp(t *testing.T) {
	loader := newTestLoader(t)
	merger := config.NewPluginConfigMerger(loader)

	err := merger.RemovePluginConfig("does-not-exist")
	assert.NoError(t, err)
}

// TestCrossNamespaceIsolation verifies that plugin A's config cannot contain
// plugin B's keys (go-best-practices: cross-namespace-testing).
func TestCrossNamespaceIsolation(t *testing.T) {
	loader := newTestLoader(t)
	merger := config.NewPluginConfigMerger(loader)

	configA := map[string]any{"secret": "plugin-a-value"}
	configB := map[string]any{"secret": "plugin-b-value"}

	require.NoError(t, merger.MergePluginConfig("plugin-a", configA))
	require.NoError(t, merger.MergePluginConfig("plugin-b", configB))

	cfg := loader.Config()

	pluginACfg, ok := cfg.Plugins["plugin-a"].(map[string]any)
	require.True(t, ok)
	pluginBCfg, ok := cfg.Plugins["plugin-b"].(map[string]any)
	require.True(t, ok)

	// Each plugin only sees its own config.
	assert.Equal(t, "plugin-a-value", pluginACfg["secret"])
	assert.Equal(t, "plugin-b-value", pluginBCfg["secret"])

	// Plugin A's config must not contain plugin B's key (same key name, different namespace).
	// The value should be A's own value, not B's.
	assert.NotEqual(t, "plugin-b-value", pluginACfg["secret"], "plugin A should not see plugin B's value")
}

// TestMergePluginConfig_NilLoader verifies nil guard on the loader field.
func TestMergePluginConfig_NilLoader(t *testing.T) {
	merger := config.NewPluginConfigMerger(nil)

	err := merger.MergePluginConfig("plugin-a", map[string]any{"key": "val"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loader is nil")
}

// TestRemovePluginConfig_NilLoader verifies nil guard on the loader field.
func TestRemovePluginConfig_NilLoader(t *testing.T) {
	merger := config.NewPluginConfigMerger(nil)

	err := merger.RemovePluginConfig("plugin-a")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loader is nil")
}

// TestMergePluginConfig_EmptyConfig verifies that an empty plugin config is
// handled gracefully without panicking.
func TestMergePluginConfig_EmptyConfig(t *testing.T) {
	loader := newTestLoader(t)
	merger := config.NewPluginConfigMerger(loader)

	err := merger.MergePluginConfig("plugin-a", map[string]any{})
	require.NoError(t, err)

	cfg := loader.Config()
	pluginCfg, ok := cfg.Plugins["plugin-a"]
	require.True(t, ok)
	assert.Empty(t, pluginCfg)
}

// TestMergePluginConfig_DeepNestingPreservation verifies that deep nesting is
// preserved correctly during recursive merges.
func TestMergePluginConfig_DeepNestingPreservation(t *testing.T) {
	loader := newTestLoader(t)
	merger := config.NewPluginConfigMerger(loader)

	base := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"level3": map[string]any{
					"base-key":   "base-val",
					"shared-key": "base-shared",
				},
			},
		},
	}
	require.NoError(t, merger.MergePluginConfig("plugin-a", base))

	upper := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"level3": map[string]any{
					"upper-key":  "upper-val",
					"shared-key": "upper-shared",
				},
			},
		},
	}
	require.NoError(t, merger.MergePluginConfig("plugin-a", upper))

	cfg := loader.Config()
	pluginCfg, ok := cfg.Plugins["plugin-a"].(map[string]any)
	require.True(t, ok)

	l1, ok := pluginCfg["level1"].(map[string]any)
	require.True(t, ok)
	l2, ok := l1["level2"].(map[string]any)
	require.True(t, ok)
	l3, ok := l2["level3"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, "base-val", l3["base-key"], "base deep key should be preserved")
	assert.Equal(t, "upper-val", l3["upper-key"], "upper deep key should be added")
	assert.Equal(t, "upper-shared", l3["shared-key"], "shared key: upper should win")
}
