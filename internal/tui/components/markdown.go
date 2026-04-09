// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package components

import (
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"siply.dev/siply/internal/tui"
)

// MarkdownView is a pure rendering component for basic markdown display.
// It does NOT implement tea.Model — other components call Render() directly.
type MarkdownView struct {
	theme        tui.Theme
	renderConfig tui.RenderConfig
}

// NewMarkdownView creates a MarkdownView configured with the given theme and config.
func NewMarkdownView(theme tui.Theme, config tui.RenderConfig) *MarkdownView {
	return &MarkdownView{
		theme:        theme,
		renderConfig: config,
	}
}

// Render processes markdown input and returns styled terminal output.
// All lines are truncated to the given width.
func (mv *MarkdownView) Render(input string, width int) string {
	if input == "" {
		return ""
	}
	if width < 1 {
		width = 1
	}

	cs := mv.renderConfig.Color
	accessible := mv.renderConfig.Verbosity == tui.VerbosityAccessible
	noColor := cs == tui.ColorNone

	lines := strings.Split(input, "\n")
	var b strings.Builder
	inCodeBlock := false

	for i, line := range lines {
		var rendered string

		// Fenced code block toggle.
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			if !inCodeBlock {
				// Opening fence.
				inCodeBlock = true
				if accessible {
					rendered = "[CODE]"
				} else {
					rendered = ""
				}
			} else {
				// Closing fence.
				inCodeBlock = false
				if accessible {
					rendered = "[/CODE]"
				} else {
					rendered = ""
				}
			}
		} else if inCodeBlock {
			// Inside code block — apply CodePath styling, no inline formatting.
			if accessible {
				rendered = line
			} else if noColor {
				style := mv.theme.CodePath.Resolve(cs)
				rendered = style.Render(line)
			} else {
				style := mv.theme.CodePath.Resolve(cs)
				rendered = style.Render(line)
			}
		} else if level := headingLevel(line); level > 0 {
			// Heading.
			text := strings.TrimSpace(line[level:])
			rendered = mv.renderHeading(text, level, cs, accessible, noColor)
		} else if isListItem(line) {
			// Unordered list item.
			text := strings.TrimSpace(line[2:]) // skip "- " or "* "
			rendered = mv.renderListItem(text, cs, accessible, noColor)
		} else {
			// Plain text with inline formatting.
			rendered = mv.renderInline(line, cs, accessible, noColor)
		}

		rendered = ansi.Truncate(rendered, width, "…")

		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(rendered)
	}

	return b.String()
}

// headingLevel returns the heading level (1-3) if the line starts with # markers,
// or 0 if not a heading. Only H1-H3 are supported.
func headingLevel(line string) int {
	if strings.HasPrefix(line, "### ") {
		return 3
	}
	if strings.HasPrefix(line, "## ") {
		return 2
	}
	if strings.HasPrefix(line, "# ") {
		return 1
	}
	return 0
}

// isListItem returns true if the line starts with "- " or "* ".
func isListItem(line string) bool {
	return strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ")
}

// renderHeading renders a heading line with appropriate styling.
func (mv *MarkdownView) renderHeading(text string, level int, cs tui.ColorSetting, accessible, noColor bool) string {
	if accessible {
		var tag string
		switch level {
		case 1:
			tag = "[H1]"
		case 2:
			tag = "[H2]"
		case 3:
			tag = "[H3]"
		}
		return tag + " " + text
	}
	if noColor {
		return lipgloss.NewStyle().Bold(true).Render(text)
	}
	style := mv.theme.Heading.Resolve(cs)
	return style.Render(text)
}

// renderListItem renders a list item with bullet styling.
func (mv *MarkdownView) renderListItem(text string, cs tui.ColorSetting, accessible, noColor bool) string {
	if accessible || noColor {
		return "- " + text
	}
	if !mv.renderConfig.Emoji {
		return "- " + text
	}
	bulletStyle := mv.theme.Secondary.Resolve(cs)
	return bulletStyle.Render("•") + " " + text
}

