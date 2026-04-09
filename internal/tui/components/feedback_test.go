// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package components

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"

	"siply.dev/siply/internal/tui"
)

func testConfigTrueColor() tui.RenderConfig {
	return tui.RenderConfig{
		Color:     tui.ColorTrueColor,
		Emoji:     true,
		Borders:   tui.BorderUnicode,
		Motion:    tui.MotionSpinner,
		Verbosity: tui.VerbosityFull,
	}
}

func testConfigNoColor() tui.RenderConfig {
	return tui.RenderConfig{
		Color:     tui.ColorNone,
		Emoji:     false,
		Borders:   tui.BorderUnicode,
		Motion:    tui.MotionSpinner,
		Verbosity: tui.VerbosityFull,
	}
}

func TestRenderFeedback_AllLevels_TrueColor(t *testing.T) {
	theme := testTheme()
	rc := testConfigTrueColor()

	tests := []struct {
		name     string
		level    tui.FeedbackLevel
		summary  string
		detail   string
		action   string
		contains []string
	}{
		{
			"Success",
			tui.LevelSuccess,
			"Plugin installed: tree-view v2.1.0",
			"", "",
			[]string{"\u2705", "Plugin installed: tree-view v2.1.0"},
		},
		{
			"Error",
			tui.LevelError,
			"Failed to install plugin",
			"Version conflict",
			"Run: siply plugins remove --force",
			[]string{"\u274C", "Failed to install plugin", "Why: Version conflict", "Fix: Run: siply plugins remove --force"},
		},
		{
			"Warning",
			tui.LevelWarning,
			"This will overwrite config",
			"Use --force to skip",
			"",
			[]string{"\u26A0\uFE0F", "This will overwrite config", "[?] Use --force to skip"},
		},
		{
			"Info",
			tui.LevelInfo,
			"3 updates available",
			"", "",
			[]string{"\u2139\uFE0F", "3 updates available"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tui.FeedbackMsg{
				Level:   tt.level,
				Summary: tt.summary,
				Detail:  tt.detail,
				Action:  tt.action,
			}
			result := RenderFeedback(msg, &theme, &rc, 120)
			stripped := ansi.Strip(result)
			for _, c := range tt.contains {
				assert.Contains(t, stripped, c)
			}
		})
	}
}

func TestRenderFeedback_AllLevels_NoColor(t *testing.T) {
	theme := testTheme()
	rc := testConfigNoColor()

	tests := []struct {
		name     string
		level    tui.FeedbackLevel
		summary  string
		contains []string
	}{
		{"Success", tui.LevelSuccess, "Done", []string{"OK:", "Done"}},
		{"Error", tui.LevelError, "Failed", []string{"ERROR:", "Failed", "Why:", "Fix:"}},
		{"Warning", tui.LevelWarning, "Watch out", []string{"!!", "Watch out"}},
		{"Info", tui.LevelInfo, "Note this", []string{"i:", "Note this"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tui.FeedbackMsg{
				Level:   tt.level,
				Summary: tt.summary,
			}
			result := RenderFeedback(msg, &theme, &rc, 120)
			stripped := ansi.Strip(result)
			for _, c := range tt.contains {
				assert.Contains(t, stripped, c)
			}
		})
	}
}

func TestRenderFeedback_AllLevels_Accessible(t *testing.T) {
	theme := testTheme()
	rc := testConfigAccessible()

	tests := []struct {
		name     string
		level    tui.FeedbackLevel
		summary  string
		detail   string
		action   string
		contains string
	}{
		{"Success", tui.LevelSuccess, "Plugin installed", "", "", "[OK] Plugin installed"},
		{"Error", tui.LevelError, "Failed", "conflict", "retry", "[ERROR] Failed | conflict | retry"},
		{"Warning", tui.LevelWarning, "Watch out", "", "", "[WARN] Watch out"},
		{"Info", tui.LevelInfo, "3 updates", "", "", "[INFO] 3 updates"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tui.FeedbackMsg{
				Level:   tt.level,
				Summary: tt.summary,
				Detail:  tt.detail,
				Action:  tt.action,
			}
			result := RenderFeedback(msg, &theme, &rc, 120)
			assert.Contains(t, result, tt.contains)
			// Accessible mode: no emoji, no ANSI codes.
			assert.NotContains(t, result, "\u2705")
			assert.NotContains(t, result, "\u274C")
			assert.Equal(t, result, ansi.Strip(result), "Accessible mode should have no ANSI codes")
		})
	}
}

func TestRenderFeedback_ErrorDefaults(t *testing.T) {
	theme := testTheme()
	rc := testConfigNoColor()

	msg := tui.FeedbackMsg{
		Level:   tui.LevelError,
		Summary: "Something broke",
	}
	result := RenderFeedback(msg, &theme, &rc, 120)
	stripped := ansi.Strip(result)
	assert.Contains(t, stripped, "Why: Unknown cause")
	assert.Contains(t, stripped, "Fix: Check logs for details")
}

func TestRenderFeedback_ErrorThreeLines(t *testing.T) {
	theme := testTheme()
	rc := testConfigNoColor()

	msg := tui.FeedbackMsg{
		Level:   tui.LevelError,
		Summary: "Failed",
		Detail:  "Because reasons",
		Action:  "Fix it",
	}
	result := RenderFeedback(msg, &theme, &rc, 120)
	lines := strings.Split(ansi.Strip(result), "\n")
	assert.Equal(t, 3, len(lines), "Error must be exactly 3 lines")
}

