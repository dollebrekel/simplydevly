// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tui

import (
	"os"
	"strings"

	"github.com/charmbracelet/colorprofile"
	"golang.org/x/term"
)

// ColorDepth represents the terminal's color capability.
type ColorDepth int

const (
	// NoColor indicates no color support.
	NoColor ColorDepth = iota
	// Color16 indicates basic 16-color ANSI support.
	Color16
	// Color256 indicates 256-color support.
	Color256
	// TrueColor indicates 24-bit true color support.
	TrueColor
)

// String returns a human-readable name for the color depth.
func (c ColorDepth) String() string {
	switch c {
	case TrueColor:
		return "truecolor"
	case Color256:
		return "256"
	case Color16:
		return "16"
	default:
		return "none"
	}
}

// Capabilities holds detected terminal capabilities.
// Values are immutable after detection (width/height updated only on resize).
type Capabilities struct {
	ColorDepth  ColorDepth
	Unicode     bool
	Emoji       bool
	Mouse       bool
	Width       int
	Height      int
	TmuxNested  bool
	SSHSession  bool
	IsTTY       bool
}

// DetectCapabilities auto-detects terminal capabilities from the environment.
func DetectCapabilities() Capabilities {
	caps := Capabilities{
		Mouse: true, // Bubble Tea handles mouse support
	}

	// TTY detection.
	caps.IsTTY = term.IsTerminal(int(os.Stdout.Fd()))

	// Color depth detection.
	caps.ColorDepth = detectColorDepth()

	// Unicode detection via LANG/LC_ALL.
	caps.Unicode = detectUnicode()

	// Emoji defaults to unicode support.
	caps.Emoji = caps.Unicode

	// Terminal dimensions (fallback; Bubble Tea provides WindowSizeMsg too).
	if caps.IsTTY {
		w, h, err := term.GetSize(int(os.Stdout.Fd()))
		if err == nil {
			caps.Width = w
			caps.Height = h
		}
	}

	// tmux nesting detection.
	caps.TmuxNested = os.Getenv("TMUX") != ""

	// SSH session detection.
	caps.SSHSession = os.Getenv("SSH_CLIENT") != "" || os.Getenv("SSH_TTY") != ""

	return caps
}

// detectColorDepth checks environment variables to determine color support.
func detectColorDepth() ColorDepth {
	// Check colorprofile library first (used by Lip Gloss v2).
	profile := colorprofile.Detect(os.Stdout, os.Environ())
	switch profile {
	case colorprofile.TrueColor:
		return TrueColor
	case colorprofile.ANSI256:
		return Color256
	case colorprofile.ANSI:
		return Color16
	case colorprofile.Ascii:
		return NoColor
	}

	// Fallback: manual env var checks.
	colorterm := os.Getenv("COLORTERM")
	if colorterm == "truecolor" || colorterm == "24bit" {
		return TrueColor
	}

	termEnv := os.Getenv("TERM")
	if strings.Contains(termEnv, "256color") {
		return Color256
	}

	return Color16
}

// detectUnicode checks locale env vars for UTF-8 support.
// Follows POSIX precedence: LC_ALL > LC_CTYPE > LANG.
func detectUnicode() bool {
	for _, env := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if val, ok := os.LookupEnv(env); ok && val != "" {
			upper := strings.ToUpper(val)
			return strings.Contains(upper, "UTF-8") || strings.Contains(upper, "UTF8")
		}
	}
	return false
}
