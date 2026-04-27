// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tui

import (
	"image/color"

	lipgloss "charm.land/lipgloss/v2"
)

// Token holds Lip Gloss styles for each color depth level.
// At render time, the appropriate style is selected based on RenderConfig.Color.
type Token struct {
	TrueColor lipgloss.Style
	Color256  lipgloss.Style
	Color16   lipgloss.Style
	NoColor   lipgloss.Style
}

// Resolve returns the appropriate style for the given color setting.
func (t Token) Resolve(cs ColorSetting) lipgloss.Style {
	switch cs {
	case ColorTrueColor:
		return t.TrueColor
	case Color256Color:
		return t.Color256
	case Color16Color:
		return t.Color16
	default:
		return t.NoColor
	}
}

// Theme contains all semantic design tokens and typography styles.
// Components reference Theme tokens for consistent, themeable rendering.
type Theme struct {
	// Semantic color tokens (10 total).
	Primary   Token
	Secondary Token
	Accent    Token
	Text      Token
	TextMuted Token
	Success   Token
	Warning   Token
	Error     Token
	Border    Token
	Highlight Token

	// Typography styles.
	Heading  Token
	Body     Token
	Muted    Token
	CodePath Token
	Link     Token
	Keybind  Token

	// OverlayBg is the background color for overlay panels (solid, prevents bleed-through).
	OverlayBg color.Color
}

// Tokyo Night-inspired hex values (default theme).
const (
	hexPrimary   = "#7AA2F7"
	hexSecondary = "#9ECE6A"
	hexAccent    = "#FF9E64"
	hexTextMuted = "#565F89"
	hexWarning   = "#E0AF68"
	hexError     = "#F7768E"
	hexBorder    = "#3B4261"
	hexHighlight = "#292E42"
)

// makeColorToken creates a Token from a true color, a 16-color ANSI color,
// and a no-color formatting style.
func makeColorToken(hex string, ansi16 color.Color, noColorStyle lipgloss.Style) Token {
	trueColor := lipgloss.Color(hex)
	return Token{
		TrueColor: lipgloss.NewStyle().Foreground(trueColor),
		Color256:  lipgloss.NewStyle().Foreground(trueColor), // Lip Gloss auto-degrades
		Color16:   lipgloss.NewStyle().Foreground(ansi16),
		NoColor:   noColorStyle,
	}
}

// DefaultTheme returns the Tokyo Night-inspired default theme.
func DefaultTheme() Theme {
	return ThemeFromColors(ThemeColors{})
}

// ThemeFromColors creates a Theme from custom hex color values.
// Missing or empty values fall back to DefaultTheme() colors.
func ThemeFromColors(colors ThemeColors) Theme {
	c := colors.withDefaults()

	t := Theme{
		Primary:   makeColorToken(c.Primary, lipgloss.Blue, lipgloss.NewStyle().Bold(true)),
		Secondary: makeColorToken(c.Secondary, lipgloss.Green, lipgloss.NewStyle().Underline(true)),
		Accent:    makeColorToken(c.Accent, lipgloss.Yellow, lipgloss.NewStyle().Bold(true)),
		Text: Token{
			TrueColor: lipgloss.NewStyle(),
			Color256:  lipgloss.NewStyle(),
			Color16:   lipgloss.NewStyle(),
			NoColor:   lipgloss.NewStyle(),
		},
		TextMuted: makeColorToken(c.TextMuted, lipgloss.BrightBlack, lipgloss.NewStyle().Faint(true)),
		Success:   makeColorToken(c.Success, lipgloss.Green, lipgloss.NewStyle().Bold(true)),
		Warning:   makeColorToken(c.Warning, lipgloss.Yellow, lipgloss.NewStyle().Bold(true)),
		Error:     makeColorToken(c.Error, lipgloss.Red, lipgloss.NewStyle().Bold(true)),
		Border: Token{
			TrueColor: lipgloss.NewStyle().Foreground(lipgloss.Color(c.Border)),
			Color256:  lipgloss.NewStyle().Foreground(lipgloss.Color(c.Border)),
			Color16:   lipgloss.NewStyle().Foreground(lipgloss.BrightBlack),
			NoColor:   lipgloss.NewStyle(),
		},
		Highlight: Token{
			TrueColor: lipgloss.NewStyle().Background(lipgloss.Color(c.Highlight)),
			Color256:  lipgloss.NewStyle().Background(lipgloss.Color(c.Highlight)),
			Color16:   lipgloss.NewStyle().Reverse(true),
			NoColor:   lipgloss.NewStyle().Reverse(true),
		},
	}

	// Typography styles.
	t.Heading = Token{
		TrueColor: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(c.Primary)),
		Color256:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(c.Primary)),
		Color16:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Blue),
		NoColor:   lipgloss.NewStyle().Bold(true),
	}
	t.Body = Token{
		TrueColor: lipgloss.NewStyle(),
		Color256:  lipgloss.NewStyle(),
		Color16:   lipgloss.NewStyle(),
		NoColor:   lipgloss.NewStyle(),
	}
	t.Muted = Token{
		TrueColor: lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color(c.TextMuted)),
		Color256:  lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color(c.TextMuted)),
		Color16:   lipgloss.NewStyle().Faint(true).Foreground(lipgloss.BrightBlack),
		NoColor:   lipgloss.NewStyle().Faint(true),
	}
	t.CodePath = Token{
		TrueColor: lipgloss.NewStyle().Background(lipgloss.Color(c.Highlight)),
		Color256:  lipgloss.NewStyle().Background(lipgloss.Color(c.Highlight)),
		Color16:   lipgloss.NewStyle().Reverse(true),
		NoColor:   lipgloss.NewStyle().Reverse(true),
	}
	t.Link = Token{
		TrueColor: lipgloss.NewStyle().Underline(true).Foreground(lipgloss.Color(c.Primary)),
		Color256:  lipgloss.NewStyle().Underline(true).Foreground(lipgloss.Color(c.Primary)),
		Color16:   lipgloss.NewStyle().Underline(true).Foreground(lipgloss.Blue),
		NoColor:   lipgloss.NewStyle().Underline(true),
	}
	t.Keybind = Token{
		TrueColor: lipgloss.NewStyle().Bold(true).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(c.Border)).
			Padding(0, 1),
		Color256: lipgloss.NewStyle().Bold(true).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(c.Border)).
			Padding(0, 1),
		Color16: lipgloss.NewStyle().Bold(true).
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1),
		NoColor: lipgloss.NewStyle().Bold(true).
			Border(lipgloss.NormalBorder()).
			Padding(0, 1),
	}

	// Overlay background: Tokyo Night panel color (solid, prevents dock bleed-through).
	t.OverlayBg = lipgloss.Color(c.Highlight)

	return t
}
