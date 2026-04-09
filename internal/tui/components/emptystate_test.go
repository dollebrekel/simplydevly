// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package components

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"

	"siply.dev/siply/internal/tui"
)

func TestRenderEmptyState_NormalMode(t *testing.T) {
	theme := testTheme()
	rc := testConfig()
	msg := tui.EmptyStateMsg{
		Reason:     "No plugins installed",
		Suggestion: "siply plugins install <name>",
	}
	result := RenderEmptyState(msg, &theme, &rc, 120)
	stripped := ansi.Strip(result)
	assert.Contains(t, stripped, "No plugins installed")
	assert.Contains(t, stripped, "Try: siply plugins install <name>")
	assert.Contains(t, stripped, "--", "No-emoji mode should use text prefix")
}

func TestRenderEmptyState_EmojiMode(t *testing.T) {
	theme := testTheme()
	rc := testConfigEmoji()
	msg := tui.EmptyStateMsg{
		Reason:     "No plugins installed",
		Suggestion: "siply plugins install <name>",
	}
	result := RenderEmptyState(msg, &theme, &rc, 120)
	stripped := ansi.Strip(result)
	assert.Contains(t, stripped, "\U0001F4ED")
	assert.Contains(t, stripped, "No plugins installed")
}

func TestRenderEmptyState_AccessibleMode(t *testing.T) {
	theme := testTheme()
	rc := testConfigAccessible()
	msg := tui.EmptyStateMsg{
		Reason:     "No plugins installed",
		Suggestion: "siply plugins install <name>",
	}
	result := RenderEmptyState(msg, &theme, &rc, 120)
	assert.Contains(t, result, "[EMPTY]")
	assert.Contains(t, result, "No plugins installed")
	assert.Contains(t, result, "siply plugins install <name>")
	assert.Equal(t, result, ansi.Strip(result), "Accessible mode should have no ANSI codes")
}

func TestRenderEmptyState_NoColor(t *testing.T) {
	theme := testTheme()
	rc := testConfigNoColor()
	msg := tui.EmptyStateMsg{
		Reason:     "No data",
		Suggestion: "Add something",
	}
	result := RenderEmptyState(msg, &theme, &rc, 120)
	stripped := ansi.Strip(result)
	assert.Contains(t, stripped, "No data")
	assert.Contains(t, stripped, "Try: Add something")
}

func TestRenderEmptyState_Truncation(t *testing.T) {
	theme := testTheme()
	rc := testConfig()
	msg := tui.EmptyStateMsg{
		Reason:     "This is a very long reason that should be truncated at narrow terminal width settings",
		Suggestion: "This is a very long suggestion that should also be truncated at narrow width",
	}
	result := RenderEmptyState(msg, &theme, &rc, 30)
	for _, l := range splitLines(result) {
		assert.LessOrEqual(t, ansi.StringWidth(l), 30)
	}
}

// splitLines is defined in diffview.go — reused here.