// renderInline processes inline formatting within a line: **bold**, *italic*/_italic_, `code`.
func (mv *MarkdownView) renderInline(line string, cs tui.ColorSetting, accessible, noColor bool) string {
	if accessible || noColor {
		return mv.renderInlinePlain(line, cs, noColor)
	}
	return mv.renderInlineStyled(line, cs)
}

// renderInlinePlain renders inline formatting without color (NoColor/Accessible mode).
func (mv *MarkdownView) renderInlinePlain(line string, cs tui.ColorSetting, noColor bool) string {
	var b strings.Builder
	i := 0
	runes := []rune(line)

	for i < len(runes) {
		// Bold: **text**
		if i+1 < len(runes) && runes[i] == '*' && runes[i+1] == '*' {
			end := findClosing(runes, i+2, "**")
			if end >= 0 {
				inner := string(runes[i+2 : end])
				if noColor {
					b.WriteString(lipgloss.NewStyle().Bold(true).Render(inner))
				} else {
					b.WriteString(inner)
				}
				i = end + 2
				continue
			}
		}

		// Italic: *text* or _text_
		if runes[i] == '*' || runes[i] == '_' {
			marker := runes[i]
			end := findClosingRune(runes, i+1, marker)
			if end >= 0 && (marker != '*' || end+1 >= len(runes) || runes[end+1] != '*') {
				inner := string(runes[i+1 : end])
				if inner != "" {
					if noColor {
						b.WriteString(lipgloss.NewStyle().Faint(true).Render(inner))
					} else {
						b.WriteString(inner)
					}
					i = end + 1
					continue
				}
			}
		}

		// Inline code: `text`
		if runes[i] == '`' {
			end := findClosingRune(runes, i+1, '`')
			if end >= 0 {
				inner := string(runes[i+1 : end])
				if noColor {
					style := mv.theme.CodePath.Resolve(cs)
					b.WriteString(style.Render(inner))
				} else {
					// Accessible mode: wrap with backticks for screen readers.
					b.WriteString("`" + inner + "`")
				}
				i = end + 1
				continue
			}
		}

		b.WriteRune(runes[i])
		i++
	}

	return b.String()
}

// renderInlineStyled renders inline formatting with full color styling.
func (mv *MarkdownView) renderInlineStyled(line string, cs tui.ColorSetting) string {
	var b strings.Builder
	i := 0
	runes := []rune(line)

	for i < len(runes) {
		// Bold: **text**
		if i+1 < len(runes) && runes[i] == '*' && runes[i+1] == '*' {
			end := findClosing(runes, i+2, "**")
			if end >= 0 {
				inner := string(runes[i+2 : end])
				if inner != "" {
					b.WriteString(lipgloss.NewStyle().Bold(true).Render(inner))
					i = end + 2
					continue
				}
			}
		}

		// Italic: *text* or _text_
		if runes[i] == '*' || runes[i] == '_' {
			marker := runes[i]
			end := findClosingRune(runes, i+1, marker)
			if end >= 0 && (marker != '*' || end+1 >= len(runes) || runes[end+1] != '*') {
				inner := string(runes[i+1 : end])
				if inner != "" {
					b.WriteString(lipgloss.NewStyle().Italic(true).Render(inner))
					i = end + 1
					continue
				}
			}
		}

		// Inline code: `text`
		if runes[i] == '`' {
			end := findClosingRune(runes, i+1, '`')
			if end >= 0 {
				inner := string(runes[i+1 : end])
				style := mv.theme.CodePath.Resolve(cs)
				b.WriteString(style.Render(inner))
				i = end + 1
				continue
			}
		}

		b.WriteRune(runes[i])
		i++
	}

	return b.String()
}

// findClosing finds the index of a two-character closing delimiter in runes starting from pos.
// Returns -1 if not found.
func findClosing(runes []rune, pos int, delim string) int {
	d := []rune(delim)
	for i := pos; i+1 < len(runes); i++ {
		if runes[i] == d[0] && runes[i+1] == d[1] {
			return i
		}
	}
	return -1
}

// findClosingRune finds the index of a single-character closing delimiter in runes starting from pos.
// Returns -1 if not found.
func findClosingRune(runes []rune, pos int, delim rune) int {
	for i := pos; i < len(runes); i++ {
		if runes[i] == delim {
			return i
		}
	}
	return -1
}
