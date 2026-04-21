// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package siplyui

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// SearchField is a single-line text input with query matching.
type SearchField struct {
	query        string
	cursor       int
	placeholder  string
	theme        Theme
	renderConfig RenderConfig
}

// NewSearchField creates a SearchField with the given placeholder, theme, and render config.
func NewSearchField(placeholder string, theme Theme, config RenderConfig) *SearchField {
	return &SearchField{
		placeholder:  placeholder,
		theme:        theme,
		renderConfig: config,
	}
}

// Render returns a single-line input representation truncated to width.
func (sf *SearchField) Render(width int) string {
	if width < 1 {
		width = 1
	}
	cs := sf.renderConfig.Color

	// Prefix icon.
	var prefix string
	if sf.renderConfig.Emoji {
		prefix = "🔍 "
	} else {
		prefix = "Search: "
	}
	prefixWidth := ansi.StringWidth(prefix)
	inputWidth := width - prefixWidth
	if inputWidth < 1 {
		inputWidth = 1
	}

	var displayText string
	if sf.query == "" {
		muted := sf.theme.TextMuted.Resolve(cs)
		displayText = muted.Render(ansi.Truncate(sf.placeholder, inputWidth, "…"))
	} else {
		runes := []rune(sf.query)
		// Show cursor as a block character at cursor position.
		before := string(runes[:sf.cursor])
		var after string
		if sf.cursor < len(runes) {
			after = string(runes[sf.cursor:])
		}
		raw := before + "█" + after
		displayText = ansi.Truncate(raw, inputWidth, "…")
	}

	line := prefix + displayText
	return ansi.Truncate(line, width, "…")
}

// HandleKey processes a key press. Returns true if the field state changed.
func (sf *SearchField) HandleKey(key string) bool {
	runes := []rune(sf.query)
	switch key {
	case "backspace", "ctrl+h":
		if sf.cursor > 0 {
			sf.query = string(runes[:sf.cursor-1]) + string(runes[sf.cursor:])
			sf.cursor--
			return true
		}
	case "ctrl+u":
		sf.query = ""
		sf.cursor = 0
		return true
	case "left":
		if sf.cursor > 0 {
			sf.cursor--
			return true
		}
	case "right":
		if sf.cursor < len(runes) {
			sf.cursor++
			return true
		}
	default:
		// Accept single printable runes.
		if utf8.RuneCountInString(key) == 1 {
			r, _ := utf8.DecodeRuneInString(key)
			if r >= 32 { // printable
				sf.query = string(runes[:sf.cursor]) + string(r) + string(runes[sf.cursor:])
				sf.cursor++
				return true
			}
		}
	}
	return false
}

// Query returns the current query string.
func (sf *SearchField) Query() string { return sf.query }

// SetQuery updates the query and moves the cursor to the end.
func (sf *SearchField) SetQuery(q string) {
	sf.query = q
	sf.cursor = len([]rune(q))
}

// Clear resets the field.
func (sf *SearchField) Clear() {
	sf.query = ""
	sf.cursor = 0
}

// Match returns the subset of items that contain the query (case-insensitive).
func (sf *SearchField) Match(items []string) []string {
	if sf.query == "" {
		result := make([]string, len(items))
		copy(result, items)
		return result
	}
	q := strings.ToLower(sf.query)
	var out []string
	for _, item := range items {
		if strings.Contains(strings.ToLower(item), q) {
			out = append(out, item)
		}
	}
	return out
}
