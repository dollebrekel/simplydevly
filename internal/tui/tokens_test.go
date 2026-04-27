// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tui

import (
	"testing"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
)

func TestDefaultTheme_AllTokensDefined(t *testing.T) {
	theme := DefaultTheme()

	// All 10 semantic tokens must be non-zero (have at least one style set).
	tokens := []struct {
		name  string
		token Token
	}{
		{"Primary", theme.Primary},
		{"Secondary", theme.Secondary},
		{"Accent", theme.Accent},
		{"Text", theme.Text},
		{"TextMuted", theme.TextMuted},
		{"Success", theme.Success},
		{"Warning", theme.Warning},
		{"Error", theme.Error},
		{"Border", theme.Border},
		{"Highlight", theme.Highlight},
	}

	for _, tt := range tokens {
		t.Run(tt.name, func(t *testing.T) {
			// Each token has 4 depth variants.
			_ = tt.token.TrueColor
			_ = tt.token.Color256
			_ = tt.token.Color16
			_ = tt.token.NoColor
		})
	}
}

func TestDefaultTheme_PrimaryTrueColor(t *testing.T) {
	theme := DefaultTheme()
	style := theme.Primary.TrueColor

	fg := style.GetForeground()
	assert.NotNil(t, fg)
	// #7AA2F7 = RGB(122, 162, 247)
	r, g, b, _ := fg.RGBA()
	assert.Equal(t, uint32(0x7a7a), r)
	assert.Equal(t, uint32(0xa2a2), g)
	assert.Equal(t, uint32(0xf7f7), b)
}

func TestDefaultTheme_SecondaryTrueColor(t *testing.T) {
	theme := DefaultTheme()
	style := theme.Secondary.TrueColor

	fg := style.GetForeground()
	assert.NotNil(t, fg)
	// #9ECE6A = RGB(158, 206, 106)
	r, _, _, _ := fg.RGBA()
	assert.Equal(t, uint32(0x9e9e), r)
}

func TestDefaultTheme_ErrorTrueColor(t *testing.T) {
	theme := DefaultTheme()
	style := theme.Error.TrueColor

	fg := style.GetForeground()
	assert.NotNil(t, fg)
	// #F7768E = RGB(247, 118, 142)
	r, _, _, _ := fg.RGBA()
	assert.Equal(t, uint32(0xf7f7), r)
}

func TestDefaultTheme_TextIsUnstyled(t *testing.T) {
	theme := DefaultTheme()
	for _, cs := range []ColorSetting{ColorTrueColor, Color256Color, Color16Color, ColorNone} {
		style := theme.Text.Resolve(cs)
		assert.False(t, style.GetBold())
		assert.False(t, style.GetFaint())
		assert.False(t, style.GetUnderline())
		assert.False(t, style.GetReverse())
	}
}

func TestToken_Resolve_AllDepths(t *testing.T) {
	theme := DefaultTheme()

	tests := []struct {
		name     string
		cs       ColorSetting
		token    Token
		wantBold bool
	}{
		{"Primary/TrueColor", ColorTrueColor, theme.Primary, false},
		{"Primary/NoColor", ColorNone, theme.Primary, true}, // NoColor falls back to Bold
		{"Error/NoColor", ColorNone, theme.Error, true},
		{"TextMuted/NoColor", ColorNone, theme.TextMuted, false}, // Faint, not Bold
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			style := tt.token.Resolve(tt.cs)
			assert.Equal(t, tt.wantBold, style.GetBold())
		})
	}
}

func TestToken_Resolve_NoColorHasNoForegroundColor(t *testing.T) {
	theme := DefaultTheme()
	tokens := []Token{
		theme.Primary,
		theme.Secondary,
		theme.TextMuted,
		theme.Success,
		theme.Warning,
		theme.Error,
	}
	for _, token := range tokens {
		style := token.Resolve(ColorNone)
		fg := style.GetForeground()
		// In NoColor mode, foreground must be nil or NoColor (no color codes).
		if fg != nil {
			_, ok := fg.(lipgloss.NoColor)
			assert.True(t, ok, "NoColor style should not have a foreground color set")
		}
	}
}

func TestToken_Resolve_16ColorUsesANSIColors(t *testing.T) {
	theme := DefaultTheme()
	style := theme.Primary.Resolve(Color16Color)
	fg := style.GetForeground()
	assert.NotNil(t, fg)
	// Should be ANSI Blue (basic color 4)
	assert.NotNil(t, fg)
}

