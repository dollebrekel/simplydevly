// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"siply.dev/siply/internal/core"
)

func TestNewApp(t *testing.T) {
	caps := Capabilities{
		ColorDepth: TrueColor,
		Unicode:    true,
		Emoji:      true,
		IsTTY:      true,
	}
	app := NewApp(caps, CLIFlags{})

	assert.NotNil(t, app)
	assert.Equal(t, TrueColor, app.caps.ColorDepth)
	assert.Equal(t, ColorTrueColor, app.renderConfig.Color)
	assert.True(t, app.renderConfig.Emoji)
}

func TestApp_Init(t *testing.T) {
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{})
	cmd := app.Init()
	assert.Nil(t, cmd, "Init should return nil; WindowSizeMsg is auto-sent by Bubble Tea v2")
}

func TestApp_Update_WindowSizeMsg(t *testing.T) {
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{})

	msg := tea.WindowSizeMsg{Width: 100, Height: 30}
	model, cmd := app.Update(msg)
	require.NotNil(t, model)
	assert.Nil(t, cmd)

	updated := model.(*App)
	assert.Equal(t, 100, updated.width)
	assert.Equal(t, 30, updated.height)
	assert.True(t, updated.ready)
	assert.Equal(t, Standard, updated.layout.Mode)
}

func TestApp_Update_Resize(t *testing.T) {
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{})

	// Initial size.
	model, _ := app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	updated := model.(*App)
	assert.Equal(t, Standard, updated.layout.Mode)

	// Resize to ultra-compact.
	model, _ = updated.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	updated = model.(*App)
	assert.Equal(t, UltraCompact, updated.layout.Mode)
	assert.Equal(t, 40, updated.width)
	assert.Equal(t, 10, updated.height)
}

func TestApp_View_NotReady(t *testing.T) {
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{})
	view := app.View()
	assert.Contains(t, view.Content, "Initializing...")
}

func TestApp_View_Standard(t *testing.T) {
	app := NewApp(Capabilities{
		ColorDepth: TrueColor,
		Unicode:    true,
		Emoji:      true,
		IsTTY:      true,
	}, CLIFlags{})

	app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	view := app.View()

	assert.Contains(t, view.Content, "siply")
	assert.Contains(t, view.Content, "Ready.")
	assert.Contains(t, view.Content, "standard")
	assert.True(t, view.AltScreen)
}

func TestApp_View_Accessible(t *testing.T) {
	app := NewApp(Capabilities{
		ColorDepth: TrueColor,
		Unicode:    true,
		IsTTY:      true,
	}, CLIFlags{Accessible: true})

	app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	view := app.View()

	assert.Contains(t, view.Content, "== siply ==")
	assert.NotContains(t, view.Content, "┌")
	assert.NotContains(t, view.Content, "│")
}

func TestApp_View_UltraCompact(t *testing.T) {
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{})
	app.Update(tea.WindowSizeMsg{Width: 40, Height: 30})
	view := app.View()

	assert.Contains(t, view.Content, "siply")
	assert.Contains(t, view.Content, "ultra-compact")
	// Ultra-compact has no borders.
	assert.NotContains(t, view.Content, "┌")
}

func TestApp_View_NoBordersFlag(t *testing.T) {
	app := NewApp(Capabilities{
		ColorDepth: TrueColor,
		Unicode:    true,
		IsTTY:      true,
	}, CLIFlags{NoBorders: true})

	app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	view := app.View()

	// NoBorders should suppress all border characters.
	assert.NotContains(t, view.Content, "┌")
	assert.NotContains(t, view.Content, "│")
	assert.Contains(t, view.Content, "siply")
}

func TestApp_Lifecycle_Init_Update_View_Quit(t *testing.T) {
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{})

	// Init.
	cmd := app.Init()
	assert.Nil(t, cmd)

	// Update with WindowSize.
	model, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 25})
	assert.NotNil(t, model)

	// View.
	view := model.(*App).View()
	assert.NotEmpty(t, view.Content)

	// Quit via Ctrl+C.
	_, quitCmd := model.(*App).Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	assert.NotNil(t, quitCmd, "Ctrl+C should return a quit command")
}

// mockSubPanel implements SubPanel for integration tests.
type mockSubPanel struct {
	initCalled   bool
	updateCalled bool
	viewCalled   bool
	lastMsg      tea.Msg
	width        int
	height       int
	viewContent  string
}

