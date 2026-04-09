// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package menu

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/tui"
)

// --- Helpers ---

func newTestOverlay(opts ...func(*tui.RenderConfig)) *Overlay {
	theme := tui.DefaultTheme()
	rc := tui.RenderConfig{
		Color:     tui.ColorTrueColor,
		Emoji:     false,
		Borders:   tui.BorderUnicode,
		Motion:    tui.MotionStatic,
		Verbosity: tui.VerbosityFull,
	}
	for _, fn := range opts {
		fn(&rc)
	}
	return NewOverlay(theme, rc)
}

func withNoColor(rc *tui.RenderConfig) {
	rc.Color = tui.ColorNone
	rc.Borders = tui.BorderNone
}

func withAccessible(rc *tui.RenderConfig) {
	rc.Color = tui.ColorNone
	rc.Borders = tui.BorderNone
	rc.Verbosity = tui.VerbosityAccessible
}

// --- Interface compliance ---

func TestOverlay_ImplementsMenuOverlay(t *testing.T) {
	var _ tui.MenuOverlay = (*Overlay)(nil)
}

// --- Toggle / Open / Close ---

func TestOverlay_Toggle(t *testing.T) {
	o := newTestOverlay()
	assert.False(t, o.IsOpen(), "should start closed")

	o.Toggle()
	assert.True(t, o.IsOpen(), "should open after Toggle")

	o.Toggle()
	assert.False(t, o.IsOpen(), "should close after second Toggle")
}

func TestOverlay_OpenClose(t *testing.T) {
	o := newTestOverlay()
	o.Open()
	assert.True(t, o.IsOpen())
	o.Close()
	assert.False(t, o.IsOpen())
}

func TestOverlay_IsOpen(t *testing.T) {
	o := newTestOverlay()
	assert.False(t, o.IsOpen())
	o.open = true
	assert.True(t, o.IsOpen())
	o.open = false
	assert.False(t, o.IsOpen())
}

// --- Menu items ---

func TestOverlay_MenuContains11ItemsInOrder(t *testing.T) {
	expected := []string{
		"Workspaces", "Extensions", "Marketplace", "Learn", "Triggers",
		"Theme", "Settings", "Install", "Remove", "Update", "About",
	}
	items := menuItems()
	require.Len(t, items, 11)

	for i, exp := range expected {
		mi := items[i].(menuItem)
		assert.Equal(t, exp, mi.Title(), "item %d should be %s", i, exp)
		assert.NotEmpty(t, mi.Description(), "item %d should have description", i)
		assert.Equal(t, exp, mi.FilterValue(), "FilterValue should match title")
	}
}

// --- Arrow key navigation ---

func TestOverlay_ArrowNavigation(t *testing.T) {
	o := newTestOverlay()
	o.Open()

	// Initially at index 0.
	item := o.list.SelectedItem().(menuItem)
	assert.Equal(t, "Workspaces", item.Title())

	// Move down.
	o.HandleKey("down")
	item = o.list.SelectedItem().(menuItem)
	assert.Equal(t, "Extensions", item.Title())

	// Move down again.
	o.HandleKey("down")
	item = o.list.SelectedItem().(menuItem)
	assert.Equal(t, "Marketplace", item.Title())

	// Move up.
	o.HandleKey("up")
	item = o.list.SelectedItem().(menuItem)
	assert.Equal(t, "Extensions", item.Title())
}

func TestOverlay_JKNavigation(t *testing.T) {
	o := newTestOverlay()
	o.Open()

	o.HandleKey("j") // down
	item := o.list.SelectedItem().(menuItem)
	assert.Equal(t, "Extensions", item.Title())

	o.HandleKey("k") // up
	item = o.list.SelectedItem().(menuItem)
	assert.Equal(t, "Workspaces", item.Title())
}

// --- Enter returns MenuItemSelectedMsg ---

