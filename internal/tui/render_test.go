// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewRenderConfig_Defaults(t *testing.T) {
	caps := Capabilities{
		ColorDepth: TrueColor,
		Unicode:    true,
		Emoji:      true,
		IsTTY:      true,
	}
	cfg := NewRenderConfig(caps, CLIFlags{})

	assert.Equal(t, ColorTrueColor, cfg.Color)
	assert.True(t, cfg.Emoji)
	assert.Equal(t, BorderUnicode, cfg.Borders)
	assert.Equal(t, MotionSpinner, cfg.Motion)
	assert.Equal(t, VerbosityFull, cfg.Verbosity)
}

func TestNewRenderConfig_SSHDefaultsASCIIBorders(t *testing.T) {
	caps := Capabilities{
		ColorDepth: Color256,
		Unicode:    true,
		Emoji:      true,
		SSHSession: true,
		IsTTY:      true,
	}
	cfg := NewRenderConfig(caps, CLIFlags{})
	assert.Equal(t, BorderASCII, cfg.Borders)
}

func TestNewRenderConfig_AccessiblePreset(t *testing.T) {
	caps := Capabilities{
		ColorDepth: TrueColor,
		Unicode:    true,
		Emoji:      true,
		IsTTY:      true,
	}
	cfg := NewRenderConfig(caps, CLIFlags{Accessible: true})

	assert.Equal(t, BorderNone, cfg.Borders)
	assert.Equal(t, MotionStatic, cfg.Motion)
	assert.Equal(t, VerbosityAccessible, cfg.Verbosity)
	assert.False(t, cfg.Emoji)
}

func TestNewRenderConfig_LowBandwidthPreset(t *testing.T) {
	caps := Capabilities{
		ColorDepth: TrueColor,
		Unicode:    true,
		Emoji:      true,
		IsTTY:      true,
	}
	cfg := NewRenderConfig(caps, CLIFlags{LowBandwidth: true})

	assert.Equal(t, BorderASCII, cfg.Borders)
	assert.Equal(t, MotionStatic, cfg.Motion)
	assert.Equal(t, VerbosityCompact, cfg.Verbosity)
	assert.False(t, cfg.Emoji)
}

func TestNewRenderConfig_IndividualFlagOverrides(t *testing.T) {
	caps := Capabilities{
		ColorDepth: TrueColor,
		Unicode:    true,
		Emoji:      true,
		IsTTY:      true,
	}
	flags := CLIFlags{
		NoColor:   true,
		NoEmoji:   true,
		NoBorders: true,
		NoMotion:  true,
	}
	cfg := NewRenderConfig(caps, flags)

	assert.Equal(t, ColorNone, cfg.Color)
	assert.False(t, cfg.Emoji)
	assert.Equal(t, BorderNone, cfg.Borders)
	assert.Equal(t, MotionStatic, cfg.Motion)
}

func TestNewRenderConfig_PipedOutput(t *testing.T) {
	caps := Capabilities{
		ColorDepth: TrueColor,
		Unicode:    true,
		Emoji:      true,
		IsTTY:      false, // piped
	}
	cfg := NewRenderConfig(caps, CLIFlags{})

	assert.Equal(t, ColorNone, cfg.Color)
	assert.False(t, cfg.Emoji)
	assert.Equal(t, MotionStatic, cfg.Motion)
}

func TestNewRenderConfig_NoColorDepth(t *testing.T) {
	caps := Capabilities{
		ColorDepth: NoColor,
		IsTTY:      true,
	}
	cfg := NewRenderConfig(caps, CLIFlags{})
	assert.Equal(t, ColorNone, cfg.Color)
}

func TestRenderBorder_Unicode(t *testing.T) {
	cfg := RenderConfig{Borders: BorderUnicode}
	result := RenderBorder("test", "hello", cfg, 20)

	assert.Contains(t, result, "┌")
	assert.Contains(t, result, "┐")
	assert.Contains(t, result, "│")
	assert.Contains(t, result, "└")
	assert.Contains(t, result, "┘")
	assert.Contains(t, result, "test")
	assert.Contains(t, result, "hello")
}

func TestRenderBorder_ASCII(t *testing.T) {
	cfg := RenderConfig{Borders: BorderASCII}
	result := RenderBorder("test", "hello", cfg, 20)

	assert.Contains(t, result, "+")
	assert.Contains(t, result, "-")
	assert.Contains(t, result, "|")
	assert.Contains(t, result, "test")
	assert.Contains(t, result, "hello")
	// Should NOT contain unicode box-drawing.
	assert.NotContains(t, result, "┌")
	assert.NotContains(t, result, "│")
}

func TestRenderBorder_None_Accessible(t *testing.T) {
	cfg := RenderConfig{Borders: BorderNone}
	result := RenderBorder("test", "hello", cfg, 20)

	assert.Contains(t, result, "== test ==")
	assert.Contains(t, result, "hello")
	// Should NOT contain any box-drawing chars.
	assert.NotContains(t, result, "┌")
	assert.NotContains(t, result, "│")
	assert.NotContains(t, result, "+")
	assert.NotContains(t, result, "|")
}

func TestRenderBorder_AccessibleNoBoxDrawing(t *testing.T) {
	cfg := RenderConfig{Borders: BorderNone, Verbosity: VerbosityAccessible}
	result := RenderBorder("Title", "Content here", cfg, 40)

	// Verify no box-drawing characters exist.
	boxChars := []string{"┌", "─", "┐", "│", "└", "┘"}
	for _, ch := range boxChars {
		assert.False(t, strings.Contains(result, ch),
			"accessible mode should not contain box-drawing char %q", ch)
	}
}

func TestNewRenderConfig_NO_COLOR_Env(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	caps := Capabilities{
		ColorDepth: TrueColor,
		Unicode:    true,
		Emoji:      true,
		IsTTY:      true,
	}
	cfg := NewRenderConfig(caps, CLIFlags{})
	assert.Equal(t, ColorNone, cfg.Color)
}

func TestNewRenderConfig_UnicodeFallbackASCII(t *testing.T) {
	caps := Capabilities{
		Unicode: false,
		IsTTY:   true,
	}
	cfg := NewRenderConfig(caps, CLIFlags{})
	assert.Equal(t, BorderASCII, cfg.Borders)
}

func TestColorSettingFromDepth(t *testing.T) {
	tests := []struct {
		depth    ColorDepth
		expected ColorSetting
	}{
		{TrueColor, ColorTrueColor},
		{Color256, Color256Color},
		{Color16, Color16Color},
		{NoColor, ColorNone},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, colorSettingFromDepth(tt.depth))
	}
}
