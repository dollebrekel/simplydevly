// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package siplyui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// BadgeStyle controls the visual style of a Badge.
type BadgeStyle int

const (
	BadgeNeutral BadgeStyle = iota
	BadgeSuccess
	BadgeWarning
	BadgeError
	BadgeInfo
)

// Badge is a compact inline label with a semantic style.
type Badge struct {
	Label string
	Style BadgeStyle
}

// RenderBadge renders a single badge inline.
func RenderBadge(badge Badge, theme Theme, config RenderConfig) string {
	cs := config.Color
	text := badge.Label
	if cs == ColorNone {
		return "[" + text + "]"
	}
	var style = theme.Text.Resolve(cs)
	switch badge.Style {
	case BadgeSuccess:
		style = theme.Success.Resolve(cs)
	case BadgeWarning:
		style = theme.Warning.Resolve(cs)
	case BadgeError:
		style = theme.Error.Resolve(cs)
	case BadgeInfo:
		style = theme.Primary.Resolve(cs)
	}
	return style.Render("[" + text + "]")
}

// Card holds metadata for a marketplace item summary.
type Card struct {
	Title        string
	Description  string
	Version      string
	Author       string
	License      string
	Rating       float64 // 0.0 – 5.0
	InstallCount int
	Badges       []Badge
}

// RenderCard renders a card inside a bordered box of given width.
func RenderCard(card Card, theme Theme, config RenderConfig, width int) string {
	if width < 2 {
		width = 2
	}
	inner := width - 2
	if inner < 1 {
		inner = 1
	}
	cs := config.Color

	var lines []string

	// Title line.
	titleStyle := theme.Heading.Resolve(cs)
	lines = append(lines, titleStyle.Render(ansi.Truncate(card.Title, inner, "…")))

	// Metadata row.
	meta := card.Version
	if card.Author != "" {
		meta += " · " + card.Author
	}
	if card.License != "" {
		meta += " · " + card.License
	}
	if meta != "" {
		mutedStyle := theme.TextMuted.Resolve(cs)
		lines = append(lines, mutedStyle.Render(ansi.Truncate(meta, inner, "…")))
	}

	// Rating.
	if card.Rating > 0 {
		lines = append(lines, renderRating(card.Rating, cs == ColorNone))
	}

	// Badges.
	if len(card.Badges) > 0 {
		var badgeRow strings.Builder
		for i, b := range card.Badges {
			if i > 0 {
				badgeRow.WriteByte(' ')
			}
			badgeRow.WriteString(RenderBadge(b, theme, config))
		}
		lines = append(lines, ansi.Truncate(badgeRow.String(), inner, "…"))
	}

	// Install count.
	if card.InstallCount > 0 {
		mutedStyle := theme.TextMuted.Resolve(cs)
		cnt := fmt.Sprintf("%d installs", card.InstallCount)
		lines = append(lines, mutedStyle.Render(cnt))
	}

	// Description.
	if card.Description != "" {
		lines = append(lines, "")
		lines = append(lines, ansi.Truncate(card.Description, inner, "…"))
	}

	content := strings.Join(lines, "\n")
	return config.RenderBorderedBox(theme, content, width)
}

// renderRating returns a star representation. noColor uses text asterisks.
func renderRating(rating float64, noColor bool) string {
	full := int(math.Round(rating))
	empty := 5 - full
	if full > 5 {
		full = 5
	}
	if empty < 0 {
		empty = 0
	}
	if noColor {
		return strings.Repeat("*", full) + strings.Repeat("-", empty)
	}
	return strings.Repeat("★", full) + strings.Repeat("☆", empty)
}