func (m *mockSubPanel) Init() tea.Cmd {
	m.initCalled = true
	return nil
}

func (m *mockSubPanel) Update(msg tea.Msg) tea.Cmd {
	m.updateCalled = true
	m.lastMsg = msg
	return nil
}

func (m *mockSubPanel) View() string {
	m.viewCalled = true
	return m.viewContent
}

func (m *mockSubPanel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

func (m *mockSubPanel) SetBordered(_ bool) {}

func TestApp_WithREPLPanel_Init(t *testing.T) {
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{})
	mock := &mockSubPanel{}
	app.SetREPLPanel(mock)

	app.Init()
	assert.True(t, mock.initCalled)
}

func TestApp_WithREPLPanel_WindowSizePropagatesToPanel(t *testing.T) {
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{})
	mock := &mockSubPanel{}
	app.SetREPLPanel(mock)

	app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	assert.Equal(t, 100, mock.width)
	assert.Greater(t, mock.height, 0)
}

func TestApp_WithREPLPanel_KeyRouting(t *testing.T) {
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{})
	mock := &mockSubPanel{}
	app.SetREPLPanel(mock)
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 25})

	// Non-global key should route to panel.
	app.Update(tea.KeyPressMsg{Code: 'a'})
	assert.True(t, mock.updateCalled)
}

func TestApp_WithREPLPanel_ViewRendersPanel(t *testing.T) {
	app := NewApp(Capabilities{
		IsTTY:      true,
		ColorDepth: TrueColor,
		Unicode:    true,
	}, CLIFlags{})
	mock := &mockSubPanel{viewContent: "REPL content here"}
	app.SetREPLPanel(mock)
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 25})

	view := app.View()
	assert.Contains(t, view.Content, "REPL content here")
	assert.True(t, mock.viewCalled)
}

func TestApp_WithREPLPanel_AccessibleMode(t *testing.T) {
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{Accessible: true})
	mock := &mockSubPanel{viewContent: "Accessible REPL"}
	app.SetREPLPanel(mock)
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 25})

	view := app.View()
	assert.Contains(t, view.Content, "Accessible REPL")
	assert.NotContains(t, view.Content, "┌")
}

func TestApp_WithREPLPanel_SubmitMsgEchoes(t *testing.T) {
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{})
	mock := &mockSubPanel{}
	app.SetREPLPanel(mock)

	// Simulate a SubmitMsg arriving.
	app.Update(SubmitMsg{Text: "hello"})

	// Should have called Update with AgentOutputMsg and AgentDoneMsg.
	assert.True(t, mock.updateCalled)
	// Last message should be AgentDoneMsg (second call).
	_, ok := mock.lastMsg.(AgentDoneMsg)
	assert.True(t, ok, "Last message to panel should be AgentDoneMsg")
}

// mockStatusRenderer implements StatusRenderer for integration tests.
type mockStatusRenderer struct {
	lastWidth int
	compact   bool
	rendered  string
}

func (m *mockStatusRenderer) Render(width int) string {
	m.lastWidth = width
	if m.rendered != "" {
		return m.rendered
	}
	return "STATUS_BAR_CONTENT"
}

func (m *mockStatusRenderer) SetSize(width int, compact bool) {
	m.lastWidth = width
	m.compact = compact
}

func (m *mockStatusRenderer) SetProfile(_ string) {}

func TestApp_WithStatusBar_ViewRendersStatusBar(t *testing.T) {
	app := NewApp(Capabilities{
		IsTTY:      true,
		ColorDepth: TrueColor,
		Unicode:    true,
	}, CLIFlags{})
	mock := &mockSubPanel{viewContent: "REPL content"}
	sb := &mockStatusRenderer{rendered: "model | $0.42 | default"}
	app.SetREPLPanel(mock)
	app.SetStatusBar(sb)
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	view := app.View()
	assert.Contains(t, view.Content, "REPL content")
	assert.Contains(t, view.Content, "model | $0.42 | default")
}

func TestApp_WithStatusBar_WindowSizePropagatesToStatusBar(t *testing.T) {
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{})
	sb := &mockStatusRenderer{}
	app.SetStatusBar(sb)

	app.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	assert.Equal(t, 120, sb.lastWidth)
	assert.False(t, sb.compact)

	// Compact height (15-24).
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	assert.True(t, sb.compact)
}