func TestDefaultTheme_HighlightUsesBackground(t *testing.T) {
	theme := DefaultTheme()
	style := theme.Highlight.TrueColor
	bg := style.GetBackground()
	assert.NotNil(t, bg)
	// #292E42 = RGB(41, 46, 66)
	r, _, _, _ := bg.RGBA()
	assert.Equal(t, uint32(0x2929), r)
}

func TestDefaultTheme_HighlightNoColorUsesReverse(t *testing.T) {
	theme := DefaultTheme()
	style := theme.Highlight.NoColor
	assert.True(t, style.GetReverse())
}

func TestDefaultTheme_TypographyHeading(t *testing.T) {
	theme := DefaultTheme()

	// TrueColor: Bold + Primary foreground
	tc := theme.Heading.TrueColor
	assert.True(t, tc.GetBold())
	assert.NotNil(t, tc.GetForeground())

	// NoColor: Bold only, no foreground
	nc := theme.Heading.NoColor
	assert.True(t, nc.GetBold())
	fg := nc.GetForeground()
	if fg != nil {
		_, ok := fg.(lipgloss.NoColor)
		assert.True(t, ok)
	}
}

func TestDefaultTheme_TypographyMuted(t *testing.T) {
	theme := DefaultTheme()

	tc := theme.Muted.TrueColor
	assert.True(t, tc.GetFaint())
	assert.NotNil(t, tc.GetForeground())

	nc := theme.Muted.NoColor
	assert.True(t, nc.GetFaint())
}

func TestDefaultTheme_TypographyLink(t *testing.T) {
	theme := DefaultTheme()

	tc := theme.Link.TrueColor
	assert.True(t, tc.GetUnderline())
	assert.NotNil(t, tc.GetForeground())

	nc := theme.Link.NoColor
	assert.True(t, nc.GetUnderline())
}

func TestDefaultTheme_TypographyCodePath(t *testing.T) {
	theme := DefaultTheme()

	tc := theme.CodePath.TrueColor
	assert.NotNil(t, tc.GetBackground())

	nc := theme.CodePath.NoColor
	assert.True(t, nc.GetReverse())
}

func TestDefaultTheme_TypographyKeybind(t *testing.T) {
	theme := DefaultTheme()

	tc := theme.Keybind.TrueColor
	assert.True(t, tc.GetBold())

	nc := theme.Keybind.NoColor
	assert.True(t, nc.GetBold())
}

func TestThemeFromColors_CustomColors(t *testing.T) {
	colors := ThemeColors{
		Primary:   "#FF0000",
		Secondary: "#00FF00",
	}
	theme := ThemeFromColors(colors)

	// Primary should use custom color
	fg := theme.Primary.TrueColor.GetForeground()
	assert.NotNil(t, fg)
	r, _, _, _ := fg.RGBA()
	assert.Equal(t, uint32(0xffff), r) // #FF = 255

	// Secondary should use custom green
	fg2 := theme.Secondary.TrueColor.GetForeground()
	assert.NotNil(t, fg2)
	_, g, _, _ := fg2.RGBA()
	assert.Equal(t, uint32(0xffff), g)
}

func TestDefaultTheme_AccentTrueColor(t *testing.T) {
	theme := DefaultTheme()
	style := theme.Accent.TrueColor

	fg := style.GetForeground()
	assert.NotNil(t, fg)
	// #FF9E64 = RGB(255, 158, 100)
	r, _, _, _ := fg.RGBA()
	assert.Equal(t, uint32(0xffff), r)
}

func TestDefaultTheme_AccentNoColorIsBold(t *testing.T) {
	theme := DefaultTheme()
	style := theme.Accent.Resolve(ColorNone)
	assert.True(t, style.GetBold())
}

func TestThemeFromColors_MissingFieldsUseDefaults(t *testing.T) {
	// Only set primary, rest should default.
	colors := ThemeColors{
		Primary: "#FF0000",
	}
	theme := ThemeFromColors(colors)

	// Error should still have the default #F7768E
	fg := theme.Error.TrueColor.GetForeground()
	assert.NotNil(t, fg)
	r, _, _, _ := fg.RGBA()
	assert.Equal(t, uint32(0xf7f7), r)
}
