// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package panels

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"siply.dev/siply/internal/skills"
	"siply.dev/siply/internal/tui"
)

const (
	maxHistory  = 1000
	maxMessages = 2000
)

type chatRole string

const (
	roleUser      chatRole = "user"
	roleAssistant chatRole = "assistant"
	roleTool      chatRole = "tool"
	roleStatus    chatRole = "status"
)

type chatMessage struct {
	role chatRole
	text string
}

// Compile-time interface check.
var _ tui.SubPanel = (*REPLPanel)(nil)

// REPLPanel implements the interactive REPL interface.
type REPLPanel struct {
	textInput        textinput.Model
	history          []string
	historyIndex     int
	currentInput     string
	panel            *tui.Panel
	messages         []chatMessage
	chatViewport     viewport.Model
	statusLine       string
	userScrolledUp   bool
	agentRunning     bool
	hasBorder        bool
	width            int
	height           int
	slashDispatcher  *skills.SlashDispatcher
	skillLoader      *skills.SkillLoader
	slashOverlay     *SlashOverlay
	builtinCmds      map[string]BuiltinCommand
	subcommandParent string
	theme            tui.Theme
	renderConfig     tui.RenderConfig
}

// NewREPLPanel creates a new REPL panel with text input and history.
func NewREPLPanel(theme tui.Theme, config tui.RenderConfig) *REPLPanel {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.Focus()

	p := tui.NewPanel("siply", theme, config)
	p.SetFocused(true)

	overlay := NewSlashOverlay(theme, config)

	ti.ShowSuggestions = true
	// Disable textinput's built-in Tab acceptance — we handle Tab via the overlay.
	ti.KeyMap.AcceptSuggestion = key.NewBinding(key.WithDisabled())

	vp := viewport.New()
	vp.MouseWheelEnabled = true
	vp.MouseWheelDelta = 3

	return &REPLPanel{
		textInput:    ti,
		history:      nil,
		historyIndex: -1,
		panel:        p,
		chatViewport: vp,
		hasBorder:    config.Borders != tui.BorderNone,
		slashOverlay: overlay,
		builtinCmds:  builtinCommandMap(),
		theme:        theme,
		renderConfig: config,
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
	case tea.MouseClickMsg:
		if r.slashOverlay != nil && r.slashOverlay.IsVisible() {
			return r.handleOverlayClick(msg)
		}
		return nil

	case tea.MouseWheelMsg:
		var cmd tea.Cmd
		r.chatViewport, cmd = r.chatViewport.Update(msg)
		r.userScrolledUp = !r.chatViewport.AtBottom()
		return cmd

	case tea.MouseMsg:
		return nil

	case tea.KeyPressMsg:
		return r.handleKey(msg)

	case tui.UserEchoMsg:
		r.appendMessage(roleUser, msg.Text)
		r.statusLine = "Thinking..."
		r.refreshChatViewport()
		return nil

	case tui.AgentOutputMsg:
		r.statusLine = ""
		r.appendMessage(roleAssistant, msg.Text)
		r.refreshChatViewport()
		return nil

	case tui.FeedEntryMsg:
		switch msg.Type {
		case "tool":
			if msg.Label != "" {
				r.statusLine = "⚙ Using: " + msg.Label
				r.appendMessage(roleTool, "⚙ "+msg.Label)
			}
		case "tool-done":
			r.statusLine = ""
		}
		r.refreshChatViewport()
		return nil

	case tui.AgentDoneMsg:
		r.agentRunning = false
		r.statusLine = ""
		r.refreshChatViewport()
		return nil
	}

	// Pass other messages to textinput.
	var cmd tea.Cmd
	r.textInput, cmd = r.textInput.Update(msg)
	return cmd
}