func TestApp_WithStatusBar_HiddenWhenHeightTooSmall(t *testing.T) {
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{})
	mock := &mockSubPanel{viewContent: "REPL"}
	sb := &mockStatusRenderer{rendered: "SHOULD_NOT_APPEAR"}
	app.SetREPLPanel(mock)
	app.SetStatusBar(sb)

	// Height < 15: status bar hidden.
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	view := app.View()
	assert.NotContains(t, view.Content, "SHOULD_NOT_APPEAR")
}

func TestApp_WithStatusBar_AccessibleMode(t *testing.T) {
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{Accessible: true})
	mock := &mockSubPanel{viewContent: "Accessible REPL"}
	sb := &mockStatusRenderer{rendered: "[MODEL: opus] [PERMISSION: default]"}
	app.SetREPLPanel(mock)
	app.SetStatusBar(sb)

	app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	view := app.View()
	assert.Contains(t, view.Content, "Accessible REPL")
	assert.Contains(t, view.Content, "[MODEL: opus] [PERMISSION: default]")
}

func TestApp_WithoutStatusBar_FallbackPlaceholder(t *testing.T) {
	app := NewApp(Capabilities{
		IsTTY:      true,
		ColorDepth: TrueColor,
		Unicode:    true,
	}, CLIFlags{})
	mock := &mockSubPanel{viewContent: "REPL"}
	app.SetREPLPanel(mock)
	// No status bar set — should use fallback.
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 25})

	view := app.View()
	assert.Contains(t, view.Content, "Ctrl+C to quit")
}

// ─── PanelManager integration tests ─────────────────────────────────────────

// mockPanelManager stubs the PanelManager interface for App integration tests.
type mockPanelManager struct {
	updateCalled bool
	viewCalled   bool
	leftW        int
	rightW       int
	viewResult   string
}

func (m *mockPanelManager) Update(_ tea.Msg) tea.Cmd { m.updateCalled = true; return nil }
func (m *mockPanelManager) View(_, _ int) string      { m.viewCalled = true; return m.viewResult }
func (m *mockPanelManager) LeftPanelWidth() int       { return m.leftW }
func (m *mockPanelManager) RightPanelWidth() int      { return m.rightW }

func TestApp_SetPanelManager_WindowSizeUsesCenter(t *testing.T) {
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{})
	mock := &mockSubPanel{}
	pm := &mockPanelManager{leftW: 25, rightW: 25}

	app.SetREPLPanel(mock)
	app.SetPanelManager(pm)

	// 120 cols wide, 25 left + 25 right = 70 center (50 after borders are ignored).
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	// REPL should NOT receive the full 120; must be at most 120-25-25 = 70.
	assert.LessOrEqual(t, mock.width, 120-25-25+5) // small tolerance for border math
	assert.Greater(t, mock.width, 0)
}

func TestApp_SetPanelManager_RoutesResizeMsg(t *testing.T) {
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{})
	pm := &mockPanelManager{}
	app.SetPanelManager(pm)

	app.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	assert.True(t, pm.updateCalled)
}

func TestApp_SetPanelManager_ViewRendersREPL(t *testing.T) {
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{})
	mock := &mockSubPanel{viewContent: "center content"}
	pm := &mockPanelManager{leftW: 0, rightW: 0}
	app.SetREPLPanel(mock)
	app.SetPanelManager(pm)

	app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	view := app.View()
	assert.Contains(t, view.Content, "center content")
}

func TestApp_NoPanelManager_BackwardCompatible(t *testing.T) {
	// Without PanelManager, App behaves as before.
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{})
	mock := &mockSubPanel{viewContent: "REPL only"}
	app.SetREPLPanel(mock)

	app.Update(tea.WindowSizeMsg{Width: 80, Height: 25})
	assert.Equal(t, 80, mock.width) // full width, no panels
	view := app.View()
	assert.Contains(t, view.Content, "REPL only")
}

func TestApp_CtrlT_NoOp(t *testing.T) {
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{})
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	model, cmd := app.Update(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	assert.NotNil(t, model)
	assert.Nil(t, cmd)
}

