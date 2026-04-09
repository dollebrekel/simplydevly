// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package statusline

import (
	"fmt"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/tui"
)

// Compile-time interface check.
var _ tui.StatusRenderer = (*StatusBar)(nil)

// BarState represents the visual state of the status bar.
type BarState int

const (
	StateNormal  BarState = iota
	StateWarning
	StateError
)

// Segment represents a single section of the status bar.
type Segment struct {
	Key      string
	Value    string
	Style    tui.Token
	Priority int // Higher = kept longer on narrow terminals.
}

// StatusBar is a pure rendering component for the bottom status line.
// It does NOT implement tea.Model — App.View() calls Render() directly.
type StatusBar struct {
	theme        tui.Theme
	renderConfig tui.RenderConfig
	width    int
	segments []Segment
	state        BarState
	profile      string
	hintText     string
}

// NewStatusBar creates a StatusBar configured for the given profile.
// Profile "minimal" shows model + permission only; "standard" shows all segments.
func NewStatusBar(theme tui.Theme, config tui.RenderConfig, profile string) *StatusBar {
	sb := &StatusBar{
		theme:        theme,
		renderConfig: config,
		width:        80,
		profile:      profile,
		hintText:     "Ctrl+Space: Menu",
	}
	sb.segments = sb.defaultSegments()
	return sb
}

// defaultSegments returns the initial segment list based on the profile.
func (sb *StatusBar) defaultSegments() []Segment {
	all := []Segment{
		{Key: "model", Value: "", Style: sb.theme.Text, Priority: 1},
		{Key: "permission", Value: "default", Style: sb.theme.TextMuted, Priority: 2},
		{Key: "cost", Value: "$0.00", Style: sb.theme.Text, Priority: 3},
		{Key: "tokens", Value: "0", Style: sb.theme.Text, Priority: 4},
		{Key: "workspace", Value: "", Style: sb.theme.Text, Priority: 5},
		{Key: "hints", Value: sb.hintText, Style: sb.theme.Muted, Priority: 6},
	}

	if sb.profile == "minimal" {
		// Minimal: model + permission only.
		return all[:2]
	}
	return all
}

// SetSize updates the status bar width. The compact parameter is accepted
// for interface compatibility but width-responsive fitting handles adaptation.
func (sb *StatusBar) SetSize(width int, _ bool) {
	if width < 1 {
		width = 1
	}
	sb.width = width
}

// SetState sets the visual state (normal, warning, error).
func (sb *StatusBar) SetState(state BarState) {
	sb.state = state
}

// SetPermissionMode updates the permission mode display and color.
func (sb *StatusBar) SetPermissionMode(mode string) {
	var style tui.Token
	switch mode {
	case "auto-accept":
		style = sb.theme.Success
	case "yolo":
		style = sb.theme.Warning
	default:
		mode = "default"
		style = sb.theme.TextMuted
	}
	sb.updateSegment("permission", mode, style)
}

// SetUpdateHint sets an update notification hint.
func (sb *StatusBar) SetUpdateHint(count int) {
	if count <= 0 {
		sb.hintText = "Ctrl+Space: Menu"
	} else {
		hint := fmt.Sprintf("%d items have updates", count)
		if count == 1 {
			hint = "1 item has updates"
		}
		if sb.renderConfig.Emoji {
			hint = "📦 " + hint
		}
		sb.hintText = hint
	}
	sb.updateSegment("hints", sb.hintText, sb.theme.Muted)
}

// SetProfile switches the active profile at runtime.
func (sb *StatusBar) SetProfile(profile string) {
	sb.profile = profile
	sb.segments = sb.defaultSegments()
}

// SetSegments replaces the current segment list. Satisfies [tui.StatusRenderer].
func (sb *StatusBar) SetSegments(segments []any) {
	result := make([]Segment, 0, len(segments))
	for _, s := range segments {
		if seg, ok := s.(Segment); ok {
			result = append(result, seg)
		}
	}
	sb.segments = result
}

// HandleUpdate processes a StatusUpdate from the StatusCollector.
func (sb *StatusBar) HandleUpdate(update core.StatusUpdate) {
	switch update.Source {
	case "provider":
		if v, ok := update.Metrics["model"]; ok {
			if s, ok := v.(string); ok {
				sb.updateSegment("model", s, sb.theme.Text)
			}
		}
		if v, ok := update.Metrics["tokens_in"]; ok {
			sb.updateSegment("tokens", fmt.Sprintf("%v", v), sb.theme.Text)
		}
		if v, ok := update.Metrics["cost_usd"]; ok {
			if f, ok := v.(float64); ok {
				sb.updateSegment("cost", fmt.Sprintf("$%.2f", f), sb.theme.Text)
			}
		}
	case "agent":
		if v, ok := update.Metrics["context_percentage"]; ok {
			sb.updateSegment("workspace", fmt.Sprintf("%v%%", v), sb.theme.TextMuted)
		}
	case "system":
		// System metrics can trigger warning/error states.
	}
}

