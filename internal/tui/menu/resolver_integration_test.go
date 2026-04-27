// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package menu

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/config"
	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/tui"
)

func TestIntegration_PluginLoadsKeybindingsAppearInResolver(t *testing.T) {
	plugins := []core.Keybinding{
		{Key: "ctrl+shift+t", Description: "Focus tree panel", PluginName: "tree-view"},
		{Key: "ctrl+shift+r", Description: "Refresh tree", PluginName: "tree-view"},
	}
	r := NewKeybindingResolver(DefaultKeyBindings(), plugins, nil, nil)
	resolved := r.Resolve()

	var found int
	for _, rb := range resolved {
		if rb.Source == "plugin:tree-view" {
			found++
		}
	}
	assert.Equal(t, 2, found, "both plugin keybindings should appear")
}

func TestIntegration_GlobalOverrideShowsInResolver(t *testing.T) {
	globalCfg := &config.KeybindingConfig{
		Keybindings: []config.KeybindingEntry{
			{Key: "ctrl+t", Action: "my-toggle"},
		},
	}
	r := NewKeybindingResolver(DefaultKeyBindings(), nil, globalCfg, nil)
	resolved := r.Resolve()

	for _, rb := range resolved {
		if rb.Key == "ctrl+t" {
			assert.Equal(t, "global", rb.Source)
			assert.Equal(t, "system", rb.OverrideOf)
			return
		}
	}
	t.Fatal("ctrl+t should be in resolved bindings")
}

func TestIntegration_ProjectOverridesGlobal(t *testing.T) {
	globalCfg := &config.KeybindingConfig{
		Keybindings: []config.KeybindingEntry{
			{Key: "ctrl+shift+f", Action: "search-files"},
		},
	}
	projectCfg := &config.KeybindingConfig{
		Keybindings: []config.KeybindingEntry{
			{Key: "ctrl+shift+f", Action: "search-project"},
		},
	}
	r := NewKeybindingResolver(DefaultKeyBindings(), nil, globalCfg, projectCfg)
	resolved := r.Resolve()

	for _, rb := range resolved {
		if rb.Key == "ctrl+shift+f" {
			assert.Equal(t, "project", rb.Source)
			assert.Equal(t, "global", rb.OverrideOf)
			return
		}
	}
	t.Fatal("ctrl+shift+f should be in resolved bindings")
}

func TestIntegration_ForceGlobalBlocksProjectWithWarning(t *testing.T) {
	globalCfg := &config.KeybindingConfig{
		Keybindings: []config.KeybindingEntry{
			{Key: "ctrl+t", Action: "forced-toggle", Force: true},
		},
	}
	projectCfg := &config.KeybindingConfig{
		Keybindings: []config.KeybindingEntry{
			{Key: "ctrl+t", Action: "project-override"},
		},
	}
	r := NewKeybindingResolver(DefaultKeyBindings(), nil, globalCfg, projectCfg)
	resolved := r.Resolve()

	for _, rb := range resolved {
		if rb.Key == "ctrl+t" {
			assert.Equal(t, "global", rb.Source, "force-global should win")
			assert.Equal(t, "forced-toggle", rb.Action)
			assert.True(t, rb.IsForced)
			return
		}
	}
	t.Fatal("ctrl+t should be in resolved bindings")
}

func TestIntegration_PluginUnregisterRemovesFromResolver(t *testing.T) {
	plugins := []core.Keybinding{
		{Key: "ctrl+shift+t", Description: "Focus tree", PluginName: "tree-view"},
	}
	r := NewKeybindingResolver(DefaultKeyBindings(), plugins, nil, nil)

	resolved1 := r.Resolve()
	var foundBefore bool
	for _, rb := range resolved1 {
		if rb.Source == "plugin:tree-view" {
			foundBefore = true
			break
		}
	}
	require.True(t, foundBefore, "should have tree-view binding before unregister")

	r.SetPlugins(nil)
	resolved2 := r.Resolve()
	for _, rb := range resolved2 {
		assert.NotEqual(t, "plugin:tree-view", rb.Source, "tree-view should be gone after unregister")
	}
}

func TestIntegration_LearnViewEndToEndPluginSections(t *testing.T) {
	theme := tui.DefaultTheme()
	rc := tui.RenderConfig{
		Color:     tui.ColorTrueColor,
		Emoji:     false,
		Borders:   tui.BorderUnicode,
		Motion:    tui.MotionStatic,
		Verbosity: tui.VerbosityFull,
	}
	lv := NewLearnView(theme, rc, &mockMarkdownRenderer{})

	plugins := []core.Keybinding{
		{Key: "ctrl+shift+t", Description: "Focus tree panel", PluginName: "tree-view"},
		{Key: "ctrl+m", Description: "Toggle preview", PluginName: "markdown-preview"},
	}
	r := NewKeybindingResolver(DefaultKeyBindings(), plugins, nil, nil)
	lv.SetResolver(r)
	lv.SetSize(80, 80)

	rendered := lv.Render(80, 80)
	stripped := ansi.Strip(rendered)

	assert.Contains(t, stripped, "Navigation")
	assert.Contains(t, stripped, "Terminal")
	assert.Contains(t, stripped, "tree-view")
	assert.Contains(t, stripped, "markdown-preview")

	termIdx := strings.Index(stripped, "Terminal")
	treeIdx := strings.Index(stripped, "tree-view")
	require.Greater(t, treeIdx, termIdx, "plugins after system categories")

	dividerMd := lv.generateMarkdown()
	assert.Contains(t, dividerMd, "---", "divider between system and plugins")
}