// handleKey processes key press messages.
func (r *REPLPanel) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	key := msg.String()

	// When the slash overlay is visible, route navigation keys to it.
	if r.slashOverlay != nil && r.slashOverlay.IsVisible() {
		switch key {
		case "tab":
			selected, _ := r.slashOverlay.HandleKey(key)
			if selected != "" {
				if r.subcommandParent != "" {
					// Selecting a subcommand: append to parent.
					r.textInput.SetValue("/" + r.subcommandParent + " " + selected + " ")
					r.textInput.CursorEnd()
					r.subcommandParent = ""
				} else {
					r.textInput.SetValue("/" + selected + " ")
					r.textInput.CursorEnd()
					// If the selected command has subcommands, show them.
					if r.showSubcommandsIfNeeded(selected) {
						return nil
					}
				}
			}
			return nil
		case "enter":
			// Enter submits the current input as-is (does not select from overlay).
			r.slashOverlay.Hide()
			return r.handleSubmit()
		case "esc":
			r.slashOverlay.HandleKey(key)
			return nil
		case "up", "down":
			r.slashOverlay.HandleKey(key)
			return nil
		case "ctrl+c":
			return r.handleCancel()
		}

		// For all other keys, pass to textinput first, then update filter.
		var cmd tea.Cmd
		r.textInput, cmd = r.textInput.Update(msg)
		r.updateOverlayVisibility()
		return cmd
	}

	switch key {
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
		r.messages = nil
		r.statusLine = ""
		r.userScrolledUp = false
		r.refreshChatViewport()
		return nil

	case "pgup", "pgdown":
		var cmd tea.Cmd
		r.chatViewport, cmd = r.chatViewport.Update(msg)
		r.userScrolledUp = !r.chatViewport.AtBottom()
		return cmd
	}

	// Pass to textinput for standard editing (ctrl+a, ctrl+e, ctrl+w, ctrl+u, etc.).
	var cmd tea.Cmd
	r.textInput, cmd = r.textInput.Update(msg)

	// Check if we should show the slash overlay after text changes.
	r.updateOverlayVisibility()

	return cmd
}