func TestRenderFeedback_WarningWithoutDetail(t *testing.T) {
	theme := testTheme()
	rc := testConfigNoColor()

	msg := tui.FeedbackMsg{
		Level:   tui.LevelWarning,
		Summary: "Simple warning",
	}
	result := RenderFeedback(msg, &theme, &rc, 120)
	stripped := ansi.Strip(result)
	assert.Contains(t, stripped, "Simple warning")
	assert.NotContains(t, stripped, "[?]")
}

func TestRenderFeedback_Truncation(t *testing.T) {
	theme := testTheme()
	rc := testConfigNoColor()

	msg := tui.FeedbackMsg{
		Level:   tui.LevelSuccess,
		Summary: strings.Repeat("A", 200),
	}
	result := RenderFeedback(msg, &theme, &rc, 40)
	for _, line := range strings.Split(result, "\n") {
		assert.LessOrEqual(t, ansi.StringWidth(line), 40, "Each line must respect width")
	}
}

func TestRenderFeedback_EmojiToggle(t *testing.T) {
	theme := testTheme()

	// Emoji on.
	rcOn := tui.RenderConfig{Color: tui.ColorNone, Emoji: true, Verbosity: tui.VerbosityFull}
	msg := tui.FeedbackMsg{Level: tui.LevelSuccess, Summary: "Done"}
	resultOn := RenderFeedback(msg, &theme, &rcOn, 120)
	assert.Contains(t, ansi.Strip(resultOn), "\u2705")

	// Emoji off.
	rcOff := tui.RenderConfig{Color: tui.ColorNone, Emoji: false, Verbosity: tui.VerbosityFull}
	resultOff := RenderFeedback(msg, &theme, &rcOff, 120)
	stripped := ansi.Strip(resultOff)
	assert.Contains(t, stripped, "OK:")
	assert.NotContains(t, stripped, "\u2705")
}

func TestRenderFeedback_MinWidth(t *testing.T) {
	theme := testTheme()
	rc := testConfigNoColor()

	msg := tui.FeedbackMsg{Level: tui.LevelInfo, Summary: "test"}
	result := RenderFeedback(msg, &theme, &rc, 0)
	assert.NotEmpty(t, result, "Should use default width when 0")
}

// --- Task 7: Integration and edge cases ---

func TestRenderFeedback_AllLevels_EmojiTrue(t *testing.T) {
	theme := testTheme()
	rc := tui.RenderConfig{Color: tui.ColorNone, Emoji: true, Verbosity: tui.VerbosityFull}
	levels := []struct {
		level tui.FeedbackLevel
		emoji string
	}{
		{tui.LevelSuccess, "\u2705"},
		{tui.LevelError, "\u274C"},
		{tui.LevelWarning, "\u26A0\uFE0F"},
		{tui.LevelInfo, "\u2139\uFE0F"},
	}
	for _, tt := range levels {
		msg := tui.FeedbackMsg{Level: tt.level, Summary: "msg", Detail: "d", Action: "a"}
		result := RenderFeedback(msg, &theme, &rc, 120)
		assert.Contains(t, ansi.Strip(result), tt.emoji)
	}
}

func TestRenderFeedback_AllLevels_EmojiFalse(t *testing.T) {
	theme := testTheme()
	rc := tui.RenderConfig{Color: tui.ColorNone, Emoji: false, Verbosity: tui.VerbosityFull}
	levels := []struct {
		level  tui.FeedbackLevel
		prefix string
	}{
		{tui.LevelSuccess, "OK:"},
		{tui.LevelError, "ERROR:"},
		{tui.LevelWarning, "!!"},
		{tui.LevelInfo, "i:"},
	}
	for _, tt := range levels {
		msg := tui.FeedbackMsg{Level: tt.level, Summary: "msg", Detail: "d", Action: "a"}
		result := RenderFeedback(msg, &theme, &rc, 120)
		assert.Contains(t, ansi.Strip(result), tt.prefix)
	}
}

func TestRenderFeedback_ColorNone_BoldOnly(t *testing.T) {
	theme := testTheme()
	rc := tui.RenderConfig{Color: tui.ColorNone, Emoji: false, Verbosity: tui.VerbosityFull}

	// Success uses Bold in NoColor mode (from Token.Resolve).
	msg := tui.FeedbackMsg{Level: tui.LevelSuccess, Summary: "Done"}
	result := RenderFeedback(msg, &theme, &rc, 120)
	// The raw result should contain ANSI bold escape (ESC[1m).
	assert.Contains(t, result, "\x1b[1m", "NoColor mode should use bold")
	// But no color codes (like 38;2;... for truecolor).
	assert.NotContains(t, result, "38;2;", "NoColor mode should NOT have foreground color")
}

func TestRenderFeedback_AccessibleNoColor(t *testing.T) {
	theme := testTheme()
	rc := testConfigAccessible()

	for _, level := range []tui.FeedbackLevel{tui.LevelSuccess, tui.LevelError, tui.LevelWarning, tui.LevelInfo} {
		msg := tui.FeedbackMsg{Level: level, Summary: "test", Detail: "why", Action: "fix"}
		result := RenderFeedback(msg, &theme, &rc, 120)
		assert.Equal(t, result, ansi.Strip(result), "Accessible mode should have zero ANSI")
	}
}
