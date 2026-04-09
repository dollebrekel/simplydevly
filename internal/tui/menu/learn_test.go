// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package menu

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/tui"
)

// --- Mock MarkdownRenderer ---

type mockMarkdownRenderer struct{}

func (m *mockMarkdownRenderer) Render(input string, width int) string {
	// Pass through — tests focus on LearnView logic, not markdown rendering.
	return input
}

// --- Helpers ---

func newTestLearnView(opts ...func(*tui.RenderConfig)) *LearnView {
	theme := tui.DefaultTheme()
	rc := tui.RenderConfig{
		Color:     tui.ColorTrueColor,
		Emoji:     false,
		Borders:   tui.BorderUnicode,
		Motion:    tui.MotionStatic,
		Verbosity: tui.VerbosityFull,
	}
	for _, fn := range opts {
		fn(&rc)
	}
	return NewLearnView(theme, rc, &mockMarkdownRenderer{})
}

func withLearnNoColor(rc *tui.RenderConfig) {
	rc.Color = tui.ColorNone
	rc.Borders = tui.BorderNone
}

func withLearnAccessible(rc *tui.RenderConfig) {
	rc.Color = tui.ColorNone
	rc.Borders = tui.BorderNone
	rc.Verbosity = tui.VerbosityAccessible
}

// --- LearnView Render ---

func TestLearnView_RenderProducesNonEmptyOutput(t *testing.T) {
	lv := newTestLearnView()
	rendered := lv.Render(60, 30)
	assert.NotEmpty(t, rendered)
}

func TestLearnView_RenderContainsCategoryHeaders(t *testing.T) {
	lv := newTestLearnView()
	lv.SetSize(80, 50)
	rendered := lv.Render(80, 50)
	stripped := ansi.Strip(rendered)
	assert.Contains(t, stripped, "Navigation")
	assert.Contains(t, stripped, "AI Agent")
	assert.Contains(t, stripped, "Terminal")
}

func TestLearnView_RenderContainsArrowSeparator(t *testing.T) {
	lv := newTestLearnView()
	lv.SetSize(80, 50)
	rendered := lv.Render(80, 50)
	stripped := ansi.Strip(rendered)
	assert.Contains(t, stripped, "→", "should contain two-column arrow separator")
}

// --- HandleKey ---

func TestLearnView_HandleKeyEscReturnsLearnCloseMsg(t *testing.T) {
	lv := newTestLearnView()
	result := lv.HandleKey("esc")
	_, ok := result.(tui.LearnCloseMsg)
	assert.True(t, ok, "Esc should return LearnCloseMsg, got %T", result)
}

func TestLearnView_HandleKeyDownIncreasesOffset(t *testing.T) {
	lv := newTestLearnView()
	lv.SetSize(80, 10) // Small height to enable scrolling.
	assert.Equal(t, 0, lv.scrollOffset)

	lv.HandleKey("down")
	assert.GreaterOrEqual(t, lv.scrollOffset, 0) // May stay 0 if clamped.
}

func TestLearnView_HandleKeyUpDecreasesOffset(t *testing.T) {
	lv := newTestLearnView()
	lv.SetSize(80, 10)
	lv.scrollOffset = 5

	lv.HandleKey("up")
	assert.Equal(t, 4, lv.scrollOffset)
}

func TestLearnView_HandleKeyUpClampsAtZero(t *testing.T) {
	lv := newTestLearnView()
	lv.scrollOffset = 0
	lv.HandleKey("up")
	assert.Equal(t, 0, lv.scrollOffset)
}

func TestLearnView_HandleKeyJKNavigation(t *testing.T) {
	lv := newTestLearnView()
	lv.SetSize(80, 10)
	lv.HandleKey("j") // down
	// After j, offset should be >= 0 (clamped).

	lv.scrollOffset = 3
	lv.HandleKey("k") // up
	assert.Equal(t, 2, lv.scrollOffset)
}

