// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package siplyui

import (
	"strings"
	"unicode"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

// MarkdownView is a pure renderer for markdown text in the terminal.
// Supports headings, code blocks, lists (nested), tables, horizontal rules, links, bold, italic, inline code.
type MarkdownView struct {
	theme        Theme
	renderConfig RenderConfig
}

// NewMarkdownView creates a MarkdownView with the given theme and config.
func NewMarkdownView(theme Theme, config RenderConfig) *MarkdownView {
	return &MarkdownView{theme: theme, renderConfig: config}
}

// Render processes markdown and returns styled terminal output within width.
func (mv *MarkdownView) Render(input string, width int) string {
	if input == "" {
		return ""
	}
	if width < 1 {
		width = 1
	}

	cs := mv.renderConfig.Color
	accessible := mv.renderConfig.Verbosity == VerbosityAccessible
	noColor := cs == ColorNone

	lines := strings.Split(input, "\n")
	var b strings.Builder
	inCodeBlock := false
	firstOutput := true

	writeLine := func(s string) {
		if !firstOutput {
			b.WriteByte('\n')
		}
		firstOutput = false
		b.WriteString(ansi.Truncate(s, width, "…"))
	}

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Fenced code block toggle.
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			if !inCodeBlock {
				inCodeBlock = true
				if accessible {
					writeLine("[CODE]")
				}
			} else {
				inCodeBlock = false
				if accessible {
					writeLine("[/CODE]")
				}
			}
			continue
		}
		if inCodeBlock {
			style := mv.theme.CodePath.Resolve(cs)
			writeLine(style.Render(line))
			continue
		}

		// Horizontal rule.
		if isHorizontalRule(line) {
			writeLine(mv.renderHR(width, cs, noColor))
			continue
		}

		// Heading.
		if level := headingLevel(line); level > 0 {
			text := strings.TrimSpace(line[level:])
			text = mv.renderInline(text, cs, accessible, noColor)
			writeLine(mv.renderHeading(text, level, cs, accessible, noColor))
			continue
		}

		// Pipe table.
		if isTableRow(line) {
			// Collect all consecutive table lines.
			tableLines := []string{line}
			for i+1 < len(lines) && isTableRow(lines[i+1]) {
				i++
				tableLines = append(tableLines, lines[i])
			}
			for _, tl := range mv.renderTable(tableLines, width, cs, noColor) {
				writeLine(tl)
			}
			continue
		}

		// List item (including nested).
		if indent, bullet, rest, ok := parseListItem(line); ok {
			rendered := mv.renderListItem(indent, bullet, rest, cs, accessible, noColor)
			writeLine(rendered)
			continue
		}

		// Link: [text](url)
		if strings.Contains(line, "](") {
			writeLine(mv.renderInline(line, cs, accessible, noColor))
			continue
		}

		// Plain text with inline formatting.
		writeLine(mv.renderInline(line, cs, accessible, noColor))
	}

	return b.String()
}

// isHorizontalRule returns true for lines like "---", "***", "___".
func isHorizontalRule(line string) bool {
	t := strings.TrimSpace(line)
	if len(t) < 3 {
		return false
	}
	first := t[0]
	if first != '-' && first != '*' && first != '_' {
		return false
	}
	for _, ch := range t {
		if ch != rune(first) {
			return false
		}
	}
	return true
}

func (mv *MarkdownView) renderHR(width int, cs ColorSetting, noColor bool) string {
	if noColor {
		return strings.Repeat("-", width)
	}
	style := mv.theme.Border.Resolve(cs)
	return style.Render(strings.Repeat("─", width))
}

// isTableRow returns true if the line looks like a pipe table row.
func isTableRow(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "|") && strings.HasSuffix(t, "|")
}

