// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package components

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"siply.dev/siply/internal/tui"
)

var _ tui.ModelPicker = (*ModelPicker)(nil)

// ModelPicker renders the /model selection overlay.
type ModelPicker struct {
	theme        tui.Theme
	renderConfig tui.RenderConfig
	width        int
	height       int
	open         bool
	loading      bool
	options      []tui.ModelOption
	err          error
	cursor       int
}

// NewModelPicker creates the model picker overlay.
func NewModelPicker(theme tui.Theme, rc tui.RenderConfig) *ModelPicker {
	return &ModelPicker{
		theme:        theme,
		renderConfig: rc,
		width:        48,
		height:       14,
	}
}

func (p *ModelPicker) IsOpen() bool { return p.open }

func (p *ModelPicker) OpenLoading() {
	p.open = true
	p.loading = true
	p.err = nil
	p.options = nil
	p.cursor = 0
}

func (p *ModelPicker) Close() {
	p.open = false
	p.loading = false
}

func (p *ModelPicker) SetSize(width, height int) {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	p.width = width
	p.height = height
}

func (p *ModelPicker) SetOptions(options []tui.ModelOption, err error) {
	p.loading = false
	p.err = err
	p.options = append([]tui.ModelOption(nil), options...)
	p.cursor = firstEnabledIndex(p.options)
}

func (p *ModelPicker) HandleKey(key string) tea.Msg {
	if !p.open {
		return nil
	}
	switch key {
	case "esc":
		p.Close()
		return nil
	case "up", "k":
		p.move(-1)
		return nil
	case "down", "j":
		p.move(1)
		return nil
	case "enter":
		if p.cursor >= 0 && p.cursor < len(p.options) {
			opt := p.options[p.cursor]
			if !opt.Disabled {
				return tui.ModelSelectedMsg{Option: opt}
			}
		}
		return nil
	default:
		return nil
	}
}

func (p *ModelPicker) Render(width, height int) string {
	if !p.open {
		return ""
	}
	if width < 1 || height < 1 {
		return ""
	}
	bodyW := width - 4
	if bodyW < 20 {
		bodyW = width
	}

	lines, cursorLine := p.contentLines(bodyW)

	// RenderBorder adds a top and bottom border row, so the body gets height-2.
	visible := max(height-2, 1)

	// Pin the hint footer to the bottom when there's room for it plus at least
	// one content row. The footer never competes with the selection for scroll
	// space, so the cursor stays reachable even in a short window.
	footer := ""
	listH := visible
	if cursorLine >= 0 && visible >= 3 {
		listH = visible - 1
		footer = p.hintFooter(len(lines) > listH)
	}
	listH = max(listH, 1)

	// Scroll the window so the cursor row is always visible (the previous
	// implementation truncated from the top, hiding the selection off-screen).
	off := 0
	if cursorLine >= 0 {
		if cursorLine >= listH {
			off = cursorLine - listH + 1
		}
		maxOff := max(len(lines)-listH, 0)
		off = min(off, maxOff)
	}
	end := min(off+listH, len(lines))
	window := lines[off:end]

	content := strings.Join(window, "\n")
	if footer != "" {
		content += "\n" + footer
	}
	return tui.RenderBorder("/model", content, p.renderConfig, p.theme, width)
}

func (p *ModelPicker) hintFooter(scrollable bool) string {
	hint := "up/down select  enter confirm  esc close"
	if scrollable {
		hint += "  ·  ↑↓ more"
	}
	return p.muted(hint)
}

// contentLines builds the scrollable body — group headings plus option rows,
// without the hint footer — and returns the index within that slice of the row
// holding the current cursor. cursorLine is -1 for states that have no
// selectable row (loading, error, empty), so the caller skips scroll handling.
func (p *ModelPicker) contentLines(width int) ([]string, int) {
	if p.loading {
		return []string{p.muted("Loading models...")}, -1
	}
	if p.err != nil {
		return []string{
			p.errorText("Could not load models"),
			p.muted(p.err.Error()),
		}, -1
	}
	if len(p.options) == 0 {
		return []string{p.muted("No models configured")}, -1
	}

	var out []string
	cursorLine := 0
	lastGroup := ""
	for i, opt := range p.options {
		group := groupKeyFor(opt)
		if group != lastGroup {
			if len(out) > 0 {
				out = append(out, "")
			}
			out = append(out, p.heading(titleForGroup(opt)))
			lastGroup = group
		}
		if i == p.cursor {
			cursorLine = len(out)
		}
		out = append(out, p.optionLine(width, i, opt))
	}
	return out, cursorLine
}

