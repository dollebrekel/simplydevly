// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-runewidth"
)

// Color setting for the render pipeline.
type ColorSetting string

const (
	ColorTrueColor ColorSetting = "truecolor"
	Color256Color  ColorSetting = "256"
	Color16Color   ColorSetting = "16"
	ColorNone      ColorSetting = "none"
)

// BorderStyle for the render pipeline.
type BorderStyle string

const (
	BorderUnicode BorderStyle = "unicode"
	BorderASCII   BorderStyle = "ascii"
	BorderNone    BorderStyle = "none"
)

// MotionStyle for the render pipeline.
type MotionStyle string

const (
	MotionSpinner MotionStyle = "spinner"
	MotionStatic  MotionStyle = "static"
)

// Verbosity level for the render pipeline.
type Verbosity string

const (
	VerbosityFull       Verbosity = "full"
	VerbosityCompact    Verbosity = "compact"
	VerbosityAccessible Verbosity = "accessible"
)

// RenderConfig holds the render pipeline configuration (UX-DR7).
// Passed to all components for consistent rendering decisions.
type RenderConfig struct {
	Color     ColorSetting
	Emoji     bool
	Borders   BorderStyle
	Motion    MotionStyle
	Verbosity Verbosity
}

// NewRenderConfig merges auto-detected capabilities with CLI flag overrides.
func NewRenderConfig(caps Capabilities, flags CLIFlags) RenderConfig {
	cfg := RenderConfig{
		Color:     colorSettingFromDepth(caps.ColorDepth),
		Emoji:     caps.Emoji,
		Borders:   BorderUnicode,
		Motion:    MotionSpinner,
		Verbosity: VerbosityFull,
	}

	// SSH sessions default to ASCII borders for fewer bytes per frame.
	if caps.SSHSession {
		cfg.Borders = BorderASCII
	}

	// Apply presets first (they set multiple fields at once).
	// LowBandwidth is applied first; Accessible re-applies after to ensure
	// accessibility always wins when both flags are set.
	if flags.LowBandwidth {
		cfg.Borders = BorderASCII
		cfg.Motion = MotionStatic
		cfg.Verbosity = VerbosityCompact
		cfg.Emoji = false
	}
	if flags.Accessible {
		cfg.Borders = BorderNone
		cfg.Motion = MotionStatic
		cfg.Verbosity = VerbosityAccessible
		cfg.Emoji = false
	}

	// Individual flags override individual settings (after presets).
	if flags.NoColor {
		cfg.Color = ColorNone
	}
	if flags.NoEmoji {
		cfg.Emoji = false
	}
	if flags.NoBorders {
		cfg.Borders = BorderNone
	}
	if flags.NoMotion {
		cfg.Motion = MotionStatic
	}

	// Respect NO_COLOR convention (https://no-color.org/).
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		cfg.Color = ColorNone
	}

	// Piped output: force no color.
	if !caps.IsTTY {
		cfg.Color = ColorNone
		cfg.Emoji = false
		cfg.Motion = MotionStatic
	}

	return cfg
}

// colorSettingFromDepth converts a ColorDepth to a ColorSetting.
func colorSettingFromDepth(depth ColorDepth) ColorSetting {
	switch depth {
	case TrueColor:
		return ColorTrueColor
	case Color256:
		return Color256Color
	case Color16:
		return Color16Color
	default:
		return ColorNone
	}
}

// RenderBorder renders a bordered section with a title, adapting to the
// current render config. In accessible mode, box-drawing chars are replaced
// by text headers.
func RenderBorder(title, content string, config RenderConfig, width int) string {
	if width < 1 {
		width = 40
	}

	switch config.Borders {
	case BorderNone:
		// Accessible mode: text headers instead of box-drawing.
		return renderTextBorder(title, content, width)
	case BorderASCII:
		return renderASCIIBorder(title, content, width)
	default:
		return renderUnicodeBorder(title, content, width)
	}
}

// renderTextBorder renders using plain text headers (accessible mode).
func renderTextBorder(title, content string, width int) string {
	var b strings.Builder
	header := fmt.Sprintf("== %s ==", title)
	headerWidth := runewidth.StringWidth(header)
	b.WriteString(header)
	b.WriteByte('\n')
	b.WriteString(content)
	b.WriteByte('\n')
	b.WriteString(strings.Repeat("=", min(headerWidth, width)))
	b.WriteByte('\n')
	return b.String()
}

// renderASCIIBorder renders using ASCII box-drawing characters.
func renderASCIIBorder(title, content string, width int) string {
	innerWidth := width - 2
	if innerWidth < 1 {
		innerWidth = 1
	}

	var b strings.Builder
	// Top border with title (truncated to fit).
	titlePart := fmt.Sprintf("[ %s ]", title)
	titleWidth := runewidth.StringWidth(titlePart)
	if titleWidth > innerWidth {
		titlePart = runewidth.Truncate(titlePart, innerWidth, "…")
		titleWidth = runewidth.StringWidth(titlePart)
	}
	remaining := innerWidth - titleWidth
	b.WriteString("+" + titlePart + strings.Repeat("-", remaining) + "+\n")

	// Content lines.
	for _, line := range strings.Split(content, "\n") {
		lineWidth := runewidth.StringWidth(line)
		if lineWidth < innerWidth {
			padded := line + strings.Repeat(" ", innerWidth-lineWidth)
			b.WriteString("|" + padded + "|\n")
		} else {
			b.WriteString("|" + runewidth.Truncate(line, innerWidth, "") + "|\n")
		}
	}

	// Bottom border.
	b.WriteString("+" + strings.Repeat("-", innerWidth) + "+\n")
	return b.String()
}

// renderUnicodeBorder renders using Unicode box-drawing characters.
func renderUnicodeBorder(title, content string, width int) string {
	innerWidth := width - 2
	if innerWidth < 1 {
		innerWidth = 1
	}

	var b strings.Builder
	// Top border with title (truncated to fit).
	titlePart := fmt.Sprintf(" %s ", title)
	titleWidth := runewidth.StringWidth(titlePart)
	if titleWidth > innerWidth {
		titlePart = runewidth.Truncate(titlePart, innerWidth, "…")
		titleWidth = runewidth.StringWidth(titlePart)
	}
	remaining := innerWidth - titleWidth
	b.WriteString("┌" + titlePart + strings.Repeat("─", remaining) + "┐\n")

	// Content lines.
	for _, line := range strings.Split(content, "\n") {
		lineWidth := runewidth.StringWidth(line)
		if lineWidth < innerWidth {
			padded := line + strings.Repeat(" ", innerWidth-lineWidth)
			b.WriteString("│" + padded + "│\n")
		} else {
			b.WriteString("│" + runewidth.Truncate(line, innerWidth, "") + "│\n")
		}
	}

	// Bottom border.
	b.WriteString("└" + strings.Repeat("─", innerWidth) + "┘\n")
	return b.String()
}
