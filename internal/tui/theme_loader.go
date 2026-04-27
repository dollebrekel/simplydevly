// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tui

import (
	"fmt"
	"log"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

// ThemeColors holds the customizable color values for a theme.
type ThemeColors struct {
	Primary   string `yaml:"primary"`
	Secondary string `yaml:"secondary"`
	Accent    string `yaml:"accent"`
	TextMuted string `yaml:"text_muted"`
	Success   string `yaml:"success"`
	Warning   string `yaml:"warning"`
	Error     string `yaml:"error"`
	Border    string `yaml:"border"`
	Highlight string `yaml:"highlight"`
}

// themeFile represents the YAML structure of a theme file.
type themeFile struct {
	Name   string      `yaml:"name"`
	Preset string      `yaml:"preset"`
	Colors ThemeColors `yaml:"colors"`
}

// Built-in theme presets keyed by name.
var presets = map[string]ThemeColors{
	"tokyo-night": {},
	"simply-purple": {
		Primary:   "#7C3AED",
		Secondary: "#F97316",
		Accent:    "#F97316",
		TextMuted: "#8B8FA3",
		Success:   "#10B981",
		Warning:   "#F97316",
		Error:     "#EF4444",
		Border:    "#5B21B6",
		Highlight: "#2E1065",
	},
}

// hexPattern validates #RRGGBB format.
var hexPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// validateHex checks whether s is a valid #RRGGBB hex color string.
func validateHex(s string) error {
	if !hexPattern.MatchString(s) {
		return fmt.Errorf("invalid hex color %q: expected #RRGGBB format", s)
	}
	return nil
}

// withDefaults fills in empty ThemeColors fields with default Tokyo Night values.
func (tc ThemeColors) withDefaults() ThemeColors {
	if tc.Primary == "" {
		tc.Primary = hexPrimary
	}
	if tc.Secondary == "" {
		tc.Secondary = hexSecondary
	}
	if tc.Accent == "" {
		tc.Accent = hexAccent
	}
	if tc.TextMuted == "" {
		tc.TextMuted = hexTextMuted
	}
	if tc.Success == "" {
		tc.Success = hexSecondary
	}
	if tc.Warning == "" {
		tc.Warning = hexWarning
	}
	if tc.Error == "" {
		tc.Error = hexError
	}
	if tc.Border == "" {
		tc.Border = hexBorder
	}
	if tc.Highlight == "" {
		tc.Highlight = hexHighlight
	}
	return tc
}

// LoadTheme reads a YAML theme file and returns a Theme.
// Falls back to DefaultTheme() if the file is not found or cannot be parsed.
// If a preset is specified, its colors are used as the base and individual
// color fields override the preset values.
func LoadTheme(path string) (Theme, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("theme file not found: %s, using defaults", path)
			return DefaultTheme(), nil
		}
		return DefaultTheme(), fmt.Errorf("read theme file: %w", err)
	}

	var tf themeFile
	if err := yaml.Unmarshal(data, &tf); err != nil {
		log.Printf("failed to parse theme file: %v, using defaults", err)
		return DefaultTheme(), nil
	}

	colors := tf.Colors

	// Apply preset as base if specified.
	if tf.Preset != "" {
		preset, ok := presets[tf.Preset]
		if !ok {
			return DefaultTheme(), fmt.Errorf("unknown theme preset %q", tf.Preset)
		}
		colors = applyPreset(preset, colors)
	}

	// Validate non-empty hex values.
	fields := map[string]string{
		"primary":    colors.Primary,
		"secondary":  colors.Secondary,
		"accent":     colors.Accent,
		"text_muted": colors.TextMuted,
		"success":    colors.Success,
		"warning":    colors.Warning,
		"error":      colors.Error,
		"border":     colors.Border,
		"highlight":  colors.Highlight,
	}
	for name, val := range fields {
		if val != "" {
			if err := validateHex(val); err != nil {
				return DefaultTheme(), fmt.Errorf("theme color %q: %w", name, err)
			}
		}
	}

	return ThemeFromColors(colors), nil
}

// applyPreset uses preset colors as the base and overlays any non-empty
// user-specified colors on top.
func applyPreset(preset, user ThemeColors) ThemeColors {
	result := preset
	if user.Primary != "" {
		result.Primary = user.Primary
	}
	if user.Secondary != "" {
		result.Secondary = user.Secondary
	}
	if user.Accent != "" {
		result.Accent = user.Accent
	}
	if user.TextMuted != "" {
		result.TextMuted = user.TextMuted
	}
	if user.Success != "" {
		result.Success = user.Success
	}
	if user.Warning != "" {
		result.Warning = user.Warning
	}
	if user.Error != "" {
		result.Error = user.Error
	}
	if user.Border != "" {
		result.Border = user.Border
	}
	if user.Highlight != "" {
		result.Highlight = user.Highlight
	}
	return result
}
