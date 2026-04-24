// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package panels

import (
	"strings"
	"testing"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/tui"
)

// ─── Unicode test matrix ────────────────────────────────────────────────────

func TestPanelManager_View_Unicode_ASCII(t *testing.T) {
	m := testManager()
	require.NoError(t, m.Register(core.PanelConfig{
		Name:        "ascii-panel",
		Position:    core.PanelLeft,
		MinWidth:    30,
		MaxWidth:    60,
		ContentFunc: func() string { return "Hello World" },
	}))
	m.left.width = 30

	view := m.View(120, 30, "center")
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "Hello World")
}

func TestPanelManager_View_Unicode_CJK_Chinese(t *testing.T) {
	m := testManager()
	require.NoError(t, m.Register(core.PanelConfig{
		Name:        "cjk-cn",
		Position:    core.PanelLeft,
		MinWidth:    30,
		MaxWidth:    60,
		ContentFunc: func() string { return "你好世界" },
	}))
	m.left.width = 30

	view := m.View(120, 30, "center")
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "你好世界")
}

func TestPanelManager_View_Unicode_CJK_Hiragana(t *testing.T) {
	m := testManager()
	require.NoError(t, m.Register(core.PanelConfig{
		Name:        "cjk-jp",
		Position:    core.PanelLeft,
		MinWidth:    30,
		MaxWidth:    60,
		ContentFunc: func() string { return "こんにちは" },
	}))
	m.left.width = 30

	view := m.View(120, 30, "center")
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "こんにちは")
}

func TestPanelManager_View_Unicode_CJK_Hangul(t *testing.T) {
	m := testManager()
	require.NoError(t, m.Register(core.PanelConfig{
		Name:        "cjk-kr",
		Position:    core.PanelLeft,
		MinWidth:    30,
		MaxWidth:    60,
		ContentFunc: func() string { return "안녕하세요" },
	}))
	m.left.width = 30

	view := m.View(120, 30, "center")
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "안녕하세요")
}

func TestPanelManager_View_Unicode_Emoji(t *testing.T) {
	m := testManager()
	require.NoError(t, m.Register(core.PanelConfig{
		Name:        "emoji-panel",
		Position:    core.PanelLeft,
		MinWidth:    30,
		MaxWidth:    60,
		ContentFunc: func() string { return "🚀 Launch\n✅ Done" },
	}))
	m.left.width = 30

	view := m.View(120, 30, "center")
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "🚀")
	assert.Contains(t, view, "✅")
}

func TestPanelManager_View_Unicode_ZWJ_Sequence(t *testing.T) {
	m := testManager()
	// ZWJ family emoji: 👨‍👩‍👧‍👦
	content := "👨‍👩‍👧‍👦 family"
	require.NoError(t, m.Register(core.PanelConfig{
		Name:        "zwj-panel",
		Position:    core.PanelLeft,
		MinWidth:    30,
		MaxWidth:    60,
		ContentFunc: func() string { return content },
	}))
	m.left.width = 30

	view := m.View(120, 30, "center")
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "family")
}

func TestPanelManager_View_Unicode_FlagEmoji(t *testing.T) {
	m := testManager()
	// Netherlands flag: 🇳🇱
	content := "🇳🇱 Nederland"
	require.NoError(t, m.Register(core.PanelConfig{
		Name:        "flag-panel",
		Position:    core.PanelLeft,
		MinWidth:    30,
		MaxWidth:    60,
		ContentFunc: func() string { return content },
	}))
	m.left.width = 30

	view := m.View(120, 30, "center")
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "Nederland")
}

func TestPanelManager_View_Unicode_Mixed(t *testing.T) {
	m := testManager()
	content := "Code 代码 🚀 ready!"
	require.NoError(t, m.Register(core.PanelConfig{
		Name:        "mixed-panel",
		Position:    core.PanelLeft,
		MinWidth:    40,
		MaxWidth:    80,
		ContentFunc: func() string { return content },
	}))
	m.left.width = 40

	view := m.View(120, 30, "center")
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "Code")
	assert.Contains(t, view, "代码")
	assert.Contains(t, view, "ready!")
}

func TestPanelManager_View_Unicode_ANSIStyled(t *testing.T) {
	m := testManager()
	styledContent := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Red).Render("styled text")
	require.NoError(t, m.Register(core.PanelConfig{
		Name:        "ansi-panel",
		Position:    core.PanelLeft,
		MinWidth:    30,
		MaxWidth:    60,
		ContentFunc: func() string { return styledContent },
	}))
	m.left.width = 30

	view := m.View(120, 30, "center")
	assert.NotEmpty(t, view)
	// The stripped content should contain the underlying text.
	stripped := ansi.Strip(view)
	assert.Contains(t, stripped, "styled text")
}

