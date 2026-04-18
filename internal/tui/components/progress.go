// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package components

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/spinner"
	"siply.dev/siply/internal/tui"
)

// ProgressIndicator is a Bubble Tea model for showing progress with a spinner.
// When Motion == MotionStatic, it renders as a static indicator without ticks.
type ProgressIndicator struct {
	spinner      spinner.Model
	label        string
	startTime    time.Time
	done         bool
	result       string
	duration     time.Duration
	theme        *tui.Theme
	renderConfig *tui.RenderConfig
}

// NewProgressIndicator creates a new progress indicator.
func NewProgressIndicator(label string, theme *tui.Theme, rc *tui.RenderConfig) *ProgressIndicator {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return &ProgressIndicator{
		spinner:      s,
		label:        label,
		startTime:    time.Now(),
		theme:        theme,
		renderConfig: rc,
	}
}

// Init returns the initial command. Only starts spinner tick when Motion == MotionSpinner
// and not in accessible mode (accessible rendering is always static).
func (p *ProgressIndicator) Init() tea.Cmd {
	if p.renderConfig.Motion == tui.MotionStatic || p.renderConfig.Verbosity == tui.VerbosityAccessible {
		return nil
	}
	return p.spinner.Tick
}

// Update handles spinner tick messages.
// Returns only tea.Cmd (SubPanel pattern) — ProgressIndicator is a render helper,
// not a standalone tea.Model (View() would need tea.View for BubbleTea v2).
func (p *ProgressIndicator) Update(msg tea.Msg) tea.Cmd {
	if p.done {
		return nil
	}
	if p.renderConfig.Motion == tui.MotionStatic || p.renderConfig.Verbosity == tui.VerbosityAccessible {
		return nil
	}
	var cmd tea.Cmd
	p.spinner, cmd = p.spinner.Update(msg)
	return cmd
}

// Render renders the progress indicator at the given width.
func (p *ProgressIndicator) Render(width int) string {
	if width < 1 {
		width = 80
	}

	if p.renderConfig.Verbosity == tui.VerbosityAccessible {
		return p.renderAccessible(width)
	}

	cs := p.renderConfig.Color

	if p.done {
		var prefix string
		if p.renderConfig.Emoji {
			prefix = "\u2705 "
		} else {
			prefix = "OK: "
		}
		dur := formatDuration(p.duration)
		line := p.theme.Success.Resolve(cs).Render(prefix + p.label + " (" + dur + ")")
		return wrapLine(line, width)
	}

	// In progress.
	if p.renderConfig.Motion == tui.MotionStatic {
		line := fmt.Sprintf("[...] %s", p.label)
		return wrapLine(line, width)
	}

	line := p.spinner.View() + " " + p.label
	return wrapLine(line, width)
}

// renderAccessible renders in accessible mode.
func (p *ProgressIndicator) renderAccessible(width int) string {
	if p.done {
		dur := formatDuration(p.duration)
		line := "[DONE] " + p.label + " (" + dur + ")"
		return wrapLine(line, width)
	}
	line := "[...] " + p.label
	return wrapLine(line, width)
}

// Complete marks the progress indicator as done and captures duration.
func (p *ProgressIndicator) Complete(result string) {
	p.done = true
	p.result = result
	p.duration = time.Since(p.startTime)
}

// CompleteWithDuration marks complete with a specific duration (useful for testing).
func (p *ProgressIndicator) CompleteWithDuration(result string, d time.Duration) {
	p.done = true
	p.result = result
	p.duration = d
}

// IsDone returns whether the indicator has completed.
func (p *ProgressIndicator) IsDone() bool {
	return p.done
}

// Label returns the label of the progress indicator.
func (p *ProgressIndicator) Label() string {
	return p.label
}

// wrapLine is defined in feedback.go — reused here.