func TestApp_CtrlB_TogglesBorders(t *testing.T) {
	app := NewApp(Capabilities{
		IsTTY:      true,
		ColorDepth: TrueColor,
		Unicode:    true,
	}, CLIFlags{Standard: true}) // standard profile = borders on
	mock := &mockSubPanel{}
	app.SetREPLPanel(mock)
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Verify initial state: borders on (standard profile).
	assert.Equal(t, BorderUnicode, app.renderConfig.Borders)
	assert.True(t, app.layout.ShowBorders)

	// Press Ctrl+B: borders off.
	app.Update(tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl})
	assert.Equal(t, BorderNone, app.renderConfig.Borders)
	assert.False(t, app.layout.ShowBorders)

	// Press Ctrl+B again: borders on.
	app.Update(tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl})
	assert.Equal(t, BorderUnicode, app.renderConfig.Borders)
	assert.True(t, app.layout.ShowBorders)
}

func TestApp_CtrlB_ASCIIFallbackForNonUnicode(t *testing.T) {
	app := NewApp(Capabilities{
		IsTTY:   true,
		Unicode: false, // no Unicode support
	}, CLIFlags{Standard: true})
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Standard without Unicode = ASCII borders.
	assert.Equal(t, BorderASCII, app.renderConfig.Borders)

	// Toggle off.
	app.Update(tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl})
	assert.Equal(t, BorderNone, app.renderConfig.Borders)

	// Toggle on → should use ASCII (not Unicode) since caps.Unicode=false.
	app.Update(tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl})
	assert.Equal(t, BorderASCII, app.renderConfig.Borders)
}

func TestApp_MinimalProfile_NoBordersMinimalStatus(t *testing.T) {
	// AC#5: minimal profile → no borders, 2-segment statusbar, no emoji.
	app := NewApp(Capabilities{
		IsTTY:      true,
		ColorDepth: TrueColor,
		Unicode:    true,
		Emoji:      true,
	}, CLIFlags{Minimal: true})

	assert.Equal(t, "minimal", app.renderConfig.Profile)
	assert.False(t, app.renderConfig.Emoji)
	assert.Equal(t, BorderNone, app.renderConfig.Borders)
}

func TestApp_StandardProfile_BordersFullStatus(t *testing.T) {
	// AC#6: standard profile → borders visible, emoji on.
	app := NewApp(Capabilities{
		IsTTY:      true,
		ColorDepth: TrueColor,
		Unicode:    true,
		Emoji:      false,
	}, CLIFlags{Standard: true})

	assert.Equal(t, "standard", app.renderConfig.Profile)
	assert.True(t, app.renderConfig.Emoji)
	assert.Equal(t, BorderUnicode, app.renderConfig.Borders)
}

func TestApp_AccessiblePlusMinimal(t *testing.T) {
	// AC#10: accessible mode takes precedence over profile defaults.
	app := NewApp(Capabilities{
		IsTTY:      true,
		ColorDepth: TrueColor,
		Unicode:    true,
		Emoji:      true,
	}, CLIFlags{Minimal: true, Accessible: true})

	assert.Equal(t, BorderNone, app.renderConfig.Borders)
	assert.Equal(t, VerbosityAccessible, app.renderConfig.Verbosity)
	assert.Equal(t, MotionStatic, app.renderConfig.Motion)
	assert.False(t, app.renderConfig.Emoji)
}

func TestApp_StandardPlusNoBordersFlag(t *testing.T) {
	// AC#10: --no-borders overrides standard profile.
	app := NewApp(Capabilities{
		IsTTY:      true,
		ColorDepth: TrueColor,
		Unicode:    true,
		Emoji:      true,
	}, CLIFlags{Standard: true, NoBorders: true})

	assert.Equal(t, "standard", app.renderConfig.Profile)
	assert.True(t, app.renderConfig.Emoji)
	assert.Equal(t, BorderNone, app.renderConfig.Borders)
}

func TestApp_WithREPLPanel_QKey_RoutesToPanel(t *testing.T) {
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{})
	mock := &mockSubPanel{}
	app.SetREPLPanel(mock)

	_, _ = app.Update(tea.KeyPressMsg{Code: 'q'})
	assert.True(t, mock.updateCalled, "q key should be routed to REPL panel, not intercepted by App")
}

// mockActivityFeedRenderer implements ActivityFeedRenderer for message routing tests.
type mockActivityFeedRenderer struct {
	feedEntries    []FeedEntryMsg
	feedStates     []FeedStateMsg
	feedbackMsgs   []FeedbackMsg
	lastWidth      int
	lastHeight     int
}