func TestOverlay_EnterReturnsMenuItemSelectedMsg(t *testing.T) {
	o := newTestOverlay()
	o.Open()

	// Select first item (Workspaces).
	result := o.HandleKey("enter")
	require.NotNil(t, result)
	msg, ok := result.(tui.MenuItemSelectedMsg)
	require.True(t, ok, "expected MenuItemSelectedMsg, got %T", result)
	assert.Equal(t, "Workspaces", msg.Label)

	// Navigate to third item and select.
	o.HandleKey("down")
	o.HandleKey("down")
	result = o.HandleKey("enter")
	require.NotNil(t, result)
	msg = result.(tui.MenuItemSelectedMsg)
	assert.Equal(t, "Marketplace", msg.Label)
}

// --- Esc closes menu ---

func TestOverlay_EscClosesMenu(t *testing.T) {
	o := newTestOverlay()
	o.Open()
	assert.True(t, o.IsOpen())

	result := o.HandleKey("esc")
	assert.Nil(t, result)
	assert.False(t, o.IsOpen())
}

// --- No-color mode ---

func TestOverlay_NoColorMode_ZeroColorCodes(t *testing.T) {
	o := newTestOverlay(withNoColor)
	o.Open()
	o.SetSize(60, 20)

	rendered := o.Render(60, 20)

	// AC #9: zero ANSI color codes; structural formatting (Bold \x1b[1m,
	// Faint \x1b[2m, Reverse \x1b[7m, Reset \x1b[m) is allowed.
	// Check for foreground/background color sequences (30-37, 38;5;, 38;2;, 40-47, 48;5;, 48;2;).
	assert.NotContains(t, rendered, "\x1b[3", "no foreground color codes (30-37, 38;...) expected")
	assert.NotContains(t, rendered, "\x1b[4", "no background color codes (40-47, 48;...) expected")
}

func TestOverlay_NoColorMode_SelectedUsesReverse(t *testing.T) {
	o := newTestOverlay(withNoColor)
	o.Open()
	o.SetSize(60, 20)

	rendered := o.Render(60, 20)
	// In no-color mode with Reverse, the stripped output should show "> Workspaces".
	stripped := ansi.Strip(rendered)
	assert.Contains(t, stripped, "> Workspaces")
}

// --- Accessible mode ---

func TestOverlay_AccessibleMode_MenuHeader(t *testing.T) {
	o := newTestOverlay(withAccessible)
	o.Open()
	o.SetSize(60, 20)

	rendered := o.Render(60, 20)
	assert.Contains(t, rendered, "[MENU]")
}

func TestOverlay_AccessibleMode_NumberedItems(t *testing.T) {
	o := newTestOverlay(withAccessible)
	o.Open()
	o.SetSize(60, 25)

	rendered := o.Render(60, 25)
	// First item should be selected with "> [1]".
	assert.Contains(t, rendered, "> [1] Workspaces")
	// Other items should be numbered.
	assert.Contains(t, rendered, "[2] Extensions")
	assert.Contains(t, rendered, "[3] Marketplace")
}

// --- SetSize clamps ---

func TestOverlay_SetSize_ClampsToMinimum(t *testing.T) {
	o := newTestOverlay()

	o.SetSize(5, 2)
	assert.Equal(t, 20, o.width, "width should clamp to minimum 20")
	assert.Equal(t, 5, o.height, "height should clamp to minimum 5")

	o.SetSize(100, 50)
	assert.Equal(t, 100, o.width)
	assert.Equal(t, 50, o.height)
}

// --- Render fits within width ---

func TestOverlay_RenderFitsWithinWidth(t *testing.T) {
	tests := []struct {
		name  string
		width int
		opts  []func(*tui.RenderConfig)
	}{
		{"standard-80", 80, nil},
		{"standard-40", 40, nil},
		{"nocolor-60", 60, []func(*tui.RenderConfig){withNoColor}},
		{"accessible-60", 60, []func(*tui.RenderConfig){withAccessible}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := newTestOverlay(tt.opts...)
			o.Open()
			o.SetSize(tt.width, 20)

			rendered := o.Render(tt.width, 20)
			for i, line := range strings.Split(rendered, "\n") {
				lineWidth := ansi.StringWidth(line)
				assert.LessOrEqual(t, lineWidth, tt.width,
					"line %d exceeds width %d: visual width=%d", i, tt.width, lineWidth)
			}
		})
	}
}

