// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tui

import siplyui "siply.dev/siply/pkg/siplyui"

// BridgeTheme converts an internal Theme to the public siplyui.Theme.
// One-way conversion: internal → public only. Never the reverse.
func BridgeTheme(t Theme) siplyui.Theme {
	bridge := func(tok Token) siplyui.Token {
		return siplyui.Token{
			TrueColor: tok.TrueColor,
			Color256:  tok.Color256,
			Color16:   tok.Color16,
			NoColor:   tok.NoColor,
		}
	}
	return siplyui.Theme{
		Primary:   bridge(t.Primary),
		Secondary: bridge(t.Secondary),
		Text:      bridge(t.Text),
		TextMuted: bridge(t.TextMuted),
		Success:   bridge(t.Success),
		Warning:   bridge(t.Warning),
		Error:     bridge(t.Error),
		Border:    bridge(t.Border),
		Highlight: bridge(t.Highlight),
		Heading:   bridge(t.Heading),
		Body:      bridge(t.Body),
		Muted:     bridge(t.Muted),
		CodePath:  bridge(t.CodePath),
		Link:      bridge(t.Link),
		Keybind:   bridge(t.Keybind),
	}
}

// BridgeRenderConfig converts an internal RenderConfig to the public siplyui.RenderConfig.
// One-way conversion: internal → public only. Never the reverse.
func BridgeRenderConfig(rc RenderConfig) siplyui.RenderConfig {
	mapColor := func(c ColorSetting) siplyui.ColorSetting {
		switch c {
		case ColorTrueColor:
			return siplyui.ColorTrueColor
		case Color256Color:
			return siplyui.Color256Color
		case Color16Color:
			return siplyui.Color16Color
		case ColorNone:
			return siplyui.ColorNone
		default:
			return siplyui.ColorNone
		}
	}
	mapBorder := func(b BorderStyle) siplyui.BorderStyle {
		switch b {
		case BorderUnicode:
			return siplyui.BorderUnicode
		case BorderASCII:
			return siplyui.BorderASCII
		case BorderNone:
			return siplyui.BorderNone
		default:
			return siplyui.BorderNone
		}
	}
	mapVerbosity := func(v Verbosity) siplyui.Verbosity {
		switch v {
		case VerbosityFull:
			return siplyui.VerbosityFull
		case VerbosityCompact:
			return siplyui.VerbosityCompact
		case VerbosityAccessible:
			return siplyui.VerbosityAccessible
		default:
			return siplyui.VerbosityFull
		}
	}
	return siplyui.RenderConfig{
		Color:     mapColor(rc.Color),
		Emoji:     rc.Emoji,
		Borders:   mapBorder(rc.Borders),
		Verbosity: mapVerbosity(rc.Verbosity),
	}
}