// renderTable renders a slice of pipe-table lines into terminal lines.
func (mv *MarkdownView) renderTable(tableLines []string, width int, cs ColorSetting, noColor bool) []string {
	// Parse rows (skip separator rows like |---|---|).
	var rows [][]string
	for _, line := range tableLines {
		if isSeparatorRow(line) {
			continue
		}
		cells := splitTableRow(line)
		rows = append(rows, cells)
	}
	if len(rows) == 0 {
		return nil
	}

	// Compute column widths.
	cols := 0
	for _, row := range rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	colWidths := make([]int, cols)
	for _, row := range rows {
		for c, cell := range row {
			w := runewidth.StringWidth(cell)
			if w > colWidths[c] {
				colWidths[c] = w
			}
		}
	}

	borderStyle := mv.theme.Border.Resolve(cs)
	headingStyle := mv.theme.Heading.Resolve(cs)

	var result []string
	for rowIdx, row := range rows {
		var sb strings.Builder
		for c := 0; c < cols; c++ {
			cell := ""
			if c < len(row) {
				cell = row[c]
			}
			pad := colWidths[c] - runewidth.StringWidth(cell)
			if pad < 0 {
				pad = 0
			}
			padded := cell + strings.Repeat(" ", pad)
			if noColor {
				sb.WriteString("| " + padded + " ")
			} else {
				sep := borderStyle.Render("|")
				if rowIdx == 0 {
					sb.WriteString(sep + " " + headingStyle.Render(padded) + " ")
				} else {
					sb.WriteString(sep + " " + padded + " ")
				}
			}
		}
		if noColor {
			sb.WriteString("|")
		} else {
			sb.WriteString(borderStyle.Render("|"))
		}
		line := ansi.Truncate(sb.String(), width, "…")
		result = append(result, line)

		// After header row, add separator line.
		if rowIdx == 0 {
			var sep strings.Builder
			for c := 0; c < cols; c++ {
				dashes := strings.Repeat("-", colWidths[c]+2)
				if noColor {
					sep.WriteString("+" + dashes)
				} else {
					sep.WriteString(borderStyle.Render("+" + dashes))
				}
			}
			if noColor {
				sep.WriteString("+")
			} else {
				sep.WriteString(borderStyle.Render("+"))
			}
			result = append(result, ansi.Truncate(sep.String(), width, "…"))
		}
	}
	return result
}

func isSeparatorRow(line string) bool {
	// Like |---|---| or | :-- | --- |
	t := strings.TrimSpace(line)
	cells := splitTableRow(t)
	for _, cell := range cells {
		trimmed := strings.TrimSpace(cell)
		if len(trimmed) == 0 {
			continue
		}
		for _, ch := range strings.Trim(trimmed, ":") {
			if ch != '-' {
				return false
			}
		}
	}
	return true
}

func splitTableRow(line string) []string {
	t := strings.TrimSpace(line)
	t = strings.TrimPrefix(t, "|")
	t = strings.TrimSuffix(t, "|")
	parts := strings.Split(t, "|")
	result := make([]string, len(parts))
	for i, p := range parts {
		result[i] = strings.TrimSpace(p)
	}
	return result
}

// parseListItem detects unordered list items including nested (indented) ones.
// Returns indent depth, bullet char, rest of text, and whether it matched.
func parseListItem(line string) (indent int, bullet string, rest string, ok bool) {
	spaces := 0
	byteOffset := 0
	for i, ch := range line {
		if ch == ' ' {
			spaces++
		} else {
			byteOffset = i
			break
		}
		byteOffset = i + len(string(ch))
	}
	trimmed := line[byteOffset:]
	for _, b := range []string{"- ", "* ", "+ "} {
		if strings.HasPrefix(trimmed, b) {
			return spaces / 2, b[:1], strings.TrimSpace(trimmed[2:]), true
		}
	}
	return 0, "", "", false
}

func (mv *MarkdownView) renderListItem(depth int, bullet, text string, cs ColorSetting, accessible, noColor bool) string {
	text = mv.renderInline(text, cs, accessible, noColor)
	indent := strings.Repeat("  ", depth)

	// Alternate bullet styles by depth.
	var bulletChar string
	if accessible || noColor {
		chars := []string{"-", "*", "+"}
		bulletChar = chars[depth%3]
	} else {
		chars := []string{"•", "◦", "▪"}
		bulletChar = chars[depth%3]
	}

	_ = bullet // original bullet is replaced by depth-based style
	style := mv.theme.Secondary.Resolve(cs)
	if accessible || noColor {
		return indent + bulletChar + " " + text
	}
	return indent + style.Render(bulletChar) + " " + text
}

// headingLevel returns 1-3 for heading lines, 0 otherwise.
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

func (mv *MarkdownView) renderHeading(text string, level int, cs ColorSetting, accessible, noColor bool) string {
	if accessible {
		tags := map[int]string{1: "[H1]", 2: "[H2]", 3: "[H3]"}
		return tags[level] + " " + text
	}
	if noColor {
		return lipgloss.NewStyle().Bold(true).Render(text)
	}
	return mv.theme.Heading.Resolve(cs).Render(text)
}

// renderInline handles bold, italic, inline code, and links.
func (mv *MarkdownView) renderInline(line string, cs ColorSetting, accessible, noColor bool) string {
	if accessible || noColor {
		return mv.renderInlinePlain(line, noColor)
	}
	return mv.renderInlineStyled(line, cs)
}

