// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package menu

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/config"
	"siply.dev/siply/internal/core"
)

func TestResolver_SystemOnlyReturnsDefaults(t *testing.T) {
	r := NewKeybindingResolver(DefaultKeyBindings(), nil, nil, nil)
	resolved := r.Resolve()
	require.NotEmpty(t, resolved)

	for _, rb := range resolved {
		assert.Equal(t, "system", rb.Source)
		assert.Empty(t, rb.OverrideOf)
	}
}

func TestResolver_PluginAddsNewCategory(t *testing.T) {
	plugins := []core.Keybinding{
		{Key: "ctrl+shift+t", Description: "Focus tree panel", PluginName: "tree-view"},
		{Key: "ctrl+shift+r", Description: "Refresh tree", PluginName: "tree-view"},
	}
	r := NewKeybindingResolver(DefaultKeyBindings(), plugins, nil, nil)
	resolved := r.Resolve()

	var pluginBindings []ResolvedKeybinding
	for _, rb := range resolved {
		if rb.Source == "plugin:tree-view" {
			pluginBindings = append(pluginBindings, rb)
		}
	}
	require.Len(t, pluginBindings, 2)
	assert.Equal(t, "tree-view", pluginBindings[0].PluginName)
}

func TestResolver_GlobalOverridesSystemBinding(t *testing.T) {
	globalCfg := &config.KeybindingConfig{
		Keybindings: []config.KeybindingEntry{
			{Key: "ctrl+t", Action: "my-custom-toggle"},
		},
	}
	r := NewKeybindingResolver(DefaultKeyBindings(), nil, globalCfg, nil)
	resolved := r.Resolve()

	var found bool
	for _, rb := range resolved {
		if rb.Key == "ctrl+t" {
			found = true
			assert.Equal(t, "global", rb.Source)
			assert.Equal(t, "my-custom-toggle", rb.Action)
			assert.Equal(t, "system", rb.OverrideOf)
			break
		}
	}
	require.True(t, found, "should find ctrl+t binding")
}

func TestResolver_ProjectOverridesGlobalBinding(t *testing.T) {
	globalCfg := &config.KeybindingConfig{
		Keybindings: []config.KeybindingEntry{
			{Key: "ctrl+shift+f", Action: "search-files"},
		},
	}
	projectCfg := &config.KeybindingConfig{
		Keybindings: []config.KeybindingEntry{
			{Key: "ctrl+shift+f", Action: "search-project-symbols"},
		},
	}
	r := NewKeybindingResolver(DefaultKeyBindings(), nil, globalCfg, projectCfg)
	resolved := r.Resolve()

	var found bool
	for _, rb := range resolved {
		if rb.Key == "ctrl+shift+f" {
			found = true
			assert.Equal(t, "project", rb.Source)
			assert.Equal(t, "search-project-symbols", rb.Action)
			assert.Equal(t, "global", rb.OverrideOf)
			break
		}
	}
	require.True(t, found, "should find ctrl+shift+f binding")
}

func TestResolver_ForceGlobalBlocksProjectOverride(t *testing.T) {
	globalCfg := &config.KeybindingConfig{
		Keybindings: []config.KeybindingEntry{
			{Key: "ctrl+t", Action: "toggle-tree-panel", Force: true},
		},
	}
	projectCfg := &config.KeybindingConfig{
		Keybindings: []config.KeybindingEntry{
			{Key: "ctrl+t", Action: "something-else"},
		},
	}
	r := NewKeybindingResolver(DefaultKeyBindings(), nil, globalCfg, projectCfg)
	resolved := r.Resolve()

	var found bool
	for _, rb := range resolved {
		if rb.Key == "ctrl+t" {
			found = true
			assert.Equal(t, "global", rb.Source)
			assert.Equal(t, "toggle-tree-panel", rb.Action)
			assert.True(t, rb.IsForced)
			break
		}
	}
	require.True(t, found, "should find ctrl+t binding with global force")
}

func TestResolver_MultiplePluginsCreateSeparateCategories(t *testing.T) {
	plugins := []core.Keybinding{
		{Key: "ctrl+shift+t", Description: "Focus tree", PluginName: "tree-view"},
		{Key: "ctrl+m", Description: "Toggle preview", PluginName: "markdown-preview"},
	}
	r := NewKeybindingResolver(DefaultKeyBindings(), plugins, nil, nil)
	cats := r.ResolveToCategories()

	var pluginCats []string
	systemDone := false
	for _, cat := range cats {
		if !systemDone {
			if cat.Name == "tree-view" || cat.Name == "markdown-preview" {
				systemDone = true
			} else {
				continue
			}
		}
		pluginCats = append(pluginCats, cat.Name)
	}
	assert.Contains(t, pluginCats, "tree-view")
	assert.Contains(t, pluginCats, "markdown-preview")
}

func TestResolver_OverrideIndicatorAppears(t *testing.T) {
	globalCfg := &config.KeybindingConfig{
		Keybindings: []config.KeybindingEntry{
			{Key: "ctrl+t", Action: "my-toggle"},
		},
	}
	r := NewKeybindingResolver(DefaultKeyBindings(), nil, globalCfg, nil)
	cats := r.ResolveToCategories()

	var found bool
	for _, cat := range cats {
		for _, kb := range cat.Bindings {
			if kb.Key == "Ctrl+T" {
				found = true
				assert.Contains(t, kb.Action, "⚙")
				break
			}
		}
	}
	require.True(t, found, "should find overridden Ctrl+T binding with indicator")
}

func TestResolver_ResolveToCategories_SystemFirst(t *testing.T) {
	plugins := []core.Keybinding{
		{Key: "ctrl+shift+t", Description: "Focus tree", PluginName: "tree-view"},
	}
	r := NewKeybindingResolver(DefaultKeyBindings(), plugins, nil, nil)
	cats := r.ResolveToCategories()

	systemNames := []string{"Navigation", "AI Agent", "Extensions", "Git", "Terminal"}
	for i, name := range systemNames {
		require.Greater(t, len(cats), i)
		assert.Equal(t, name, cats[i].Name)
	}
	require.Greater(t, len(cats), 5)
	assert.Equal(t, "tree-view", cats[5].Name)
}