// Render produces the status bar string for the given width.
// Returns empty string when the bar should not be shown.
func (sb *StatusBar) Render(width int) string {
	if width < 1 {
		return ""
	}

	if sb.renderConfig.Verbosity == tui.VerbosityAccessible {
		return sb.renderAccessible(width)
	}

	return sb.renderStyled(width)
}

// renderStyled produces the colored, segmented status bar.
func (sb *StatusBar) renderStyled(width int) string {
	cs := sb.renderConfig.Color
	sepStyle := sb.theme.Border.Resolve(cs)
	sep := sepStyle.Render(" │ ")

	// Collect visible segments (non-empty values) sorted by priority.
	visible := sb.visibleSegments()
	if len(visible) == 0 {
		return ""
	}

	// Width-responsive: drop from right (lowest priority = highest number) until it fits.
	rendered := sb.fitSegments(visible, sep, cs, width)
	if rendered == "" {
		return ""
	}

	// Apply state background.
	rendered = sb.applyStateBackground(rendered, cs)

	// Pad to full width.
	renderedWidth := ansi.StringWidth(rendered)
	if renderedWidth < width {
		rendered += strings.Repeat(" ", width-renderedWidth)
	}

	return rendered
}

// renderAccessible produces a plain-text, labeled status bar.
func (sb *StatusBar) renderAccessible(width int) string {
	visible := sb.visibleSegments()
	parts := make([]string, 0, len(visible))
	for _, seg := range visible {
		label := strings.ToUpper(seg.Key)
		parts = append(parts, fmt.Sprintf("[%s: %s]", label, seg.Value))
	}

	line := strings.Join(parts, " ")

	// Truncate if too wide (rune-safe).
	if ansi.StringWidth(line) > width {
		line = ansi.Truncate(line, width, "")
	}

	return line
}

// visibleSegments returns segments with non-empty values, ordered by priority (ascending).
func (sb *StatusBar) visibleSegments() []Segment {
	result := make([]Segment, 0, len(sb.segments))
	for _, s := range sb.segments {
		if s.Value != "" {
			result = append(result, s)
		}
	}
	return result
}

// fitSegments tries to fit segments within width, dropping lowest priority first.
// Model (P1) and permission (P2) are never dropped per spec.
func (sb *StatusBar) fitSegments(segs []Segment, sep string, cs tui.ColorSetting, width int) string {
	// Count undropable segments (priority <= 2).
	minKeep := 0
	for _, seg := range segs {
		if seg.Priority <= 2 {
			minKeep++
		}
	}
	if minKeep == 0 {
		minKeep = 1
	}

	for len(segs) > minKeep {
		parts := make([]string, len(segs))
		for i, seg := range segs {
			style := seg.Style.Resolve(cs)
			parts[i] = style.Render(seg.Value)
		}

		line := strings.Join(parts, sep)
		if ansi.StringWidth(line) <= width {
			return line
		}

		// Drop the last segment (lowest priority = highest Priority number).
		segs = segs[:len(segs)-1]
	}

	// Render the remaining undropable segments.
	parts := make([]string, len(segs))
	for i, seg := range segs {
		style := seg.Style.Resolve(cs)
		parts[i] = style.Render(seg.Value)
	}
	return strings.Join(parts, sep)
}

// applyStateBackground wraps the rendered line with a background color for warning/error.
func (sb *StatusBar) applyStateBackground(line string, cs tui.ColorSetting) string {
	switch sb.state {
	case StateWarning:
		bg := sb.warningBgStyle(cs)
		return bg.Render(line)
	case StateError:
		bg := sb.errorBgStyle(cs)
		return bg.Render(line)
	default:
		return line
	}
}

// warningBgStyle returns a style with warning background color.
func (sb *StatusBar) warningBgStyle(cs tui.ColorSetting) lipgloss.Style {
	switch cs {
	case tui.ColorTrueColor, tui.Color256Color:
		return lipgloss.NewStyle().Background(lipgloss.Color(hexWarning))
	case tui.Color16Color:
		return lipgloss.NewStyle().Background(lipgloss.Yellow)
	default:
		return lipgloss.NewStyle().Reverse(true)
	}
}

// errorBgStyle returns a style with error background color.
func (sb *StatusBar) errorBgStyle(cs tui.ColorSetting) lipgloss.Style {
	switch cs {
	case tui.ColorTrueColor, tui.Color256Color:
		return lipgloss.NewStyle().Background(lipgloss.Color(hexError))
	case tui.Color16Color:
		return lipgloss.NewStyle().Background(lipgloss.Red)
	default:
		return lipgloss.NewStyle().Reverse(true)
	}
}

// updateSegment updates the value and style of a segment by key.
func (sb *StatusBar) updateSegment(key, value string, style tui.Token) {
	for i := range sb.segments {
		if sb.segments[i].Key == key {
			sb.segments[i].Value = value
			sb.segments[i].Style = style
			return
		}
	}
}

// Hex constants for background colors (reuse from tokens.go).
const (
	hexWarning = "#E0AF68"
	hexError   = "#F7768E"
)