func (mv *MarkdownView) renderInlinePlain(line string, noColor bool) string {
	var b strings.Builder
	i := 0
	runes := []rune(line)

	for i < len(runes) {
		// Link: [text](url)
		if runes[i] == '[' {
			if end, urlStart, urlEnd, ok := parseLink(runes, i); ok {
				text := string(runes[i+1 : end])
				url := string(runes[urlStart:urlEnd])
				if noColor {
					b.WriteString(lipgloss.NewStyle().Underline(true).Render(text) + " (" + url + ")")
				} else {
					b.WriteString(text + " (" + url + ")")
				}
				i = urlEnd + 1
				continue
			}
		}

		// Bold **text**
		if i+1 < len(runes) && runes[i] == '*' && runes[i+1] == '*' {
			if end := findClosing(runes, i+2, "**"); end >= 0 {
				inner := string(runes[i+2 : end])
				if inner != "" {
					if noColor {
						b.WriteString(lipgloss.NewStyle().Bold(true).Render(inner))
					} else {
						b.WriteString("**" + inner + "**")
					}
					i = end + 2
					continue
				}
			}
		}

		// Italic *text* or _text_
		if (runes[i] == '*' || runes[i] == '_') && (i == 0 || !isWordChar(runes[i-1])) {
			marker := runes[i]
			if end := findClosingRune(runes, i+1, marker); end >= 0 && (end+1 >= len(runes) || !isWordChar(runes[end+1])) {
				inner := string(runes[i+1 : end])
				if inner != "" {
					if noColor {
						b.WriteString(lipgloss.NewStyle().Faint(true).Render(inner))
					} else {
						b.WriteString(string(marker) + inner + string(marker))
					}
					i = end + 1
					continue
				}
			}
		}

		// Inline code `text`
		if runes[i] == '`' {
			if end := findClosingRune(runes, i+1, '`'); end >= 0 {
				inner := string(runes[i+1 : end])
				b.WriteString("`" + inner + "`")
				i = end + 1
				continue
			}
		}

		b.WriteRune(runes[i])
		i++
	}
	return b.String()
}

func (mv *MarkdownView) renderInlineStyled(line string, cs ColorSetting) string {
	var b strings.Builder
	i := 0
	runes := []rune(line)

	for i < len(runes) {
		// Link: [text](url)
		if runes[i] == '[' {
			if end, urlStart, urlEnd, ok := parseLink(runes, i); ok {
				text := string(runes[i+1 : end])
				url := string(runes[urlStart:urlEnd])
				linkStyle := mv.theme.Link.Resolve(cs)
				mutedStyle := mv.theme.TextMuted.Resolve(cs)
				b.WriteString(linkStyle.Render(text) + " " + mutedStyle.Render("("+url+")"))
				i = urlEnd + 1
				continue
			}
		}

		// Bold **text**
		if i+1 < len(runes) && runes[i] == '*' && runes[i+1] == '*' {
			if end := findClosing(runes, i+2, "**"); end >= 0 {
				inner := string(runes[i+2 : end])
				if inner != "" {
					b.WriteString(lipgloss.NewStyle().Bold(true).Render(inner))
					i = end + 2
					continue
				}
			}
		}

		// Italic *text* or _text_
		if (runes[i] == '*' || runes[i] == '_') && (i == 0 || !isWordChar(runes[i-1])) {
			marker := runes[i]
			if end := findClosingRune(runes, i+1, marker); end >= 0 && (end+1 >= len(runes) || !isWordChar(runes[end+1])) {
				inner := string(runes[i+1 : end])
				if inner != "" {
					b.WriteString(lipgloss.NewStyle().Italic(true).Render(inner))
					i = end + 1
					continue
				}
			}
		}

		// Inline code `text`
		if runes[i] == '`' {
			if end := findClosingRune(runes, i+1, '`'); end >= 0 {
				inner := string(runes[i+1 : end])
				b.WriteString(mv.theme.CodePath.Resolve(cs).Render(inner))
				i = end + 1
				continue
			}
		}

		b.WriteRune(runes[i])
		i++
	}
	return b.String()
}

// parseLink parses [text](url) starting at position i in runes.
// Returns textEnd, urlStart, urlEnd, ok.
func parseLink(runes []rune, i int) (textEnd, urlStart, urlEnd int, ok bool) {
	if i >= len(runes) || runes[i] != '[' {
		return 0, 0, 0, false
	}
	textEndIdx := -1
	for j := i + 1; j < len(runes); j++ {
		if runes[j] == ']' {
			textEndIdx = j
			break
		}
	}
	if textEndIdx < 0 || textEndIdx+1 >= len(runes) || runes[textEndIdx+1] != '(' {
		return 0, 0, 0, false
	}
	urlEndIdx := -1
	for j := textEndIdx + 2; j < len(runes); j++ {
		if runes[j] == ')' {
			urlEndIdx = j
			break
		}
	}
	if urlEndIdx < 0 {
		return 0, 0, 0, false
	}
	return textEndIdx, textEndIdx + 2, urlEndIdx, true
}

func findClosing(runes []rune, pos int, delim string) int {
	d := []rune(delim)
	for i := pos; i+1 < len(runes); i++ {
		if runes[i] == d[0] && runes[i+1] == d[1] {
			return i
		}
	}
	return -1
}

func findClosingRune(runes []rune, pos int, delim rune) int {
	for i := pos; i < len(runes); i++ {
		if runes[i] == delim {
			return i
		}
	}
	return -1
}

func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}
