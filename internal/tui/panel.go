// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tui

// Panel is a simple container component for TUI content.
// It supports bordered/borderless modes and focused/unfocused states.
// Panel is a pure rendering component — it does not implement tea.Model.
type Panel struct {
	title        string
	content      string
	focused      bool
	bordered     bool
	theme        Theme
	renderConfig RenderConfig
	width        int
	height       int
}

// NewPanel creates a new Panel with the given title, theme, and render config.
// Bordered mode is determined by the render config's border setting.
func NewPanel(title string, theme Theme, config RenderConfig) *Panel {
	return &Panel{
		title:        title,
		theme:        theme,
		renderConfig: config,
		bordered:     config.Borders != BorderNone,
		width:        40,
		height:       10,
	}
}

// SetContent updates the panel's content area.
func (p *Panel) SetContent(content string) {
	p.content = content
}

// SetFocused toggles the panel's focus state.
func (p *Panel) SetFocused(focused bool) {
	p.focused = focused
}

// SetBordered toggles the panel's border display.
func (p *Panel) SetBordered(bordered bool) {
	p.bordered = bordered
}

// SetSize updates the panel's dimensions, clamping to a minimum of 1.
func (p *Panel) SetSize(width, height int) {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	p.width = width
	p.height = height
}

// Render produces the panel's visual output.
// Focused + bordered: primary-colored border.
// Unfocused + bordered: muted border.
// Borderless: raw content, no wrapping.
func (p *Panel) Render() string {
	if !p.bordered {
		return p.content
	}
	if p.focused {
		return RenderBorderFocused(p.title, p.content, p.renderConfig, p.theme, p.width)
	}
	return RenderBorder(p.title, p.content, p.renderConfig, p.theme, p.width)
}
