// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package siplyui

import (
	"image/color"

	lipgloss "charm.land/lipgloss/v2"
)

// ColorSetting controls ANSI color depth in rendered output.
type ColorSetting string

const (
	ColorTrueColor ColorSetting = "truecolor"
	Color256Color  ColorSetting = "256"
	Color16Color   ColorSetting = "16"
	ColorNone      ColorSetting = "none"
)

// BorderStyle controls box-drawing characters in rendered output.
type BorderStyle string

const (
	BorderUnicode BorderStyle = "unicode"
	BorderASCII   BorderStyle = "ascii"
	BorderNone    BorderStyle = "none"
)

// Verbosity controls how much information is shown.
type Verbosity string

const (
	VerbosityFull       Verbosity = "full"
	VerbosityCompact    Verbosity = "compact"
	VerbosityAccessible Verbosity = "accessible"
)

// RenderConfig holds render pipeline configuration passed to all components.
type RenderConfig struct {
	Color     ColorSetting
	Emoji     bool
	Borders   BorderStyle
	Verbosity Verbosity
}

// Token holds Lip Gloss styles for each color depth level.
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

// Theme contains all semantic design tokens used by siplyui components.
type Theme struct {
	Primary   Token
	Secondary Token
	Text      Token
	TextMuted Token
	Success   Token
	Warning   Token
	Error     Token
	Border    Token
	Highlight Token

	Heading  Token
	Body     Token
	Muted    Token
	CodePath Token
	Link     Token
	Keybind  Token
}

// DefaultTheme returns the Tokyo Night-inspired default theme.
func DefaultTheme() Theme {
	makeColor := func(hex string, ansi16 color.Color, noStyle lipgloss.Style) Token {
		c := lipgloss.Color(hex)
		return Token{
			TrueColor: lipgloss.NewStyle().Foreground(c),
			Color256:  lipgloss.NewStyle().Foreground(c),
			Color16:   lipgloss.NewStyle().Foreground(ansi16),
			NoColor:   noStyle,
		}
	}

	t := Theme{
		Primary:   makeColor("#7AA2F7", lipgloss.Blue, lipgloss.NewStyle().Bold(true)),
		Secondary: makeColor("#9ECE6A", lipgloss.Green, lipgloss.NewStyle().Underline(true)),
		Text: Token{
			TrueColor: lipgloss.NewStyle(),
			Color256:  lipgloss.NewStyle(),
			Color16:   lipgloss.NewStyle(),
			NoColor:   lipgloss.NewStyle(),
		},
		TextMuted: makeColor("#565F89", lipgloss.BrightBlack, lipgloss.NewStyle().Faint(true)),
		Success:   makeColor("#9ECE6A", lipgloss.Green, lipgloss.NewStyle().Bold(true)),
		Warning:   makeColor("#E0AF68", lipgloss.Yellow, lipgloss.NewStyle().Bold(true)),
		Error:     makeColor("#F7768E", lipgloss.Red, lipgloss.NewStyle().Bold(true)),
		Border: Token{
			TrueColor: lipgloss.NewStyle().Foreground(lipgloss.Color("#3B4261")),
			Color256:  lipgloss.NewStyle().Foreground(lipgloss.Color("#3B4261")),
			Color16:   lipgloss.NewStyle().Foreground(lipgloss.BrightBlack),
			NoColor:   lipgloss.NewStyle(),
		},
		Highlight: Token{
			TrueColor: lipgloss.NewStyle().Background(lipgloss.Color("#292E42")),
			Color256:  lipgloss.NewStyle().Background(lipgloss.Color("#292E42")),
			Color16:   lipgloss.NewStyle().Reverse(true),
			NoColor:   lipgloss.NewStyle().Reverse(true),
		},
	}
	t.Heading = Token{
		TrueColor: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7AA2F7")),
		Color256:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7AA2F7")),
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
		TrueColor: lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("#565F89")),
		Color256:  lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("#565F89")),
		Color16:   lipgloss.NewStyle().Faint(true).Foreground(lipgloss.BrightBlack),
		NoColor:   lipgloss.NewStyle().Faint(true),
	}
	t.CodePath = Token{
		TrueColor: lipgloss.NewStyle().Background(lipgloss.Color("#292E42")),
		Color256:  lipgloss.NewStyle().Background(lipgloss.Color("#292E42")),
		Color16:   lipgloss.NewStyle().Reverse(true),
		NoColor:   lipgloss.NewStyle().Reverse(true),
	}
	t.Link = Token{
		TrueColor: lipgloss.NewStyle().Underline(true).Foreground(lipgloss.Color("#7AA2F7")),
		Color256:  lipgloss.NewStyle().Underline(true).Foreground(lipgloss.Color("#7AA2F7")),
		Color16:   lipgloss.NewStyle().Underline(true).Foreground(lipgloss.Blue),
		NoColor:   lipgloss.NewStyle().Underline(true),
	}
	t.Keybind = Token{
		TrueColor: lipgloss.NewStyle().Bold(true).Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#3B4261")).Padding(0, 1),
		Color256: lipgloss.NewStyle().Bold(true).Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#3B4261")).Padding(0, 1),
		Color16: lipgloss.NewStyle().Bold(true).Border(lipgloss.RoundedBorder()).Padding(0, 1),
		NoColor: lipgloss.NewStyle().Bold(true).Border(lipgloss.NormalBorder()).Padding(0, 1),
	}
	return t
}

// DefaultRenderConfig returns a safe default render config (truecolor, emoji on, unicode borders).
func DefaultRenderConfig() RenderConfig {
	return RenderConfig{
		Color:     ColorTrueColor,
		Emoji:     true,
		Borders:   BorderUnicode,
		Verbosity: VerbosityFull,
	}
}
