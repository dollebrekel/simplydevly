// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package panels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"siply.dev/siply/internal/permission"
	"siply.dev/siply/internal/skills"
	"siply.dev/siply/internal/tui"
	"siply.dev/siply/internal/tui/components"
)

const (
	maxHistory      = 1000
	maxMessages     = 2000
	spinnerInterval = 80 * time.Millisecond
)

var spinnerFrames = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

type spinnerTickMsg struct{}

type spinner struct {
	frameIndex int
	startTime  time.Time
	tokenCount int
	label      string
	active     bool
}

func (s *spinner) frame() rune {
	if len(spinnerFrames) == 0 {
		return '⠋'
	}
	return spinnerFrames[s.frameIndex%len(spinnerFrames)]
}

func (s *spinner) advance() {
	s.frameIndex = (s.frameIndex + 1) % len(spinnerFrames)
}

func (s *spinner) elapsed() time.Duration {
	return time.Since(s.startTime).Truncate(time.Second)
}

func (s *spinner) render() string {
	if !s.active {
		return ""
	}
	var b strings.Builder
	b.WriteRune(s.frame())
	b.WriteByte(' ')
	b.WriteString(s.label)
	b.WriteString(fmt.Sprintf(" (%s", s.elapsed()))
	if s.tokenCount > 0 {
		if s.tokenCount >= 1000 {
			b.WriteString(fmt.Sprintf(" · ↓ %.1fk tokens", float64(s.tokenCount)/1000))
		} else {
			b.WriteString(fmt.Sprintf(" · ↓ %d tokens", s.tokenCount))
		}
	}
	b.WriteByte(')')
	return b.String()
}

type chatRole string

const (
	roleUser      chatRole = "user"
	roleAssistant chatRole = "assistant"
	roleTool      chatRole = "tool"
	roleStatus    chatRole = "status"
	roleSplash    chatRole = "splash"
)

type chatMessage struct {
	role         chatRole
	text         string
	detail       string
	collapsed    bool
	toolID       string
	toolInput    string
	toolOutput   string
	toolStatus   string
	toolExitCode *int
	duration     time.Duration
	isError      bool
}

var bashExitCodeRE = regexp.MustCompile(`(?m)^exit code:\s*(-?\d+)\s*$`)

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
	spinner          spinner
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
	markdownView     *components.MarkdownView
	agentStatus      tui.AgentStatusRenderer
	permissionCtrl   permissionController
}

