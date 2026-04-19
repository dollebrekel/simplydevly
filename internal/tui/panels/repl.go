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
	tea "charm.land/bubbletea/v2"
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
	skillLoader     *skills.SkillLoader
	slashOverlay    *SlashOverlay
	builtinCmds     map[string]BuiltinCommand
	subcommandParent string // tracks which parent command is showing subcommands
	theme           tui.Theme
	renderConfig    tui.RenderConfig
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

	return &REPLPanel{
		textInput:    ti,
		history:      nil,
		historyIndex: -1,
		panel:        p,
		output:       nil,
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

	case tea.MouseMsg:
		return nil

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
		r.output = nil
		return nil
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
			r.output = append(r.output, "Skill error: "+err.Error())
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

// View renders the REPL panel: output area + slash overlay + input line.
func (r *REPLPanel) View() string {
	var b strings.Builder

	for _, line := range r.output {
		b.WriteString(line)
		b.WriteByte('\n')
	}

	b.WriteString(r.textInput.View())

	// Render slash command overlay below the input line when visible.
	if r.slashOverlay != nil && r.slashOverlay.IsVisible() {
		overlayView := r.slashOverlay.View()
		if overlayView != "" {
			b.WriteByte('\n')
			b.WriteString(overlayView)
		}
	}

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
// Calculates which item was clicked based on Y coordinate and selects it.
func (r *REPLPanel) handleOverlayClick(msg tea.MouseClickMsg) tea.Cmd {
	if msg.Button != tea.MouseLeft {
		return nil
	}
	// The overlay starts after: output lines + input line (1) + border top (1) + title (1).
	// Each item is 1 line tall (Height=1). The first item starts at offset 3 from overlay top.
	overlayStartY := len(r.output) + 1 // output lines + input line
	// Border top (1) + title line (1) = 2 lines before first item.
	itemStartY := overlayStartY + 2
	clickedIndex := msg.Y - itemStartY
	if clickedIndex >= 0 && clickedIndex < r.slashOverlay.ItemCount() {
		r.slashOverlay.SelectIndex(clickedIndex)
		selected := r.slashOverlay.SelectedName()
		if selected != "" {
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
		}
	}
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
			r.output = append(r.output, line)
		}
	}
	if err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			r.output = append(r.output, fmt.Sprintf("Error: %s", errMsg))
		} else {
			r.output = append(r.output, fmt.Sprintf("Error: %v", err))
		}
	}
	if len(r.output) > maxOutput {
		r.output = r.output[len(r.output)-maxOutput:]
	}
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