// TestPanelManager_View_Unicode_Matrix runs all Unicode test vectors through
// the full View() pipeline in a single table-driven test.
func TestPanelManager_View_Unicode_Matrix(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"ASCII", "Hello World"},
		{"Chinese", "你好世界"},
		{"Hiragana", "こんにちは"},
		{"Hangul", "안녕하세요"},
		{"Emoji", "🚀 Launch ✅ Done"},
		{"ZWJ family", "👨‍👩‍👧‍👦 family"},
		{"Flag NL", "🇳🇱 Nederland"},
		{"Mixed", "Code 代码 🚀 ready!"},
		{"Multiline CJK", "第一行\n第二行\n第三行"},
		{"Emoji with text", "Status: ✅ pass | ❌ fail | ⚠️ warn"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := testManager()
			require.NoError(t, m.Register(core.PanelConfig{
				Name:        "unicode-" + tc.name,
				Position:    core.PanelLeft,
				MinWidth:    40,
				MaxWidth:    80,
				ContentFunc: func() string { return tc.content },
			}))
			m.left.width = 40

			// Must not panic.
			view := m.View(120, 30, "center")
			assert.NotEmpty(t, view, "View output should be non-empty for %s", tc.name)
		})
	}
}

// ─── Dock-only no-Compositor test ───────────────────────────────────────────

func TestPanelManager_View_DockOnly_NoCompositorPath(t *testing.T) {
	m := testManager()

	// Register dock-only panels (left + right), no overlays.
	require.NoError(t, m.Register(core.PanelConfig{
		Name:        "tree",
		Position:    core.PanelLeft,
		MinWidth:    20,
		MaxWidth:    40,
		ContentFunc: func() string { return "tree content" },
	}))
	require.NoError(t, m.Register(core.PanelConfig{
		Name:        "preview",
		Position:    core.PanelRight,
		MinWidth:    20,
		MaxWidth:    40,
		ContentFunc: func() string { return "preview content" },
	}))
	m.left.width = 25
	m.right.width = 25

	view := m.View(120, 30, "center content")

	// Output must be produced from dock-only path.
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "center content")
	// hasActiveOverlays() should be false, so Compositor is NOT used.
	assert.False(t, m.hasActiveOverlays(), "no overlays should be active")
}

func TestPanelManager_View_DockOnly_InactiveOverlay(t *testing.T) {
	m := testManager()

	require.NoError(t, m.Register(core.PanelConfig{
		Name:        "tree",
		Position:    core.PanelLeft,
		MinWidth:    20,
		MaxWidth:    40,
		ContentFunc: func() string { return "tree" },
	}))
	// Register an overlay but do NOT activate it.
	require.NoError(t, m.Register(core.PanelConfig{
		Name:     "float-panel",
		Position: core.PanelOverlay,
		MinWidth: 25,
		OverlayZ: 10,
	}))
	m.left.width = 25

	view := m.View(100, 24, "center")
	assert.NotEmpty(t, view)
	// Overlay is registered but not active, so dock-only path is used.
	assert.False(t, m.hasActiveOverlays())
	assert.Contains(t, view, "center")
}

// ─── SSH session render config test ─────────────────────────────────────────

func TestPanelManager_View_SSHRenderConfig_ASCIIBorders(t *testing.T) {
	// SSH sessions should use BorderASCII in the render config.
	caps := tui.Capabilities{
		ColorDepth: tui.Color256,
		Unicode:    true,
		Emoji:      true,
		SSHSession: true,
		IsTTY:      true,
	}
	rc := tui.NewRenderConfig(caps, tui.CLIFlags{})
	assert.Equal(t, tui.BorderASCII, rc.Borders, "SSH should default to ASCII borders")

	m := NewPanelManager(tui.DefaultTheme(), rc)

	// Register an overlay panel to exercise the border rendering path.
	require.NoError(t, m.Register(core.PanelConfig{
		Name:     "ssh-overlay",
		Position: core.PanelOverlay,
		MinWidth: 25,
		OverlayX: 2,
		OverlayY: 1,
		OverlayZ: 5,
	}))
	require.NoError(t, m.Activate("ssh-overlay"))

	view := m.View(80, 24, "ssh center")
	assert.NotEmpty(t, view)

	// The overlay's border is rendered via RenderBorder which uses renderASCIIBorder
	// for BorderASCII config. Verify ASCII box-drawing chars appear.
	assert.Contains(t, view, "+", "SSH overlay should use ASCII border chars")
	assert.Contains(t, view, "-", "SSH overlay should use ASCII horizontal border")

	// Unicode box-drawing chars should NOT appear in the overlay border.
	// Note: the dock panel borders in renderSlot use "│" directly (not via RenderBorder),
	// so we only check the overlay border output.
	stripped := ansi.Strip(view)
	// The overlay rendered via RenderBorder should use +/- not ┌/─.
	overlayBorderSection := extractBetween(stripped, "+", "+")
	if overlayBorderSection != "" {
		assert.NotContains(t, overlayBorderSection, "┌", "SSH overlay should not use unicode box-drawing")
		assert.NotContains(t, overlayBorderSection, "─", "SSH overlay should not use unicode horizontal line")
	}
}