func (m *mockActivityFeedRenderer) Render(width, height int) string {
	m.lastWidth = width
	m.lastHeight = height
	return ""
}

func (m *mockActivityFeedRenderer) SetSize(width, height int) {
	m.lastWidth = width
	m.lastHeight = height
}

func (m *mockActivityFeedRenderer) HandleFeedEntry(msg FeedEntryMsg) {
	m.feedEntries = append(m.feedEntries, msg)
}

func (m *mockActivityFeedRenderer) HandleFeedState(msg FeedStateMsg) {
	m.feedStates = append(m.feedStates, msg)
}

func (m *mockActivityFeedRenderer) HandleFeedback(msg FeedbackMsg) {
	m.feedbackMsgs = append(m.feedbackMsgs, msg)
}

func TestApp_FeedbackMsg_RoutesToActivityFeed(t *testing.T) {
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{})
	mockFeed := &mockActivityFeedRenderer{}
	app.SetActivityFeed(mockFeed)

	app.Update(FeedbackMsg{Level: LevelSuccess, Summary: "Done"})

	require.Equal(t, 1, len(mockFeed.feedbackMsgs))
	assert.Equal(t, LevelSuccess, mockFeed.feedbackMsgs[0].Level)
	assert.Equal(t, "Done", mockFeed.feedbackMsgs[0].Summary)
}

func TestApp_ProgressStartMsg_RoutesToActivityFeed(t *testing.T) {
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{})
	mockFeed := &mockActivityFeedRenderer{}
	app.SetActivityFeed(mockFeed)

	app.Update(ProgressStartMsg{Label: "Installing..."})

	require.Equal(t, 1, len(mockFeed.feedbackMsgs))
	assert.Equal(t, LevelInfo, mockFeed.feedbackMsgs[0].Level)
	assert.Contains(t, mockFeed.feedbackMsgs[0].Summary, "Installing...")
}

func TestApp_ProgressDoneMsg_RoutesToActivityFeed(t *testing.T) {
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{})
	mockFeed := &mockActivityFeedRenderer{}
	app.SetActivityFeed(mockFeed)

	app.Update(ProgressDoneMsg{Label: "Installing", Result: "ok"})

	require.Equal(t, 1, len(mockFeed.feedbackMsgs))
	assert.Equal(t, LevelSuccess, mockFeed.feedbackMsgs[0].Level)
	assert.Equal(t, "Installing: ok", mockFeed.feedbackMsgs[0].Summary)
}

func TestApp_FeedbackMsg_NilFeed_NoPanic(t *testing.T) {
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{})
	// No activity feed set — should not panic.
	model, cmd := app.Update(FeedbackMsg{Level: LevelError, Summary: "error"})
	assert.NotNil(t, model)
	assert.Nil(t, cmd)
}

func TestApp_SetExtensionManager(t *testing.T) {
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{})
	em := &mockExtensionManager{}
	app.SetExtensionManager(em)
	assert.NotNil(t, app.extensionManager)
}

func TestApp_ExtensionKeybind_Routing(t *testing.T) {
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{})
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	called := false
	em := &mockExtensionManager{
		keybindings: []core.Keybinding{
			{Key: "ctrl+e", Handler: func() error { called = true; return nil }, PluginName: "test"},
		},
	}
	app.SetExtensionManager(em)

	model, cmd := app.Update(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	assert.NotNil(t, model)
	assert.NotNil(t, cmd, "expected a tea.Cmd for async handler dispatch")
	cmd()
	assert.True(t, called, "extension keybind handler should have been called after cmd execution")
}

func TestApp_MenuChangedMsg(t *testing.T) {
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{})
	model, cmd := app.Update(MenuChangedMsg{})
	assert.NotNil(t, model)
	assert.Nil(t, cmd)
}

func TestApp_KeybindChangedMsg(t *testing.T) {
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{})
	model, cmd := app.Update(KeybindChangedMsg{})
	assert.NotNil(t, model)
	assert.Nil(t, cmd)
}

type mockExtensionManager struct {
	menuItems   []core.MenuItem
	keybindings []core.Keybinding
}

func (m *mockExtensionManager) AllMenuItems() []core.MenuItem    { return m.menuItems }
func (m *mockExtensionManager) AllKeybindings() []core.Keybinding { return m.keybindings }

