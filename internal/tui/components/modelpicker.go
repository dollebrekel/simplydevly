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
	lines := p.lines(bodyW)
	if len(lines) > height-2 && height > 2 {
		lines = lines[:height-2]
	}
	content := strings.Join(lines, "\n")
	return tui.RenderBorder("/model", content, p.renderConfig, p.theme, width)
}

func (p *ModelPicker) lines(width int) []string {
	if p.loading {
		return []string{p.muted("Loading models...")}
	}
	if p.err != nil {
		return []string{
			p.errorText("Could not load models"),
			p.muted(p.err.Error()),
		}
	}
	if len(p.options) == 0 {
		return []string{p.muted("No models configured")}
	}

	var out []string
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
		out = append(out, p.optionLine(width, i, opt))
	}
	out = append(out, "", p.muted("up/down select  enter confirm  esc close"))
	return out
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
	// The provider name is shown in the group heading, so each row only needs
	// the model name (avoids "anthropic/claude-..." duplication under the
	// "Anthropic" heading). Story 12.13 D2, Task 3.
	line := prefix + opt.Model + active
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
