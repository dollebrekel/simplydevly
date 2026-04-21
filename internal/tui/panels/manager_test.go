// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package panels

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/tui"
)

func testManager() *PanelManager {
	return NewPanelManager(tui.DefaultTheme(), tui.RenderConfig{})
}

func leftCfg(name string) core.PanelConfig {
	return core.PanelConfig{
		Name:        name,
		Position:    core.PanelLeft,
		MinWidth:    20,
		MaxWidth:    40,
		Collapsible: true,
		Keybind:     "ctrl+1",
	}
}

func rightCfg(name string) core.PanelConfig {
	return core.PanelConfig{
		Name:        name,
		Position:    core.PanelRight,
		MinWidth:    20,
		MaxWidth:    60,
		Collapsible: true,
		Keybind:     "ctrl+2",
	}
}

// ─── Register / Unregister ───────────────────────────────────────────────────

func TestPanelManager_Register(t *testing.T) {
	m := testManager()
	require.NoError(t, m.Register(leftCfg("tree")))

	info, ok := m.Panel("tree")
	assert.True(t, ok)
	assert.Equal(t, "tree", info.Config.Name)
}

func TestPanelManager_RegisterDuplicate(t *testing.T) {
	m := testManager()
	require.NoError(t, m.Register(leftCfg("tree")))
	assert.Error(t, m.Register(leftCfg("tree")))
}

func TestPanelManager_Unregister(t *testing.T) {
	m := testManager()
	require.NoError(t, m.Register(leftCfg("tree")))
	require.NoError(t, m.Unregister("tree"))

	_, ok := m.Panel("tree")
	assert.False(t, ok)
}

func TestPanelManager_UnregisterMissing(t *testing.T) {
	m := testManager()
	assert.Error(t, m.Unregister("nope"))
}

// ─── Activate / Deactivate ───────────────────────────────────────────────────

func TestPanelManager_ActivateDeactivate(t *testing.T) {
	m := testManager()
	require.NoError(t, m.Register(leftCfg("tree")))

	require.NoError(t, m.Activate("tree"))
	info, _ := m.Panel("tree")
	assert.True(t, info.Active)

	require.NoError(t, m.Deactivate("tree"))
	info, _ = m.Panel("tree")
	assert.False(t, info.Active)
}

// ─── LazyInit ────────────────────────────────────────────────────────────────

func TestPanelManager_LazyInit_CalledOnce(t *testing.T) {
	m := testManager()
	callCount := 0
	cfg := core.PanelConfig{
		Name:       "lazy",
		Position:   core.PanelLeft,
		MinWidth:   20,
		MaxWidth:   40,
		LazyInit:   true,
		OnActivate: func() error { callCount++; return nil },
	}
	require.NoError(t, m.Register(cfg))

	require.NoError(t, m.Activate("lazy"))
	assert.Equal(t, 1, callCount)

	// Deactivate and re-activate: OnActivate must NOT be called again.
	require.NoError(t, m.Deactivate("lazy"))
	require.NoError(t, m.Activate("lazy"))
	assert.Equal(t, 1, callCount)
}

func TestPanelManager_NoLazyInit_CalledEveryActivate(t *testing.T) {
	m := testManager()
	callCount := 0
	cfg := core.PanelConfig{
		Name:       "eager",
		Position:   core.PanelLeft,
		MinWidth:   20,
		MaxWidth:   40,
		LazyInit:   false,
		OnActivate: func() error { callCount++; return nil },
	}
	require.NoError(t, m.Register(cfg))

	require.NoError(t, m.Activate("eager"))
	assert.Equal(t, 1, callCount)

	require.NoError(t, m.Deactivate("eager"))
	require.NoError(t, m.Activate("eager"))
	assert.Equal(t, 2, callCount)
}

// ─── Focus cycling ───────────────────────────────────────────────────────────

func TestPanelManager_FocusCycling(t *testing.T) {
	m := testManager()
	// Register panels in all positions so focus can cycle through them.
	require.NoError(t, m.Register(leftCfg("tree")))
	require.NoError(t, m.Register(rightCfg("console")))
	require.NoError(t, m.Register(core.PanelConfig{Name: "logs", Position: core.PanelBottom, MinWidth: 10}))
	assert.Equal(t, "repl", m.focus)

	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	assert.Equal(t, "left", m.focus)

	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	assert.Equal(t, "right", m.focus)

	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	assert.Equal(t, "bottom", m.focus)

	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	assert.Equal(t, "repl", m.focus)
}

func TestPanelManager_FocusCycling_SkipsEmptySlots(t *testing.T) {
	m := testManager()
	// Only register a left panel — right and bottom are empty.
	require.NoError(t, m.Register(leftCfg("tree")))
	assert.Equal(t, "repl", m.focus)

	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	assert.Equal(t, "left", m.focus)

	// Next tab should skip right and bottom (empty), back to repl.
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	assert.Equal(t, "repl", m.focus)
}

func TestPanelManager_FocusReverse(t *testing.T) {
	m := testManager()
	require.NoError(t, m.Register(leftCfg("tree")))
	require.NoError(t, m.Register(rightCfg("console")))
	require.NoError(t, m.Register(core.PanelConfig{Name: "logs", Position: core.PanelBottom, MinWidth: 10}))

	m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	assert.Equal(t, "bottom", m.focus)
}

// ─── Tab switching ───────────────────────────────────────────────────────────

