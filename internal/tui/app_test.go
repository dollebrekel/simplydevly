// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestApp_WithREPLPanel_QKey_RoutesToPanel(t *testing.T) {
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{})
	mock := &mockSubPanel{}
	app.SetREPLPanel(mock)

	_, _ = app.Update(tea.KeyPressMsg{Code: 'q'})
	assert.True(t, mock.updateCalled, "q key should be routed to REPL panel, not intercepted by App")
}
