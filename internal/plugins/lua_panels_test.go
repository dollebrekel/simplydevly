// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package plugins

import (
	"context"
	"testing"

	lua "github.com/yuin/gopher-lua"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/core"
)

// mockExtensionManager captures registrations for testing.
type mockExtensionManager struct {
	panels    []core.PanelConfig
	menuItems []core.MenuItem
	keybinds  []core.Keybinding
}

func (m *mockExtensionManager) RegisterPanel(cfg core.PanelConfig) error {
	m.panels = append(m.panels, cfg)
	return nil
}

func (m *mockExtensionManager) RegisterMenuItem(item core.MenuItem) error {
	m.menuItems = append(m.menuItems, item)
	return nil
}

func (m *mockExtensionManager) RegisterKeybinding(kb core.Keybinding) error {
	m.keybinds = append(m.keybinds, kb)
	return nil
}

func (m *mockExtensionManager) Registrations(pluginName string) []core.Registration {
	return nil
}

func TestSiplyPanelCreate(t *testing.T) {
	t.Parallel()
	L := NewSandboxedState(context.Background())
	defer L.Close()

	extMgr := &mockExtensionManager{}
	plugin := &Tier2Plugin{Name: "panel-test"}
	registerSiplyAPI(L, plugin, nil, extMgr)

	err := L.DoString(`
		siply.panel.create({
			name = "my-panel",
			position = "right",
			collapsible = true,
			keybind = "ctrl+p",
			on_render = function(w, h) return "Hello from Lua" end,
			on_activate = function() end,
		})
	`)
	require.NoError(t, err)

	require.Len(t, extMgr.panels, 1)
	assert.Equal(t, "my-panel", extMgr.panels[0].Name)
	assert.Equal(t, core.PanelRight, extMgr.panels[0].Position)
	assert.True(t, extMgr.panels[0].Collapsible)
	assert.Equal(t, "ctrl+p", extMgr.panels[0].Keybind)
	assert.Equal(t, "panel-test", extMgr.panels[0].PluginName)
	assert.NotNil(t, extMgr.panels[0].ContentFunc)
	assert.NotNil(t, extMgr.panels[0].OnActivate)
}

func TestSiplyPanelContentFunc(t *testing.T) {
	t.Parallel()
	L := NewSandboxedState(context.Background())
	defer L.Close()

	extMgr := &mockExtensionManager{}
	plugin := &Tier2Plugin{Name: "content-test"}
	registerSiplyAPI(L, plugin, nil, extMgr)

	err := L.DoString(`
		siply.panel.create({
			name = "render-panel",
			position = "left",
			on_render = function(w, h) return "Width: " .. tostring(w) end,
		})
	`)
	require.NoError(t, err)

	require.Len(t, extMgr.panels, 1)
	content := extMgr.panels[0].ContentFunc()
	assert.Equal(t, "Width: 80", content)
}

func TestSiplyUITree(t *testing.T) {
	t.Parallel()
	L := NewSandboxedState(context.Background())
	defer L.Close()

	plugin := &Tier2Plugin{Name: "ui-tree-test"}
	registerSiplyAPI(L, plugin, nil, nil)

	err := L.DoString(`
		result = siply.ui.tree({
			{label = "Root", expanded = true, children = {
				{label = "Child 1"},
				{label = "Child 2"},
			}},
		})
	`)
	require.NoError(t, err)

	val := L.GetGlobal("result")
	s, ok := val.(lua.LString)
	require.True(t, ok)
	assert.Contains(t, string(s), "Root")
}

func TestSiplyUIMarkdown(t *testing.T) {
	t.Parallel()
	L := NewSandboxedState(context.Background())
	defer L.Close()

	plugin := &Tier2Plugin{Name: "ui-md-test"}
	registerSiplyAPI(L, plugin, nil, nil)

	err := L.DoString(`
		result = siply.ui.markdown("# Hello\n\nWorld")
	`)
	require.NoError(t, err)

	val := L.GetGlobal("result")
	s, ok := val.(lua.LString)
	require.True(t, ok)
	assert.Contains(t, string(s), "Hello")
}

func TestSiplyUIToast(t *testing.T) {
	t.Parallel()
	L := NewSandboxedState(context.Background())
	defer L.Close()

	plugin := &Tier2Plugin{Name: "ui-toast-test"}
	registerSiplyAPI(L, plugin, nil, nil)

	err := L.DoString(`result = siply.ui.toast("Success!", "success")`)
	require.NoError(t, err)

	val := L.GetGlobal("result")
	s, ok := val.(lua.LString)
	require.True(t, ok)
	assert.NotEmpty(t, string(s))
}

func TestSiplyMenuAdd(t *testing.T) {
	t.Parallel()
	L := NewSandboxedState(context.Background())
	defer L.Close()

	extMgr := &mockExtensionManager{}
	plugin := &Tier2Plugin{Name: "menu-test"}
	registerSiplyAPI(L, plugin, nil, extMgr)

	err := L.DoString(`
		siply.menu.add({
			label = "My Action",
			icon = "🔧",
			category = "Tools",
			action = function() end,
		})
	`)
	require.NoError(t, err)

	require.Len(t, extMgr.menuItems, 1)
	assert.Equal(t, "My Action", extMgr.menuItems[0].Label)
	assert.Equal(t, "🔧", extMgr.menuItems[0].Icon)
	assert.Equal(t, "Tools", extMgr.menuItems[0].Category)
	assert.Equal(t, "menu-test", extMgr.menuItems[0].PluginName)
}

func TestSiplyKeybindAdd(t *testing.T) {
	t.Parallel()
	L := NewSandboxedState(context.Background())
	defer L.Close()

	extMgr := &mockExtensionManager{}
	plugin := &Tier2Plugin{Name: "kb-test"}
	registerSiplyAPI(L, plugin, nil, extMgr)

	err := L.DoString(`
		siply.keybind.add({
			key = "ctrl+k",
			description = "Kill it",
			handler = function() end,
		})
	`)
	require.NoError(t, err)

	require.Len(t, extMgr.keybinds, 1)
	assert.Equal(t, "ctrl+k", extMgr.keybinds[0].Key)
	assert.Equal(t, "Kill it", extMgr.keybinds[0].Description)
	assert.Equal(t, "kb-test", extMgr.keybinds[0].PluginName)
}