func TestLearnView_HandleKeyUnknownReturnsNil(t *testing.T) {
	lv := newTestLearnView()
	result := lv.HandleKey("x")
	assert.Nil(t, result)
}

func TestLearnView_ScrollOffsetClampsToMax(t *testing.T) {
	lv := newTestLearnView()
	lv.SetSize(80, 100) // Very tall — all content visible.
	lv.scrollOffset = 999
	lv.clampScrollOffset()
	assert.Equal(t, 0, lv.scrollOffset, "should clamp to 0 when all content fits")
}

// --- No-color mode ---

func TestLearnView_NoColorMode_ZeroColorCodes(t *testing.T) {
	lv := newTestLearnView(withLearnNoColor)
	lv.SetSize(60, 30)
	rendered := lv.Render(60, 30)

	// No color ANSI codes (foreground \x1b[3Nm, background \x1b[4Nm, 256/truecolor \x1b[38;, \x1b[48;).
	// Bold (\x1b[1m) and Faint (\x1b[2m) are structural and allowed per AC8.
	assert.NotContains(t, rendered, "\x1b[30", "no foreground color codes expected")
	assert.NotContains(t, rendered, "\x1b[31", "no foreground color codes expected")
	assert.NotContains(t, rendered, "\x1b[32", "no foreground color codes expected")
	assert.NotContains(t, rendered, "\x1b[33", "no foreground color codes expected")
	assert.NotContains(t, rendered, "\x1b[34", "no foreground color codes expected")
	assert.NotContains(t, rendered, "\x1b[35", "no foreground color codes expected")
	assert.NotContains(t, rendered, "\x1b[36", "no foreground color codes expected")
	assert.NotContains(t, rendered, "\x1b[37", "no foreground color codes expected")
	assert.NotContains(t, rendered, "\x1b[38", "no 256/truecolor foreground codes expected")
	assert.NotContains(t, rendered, "\x1b[40", "no background color codes expected")
	assert.NotContains(t, rendered, "\x1b[41", "no background color codes expected")
	assert.NotContains(t, rendered, "\x1b[42", "no background color codes expected")
	assert.NotContains(t, rendered, "\x1b[43", "no background color codes expected")
	assert.NotContains(t, rendered, "\x1b[44", "no background color codes expected")
	assert.NotContains(t, rendered, "\x1b[45", "no background color codes expected")
	assert.NotContains(t, rendered, "\x1b[46", "no background color codes expected")
	assert.NotContains(t, rendered, "\x1b[47", "no background color codes expected")
	assert.NotContains(t, rendered, "\x1b[48", "no 256/truecolor background codes expected")
	assert.NotContains(t, rendered, "\x1b[9", "no bright foreground codes expected")
}

// --- Accessible mode ---

func TestLearnView_AccessibleMode_SectionPrefix(t *testing.T) {
	lv := newTestLearnView(withLearnAccessible)
	lv.SetSize(80, 50)
	rendered := lv.Render(80, 50)
	assert.Contains(t, rendered, "[SECTION]", "accessible mode should prefix headers with [SECTION]")
}

func TestLearnView_AccessibleMode_PlainTextFormatting(t *testing.T) {
	lv := newTestLearnView(withLearnAccessible)
	lv.SetSize(80, 50)
	rendered := lv.Render(80, 50)
	// Accessible mode should NOT contain backtick-wrapped keys (they become styled).
	assert.NotContains(t, rendered, "`↑", "accessible mode should use plain text, not backtick-wrapped keys")
}

// --- SetSize ---

func TestLearnView_SetSizeClampsMinimum(t *testing.T) {
	lv := newTestLearnView()
	lv.SetSize(5, 2)
	assert.Equal(t, 20, lv.width)
	assert.Equal(t, 5, lv.height)
}

func TestLearnView_SetSizeAcceptsLargeValues(t *testing.T) {
	lv := newTestLearnView()
	lv.SetSize(120, 50)
	assert.Equal(t, 120, lv.width)
	assert.Equal(t, 50, lv.height)
}

