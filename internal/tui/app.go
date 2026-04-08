// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// App is the root Bubble Tea Model for the siply TUI.
// It implements the Model-View-Update pattern.
type App struct {
	caps         Capabilities
	renderConfig RenderConfig
	layout       LayoutConstraints
	width        int
	height       int
	ready        bool
}

// NewApp creates a new App with the given capabilities and CLI flags.
func NewApp(caps Capabilities, flags CLIFlags) *App {
	return &App{
		caps:         caps,
		renderConfig: NewRenderConfig(caps, flags),
	}
}

// Init returns initial commands. Window size is automatically provided by
// Bubble Tea v2 at program start via WindowSizeMsg.
func (a *App) Init() tea.Cmd {
	return nil
}

// Update handles incoming messages and updates the model state.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.layout = CalculateLayout(a.width, a.height)
		a.ready = true
		return a, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return a, tea.Quit
		}
	}

	return a, nil
}

// View renders the TUI, adapting to the current layout mode.
func (a *App) View() tea.View {
	if !a.ready {
		v := tea.NewView("Initializing...")
		v.AltScreen = true
		return v
	}

	var content string
	switch a.renderConfig.Verbosity {
	case VerbosityAccessible:
		content = a.renderAccessible()
	default:
		content = a.renderStandard()
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// renderStandard renders the standard TUI view.
func (a *App) renderStandard() string {
	var b strings.Builder

	body := "Ready."
	if a.renderConfig.Emoji {
		body = "✨ Ready."
	}

	// Add layout info.
	info := fmt.Sprintf("%s | %dx%d", a.layout.Mode, a.width, a.height)
	body += "\n" + info

	if a.layout.ShowBorders && a.renderConfig.Borders != BorderNone {
		b.WriteString(RenderBorder("siply", body, a.renderConfig, a.width))
	} else {
		// Ultra-compact: no borders.
		b.WriteString("siply\n")
		b.WriteString(body)
		b.WriteByte('\n')
	}

	// Status bar placeholder.
	if a.layout.ShowStatusBar {
		statusText := "Press q to quit"
		if a.layout.CompactStatusBar {
			b.WriteString(statusText)
		} else {
			b.WriteString(statusText + " | siply " + a.layout.Mode.String())
		}
		b.WriteByte('\n')
	}

	return b.String()
}

// renderAccessible renders the accessible mode view.
// Box-drawing chars are replaced by text headers.
// Spinners are replaced by static messages.
func (a *App) renderAccessible() string {
	var b strings.Builder

	body := "Ready."
	info := fmt.Sprintf("%s | %dx%d", a.layout.Mode, a.width, a.height)
	body += "\n" + info

	b.WriteString(RenderBorder("siply", body, a.renderConfig, a.width))

	if a.layout.ShowStatusBar {
		b.WriteString("Press q to quit")
		b.WriteByte('\n')
	}

	return b.String()
}

// Run starts the Bubble Tea program. This blocks until the program exits.
func Run(caps Capabilities, flags CLIFlags) error {
	app := NewApp(caps, flags)

	var opts []tea.ProgramOption

	// SSH sessions use reduced FPS for lower bandwidth (v2 equivalent of
	// WithBatchedRenderer which was removed in Bubble Tea v2).
	if caps.SSHSession {
		opts = append(opts, tea.WithFPS(10))
	}

	p := tea.NewProgram(app, opts...)
	_, err := p.Run()
	return err
}
