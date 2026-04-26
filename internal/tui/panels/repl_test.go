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
	assert.Empty(t, r.messages)
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

func TestREPLPanel_CtrlL_ClearsMessages(t *testing.T) {
	r := defaultREPL()
	r.appendMessage(roleUser, "hello")
	r.appendMessage(roleAssistant, "world")
	r.statusLine = "⚙ Using: bash"
	r.userScrolledUp = true

	r.Update(tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
	assert.Empty(t, r.messages, "Ctrl+L should clear messages")
	assert.Equal(t, "", r.statusLine, "Ctrl+L should clear statusLine")
	assert.False(t, r.userScrolledUp, "Ctrl+L should reset scroll state")
}

func TestREPLPanel_AgentOutputMsg(t *testing.T) {
	r := defaultREPL()

	r.Update(tui.AgentOutputMsg{Text: "response line"})

	require.Len(t, r.messages, 1)
	assert.Equal(t, roleAssistant, r.messages[0].role)
	assert.Equal(t, "response line", r.messages[0].text)
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
	r.appendMessage(roleAssistant, "Welcome")
	r.refreshChatViewport()

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

	// Zero dimensions clamp to 1.
	r.SetSize(0, 0)
	assert.Equal(t, 1, r.width)
	assert.Equal(t, 1, r.height)

	// Negative dimensions clamp to 1.
	r.SetSize(-5, -9)
	assert.Equal(t, 1, r.width)
	assert.Equal(t, 1, r.height)
}

func TestREPLPanel_ImplementsSubPanel(t *testing.T) {
	var _ tui.SubPanel = (*REPLPanel)(nil)
}

// --- Chat message and viewport tests ---

func TestChatMessage_AppendAndCap(t *testing.T) {
	r := defaultREPL()

	for i := 0; i < maxMessages+50; i++ {
		r.appendMessage(roleUser, "msg")
		r.appendMessage(roleAssistant, "reply")
	}
	assert.LessOrEqual(t, len(r.messages), maxMessages)
}

func TestChatMessage_CoalesceAssistant(t *testing.T) {
	r := defaultREPL()

	r.appendMessage(roleAssistant, "hello ")
	r.appendMessage(roleAssistant, "world")

	require.Len(t, r.messages, 1)
	assert.Equal(t, "hello world", r.messages[0].text)
}

func TestChatMessage_CoalesceOnlyAssistant(t *testing.T) {
	r := defaultREPL()

	r.appendMessage(roleUser, "q1")
	r.appendMessage(roleUser, "q2")

	require.Len(t, r.messages, 2, "non-assistant roles should not coalesce")
}

func TestChatMessage_RolesPreserved(t *testing.T) {
	r := defaultREPL()

	r.appendMessage(roleUser, "hello")
	r.appendMessage(roleAssistant, "hi there")
	r.appendMessage(roleTool, "⚙ bash")
	r.appendMessage(roleStatus, "info")

	require.Len(t, r.messages, 4)
	assert.Equal(t, roleUser, r.messages[0].role)
	assert.Equal(t, roleAssistant, r.messages[1].role)
	assert.Equal(t, roleTool, r.messages[2].role)
	assert.Equal(t, roleStatus, r.messages[3].role)
}

func TestUserEchoMsg_CreatesUserMessage(t *testing.T) {
	r := defaultREPL()

	r.Update(tui.UserEchoMsg{Text: "my question"})

	require.Len(t, r.messages, 1)
	assert.Equal(t, roleUser, r.messages[0].role)
	assert.Equal(t, "my question", r.messages[0].text)
}

func TestUserEchoMsg_SetsThinkingStatus(t *testing.T) {
	r := defaultREPL()

	r.Update(tui.UserEchoMsg{Text: "hello"})
	assert.Equal(t, "Thinking...", r.statusLine)
}

func TestRenderChat_UserMessages(t *testing.T) {
	r := defaultREPL()
	r.appendMessage(roleUser, "hello")

	rendered := r.renderChat()
	assert.Contains(t, rendered, "> hello")
}

func TestRenderChat_AssistantMessages(t *testing.T) {
	r := defaultREPL()
	r.appendMessage(roleAssistant, "response text")

	rendered := r.renderChat()
	assert.Contains(t, rendered, "response text")
}

func TestRenderChat_ToolMessages(t *testing.T) {
	r := defaultREPL()
	r.appendMessage(roleTool, "⚙ bash")

	rendered := r.renderChat()
	assert.Contains(t, rendered, "⚙ bash")
}

func TestRenderChat_Empty(t *testing.T) {
	r := defaultREPL()
	assert.Equal(t, "", r.renderChat())
}

func TestRenderChat_MixedMessages(t *testing.T) {
	r := defaultREPL()
	r.appendMessage(roleUser, "hello")
	r.appendMessage(roleAssistant, "hi")
	r.appendMessage(roleTool, "⚙ bash")
	r.appendMessage(roleAssistant, "done")

	rendered := r.renderChat()
	assert.Contains(t, rendered, "> hello")
	assert.Contains(t, rendered, "hi")
	assert.Contains(t, rendered, "⚙ bash")
	assert.Contains(t, rendered, "done")
}

func TestStatusLine_ThinkingToToolToCleared(t *testing.T) {
	r := defaultREPL()

	r.Update(tui.UserEchoMsg{Text: "question"})
	assert.Equal(t, "Thinking...", r.statusLine)

	r.Update(tui.FeedEntryMsg{Type: "tool", Label: "bash"})
	assert.Equal(t, "⚙ Using: bash", r.statusLine)

	r.Update(tui.FeedEntryMsg{Type: "tool-done"})
	assert.Equal(t, "", r.statusLine)
}

func TestStatusLine_ThinkingClearedByFirstToken(t *testing.T) {
	r := defaultREPL()

	r.Update(tui.UserEchoMsg{Text: "question"})
	assert.Equal(t, "Thinking...", r.statusLine)

	r.Update(tui.AgentOutputMsg{Text: "first token"})
	assert.Equal(t, "", r.statusLine)
}

func TestStatusLine_ToolStatusClearedByOutputToken(t *testing.T) {
	r := defaultREPL()

	r.Update(tui.UserEchoMsg{Text: "question"})
	r.Update(tui.FeedEntryMsg{Type: "tool", Label: "bash"})
	assert.Equal(t, "⚙ Using: bash", r.statusLine)

	r.Update(tui.AgentOutputMsg{Text: "result"})
	assert.Equal(t, "", r.statusLine, "output should clear tool status too")
}

func TestStatusLine_ClearedOnAgentDone(t *testing.T) {
	r := defaultREPL()
	r.statusLine = "⚙ Using: bash"
	r.agentRunning = true

	r.Update(tui.AgentDoneMsg{})
	assert.Equal(t, "", r.statusLine)
	assert.False(t, r.agentRunning)
}

func TestStatusLine_RenderedInView(t *testing.T) {
	r := defaultREPL()
	r.SetSize(80, 24)
	r.statusLine = "⚙ Using: bash"

	view := r.View()
	assert.Contains(t, view, "Using: bash")
}

func TestFeedEntryMsg_ToolCreatesMessage(t *testing.T) {
	r := defaultREPL()

	r.Update(tui.FeedEntryMsg{Type: "tool", Label: "grep"})

	require.Len(t, r.messages, 1)
	assert.Equal(t, roleTool, r.messages[0].role)
	assert.Contains(t, r.messages[0].text, "grep")
}

func TestFeedEntryMsg_ToolEmptyLabelIgnored(t *testing.T) {
	r := defaultREPL()

	r.Update(tui.FeedEntryMsg{Type: "tool", Label: ""})

	assert.Empty(t, r.messages, "empty Label should not create a message")
	assert.Equal(t, "", r.statusLine, "empty Label should not set status")
}

func TestFeedEntryMsg_ToolDoneNoMessage(t *testing.T) {
	r := defaultREPL()

	r.Update(tui.FeedEntryMsg{Type: "tool-done"})

	assert.Empty(t, r.messages)
}

func TestViewport_ScrollState(t *testing.T) {
	r := defaultREPL()
	r.SetSize(80, 10)

	for i := 0; i < 50; i++ {
		r.appendMessage(roleAssistant, "line")
	}
	r.refreshChatViewport()

	assert.False(t, r.userScrolledUp, "should auto-scroll to bottom")
	assert.True(t, r.chatViewport.AtBottom())
}

func TestViewport_SetSizeAllocatesCorrectly(t *testing.T) {
	r := defaultREPL()
	r.SetSize(80, 24)

	assert.Equal(t, 80-2, r.chatViewport.Width())
	assert.Equal(t, 24-3-2, r.chatViewport.Height())
}

func TestExistingFeatures_SlashCommands_StillWork(t *testing.T) {
	r := defaultREPL()
	typeText(r, "/help")
	r.updateOverlayVisibility()
	assert.True(t, r.slashOverlay.IsVisible())
}

func TestExistingFeatures_History_StillWorks(t *testing.T) {
	r := defaultREPL()

	typeText(r, "cmd1")
	r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	r.agentRunning = false

	typeText(r, "cmd2")
	r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	r.agentRunning = false

	r.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, "cmd2", r.textInput.Value())

	r.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, "cmd1", r.textInput.Value())
}

func TestExistingFeatures_Cancel_StillWorks(t *testing.T) {
	r := defaultREPL()
	r.agentRunning = true

	cmd := r.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(tui.CancelMsg)
	assert.True(t, ok)
}