type permissionController interface {
	SetMode(permission.Mode) error
	Mode() permission.Mode
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

	r := &REPLPanel{
		textInput:    ti,
		history:      nil,
		historyIndex: -1,
		panel:        p,
		messages:     []chatMessage{{role: roleSplash}},
		chatViewport: vp,
		hasBorder:    config.Borders != tui.BorderNone,
		slashOverlay: overlay,
		builtinCmds:  builtinCommandMap(),
		theme:        theme,
		renderConfig: config,
		markdownView: components.NewMarkdownView(theme, config),
		agentStatus:  components.NewAgentStatusPanel(theme, config),
	}
	r.builtinCmds = r.builtinCommandMap()
	return r
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
		r.spinner = spinner{active: true, label: "Thinking...", startTime: time.Now()}
		r.refreshChatViewport()
		return r.tickSpinner()

	case tui.AgentOutputMsg:
		r.spinner.tokenCount += len([]rune(msg.Text))
		r.spinner.label = "Generating..."
		r.appendMessage(roleAssistant, msg.Text)
		r.refreshChatViewport()
		return nil

	case tui.FeedEntryMsg:
		switch msg.Type {
		case "tool":
			if msg.Label != "" {
				r.spinner.label = "⚙ Using: " + msg.Label
				r.appendToolStart(msg)
			}
		case "tool-done":
			r.spinner.label = "Thinking..."
			r.updateToolDone(msg)
		}
		r.refreshChatViewport()
		return nil

	case tui.AgentStatusUpdateMsg:
		if r.agentStatus != nil {
			r.agentStatus.HandleAgentStatus(msg)
			r.refreshChatViewport()
		}
		return nil

	case tui.AgentDoneMsg:
		r.agentRunning = false
		r.spinner.active = false
		r.refreshChatViewport()
		if r.agentStatus != nil && r.agentStatus.HasAgents() {
			return r.tickSpinner()
		}
		return nil

	case spinnerTickMsg:
		hasAgentDisplay := r.agentStatus != nil && r.agentStatus.HasAgents()
		if !r.spinner.active && !hasAgentDisplay {
			return nil
		}
		if r.spinner.active {
			r.spinner.advance()
		}
		if r.agentStatus != nil {
			r.agentStatus.Tick()
		}
		r.refreshChatViewport()
		return r.tickSpinner()
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
			if !r.slashOverlay.IsVisible() {
				r.SetSize(r.width, r.height)
			}
			return nil
		case "enter":
			// Enter submits the current input as-is (does not select from overlay).
			// handleSubmit handles hiding the overlay and recalculating layout.
			return r.handleSubmit()
		case "esc":
			r.slashOverlay.HandleKey(key)
			r.SetSize(r.width, r.height)
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

	case "esc":
		if r.agentRunning {
			return r.handleCancel()
		}
		r.textInput.Reset()
		return nil

	case "up":
		r.navigateHistoryBack()
		return nil

	case "down":
		r.navigateHistoryForward()
		return nil

	case "ctrl+l":
		r.messages = nil
		r.spinner.active = false
		r.userScrolledUp = false
		r.refreshChatViewport()
		return nil

	case "ctrl+o":
		r.toggleLastToolBlock()
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

func (r *REPLPanel) tickSpinner() tea.Cmd {
	return tea.Tick(spinnerInterval, func(time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

// SetSlashDispatcher attaches a SlashDispatcher and SkillLoader for skill
// slash-command expansion (AC#2, AC#3) and dynamic skill discovery (AC#7).
func (r *REPLPanel) SetSlashDispatcher(d *skills.SlashDispatcher, loader *skills.SkillLoader) {
	r.slashDispatcher = d
	r.skillLoader = loader
	r.refreshOverlayItems()
}

// SetPermissionController attaches the runtime permission controller used by
// built-in mode commands such as /yolo and /default.
func (r *REPLPanel) SetPermissionController(ctrl permissionController) {
	r.permissionCtrl = ctrl
	r.builtinCmds = r.builtinCommandMap()
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
	if r.slashOverlay != nil && r.slashOverlay.IsVisible() {
		r.slashOverlay.Hide()
		r.SetSize(r.width, r.height)
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
				// Dispatch to subcommand handler if available.
				if len(cmdParts) > 1 && len(builtin.Subcommands) > 0 {
					for _, sub := range builtin.Subcommands {
						if sub.Name == cmdParts[1] && sub.Handler != nil {
							return sub.Handler()
						}
					}
				}
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

func (r *REPLPanel) appendToolMessage(label, detail string) {
	r.messages = append(r.messages, chatMessage{
		role:       roleTool,
		text:       label,
		detail:     detail,
		collapsed:  true,
		toolStatus: "running",
	})
	if len(r.messages) > maxMessages {
		r.messages = r.messages[len(r.messages)-maxMessages:]
	}
}

func (r *REPLPanel) appendToolStart(msg tui.FeedEntryMsg) {
	detail := msg.Detail
	if detail == "" {
		detail = toolInputPreview(msg.Label, msg.Input)
	}
	r.messages = append(r.messages, chatMessage{
		role:       roleTool,
		text:       msg.Label,
		detail:     detail,
		collapsed:  true,
		toolID:     msg.ToolID,
		toolInput:  toolInputPreview(msg.Label, msg.Input),
		toolStatus: "running",
	})
	if len(r.messages) > maxMessages {
		r.messages = r.messages[len(r.messages)-maxMessages:]
	}
}

func (r *REPLPanel) updateToolDone(msg tui.FeedEntryMsg) {
	if msg.Label == "" && msg.ToolID == "" {
		return
	}
	idx := -1
	for i := len(r.messages) - 1; i >= 0; i-- {
		m := r.messages[i]
		if m.role != roleTool {
			continue
		}
		if msg.ToolID != "" && m.toolID == msg.ToolID {
			idx = i
			break
		}
		if msg.ToolID == "" && m.text == msg.Label && m.toolStatus == "running" {
			idx = i
			break
		}
	}
	if idx < 0 {
		r.appendToolStart(msg)
		idx = len(r.messages) - 1
	}
	m := &r.messages[idx]
	if msg.Label != "" {
		m.text = msg.Label
	}
	if m.toolInput == "" {
		m.toolInput = toolInputPreview(m.text, msg.Input)
	}
	m.toolOutput = strings.TrimSpace(msg.Output)
	m.duration = msg.Duration
	m.isError = msg.IsError
	m.toolStatus = "done"
	if msg.IsError {
		m.toolStatus = "failed"
	}
	if msg.ExitCode != nil {
		m.toolExitCode = msg.ExitCode
	} else if code, ok := parseExitCode(msg.Output); ok {
		m.toolExitCode = &code
	}
	if m.detail == "" || m.detail == m.toolInput {
		m.detail = buildToolDetail(*m)
	}
}

func (r *REPLPanel) renderToolBlock(m chatMessage, width int) string {
	cs := r.renderConfig.Color
	accentStyle := r.theme.Accent.Resolve(cs)
	borderStyle := r.theme.Border.Resolve(cs)
	textMutedStyle := r.theme.TextMuted.Resolve(cs)
	successStyle := r.theme.Success.Resolve(cs)
	errorStyle := r.theme.Error.Resolve(cs)

	if m.toolStatus == "" && m.detail == "" && m.toolInput == "" && m.toolOutput == "" && m.duration == 0 {
		return textMutedStyle.Render(toolDisplayName(m.text))
	}

	innerW := width - 4
	if innerW < 10 {
		innerW = 10
	}

	// Top border with tool name.
	status := m.toolStatus
	if status == "" {
		status = "running"
	}
	statusText := status
	if m.duration > 0 {
		statusText += " · " + m.duration.Truncate(time.Millisecond).String()
	}
	if m.toolExitCode != nil {
		statusText += " · exit " + strconv.Itoa(*m.toolExitCode)
	}
	statusStyle := textMutedStyle
	if status == "done" {
		statusStyle = successStyle
	} else if status == "failed" {
		statusStyle = errorStyle
	}
	toolName := toolDisplayName(m.text)
	topPlain := toolName + " " + statusText
	topLabel := accentStyle.Render(toolName) + " " + statusStyle.Render(statusText)
	padLen := innerW - ansi.StringWidth(topPlain) - 3
	if padLen < 0 {
		padLen = 0
	}
	top := borderStyle.Render("┌─ ") + topLabel + " " + borderStyle.Render(strings.Repeat("─", padLen)+"┐")

	var b strings.Builder
	b.WriteString(top)

	lines := toolCollapsedLines(m)
	if !m.collapsed {
		lines = strings.Split(buildToolDetail(m), "\n")
	}
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			b.WriteByte('\n')
			renderedLine := textMutedStyle.Render(ansi.Truncate(line, innerW-2, "…"))
			if !m.collapsed {
				renderedLine = r.renderToolDetailLine(line, innerW-2)
			}
			b.WriteString(borderStyle.Render("│ ") + renderedLine)
		}
	}

	indicator := "▸"
	if !m.collapsed {
		indicator = "▾"
	}
	b.WriteByte('\n')
	footer := " " + indicator + " Ctrl+O "
	footerPad := innerW - ansi.StringWidth(footer) - 1
	if footerPad < 0 {
		footerPad = 0
	}
	b.WriteString(borderStyle.Render("└─") + textMutedStyle.Render(footer) + borderStyle.Render(strings.Repeat("─", footerPad)+"┘"))

	return b.String()
}

func (r *REPLPanel) renderToolDetailLine(line string, width int) string {
	cs := r.renderConfig.Color
	noColor := cs == tui.ColorNone || r.renderConfig.Verbosity == tui.VerbosityAccessible
	var rendered string
	if !noColor {
		switch {
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			rendered = r.theme.Secondary.Resolve(cs).Bold(true).Render(line)
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			rendered = r.theme.Error.Resolve(cs).Faint(true).Render(line)
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			rendered = r.theme.Heading.Resolve(cs).Render(line)
		default:
			rendered = r.theme.TextMuted.Resolve(cs).Render(line)
		}
	} else {
		rendered = r.theme.TextMuted.Resolve(cs).Render(line)
	}
	return ansi.Truncate(rendered, width, "…")
}

// toggleLastToolBlock toggles the collapsed state of the nearest tool block
// above the current viewport position.
func (r *REPLPanel) toggleLastToolBlock() {
	for i := len(r.messages) - 1; i >= 0; i-- {
		if r.messages[i].role == roleTool && r.messages[i].detail != "" {
			r.messages[i].collapsed = !r.messages[i].collapsed
			r.refreshChatViewport()
			return
		}
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
	textMutedStyle := r.theme.TextMuted.Resolve(cs)

	vpWidth := r.chatViewport.Width()
	if vpWidth < 1 {
		vpWidth = 80
	}

	var b strings.Builder
	var prevRole chatRole
	for i, m := range r.messages {
		if i > 0 {
			if prevRole == roleSplash {
				b.WriteString("\n\n\n\n")
			} else if (prevRole == roleUser && m.role == roleAssistant) ||
				(prevRole == roleAssistant && m.role == roleUser) {
				b.WriteString("\n\n")
			} else {
				b.WriteByte('\n')
			}
		}
		switch m.role {
		case roleSplash:
			splash := components.RenderSplash(vpWidth, cs)
			if len(r.messages) == 1 {
				vpHeight := r.chatViewport.Height()
				splashH := components.SplashLines(vpWidth)
				topPad := (vpHeight - splashH) / 2
				if topPad > 0 {
					b.WriteString(strings.Repeat("\n", topPad))
				}
			}
			b.WriteString(splash)
		case roleUser:
			bubbleStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#06b6d4")).
				Background(lipgloss.Color("#1E0A3C")).
				Padding(0, 1)
			if cs == tui.ColorNone {
				bubbleStyle = lipgloss.NewStyle().Bold(true).Reverse(true).Padding(0, 1)
			} else if cs == tui.Color16Color {
				bubbleStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Cyan).
					Background(lipgloss.BrightBlack).
					Padding(0, 1)
			}
			maxW := vpWidth*70/100 - 2
			if maxW < 20 {
				maxW = 20
			}
			if maxW > vpWidth-4 {
				maxW = vpWidth - 4
			}
			if maxW < 1 {
				maxW = 1
			}
			wrapStyle := lipgloss.NewStyle().Width(maxW)
			wrapped := wrapStyle.Render("> " + m.text)
			lines := strings.Split(wrapped, "\n")
			for j, line := range lines {
				trimmed := strings.TrimRight(line, " ")
				styled := bubbleStyle.Render(trimmed)
				bw := ansi.StringWidth(styled)
				pad := vpWidth - bw
				if pad < 0 {
					pad = 0
				}
				if j > 0 {
					b.WriteByte('\n')
				}
				b.WriteString(strings.Repeat(" ", pad) + styled)
			}
		case roleAssistant:
			b.WriteString(r.renderAssistantRich(m.text, vpWidth))
		case roleTool:
			b.WriteString(r.renderToolBlock(m, vpWidth))
		case roleStatus:
			b.WriteString(ansi.Wrap(textMutedStyle.Render(m.text), vpWidth, ""))
		}
		prevRole = m.role
	}
	return b.String()
}

func (r *REPLPanel) renderAssistantRich(text string, width int) string {
	if text == "" {
		return ""
	}
	var b strings.Builder
	var markdown strings.Builder
	lines := strings.Split(text, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if isDiffFence(trimmed) {
			if markdown.Len() > 0 {
				b.WriteString(r.markdownView.Render(strings.TrimSuffix(markdown.String(), "\n"), width))
				b.WriteByte('\n')
				markdown.Reset()
			}
			var diff strings.Builder
			for i++; i < len(lines); i++ {
				if strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
					break
				}
				diff.WriteString(lines[i])
				if i < len(lines)-1 {
					diff.WriteByte('\n')
				}
			}
			b.WriteString(r.renderUnifiedDiffBlock(diff.String(), width))
			if i < len(lines)-1 {
				b.WriteByte('\n')
			}
			continue
		}
		markdown.WriteString(line)
		if i < len(lines)-1 {
			markdown.WriteByte('\n')
		}
	}
	if markdown.Len() > 0 {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		remaining := strings.TrimSuffix(markdown.String(), "\n")
		if looksLikeUnifiedDiff(remaining) {
			b.WriteString(r.renderUnifiedDiffBlock(remaining, width))
		} else {
			b.WriteString(r.markdownView.Render(remaining, width))
		}
	}
	return b.String()
}

func isDiffFence(line string) bool {
	line = strings.TrimPrefix(line, "```")
	lang := strings.ToLower(strings.TrimSpace(line))
	return lang == "diff" || lang == "patch" || lang == "udiff"
}

func looksLikeUnifiedDiff(text string) bool {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "diff --git ") {
		return true
	}
	return strings.Contains(trimmed, "\n@@ ") &&
		(strings.Contains(trimmed, "\n--- ") || strings.Contains(trimmed, "\n+++ "))
}

func (r *REPLPanel) renderUnifiedDiffBlock(diff string, width int) string {
	cs := r.renderConfig.Color
	noColor := cs == tui.ColorNone || r.renderConfig.Verbosity == tui.VerbosityAccessible
	addStyle := r.theme.Secondary.Resolve(cs).Bold(true)
	delStyle := r.theme.Error.Resolve(cs).Faint(true)
	mutedStyle := r.theme.TextMuted.Resolve(cs)
	headingStyle := r.theme.Heading.Resolve(cs)

	lines := strings.Split(strings.TrimSuffix(diff, "\n"), "\n")
	var b strings.Builder
	for i, line := range lines {
		rendered := line
		if !noColor {
			switch {
			case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
				rendered = headingStyle.Render(line)
			case strings.HasPrefix(line, "+"):
				rendered = addStyle.Render(line)
			case strings.HasPrefix(line, "-"):
				rendered = delStyle.Render(line)
			case strings.HasPrefix(line, "@@"), strings.HasPrefix(line, "diff --git "):
				rendered = mutedStyle.Render(line)
			default:
				rendered = mutedStyle.Render(line)
			}
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(ansi.Truncate(rendered, width, "…"))
	}
	return b.String()
}

func toolCollapsedLines(m chatMessage) []string {
	var lines []string
	if m.toolInput != "" {
		lines = append(lines, "input: "+singleLine(m.toolInput, 120))
	}
	if m.toolOutput != "" {
		lines = append(lines, "output: "+singleLine(m.toolOutput, 120))
	}
	if len(lines) == 0 {
		lines = append(lines, "status: "+m.toolStatus)
	}
	return lines
}

func toolDisplayName(name string) string {
	if strings.HasPrefix(name, "⚙ ") {
		return name
	}
	return "⚙ " + name
}

func buildToolDetail(m chatMessage) string {
	var b strings.Builder
	if m.toolInput != "" {
		b.WriteString("input:\n")
		b.WriteString(m.toolInput)
	}
	if m.detail != "" && m.detail != m.toolInput {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("detail:\n")
		b.WriteString(m.detail)
	}
	if m.toolOutput != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("output:\n")
		b.WriteString(m.toolOutput)
	}
	return b.String()
}

func toolInputPreview(tool string, input []byte) string {
	trimmed := strings.TrimSpace(string(input))
	if trimmed == "" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(input, &payload); err != nil {
		return trimmed
	}
	if tool == "bash" {
		if command, ok := payload["command"].(string); ok {
			return "command: " + command
		}
	}
	if path, ok := payload["path"].(string); ok {
		switch tool {
		case "file_edit":
			oldText, _ := payload["old_string"].(string)
			newText, _ := payload["new_string"].(string)
			return "path: " + path + "\n" + inlineEditDiff(path, oldText, newText)
		case "file_write":
			content, _ := payload["content"].(string)
			return "path: " + path + "\ncontent:\n" + content
		default:
			return "path: " + path
		}
	}
	pretty, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return trimmed
	}
	return string(pretty)
}

func inlineEditDiff(path, oldText, newText string) string {
	var b strings.Builder
	b.WriteString("--- a/")
	b.WriteString(path)
	b.WriteString("\n+++ b/")
	b.WriteString(path)
	b.WriteByte('\n')
	for _, line := range strings.Split(strings.TrimSuffix(oldText, "\n"), "\n") {
		if line != "" {
			b.WriteString("-")
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	for _, line := range strings.Split(strings.TrimSuffix(newText, "\n"), "\n") {
		if line != "" {
			b.WriteString("+")
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func singleLine(s string, limit int) string {
	s = strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	if limit > 0 && len([]rune(s)) > limit {
		return string([]rune(s)[:limit]) + "…"
	}
	return s
}

func parseExitCode(output string) (int, bool) {
	match := bashExitCodeRE.FindStringSubmatch(output)
	if len(match) != 2 {
		return 0, false
	}
	code, err := strconv.Atoi(match[1])
	return code, err == nil
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

// View renders the REPL panel with slash overlay inline above the text input.
func (r *REPLPanel) View() string {
	var content strings.Builder
	content.WriteString(r.chatViewport.View())
	agentView := r.agentStatus.Render(r.width)
	if agentView != "" {
		content.WriteByte('\n')
		content.WriteString(agentView)
	} else if r.spinner.active {
		content.WriteByte('\n')
		cs := r.renderConfig.Color
		accentStyle := r.theme.Accent.Resolve(cs)
		content.WriteString(accentStyle.Render(r.spinner.render()))
	}
	content.WriteByte('\n')
	divW := r.width
	if r.hasBorder {
		divW -= 2
	}
	if divW < 1 {
		divW = 1
	}
	cs := r.renderConfig.Color
	borderStyle := r.theme.Border.Resolve(cs)
	divChar := "─"
	if r.renderConfig.Borders == tui.BorderASCII {
		divChar = "-"
	} else if r.renderConfig.Borders == tui.BorderNone {
		divChar = "-"
	}
	content.WriteString(borderStyle.Render(strings.Repeat(divChar, divW)))

	// Render slash overlay inline between divider and text input.
	overlayView := ""
	if r.slashOverlay != nil && r.slashOverlay.IsVisible() {
		overlayView = r.slashOverlay.View()
	}
	if overlayView != "" {
		content.WriteByte('\n')
		content.WriteString(strings.TrimRight(overlayView, "\n"))
	}

	content.WriteByte('\n')
	content.WriteString(r.textInput.View())

	r.panel.SetContent(content.String())
	r.panel.SetSize(r.width, r.height)
	panelView := r.panel.Render()

	// Register hitmap for inline overlay click detection.
	// Compute from panelView (post-wrapping) by counting backward from the end.
	if overlayView != "" {
		panelNewlines := strings.Count(panelView, "\n")
		overlayLines := strings.Count(overlayView, "\n")
		linesAfterOverlay := 1
		if r.hasBorder {
			linesAfterOverlay = 2
		}
		firstItemY := panelNewlines - linesAfterOverlay - overlayLines + 1
		if firstItemY < 0 {
			firstItemY = 0
		}
		r.slashOverlay.RegisterHitmap(firstItemY)
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
	vpHeight := height - 4
	if r.hasBorder {
		vpHeight -= 2
	}

	// Propagate size to slash overlay and reserve viewport space when visible.
	if r.slashOverlay != nil {
		overlayH := height / 2
		if overlayH > 14 {
			overlayH = 14
		}
		if overlayH < 3 {
			overlayH = 3
		}
		if r.slashOverlay.IsVisible() && overlayH >= vpHeight {
			overlayH = vpHeight - 1
			if overlayH < 3 {
				overlayH = 3
			}
		}
		overlayW := vpWidth
		r.slashOverlay.SetSize(overlayW, overlayH)
		if r.slashOverlay.IsVisible() {
			vpHeight -= overlayH
		}
	}

	if vpHeight < 1 {
		vpHeight = 1
	}
	r.chatViewport.SetWidth(vpWidth)
	r.chatViewport.SetHeight(vpHeight)
	r.refreshChatViewport()
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
				r.SetSize(r.width, r.height)
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

	wasVisible := r.slashOverlay.IsVisible()
	if strings.HasPrefix(val, "/") && !strings.Contains(val, " ") {
		// Reload skills dynamically on "/" keystroke (AC#7 — Option A).
		if !wasVisible {
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
	if r.slashOverlay.IsVisible() != wasVisible {
		r.SetSize(r.width, r.height)
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