func (p *ModelPicker) optionLine(width, index int, opt tui.ModelOption) string {
	prefix := "  "
	if index == p.cursor {
		prefix = "> "
	}
	active := ""
	if opt.Active {
		active = " (active)"
	}
	// The provider name is shown in the group heading, so each row shows the
	// human-readable model name (falling back to the raw ID when no friendly name
	// is set, e.g. an injected active model) plus its category. This avoids
	// "anthropic/claude-..." duplication under the "Anthropic" heading.
	// Story 12.13 D2, Task 3; model-catalog-restructure.
	name := opt.Name
	if name == "" {
		name = opt.Model
	}
	line := prefix + name + active
	if opt.Category != "" {
		line += "  " + opt.Category
	}
	if opt.Description != "" {
		line += "  " + opt.Description
	}
	line = ansi.Truncate(line, width, "...")
	if opt.Disabled {
		return p.muted(line)
	}
	if index == p.cursor {
		if p.renderConfig.Color == tui.ColorNone {
			return lipgloss.NewStyle().Reverse(true).Render(line)
		}
		return p.theme.Primary.Resolve(p.renderConfig.Color).Bold(true).Render(line)
	}
	if opt.Active {
		return p.theme.Success.Resolve(p.renderConfig.Color).Render(line)
	}
	return p.theme.Text.Resolve(p.renderConfig.Color).Render(line)
}

func (p *ModelPicker) move(delta int) {
	if len(p.options) == 0 {
		return
	}
	next := p.cursor
	for range p.options {
		next += delta
		if next < 0 {
			next = len(p.options) - 1
		}
		if next >= len(p.options) {
			next = 0
		}
		if !p.options[next].Disabled {
			p.cursor = next
			return
		}
	}
}

func firstEnabledIndex(options []tui.ModelOption) int {
	for i, opt := range options {
		if opt.Active && !opt.Disabled {
			return i
		}
	}
	for i, opt := range options {
		if !opt.Disabled {
			return i
		}
	}
	return 0
}

// groupKeyFor returns the grouping key for an option: local models share one
// "Local" group, while cloud models are grouped per provider (story 12.13 D2).
func groupKeyFor(opt tui.ModelOption) string {
	if opt.Kind == "local" {
		return "local"
	}
	return "cloud:" + opt.Provider
}

// titleForGroup returns the heading text for an option's group: "Local" for
// local models, or a display-cased provider name for cloud models.
func titleForGroup(opt tui.ModelOption) string {
	if opt.Kind == "local" {
		return "Local"
	}
	return providerDisplayName(opt.Provider)
}

// providerDisplayName maps a provider key to a human-friendly heading. Unknown
// providers fall back to a capitalized key so future providers still render.
func providerDisplayName(provider string) string {
	switch provider {
	case "anthropic":
		return "Anthropic"
	case "openai":
		return "OpenAI"
	case "google":
		return "Google"
	case "xai":
		return "xAI"
	case "deepseek":
		return "DeepSeek"
	case "mistral":
		return "Mistral"
	case "openrouter":
		return "OpenRouter"
	case "kimi":
		return "Kimi"
	case "ollama":
		return "Ollama"
	case "":
		return "Cloud"
	default:
		r := []rune(provider)
		r[0] = unicode.ToUpper(r[0])
		return string(r)
	}
}

func (p *ModelPicker) heading(text string) string {
	return p.theme.Heading.Resolve(p.renderConfig.Color).Bold(true).Render(text)
}

func (p *ModelPicker) muted(text string) string {
	return p.theme.Muted.Resolve(p.renderConfig.Color).Render(text)
}

func (p *ModelPicker) errorText(text string) string {
	return p.theme.Error.Resolve(p.renderConfig.Color).Render(text)
}
