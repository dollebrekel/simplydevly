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
	Colors ThemeColors `yaml:"colors"`
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

	// Validate non-empty hex values.
	fields := map[string]string{
		"primary":   tf.Colors.Primary,
		"secondary": tf.Colors.Secondary,
		"text_muted": tf.Colors.TextMuted,
		"success":   tf.Colors.Success,
		"warning":   tf.Colors.Warning,
		"error":     tf.Colors.Error,
		"border":    tf.Colors.Border,
		"highlight": tf.Colors.Highlight,
	}
	for name, val := range fields {
		if val != "" {
			if err := validateHex(val); err != nil {
				return DefaultTheme(), fmt.Errorf("theme color %q: %w", name, err)
			}
		}
	}

	return ThemeFromColors(tf.Colors), nil
}
