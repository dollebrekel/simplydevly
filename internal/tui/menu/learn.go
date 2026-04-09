// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package menu

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/mattn/go-runewidth"

	tui "siply.dev/siply/internal/tui"
)

// LearnView renders keybindings organized by category using MarkdownRenderer.
type LearnView struct {
	categories   []KeyBindingCategory
	theme        tui.Theme
	renderConfig tui.RenderConfig
	markdownView tui.MarkdownRenderer
	scrollOffset int
	width        int
	height       int
}

// NewLearnView creates a new LearnView with default keybindings.
func NewLearnView(theme tui.Theme, renderConfig tui.RenderConfig, markdownView tui.MarkdownRenderer) *LearnView {
	return &LearnView{
		categories:   DefaultKeyBindings(),
		theme:        theme,
		renderConfig: renderConfig,
		markdownView: markdownView,
		width:        40,
		height:       20,
	}
}

// generateMarkdown converts categories into markdown with two-column format (FR45).
func (lv *LearnView) generateMarkdown() string {
	accessible := lv.renderConfig.Verbosity == tui.VerbosityAccessible

	// Find max key display width for alignment (using runewidth for Unicode).
	maxKeyWidth := 0
	for _, cat := range lv.categories {
		for _, kb := range cat.Bindings {
			w := runewidth.StringWidth(kb.Key)
			if w > maxKeyWidth {
				maxKeyWidth = w
			}
		}
	}

	var b strings.Builder
	for i, cat := range lv.categories {
		if i > 0 {
			b.WriteByte('\n')
		}
		if accessible {
			b.WriteString(fmt.Sprintf("## [SECTION] %s\n", cat.Name))
		} else {
			b.WriteString(fmt.Sprintf("## %s\n", cat.Name))
		}
		b.WriteByte('\n')
		for _, kb := range cat.Bindings {
			padding := strings.Repeat(" ", maxKeyWidth-runewidth.StringWidth(kb.Key))
			if accessible {
				b.WriteString(fmt.Sprintf("%s%s → %s\n", kb.Key, padding, kb.Action))
			} else {
				b.WriteString(fmt.Sprintf("`%s`%s → %s\n", kb.Key, padding, kb.Action))
			}
		}
	}

	return b.String()
}

// Render returns the rendered learn view content within a border.
func (lv *LearnView) Render(width, height int) string {
	w := lv.width
	h := lv.height

	md := lv.generateMarkdown()
	innerWidth := w - 4 // Account for border
	if innerWidth < 1 {
		innerWidth = 1
	}
	rendered := lv.markdownView.Render(md, innerWidth)
	lines := strings.Split(rendered, "\n")

	// Apply scroll offset.
	if lv.scrollOffset > 0 && lv.scrollOffset < len(lines) {
		lines = lines[lv.scrollOffset:]
	}

	contentHeight := h - 2 // Account for border + title
	if contentHeight < 1 {
		contentHeight = 1
	}
	if len(lines) > contentHeight {
		lines = lines[:contentHeight]
	}

	content := strings.Join(lines, "\n")

	title := "[LEARN — Keybindings]"
	if lv.renderConfig.Verbosity != tui.VerbosityAccessible {
		cs := lv.renderConfig.Color
		headingStyle := lv.theme.Heading.Resolve(cs)
		title = headingStyle.Render("Learn — Keybindings")
	}

	return tui.RenderBorder(title, content, lv.renderConfig, lv.theme, w)
}

// HandleKey processes key events for scrolling and closing.
func (lv *LearnView) HandleKey(key string) tea.Msg {
	switch key {
	case "up", "k":
		if lv.scrollOffset > 0 {
			lv.scrollOffset--
		}
		return nil
	case "down", "j":
		lv.scrollOffset++
		lv.clampScrollOffset()
		return nil
	case "esc":
		return tui.LearnCloseMsg{}
	default:
		return nil
	}
}

// SetSize updates the learn view dimensions with minimum clamping.
func (lv *LearnView) SetSize(width, height int) {
	if width < 20 {
		width = 20
	}
	if height < 5 {
		height = 5
	}
	lv.width = width
	lv.height = height
	lv.clampScrollOffset()
}

// SetCategories allows replacing the keybinding categories (future extensibility).
func (lv *LearnView) SetCategories(categories []KeyBindingCategory) {
	lv.categories = categories
	lv.scrollOffset = 0
}

// maxScrollOffset calculates the maximum scroll offset based on content.
func (lv *LearnView) maxScrollOffset() int {
	md := lv.generateMarkdown()
	innerWidth := lv.width - 4
	if innerWidth < 1 {
		innerWidth = 1
	}
	rendered := lv.markdownView.Render(md, innerWidth)
	lines := strings.Split(rendered, "\n")

	contentHeight := lv.height - 2
	if contentHeight < 1 {
		contentHeight = 1
	}

	maxOffset := len(lines) - contentHeight
	if maxOffset < 0 {
		return 0
	}
	return maxOffset
}

// clampScrollOffset ensures scrollOffset doesn't exceed content bounds.
func (lv *LearnView) clampScrollOffset() {
	maxOff := lv.maxScrollOffset()
	if lv.scrollOffset > maxOff {
		lv.scrollOffset = maxOff
	}
	if lv.scrollOffset < 0 {
		lv.scrollOffset = 0
	}
}