// --- App integration tests ---

func TestApp_CtrlSpaceTogglesMenu(t *testing.T) {
	app := tui.NewApp(tui.Capabilities{IsTTY: true}, tui.CLIFlags{})
	overlay := newTestOverlay()
	app.SetMenuOverlay(overlay)

	// Simulate Ctrl+Space (Ctrl+@ in terminal).
	app.Update(ctrlSpaceMsg())
	assert.True(t, overlay.IsOpen(), "menu should open on Ctrl+Space")

	app.Update(ctrlSpaceMsg())
	assert.False(t, overlay.IsOpen(), "menu should close on second Ctrl+Space")
}

func TestApp_MenuOpenRoutesKeysToMenu(t *testing.T) {
	app := tui.NewApp(tui.Capabilities{IsTTY: true}, tui.CLIFlags{})
	mock := &mockSubPanel{}
	overlay := newTestOverlay()
	app.SetREPLPanel(mock)
	app.SetMenuOverlay(overlay)

	// Open menu.
	overlay.Open()

	// Press a key — should NOT reach REPL.
	mock.updateCalled = false
	app.Update(keyMsg('a'))
	assert.False(t, mock.updateCalled, "keys should NOT reach REPL when menu is open")
}

func TestApp_MenuClosedRoutesKeysToREPL(t *testing.T) {
	app := tui.NewApp(tui.Capabilities{IsTTY: true}, tui.CLIFlags{})
	mock := &mockSubPanel{}
	overlay := newTestOverlay()
	app.SetREPLPanel(mock)
	app.SetMenuOverlay(overlay)

	// Menu closed — key should reach REPL.
	app.Update(keyMsg('a'))
	assert.True(t, mock.updateCalled, "keys should reach REPL when menu is closed")
}

func TestApp_MenuItemSelectedClosesMenu(t *testing.T) {
	app := tui.NewApp(tui.Capabilities{IsTTY: true}, tui.CLIFlags{})
	overlay := newTestOverlay()
	app.SetMenuOverlay(overlay)
	overlay.Open()

	app.Update(tui.MenuItemSelectedMsg{Label: "Settings"})
	assert.False(t, overlay.IsOpen(), "menu should close after item selection")
}

func TestApp_MenuRendersInView(t *testing.T) {
	app := tui.NewApp(tui.Capabilities{
		IsTTY:      true,
		ColorDepth: tui.TrueColor,
		Unicode:    true,
	}, tui.CLIFlags{})
	mock := &mockSubPanel{viewContent: "REPL content"}
	overlay := newTestOverlay()
	app.SetREPLPanel(mock)
	app.SetMenuOverlay(overlay)

	app.Update(windowSizeMsg(80, 25))
	overlay.Open()

	view := app.View()
	// Should show menu, not REPL content.
	assert.Contains(t, view.Content, "Workspaces")
	assert.NotContains(t, view.Content, "REPL content")
}

// --- Test helpers ---

func ctrlSpaceMsg() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: '@', Mod: tea.ModCtrl}
}

func keyMsg(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r}
}

func windowSizeMsg(w, h int) tea.WindowSizeMsg {
	return tea.WindowSizeMsg{Width: w, Height: h}
}

// mockSubPanel implements tui.SubPanel for testing.
type mockSubPanel struct {
	updateCalled bool
	viewContent  string
}

func (m *mockSubPanel) Init() tea.Cmd                { return nil }
func (m *mockSubPanel) Update(msg tea.Msg) tea.Cmd   { m.updateCalled = true; return nil }
func (m *mockSubPanel) View() string                 { return m.viewContent }
func (m *mockSubPanel) SetSize(width, height int)    {}