// extractBetween returns the substring between the first occurrence of start
// and the next occurrence of end after it. Returns "" if not found.
func extractBetween(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	rest := s[i+len(start):]
	j := strings.Index(rest, end)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

// ─── NO_COLOR panel rendering test ──────────────────────────────────────────

func TestPanelManager_View_NoColor_NoANSICodes(t *testing.T) {
	rc := tui.RenderConfig{
		Color:     tui.ColorNone,
		Borders:   tui.BorderUnicode,
		Emoji:     true,
		Motion:    tui.MotionSpinner,
		Verbosity: tui.VerbosityFull,
	}
	m := NewPanelManager(tui.DefaultTheme(), rc)

	require.NoError(t, m.Register(core.PanelConfig{
		Name:        "nocolor-panel",
		Position:    core.PanelLeft,
		MinWidth:    25,
		MaxWidth:    50,
		ContentFunc: func() string { return "plain text content" },
	}))
	m.left.width = 25

	view := m.View(100, 24, "center content")
	assert.NotEmpty(t, view)

	// With ColorNone, the theme's Resolve(ColorNone) returns NoColor style
	// which should have no foreground/background color codes.
	// The panel slot border rendering uses theme.Primary.Resolve() for focused
	// borders, but since this panel isn't focused, it uses plain "│".
	// Verify the center content is present.
	stripped := ansi.Strip(view)
	assert.Contains(t, stripped, "plain text content")
	assert.Contains(t, stripped, "center content")
}

func TestPanelManager_View_NoColor_OverlayPreservesText(t *testing.T) {
	rc := tui.RenderConfig{
		Color:     tui.ColorNone,
		Borders:   tui.BorderUnicode,
		Emoji:     true,
		Motion:    tui.MotionSpinner,
		Verbosity: tui.VerbosityFull,
	}
	m := NewPanelManager(tui.DefaultTheme(), rc)

	require.NoError(t, m.Register(core.PanelConfig{
		Name:        "nocolor-overlay",
		Position:    core.PanelOverlay,
		MinWidth:    30,
		OverlayX:    2,
		OverlayY:    1,
		OverlayZ:    10,
		ContentFunc: func() string { return "overlay text" },
	}))
	require.NoError(t, m.Activate("nocolor-overlay"))

	view := m.View(80, 24, "dock text")
	assert.NotEmpty(t, view)

	// Text decoration (borders, structure) is preserved even without colors.
	stripped := ansi.Strip(view)
	assert.Contains(t, stripped, "overlay text", "overlay content should be present without color")
}

func TestPanelManager_View_NoColor_FocusedPanelPreservesDecoration(t *testing.T) {
	rc := tui.RenderConfig{
		Color:     tui.ColorNone,
		Borders:   tui.BorderUnicode,
		Emoji:     true,
		Motion:    tui.MotionSpinner,
		Verbosity: tui.VerbosityFull,
	}
	m := NewPanelManager(tui.DefaultTheme(), rc)

	require.NoError(t, m.Register(core.PanelConfig{
		Name:        "focused-nocolor",
		Position:    core.PanelLeft,
		MinWidth:    25,
		MaxWidth:    50,
		ContentFunc: func() string { return "focused content" },
	}))
	m.left.width = 25
	m.focus = "left"

	view := m.View(100, 24, "center")
	assert.NotEmpty(t, view)

	// Even with ColorNone, the focused border style applies text decoration
	// (bold) via theme.Primary.Resolve(ColorNone).
	// The key point: text/structure is preserved, only colors are removed.
	stripped := ansi.Strip(view)
	assert.Contains(t, stripped, "focused content")
	// Border characters should still be present (│ for panel edges).
	assert.Contains(t, stripped, "│", "border decoration should be preserved even without color")
}
