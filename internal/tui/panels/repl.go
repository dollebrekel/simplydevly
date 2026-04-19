// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package panels

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/textinput"
	"siply.dev/siply/internal/skills"
	"siply.dev/siply/internal/tui"
)

const (
	maxHistory = 1000
	maxOutput  = 2000
)

// Compile-time interface check.
var _ tui.SubPanel = (*REPLPanel)(nil)

// REPLPanel implements the interactive REPL interface.
type REPLPanel struct {
	textInput       textinput.Model
	history         []string
	historyIndex    int
	currentInput    string
	panel           *tui.Panel
	output          []string
	agentRunning    bool
	hasBorder       bool
	width           int
	height          int
	slashDispatcher *skills.SlashDispatcher
}

// NewREPLPanel creates a new REPL panel with text input and history.
func NewREPLPanel(theme tui.Theme, config tui.RenderConfig) *REPLPanel {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.Focus()

	p := tui.NewPanel("siply", theme, config)
	p.SetFocused(true)

	return &REPLPanel{
		textInput:    ti,
		history:      nil,
		historyIndex: -1,
		panel:        p,
		output:       nil,
		hasBorder:    config.Borders != tui.BorderNone,
	}
}

// Init returns the initial command (cursor blink).
func (r *REPLPanel) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles incoming messages. Mutates in place and returns tea.Cmd
// (satisfies tui.SubPanel interface).
func (r *REPLPanel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return r.handleKey(msg)

	case tui.AgentOutputMsg:
		r.output = append(r.output, msg.Text)
		if len(r.output) > maxOutput {
			r.output = r.output[len(r.output)-maxOutput:]
		}
		return nil

	case tui.AgentDoneMsg:
		r.agentRunning = false
		return nil
	}

	// Pass other messages to textinput.
	var cmd tea.Cmd
	r.textInput, cmd = r.textInput.Update(msg)
	return cmd
}

// handleKey processes key press messages.
func (r *REPLPanel) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		return r.handleSubmit()

	case "ctrl+c":
		return r.handleCancel()

	case "up":
		r.navigateHistoryBack()
		return nil

	case "down":
		r.navigateHistoryForward()
		return nil

	case "ctrl+l":
		r.output = nil
		return nil
	}

	// Pass to textinput for standard editing (ctrl+a, ctrl+e, ctrl+w, ctrl+u, etc.).
	var cmd tea.Cmd
	r.textInput, cmd = r.textInput.Update(msg)
	return cmd
}

// SetSlashDispatcher attaches a SlashDispatcher for skill slash-command expansion (AC#2, AC#3).
func (r *REPLPanel) SetSlashDispatcher(d *skills.SlashDispatcher) {
	r.slashDispatcher = d
}

// handleSubmit processes Enter key — submits input to agent.
// If the input is a skill slash command, it is expanded to the rendered prompt
// template before being submitted (AC#2, AC#3).
func (r *REPLPanel) handleSubmit() tea.Cmd {
	if r.agentRunning {
		return nil
	}

	text := strings.TrimSpace(r.textInput.Value())
	if text == "" {
		return nil
	}

	// Expand slash commands to their rendered prompt template (AC#2, AC#3).
	if r.slashDispatcher != nil && r.slashDispatcher.IsSlashCommand(text) {
		expanded, err := r.slashDispatcher.Dispatch(text)
		if err != nil {
			r.output = append(r.output, "❌ Skill error: "+err.Error())
			if len(r.output) > maxOutput {
				r.output = r.output[len(r.output)-maxOutput:]
			}
			r.textInput.Reset()
			r.historyIndex = -1
			r.currentInput = ""
			return nil
		}
		text = expanded
	}

	// Add to history (skip consecutive duplicates).
	if len(r.history) == 0 || r.history[len(r.history)-1] != text {
		r.history = append(r.history, text)
		// Cap history — trim from the front to stay at maxHistory.
		if len(r.history) > maxHistory {
			r.history = r.history[len(r.history)-maxHistory:]
		}
	}

	r.textInput.Reset()
	r.historyIndex = -1
	r.currentInput = ""
	r.agentRunning = true

	return func() tea.Msg {
		return tui.SubmitMsg{Text: text}
	}
}

// handleCancel processes Ctrl+C.
func (r *REPLPanel) handleCancel() tea.Cmd {
	if r.agentRunning {
		return func() tea.Msg {
			return tui.CancelMsg{}
		}
	}
	return tea.Quit
}

// navigateHistoryBack moves to the previous history entry.
func (r *REPLPanel) navigateHistoryBack() {
	if len(r.history) == 0 {
		return
	}
	if r.historyIndex == -1 {
		r.currentInput = r.textInput.Value()
		r.historyIndex = len(r.history) - 1
	} else if r.historyIndex > 0 {
		r.historyIndex--
	} else {
		return
	}
	r.textInput.SetValue(r.history[r.historyIndex])
	r.textInput.CursorEnd()
}

// navigateHistoryForward moves to the next history entry.
func (r *REPLPanel) navigateHistoryForward() {
	if r.historyIndex == -1 {
		return
	}
	if r.historyIndex < len(r.history)-1 {
		r.historyIndex++
		r.textInput.SetValue(r.history[r.historyIndex])
		r.textInput.CursorEnd()
	} else {
		r.historyIndex = -1
		r.textInput.SetValue(r.currentInput)
		r.textInput.CursorEnd()
	}
}

// View renders the REPL panel: output area + input line.
func (r *REPLPanel) View() string {
	var b strings.Builder

	for _, line := range r.output {
		b.WriteString(line)
		b.WriteByte('\n')
	}

	b.WriteString(r.textInput.View())

	r.panel.SetContent(b.String())
	r.panel.SetSize(r.width, r.height)
	return r.panel.Render()
}

// SetSize updates the REPL panel dimensions.
func (r *REPLPanel) SetSize(width, height int) {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	r.width = width
	r.height = height
	// Compute text input width from actual chrome: prompt + optional borders.
	chrome := len([]rune(r.textInput.Prompt))
	if r.hasBorder {
		chrome += 2 // left + right border columns
	}
	tiWidth := width - chrome
	if tiWidth < 1 {
		tiWidth = 1
	}
	r.textInput.SetWidth(tiWidth)
}

// SetBordered toggles the border display for the REPL panel.
func (r *REPLPanel) SetBordered(bordered bool) {
	r.hasBorder = bordered
	r.panel.SetBordered(bordered)
	// Recalculate text input width to account for border chrome change.
	r.SetSize(r.width, r.height)
}

// AgentRunning returns whether the agent is currently processing.
func (r *REPLPanel) AgentRunning() bool {
	return r.agentRunning
}

// SetAgentRunning sets the agent running state.
func (r *REPLPanel) SetAgentRunning(running bool) {
	r.agentRunning = running
}