// SetSlashDispatcher attaches a SlashDispatcher and SkillLoader for skill
// slash-command expansion (AC#2, AC#3) and dynamic skill discovery (AC#7).
func (r *REPLPanel) SetSlashDispatcher(d *skills.SlashDispatcher, loader *skills.SkillLoader) {
	r.slashDispatcher = d
	r.skillLoader = loader
	r.refreshOverlayItems()
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

	// Hide slash overlay on submit.
	if r.slashOverlay != nil {
		r.slashOverlay.Hide()
	}

	// Check built-in slash commands first (AC#6).
	if strings.HasPrefix(text, "/") {
		cmdParts := strings.Fields(strings.TrimPrefix(text, "/"))
		if len(cmdParts) > 0 {
			cmdName := cmdParts[0]
			if builtin, ok := r.builtinCmds[cmdName]; ok {
				r.textInput.Reset()
				r.textInput.SetSuggestions(nil)
				r.historyIndex = -1
				r.currentInput = ""
				if builtin.Handler != nil {
					return builtin.Handler()
				}
				// Execute as CLI subprocess: siply <args...>
				r.executeBuiltinCommand(cmdParts)
				return nil
			}
		}
	}

	// Expand slash commands to their rendered prompt template (AC#2, AC#3).
	if r.slashDispatcher != nil && r.slashDispatcher.IsSlashCommand(text) {
		expanded, err := r.slashDispatcher.Dispatch(text)
		if err != nil {
			r.appendMessage(roleStatus, "Skill error: "+err.Error())
			r.refreshChatViewport()
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

func (r *REPLPanel) appendMessage(role chatRole, text string) {
	if role == roleAssistant && len(r.messages) > 0 && r.messages[len(r.messages)-1].role == roleAssistant {
		r.messages[len(r.messages)-1].text += text
		return
	}
	r.messages = append(r.messages, chatMessage{role: role, text: text})
	if len(r.messages) > maxMessages {
		r.messages = r.messages[len(r.messages)-maxMessages:]
	}
}

func (r *REPLPanel) refreshChatViewport() {
	r.chatViewport.SetContent(r.renderChat())
	if !r.userScrolledUp {
		r.chatViewport.GotoBottom()
	}
}

func (r *REPLPanel) renderChat() string {
	if len(r.messages) == 0 {
		return ""
	}
	cs := r.renderConfig.Color
	mutedStyle := r.theme.Muted.Resolve(cs)
	textMutedStyle := r.theme.TextMuted.Resolve(cs)

	var b strings.Builder
	for i, m := range r.messages {
		if i > 0 {
			b.WriteByte('\n')
		}
		switch m.role {
		case roleUser:
			b.WriteString(mutedStyle.Render("> " + m.text))
		case roleAssistant:
			b.WriteString(m.text)
		case roleTool:
			b.WriteString(textMutedStyle.Render(m.text))
		case roleStatus:
			b.WriteString(textMutedStyle.Render(m.text))
		}
	}
	return b.String()
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

// IsOverlayActive returns true when the slash command overlay is visible.
func (r *REPLPanel) IsOverlayActive() bool {
	return r.slashOverlay != nil && r.slashOverlay.IsVisible()
}

// View renders the REPL panel + slash overlay below it.
// The overlay renders OUTSIDE the panel border so its Y position is
// deterministic (no nested border offset issues).
func (r *REPLPanel) View() string {
	var content strings.Builder
	content.WriteString(r.chatViewport.View())
	if r.statusLine != "" {
		content.WriteByte('\n')
		cs := r.renderConfig.Color
		content.WriteString(r.theme.TextMuted.Resolve(cs).Render(r.statusLine))
	}
	content.WriteByte('\n')
	content.WriteString(r.textInput.View())

	r.panel.SetContent(content.String())
	r.panel.SetSize(r.width, r.height)
	panelView := r.panel.Render()

	// Overlay renders below the panel, not inside it.
	if r.slashOverlay != nil && r.slashOverlay.IsVisible() {
		overlayView := r.slashOverlay.View()
		if overlayView != "" {
			combined := panelView + "\n" + overlayView
			// Register hitmap: first item Y = panel lines + separator newline (1) + overlay border top (1).
			firstItemY := strings.Count(panelView, "\n") + 2
			r.slashOverlay.RegisterHitmap(firstItemY)
			return combined
		}
	}

	return panelView
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

	chrome := len([]rune(r.textInput.Prompt))
	if r.hasBorder {
		chrome += 2
	}
	tiWidth := width - chrome
	if tiWidth < 1 {
		tiWidth = 1
	}
	r.textInput.SetWidth(tiWidth)

	vpWidth := width
	if r.hasBorder {
		vpWidth -= 2
	}
	if vpWidth < 1 {
		vpWidth = 1
	}
	vpHeight := height - 3
	if r.hasBorder {
		vpHeight -= 2
	}
	if vpHeight < 1 {
		vpHeight = 1
	}
	r.chatViewport.SetWidth(vpWidth)
	r.chatViewport.SetHeight(vpHeight)
	r.refreshChatViewport()

	// Propagate size to slash overlay (use half the height, capped at 14 lines).
	if r.slashOverlay != nil {
		overlayH := height / 2
		if overlayH > 14 {
			overlayH = 14
		}
		if overlayH < 3 {
			overlayH = 3
		}
		r.slashOverlay.SetSize(width, overlayH)
	}
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

// updateOverlayVisibility checks the current input and shows/hides the slash
// overlay accordingly. The overlay appears when input starts with "/" and has
// no space yet (indicating the user is still typing a command name), or when
// in subcommand mode (parent command already selected via Tab).
func (r *REPLPanel) updateOverlayVisibility() {
	if r.slashOverlay == nil {
		return
	}
	val := r.textInput.Value()

	// Subcommand mode: parent was selected, user may be typing a subcommand name.
	if r.subcommandParent != "" {
		parentPrefix := "/" + r.subcommandParent + " "
		if strings.HasPrefix(val, parentPrefix) {
			subPrefix := strings.TrimPrefix(val, parentPrefix)
			// If there's another space, user moved past subcommand selection.
			if strings.Contains(subPrefix, " ") {
				r.slashOverlay.Hide()
				r.subcommandParent = ""
				r.textInput.SetSuggestions(nil)
				return
			}
			r.slashOverlay.Filter(subPrefix)
			r.updateTextInputSuggestions()
			return
		}
		// Input no longer matches parent prefix — exit subcommand mode.
		r.subcommandParent = ""
		r.refreshOverlayItems()
	}

	if strings.HasPrefix(val, "/") && !strings.Contains(val, " ") {
		// Reload skills dynamically on "/" keystroke (AC#7 — Option A).
		if !r.slashOverlay.IsVisible() {
			r.refreshOverlayItems()
		}
		r.slashOverlay.Show()
		// Filter by the text after "/".
		prefix := strings.TrimPrefix(val, "/")
		r.slashOverlay.Filter(prefix)
		r.updateTextInputSuggestions()
	} else {
		r.slashOverlay.Hide()
		r.textInput.SetSuggestions(nil)
	}
}

// showSubcommandsIfNeeded checks if the selected command has subcommands and
// shows them in the overlay. Returns true if subcommands were shown.
func (r *REPLPanel) showSubcommandsIfNeeded(cmdName string) bool {
	if r.slashOverlay == nil {
		return false
	}
	builtin, ok := r.builtinCmds[cmdName]
	if !ok || len(builtin.Subcommands) == 0 {
		r.subcommandParent = ""
		return false
	}
	r.subcommandParent = cmdName
	r.slashOverlay.SetSubcommandItems(builtin.Subcommands)
	return true
}

// handleOverlayClick processes a mouse click on the slash overlay.
// Uses the hitmap registered during View() for pixel-accurate item detection.
func (r *REPLPanel) handleOverlayClick(msg tea.MouseClickMsg) tea.Cmd {
	if msg.Button != tea.MouseLeft {
		return nil
	}
	absIndex, ok := r.slashOverlay.HitTest(msg.Y)
	if !ok {
		return nil
	}
	r.slashOverlay.Select(absIndex)
	selected := r.slashOverlay.SelectedName()
	if selected == "" {
		return nil
	}
	r.slashOverlay.Hide()
	if r.subcommandParent != "" {
		r.textInput.SetValue("/" + r.subcommandParent + " " + selected + " ")
		r.textInput.CursorEnd()
		r.subcommandParent = ""
	} else {
		r.textInput.SetValue("/" + selected + " ")
		r.textInput.CursorEnd()
		if r.showSubcommandsIfNeeded(selected) {
			return nil
		}
	}
	r.textInput.SetSuggestions(nil)
	return nil
}

// updateTextInputSuggestions sets the textinput's built-in suggestions
// based on the current overlay selection. This renders ghost text inline
// (faded remaining characters) exactly like IDE autocomplete.
func (r *REPLPanel) updateTextInputSuggestions() {
	if r.slashOverlay == nil || !r.slashOverlay.IsVisible() {
		r.textInput.SetSuggestions(nil)
		return
	}
	selected := r.slashOverlay.SelectedName()
	if selected == "" {
		r.textInput.SetSuggestions(nil)
		return
	}
	if r.subcommandParent != "" {
		r.textInput.SetSuggestions([]string{"/" + r.subcommandParent + " " + selected})
	} else {
		r.textInput.SetSuggestions([]string{"/" + selected})
	}
}

// executeBuiltinCommand runs a siply CLI command as a subprocess and returns
// the output as REPL feedback lines.
func (r *REPLPanel) executeBuiltinCommand(args []string) {
	cmd := exec.Command("siply", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	output := strings.TrimSpace(stdout.String())
	if output != "" {
		for _, line := range strings.Split(output, "\n") {
			r.appendMessage(roleStatus, line)
		}
	}
	if err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		addedErrorLine := false
		if errMsg != "" {
			for _, line := range strings.Split(errMsg, "\n") {
				if strings.Contains(line, " INFO ") || strings.Contains(line, " DEBUG ") {
					continue
				}
				r.appendMessage(roleStatus, "Error: "+line)
				addedErrorLine = true
			}
		}
		if !addedErrorLine && err.Error() != "exit status 1" {
			r.appendMessage(roleStatus, fmt.Sprintf("Error: %v", err))
		}
	}
	r.refreshChatViewport()
}

// refreshOverlayItems reloads skills and rebuilds the overlay item list.
func (r *REPLPanel) refreshOverlayItems() {
	if r.slashOverlay == nil {
		return
	}
	var skillList []skills.Skill
	if r.skillLoader != nil {
		// Reload skills from disk for dynamic discovery (AC#7).
		if err := r.skillLoader.LoadAll(context.Background()); err != nil {
			slog.Debug("slash overlay: reload skills", "err", err)
		}
		skillList = r.skillLoader.List()
	}
	r.slashOverlay.SetItems(BuiltinCommands(), skillList)
}