func TestPanelManager_TabSwitching(t *testing.T) {
	m := testManager()
	require.NoError(t, m.Register(core.PanelConfig{Name: "A", Position: core.PanelLeft, MinWidth: 10}))
	require.NoError(t, m.Register(core.PanelConfig{Name: "B", Position: core.PanelLeft, MinWidth: 10}))

	// Switch focus to left.
	m.focus = "left"
	assert.Equal(t, 0, m.left.activeTab)

	m.Update(tea.KeyPressMsg{Code: 0x5D, Mod: tea.ModCtrl}) // ctrl+]
	assert.Equal(t, 1, m.left.activeTab)

	m.Update(tea.KeyPressMsg{Code: 0x5D, Mod: tea.ModCtrl})
	assert.Equal(t, 0, m.left.activeTab) // wraps
}

// ─── Collapse ────────────────────────────────────────────────────────────────

func TestPanelManager_Collapse_ByKeybind(t *testing.T) {
	m := testManager()
	require.NoError(t, m.Register(leftCfg("tree")))

	assert.False(t, m.left.collapsed)

	// Send the left panel's keybind (ctrl+1).
	m.Update(tea.KeyPressMsg{Code: '1', Mod: tea.ModCtrl})
	assert.True(t, m.left.collapsed)

	m.Update(tea.KeyPressMsg{Code: '1', Mod: tea.ModCtrl})
	assert.False(t, m.left.collapsed)
}

// ─── Resize ──────────────────────────────────────────────────────────────────

func TestPanelManager_Resize_AltArrow(t *testing.T) {
	m := testManager()
	require.NoError(t, m.Register(leftCfg("tree")))
	m.left.width = 20
	m.focus = "left"

	m.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModAlt})
	assert.Equal(t, 22, m.left.width)

	m.Update(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModAlt})
	assert.Equal(t, 20, m.left.width)
}

func TestPanelManager_Resize_RespectsMinMax(t *testing.T) {
	m := testManager()
	require.NoError(t, m.Register(leftCfg("tree")))
	m.left.width = 20
	m.focus = "left"

	// Shrink below MinWidth (20).
	for i := 0; i < 20; i++ {
		m.Update(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModAlt})
	}
	assert.Equal(t, 20, m.left.width) // clamped to MinWidth

	// Expand beyond MaxWidth (40).
	for i := 0; i < 20; i++ {
		m.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModAlt})
	}
	assert.Equal(t, 40, m.left.width) // clamped to MaxWidth
}

// ─── Auto-collapse on narrow terminal ────────────────────────────────────────

func TestPanelManager_AutoCollapse_NarrowTerminal(t *testing.T) {
	m := testManager()
	require.NoError(t, m.Register(rightCfg("console")))
	require.NoError(t, m.Register(leftCfg("tree")))
	m.left.width = 20
	m.right.width = 20

	// 50 cols: 20 left + 20 right = 10 center < 40 → collapse right first.
	m.Update(tea.WindowSizeMsg{Width: 50, Height: 30})
	assert.True(t, m.right.collapsed)
	assert.False(t, m.left.collapsed)
}

// ─── Panels listing ──────────────────────────────────────────────────────────

func TestPanelManager_Panels(t *testing.T) {
	m := testManager()
	require.NoError(t, m.Register(leftCfg("tree")))
	require.NoError(t, m.Register(rightCfg("console")))

	all := m.Panels()
	assert.Len(t, all, 2)
}

// ─── Layout persistence round-trip ───────────────────────────────────────────

func TestPanelManager_SaveRestoreLayout(t *testing.T) {
	m := testManager()
	require.NoError(t, m.Register(leftCfg("tree")))
	require.NoError(t, m.Activate("tree"))
	m.left.width = 30
	m.focus = "left"

	saved := m.SaveLayout()
	assert.Equal(t, "left", saved.Focus)
	assert.Equal(t, 30, saved.Panels["tree"].Width)
	assert.True(t, saved.Panels["tree"].Active)

	// Create a new manager and restore.
	m2 := testManager()
	require.NoError(t, m2.Register(leftCfg("tree")))
	m2.RestoreLayout(saved)

	assert.Equal(t, "left", m2.focus)
	assert.Equal(t, 30, m2.left.width)

	info, _ := m2.Panel("tree")
	assert.True(t, info.Active)
}

func TestPanelManager_SaveLoadConfig(t *testing.T) {
	// Redirect HOME to a temp dir.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, ".siply"), 0o700))

	m := testManager()
	require.NoError(t, m.Register(leftCfg("tree")))
	require.NoError(t, m.Activate("tree"))
	m.left.width = 35
	m.focus = "right"

	require.NoError(t, m.SaveLayoutToConfig())

	loaded, err := LoadLayoutFromConfig()
	require.NoError(t, err)
	assert.Equal(t, "right", loaded.Focus)
	assert.Equal(t, 35, loaded.Panels["tree"].Width)
}

// ─── View / LeftPanelWidth / RightPanelWidth ─────────────────────────────────

func TestPanelManager_PanelWidths(t *testing.T) {
	m := testManager()
	require.NoError(t, m.Register(leftCfg("tree")))
	require.NoError(t, m.Register(rightCfg("console")))
	m.left.width = 25
	m.right.width = 30

	assert.Equal(t, 25, m.LeftPanelWidth())
	assert.Equal(t, 30, m.RightPanelWidth())

	m.left.collapsed = true
	assert.Equal(t, 0, m.LeftPanelWidth())
}

func TestPanelManager_View_ReturnsString(t *testing.T) {
	m := testManager()
	require.NoError(t, m.Register(leftCfg("tree")))
	m.left.width = 20

	view := m.View(120, 30)
	assert.NotEmpty(t, view)
}
