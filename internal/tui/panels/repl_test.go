// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package panels

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"siply.dev/siply/internal/tui"
)

func defaultREPL() *REPLPanel {
	return NewREPLPanel(tui.DefaultTheme(), tui.RenderConfig{
		Borders: tui.BorderUnicode,
		Color:   tui.ColorNone,
	})
}

func typeText(r *REPLPanel, text string) {
	r.textInput.SetValue(text)
	r.textInput.CursorEnd()
}

func TestNewREPLPanel(t *testing.T) {
	r := defaultREPL()

	assert.NotNil(t, r)
	assert.NotNil(t, r.panel)
	assert.Equal(t, "> ", r.textInput.Prompt)
	assert.True(t, r.textInput.Focused())
	assert.Equal(t, -1, r.historyIndex)
	assert.Empty(t, r.history)
	assert.Empty(t, r.output)
	assert.False(t, r.agentRunning)
}

func TestREPLPanel_Init(t *testing.T) {
	r := defaultREPL()
	cmd := r.Init()
	assert.NotNil(t, cmd, "Init should return blink command")
}

func TestREPLPanel_EnterSubmit(t *testing.T) {
	r := defaultREPL()
	typeText(r, "hello world")

	cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	require.NotNil(t, cmd)
	msg := cmd()
	submitMsg, ok := msg.(tui.SubmitMsg)
	assert.True(t, ok)
	assert.Equal(t, "hello world", submitMsg.Text)

	assert.Equal(t, "", r.textInput.Value())
	assert.Equal(t, []string{"hello world"}, r.history)
	assert.True(t, r.agentRunning)
}

func TestREPLPanel_EnterEmptyInput(t *testing.T) {
	r := defaultREPL()

	cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	assert.Nil(t, cmd, "Empty input should not produce a command")
	assert.Empty(t, r.history)
	assert.False(t, r.agentRunning)
}

func TestREPLPanel_EnterWhitespaceOnly(t *testing.T) {
	r := defaultREPL()
	typeText(r, "   ")

	cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, cmd, "Whitespace-only input should not submit")
}

func TestREPLPanel_CtrlC_Idle(t *testing.T) {
	r := defaultREPL()

	cmd := r.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok, "Ctrl+C when idle should quit")
}

func TestREPLPanel_CtrlC_AgentRunning(t *testing.T) {
	r := defaultREPL()
	r.agentRunning = true

	cmd := r.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(tui.CancelMsg)
	assert.True(t, ok, "Ctrl+C when agent running should send CancelMsg")
}

func TestREPLPanel_HistoryNavigation(t *testing.T) {
	r := defaultREPL()

	// Submit three commands.
	typeText(r, "first")
	r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	r.agentRunning = false

	typeText(r, "second")
	r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	r.agentRunning = false

	typeText(r, "third")
	r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	r.agentRunning = false

	assert.Equal(t, []string{"first", "second", "third"}, r.history)

	// Type something new, then browse history.
	typeText(r, "current")

	// Up -> third
	r.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, "third", r.textInput.Value())

	// Up -> second
	r.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, "second", r.textInput.Value())

	// Up -> first
	r.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, "first", r.textInput.Value())

	// Up again -> still first (at bounds)
	r.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, "first", r.textInput.Value())

	// Down -> second
	r.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, "second", r.textInput.Value())

	// Down -> third
	r.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, "third", r.textInput.Value())

	// Down -> back to current
	r.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, "current", r.textInput.Value())

	// Down again -> still current (no-op)
	r.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, "current", r.textInput.Value())
}

func TestREPLPanel_HistoryEmpty(t *testing.T) {
	r := defaultREPL()

	r.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, "", r.textInput.Value())
	r.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, "", r.textInput.Value())
}

func TestREPLPanel_HistoryNoDuplicates(t *testing.T) {
	r := defaultREPL()

	typeText(r, "same")
	r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	r.agentRunning = false

	typeText(r, "same")
	r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	r.agentRunning = false

	typeText(r, "same")
	r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	r.agentRunning = false

	assert.Equal(t, []string{"same"}, r.history, "Consecutive duplicates should not be stored")
}

func TestREPLPanel_HistoryCap(t *testing.T) {
	r := defaultREPL()

	for i := 0; i < maxHistory+10; i++ {
		r.history = append(r.history, "cmd")
	}
	typeText(r, "overflow")
	r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	assert.LessOrEqual(t, len(r.history), maxHistory)
}

func TestREPLPanel_CtrlL_ClearsOutput(t *testing.T) {
	r := defaultREPL()
	r.output = []string{"line1", "line2", "line3"}

	r.Update(tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
	assert.Empty(t, r.output, "Ctrl+L should clear output")
}

func TestREPLPanel_AgentOutputMsg(t *testing.T) {
	r := defaultREPL()

	r.Update(tui.AgentOutputMsg{Text: "response line"})

	assert.Equal(t, []string{"response line"}, r.output)
}

func TestREPLPanel_AgentDoneMsg(t *testing.T) {
	r := defaultREPL()
	r.agentRunning = true

	r.Update(tui.AgentDoneMsg{})

	assert.False(t, r.agentRunning)
}

func TestREPLPanel_View(t *testing.T) {
	r := defaultREPL()
	r.SetSize(80, 24)
	r.output = []string{"Welcome"}

	view := r.View()
	assert.Contains(t, view, "Welcome")
	assert.Contains(t, view, ">")
}

func TestREPLPanel_View_Borderless(t *testing.T) {
	r := NewREPLPanel(tui.DefaultTheme(), tui.RenderConfig{
		Borders: tui.BorderNone,
		Color:   tui.ColorNone,
	})
	r.SetSize(80, 24)

	view := r.View()
	assert.NotContains(t, view, "┌")
	assert.NotContains(t, view, "│")
}

func TestREPLPanel_SetSize(t *testing.T) {
	r := defaultREPL()
	r.SetSize(120, 40)
	assert.Equal(t, 120, r.width)
	assert.Equal(t, 40, r.height)
}

func TestREPLPanel_ImplementsSubPanel(t *testing.T) {
	var _ tui.SubPanel = (*REPLPanel)(nil)
}
