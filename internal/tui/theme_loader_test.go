// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadTheme_ValidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.yaml")

	yaml := `name: "custom"
colors:
  primary: "#FF0000"
  secondary: "#00FF00"
  text_muted: "#888888"
  success: "#00FF00"
  warning: "#FFAA00"
  error: "#FF0000"
  border: "#333333"
  highlight: "#222222"
`
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0644))

	theme, err := LoadTheme(path)
	require.NoError(t, err)

	// Primary should use custom red.
	fg := theme.Primary.TrueColor.GetForeground()
	assert.NotNil(t, fg)
	r, _, _, _ := fg.RGBA()
	assert.Equal(t, uint32(0xffff), r)
}

func TestLoadTheme_PartialYAML_UsesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.yaml")

	yaml := `name: "partial"
colors:
  primary: "#FF0000"
`
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0644))

	theme, err := LoadTheme(path)
	require.NoError(t, err)

	// Primary should be custom.
	fg := theme.Primary.TrueColor.GetForeground()
	assert.NotNil(t, fg)
	r, _, _, _ := fg.RGBA()
	assert.Equal(t, uint32(0xffff), r)

	// Error should be default #F7768E.
	fg2 := theme.Error.TrueColor.GetForeground()
	assert.NotNil(t, fg2)
	r2, _, _, _ := fg2.RGBA()
	assert.Equal(t, uint32(0xf7f7), r2)
}

func TestLoadTheme_FileNotFound_ReturnsDefault(t *testing.T) {
	theme, err := LoadTheme("/nonexistent/path/theme.yaml")
	require.NoError(t, err)

	// Should be default theme.
	fg := theme.Primary.TrueColor.GetForeground()
	assert.NotNil(t, fg)
	r, _, _, _ := fg.RGBA()
	assert.Equal(t, uint32(0x7a7a), r) // #7A from #7AA2F7
}

func TestLoadTheme_Unreadable_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.yaml")

	require.NoError(t, os.WriteFile(path, []byte("name: test"), 0000))

	theme, err := LoadTheme(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "read theme file")
	// Should still return default theme as fallback value.
	fg := theme.Primary.TrueColor.GetForeground()
	assert.NotNil(t, fg)
}

func TestLoadTheme_InvalidYAML_ReturnsDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.yaml")

	require.NoError(t, os.WriteFile(path, []byte("{{{{not yaml"), 0644))

	theme, err := LoadTheme(path)
	require.NoError(t, err) // Falls back, no error returned

	// Should be default.
	fg := theme.Primary.TrueColor.GetForeground()
	assert.NotNil(t, fg)
	r, _, _, _ := fg.RGBA()
	assert.Equal(t, uint32(0x7a7a), r)
}

func TestLoadTheme_InvalidHex_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.yaml")

	yaml := `name: "bad"
colors:
  primary: "not-a-color"
`
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0644))

	_, err := LoadTheme(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid hex color")
}

func TestLoadTheme_ShortHex_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.yaml")

	yaml := `name: "short"
colors:
  primary: "#FFF"
`
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0644))

	_, err := LoadTheme(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid hex color")
}

func TestValidateHex(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"#7AA2F7", true},
		{"#000000", true},
		{"#FFFFFF", true},
		{"#ffffff", true},
		{"#aabbcc", true},
		{"#FFF", false},      // Too short
		{"7AA2F7", false},    // Missing #
		{"#GGGGGG", false},   // Invalid chars
		{"not-a-color", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			err := validateHex(tt.input)
			if tt.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestThemeColors_WithDefaults(t *testing.T) {
	empty := ThemeColors{}
	filled := empty.withDefaults()

	assert.Equal(t, hexPrimary, filled.Primary)
	assert.Equal(t, hexSecondary, filled.Secondary)
	assert.Equal(t, hexTextMuted, filled.TextMuted)
	assert.Equal(t, hexSecondary, filled.Success) // Success defaults to same as Secondary
	assert.Equal(t, hexWarning, filled.Warning)
	assert.Equal(t, hexError, filled.Error)
	assert.Equal(t, hexBorder, filled.Border)
	assert.Equal(t, hexHighlight, filled.Highlight)
}

func TestThemeColors_WithDefaults_PreservesCustom(t *testing.T) {
	custom := ThemeColors{
		Primary: "#FF0000",
		Error:   "#00FF00",
	}
	filled := custom.withDefaults()

	assert.Equal(t, "#FF0000", filled.Primary)
	assert.Equal(t, "#00FF00", filled.Error)
	// Others should be defaults.
	assert.Equal(t, hexSecondary, filled.Secondary)
}