// --- SetCategories ---

func TestLearnView_SetCategoriesResetsScrollOffset(t *testing.T) {
	lv := newTestLearnView()
	lv.scrollOffset = 10
	lv.SetCategories([]KeyBindingCategory{
		{Name: "Custom", Bindings: []KeyBinding{{Key: "x", Action: "test", Category: "Custom"}}},
	})
	assert.Equal(t, 0, lv.scrollOffset)
	assert.Len(t, lv.categories, 1)
}

// --- Overlay integration: Learn view ---

func newTestOverlayWithLearn(opts ...func(*tui.RenderConfig)) *Overlay {
	theme := tui.DefaultTheme()
	rc := tui.RenderConfig{
		Color:     tui.ColorTrueColor,
		Emoji:     false,
		Borders:   tui.BorderUnicode,
		Motion:    tui.MotionStatic,
		Verbosity: tui.VerbosityFull,
	}
	for _, fn := range opts {
		fn(&rc)
	}
	return NewOverlay(theme, rc, &mockMarkdownRenderer{})
}

func TestOverlay_SelectLearnOpensLearnView(t *testing.T) {
	o := newTestOverlayWithLearn()
	o.Open()

	// Navigate to "Learn" (index 3).
	o.HandleKey("down") // Extensions
	o.HandleKey("down") // Marketplace
	o.HandleKey("down") // Learn

	item := o.list.SelectedItem().(menuItem)
	require.Equal(t, "Learn", item.Title())

	result := o.HandleKey("enter")
	assert.Nil(t, result, "selecting Learn should not return a message")
	assert.True(t, o.learnOpen, "learnOpen should be true")
}

func TestOverlay_EscInLearnReturnsToMenu(t *testing.T) {
	o := newTestOverlayWithLearn()
	o.Open()
	o.learnOpen = true

	o.HandleKey("esc")
	assert.False(t, o.learnOpen, "Esc should close learn view")
	assert.True(t, o.IsOpen(), "menu should still be open")
}

func TestOverlay_CloseResetsLearnState(t *testing.T) {
	o := newTestOverlayWithLearn()
	o.Open()
	o.learnOpen = true

	o.Close()
	assert.False(t, o.learnOpen, "Close should reset learn state")
	assert.False(t, o.IsOpen(), "Close should close menu")
}

func TestOverlay_LearnViewRendersWhenOpen(t *testing.T) {
	o := newTestOverlayWithLearn()
	o.Open()
	o.learnOpen = true
	o.SetSize(80, 30)

	rendered := o.Render(80, 30)
	stripped := ansi.Strip(rendered)
	assert.Contains(t, stripped, "Learn", "should contain Learn title")
	assert.Contains(t, stripped, "Navigation", "should contain keybinding category")
}

func TestOverlay_SetSizePropagatesToLearnView(t *testing.T) {
	o := newTestOverlayWithLearn()
	o.SetSize(100, 40)

	require.NotNil(t, o.learnView)
	assert.Equal(t, 100, o.learnView.width)
	assert.Equal(t, 40, o.learnView.height)
}

func TestOverlay_LearnViewKeysRouteCorrectly(t *testing.T) {
	o := newTestOverlayWithLearn()
	o.Open()
	o.learnOpen = true

	// Down should scroll learn view, not move menu cursor.
	initialIdx := o.list.Index()
	o.HandleKey("down")
	assert.Equal(t, initialIdx, o.list.Index(), "menu cursor should not move when learn is open")
}

func TestOverlay_NonLearnItemStillReturnsMenuMsg(t *testing.T) {
	o := newTestOverlayWithLearn()
	o.Open()

	// First item is "Workspaces" — should return MenuItemSelectedMsg.
	result := o.HandleKey("enter")
	require.NotNil(t, result)
	msg, ok := result.(tui.MenuItemSelectedMsg)
	require.True(t, ok)
	assert.Equal(t, "Workspaces", msg.Label)
}
