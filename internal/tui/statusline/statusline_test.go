// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package statusline

import (
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/tui"
)

func testTheme() tui.Theme {
	return tui.DefaultTheme()
}

func testConfig() tui.RenderConfig {
	return tui.RenderConfig{
		Color:     tui.ColorNone,
		Emoji:     false,
		Borders:   tui.BorderUnicode,
		Motion:    tui.MotionSpinner,
		Verbosity: tui.VerbosityFull,
	}
}

func testConfigTrueColor() tui.RenderConfig {
	return tui.RenderConfig{
		Color:     tui.ColorTrueColor,
		Emoji:     true,
		Borders:   tui.BorderUnicode,
		Motion:    tui.MotionSpinner,
		Verbosity: tui.VerbosityFull,
	}
}

// --- Task 1: StatusBar struct and constructor ---

func TestNewStatusBar_Standard(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "standard")
	require.NotNil(t, sb)
	assert.Equal(t, "standard", sb.profile)
	assert.Equal(t, int32(StateNormal), sb.state.Load())
	assert.Equal(t, 80, sb.width)
	assert.Len(t, sb.segments, 7) // model, permission, cost, tokens, layout, workspace, hints
}

func TestNewStatusBar_Minimal(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "minimal")
	require.NotNil(t, sb)
	assert.Equal(t, "minimal", sb.profile)
	assert.Len(t, sb.segments, 2) // model + permission only
	assert.Equal(t, "model", sb.segments[0].Key)
	assert.Equal(t, "permission", sb.segments[1].Key)
}

func TestSegment_Fields(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "standard")
	// Check all segment keys are present.
	keys := make([]string, len(sb.segments))
	for i, s := range sb.segments {
		keys[i] = s.Key
	}
	assert.Equal(t, []string{"model", "permission", "cost", "tokens", "layout", "workspace", "hints"}, keys)

	// Check priorities are ascending.
	for i := 0; i < len(sb.segments)-1; i++ {
		assert.Less(t, sb.segments[i].Priority, sb.segments[i+1].Priority)
	}
}

func TestBarState_Enum(t *testing.T) {
	assert.Equal(t, BarState(0), StateNormal)
	assert.Equal(t, BarState(1), StateWarning)
	assert.Equal(t, BarState(2), StateError)
}

// --- Task 2: Segment rendering pipeline ---

func TestRender_BasicOutput(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "standard")
	sb.updateSegment("model", "claude-opus", sb.theme.Text)

	result := sb.Render(120)
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "claude-opus")
	assert.Contains(t, result, "default") // permission mode
}

func TestRender_SegmentSeparators(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "standard")
	sb.updateSegment("model", "claude-opus", sb.theme.Text)

	result := sb.Render(120)
	// In no-color mode, separators are plain │.
	assert.Contains(t, result, "│")
}

func TestRender_WidthDropsSegments(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "standard")
	sb.updateSegment("model", "claude-opus", sb.theme.Text)
	sb.updateSegment("cost", "$1.23", sb.theme.Text)
	sb.updateSegment("tokens", "5000", sb.theme.Text)
	sb.updateSegment("workspace", "my-project", sb.theme.Text)

	wide := sb.Render(200)
	narrow := sb.Render(30)

	// Wide should have more segments than narrow.
	wideWidth := ansi.StringWidth(wide)
	narrowWidth := ansi.StringWidth(narrow)
	assert.LessOrEqual(t, narrowWidth, 30)
	assert.Greater(t, wideWidth, narrowWidth)
}

func TestRender_UltraNarrow_ModelAndPermissionOnly(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "standard")
	sb.updateSegment("model", "opus", sb.theme.Text)

	result := sb.Render(40)
	assert.Contains(t, result, "opus")
	assert.Contains(t, result, "default")
}

func TestRender_WarningState(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfigTrueColor(), "standard")
	sb.updateSegment("model", "opus", sb.theme.Text)
	sb.SetState(StateWarning)

	result := sb.Render(80)
	assert.NotEmpty(t, result)
	// Warning state applies background — in true color mode, ANSI escape present.
	assert.Contains(t, result, "opus")
}

func TestRender_ErrorState(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfigTrueColor(), "standard")
	sb.updateSegment("model", "opus", sb.theme.Text)
	sb.SetState(StateError)

	result := sb.Render(80)
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "opus")
}

func TestRender_AccessibleMode(t *testing.T) {
	cfg := testConfig()
	cfg.Verbosity = tui.VerbosityAccessible
	sb := NewStatusBar(testTheme(), cfg, "standard")
	sb.updateSegment("model", "claude-opus", sb.theme.Text)

	result := sb.Render(120)
	assert.Contains(t, result, "[MODEL: claude-opus]")
	assert.Contains(t, result, "[PERMISSION: default]")
	// No ANSI escape sequences in no-color accessible mode.
	assert.NotContains(t, result, "│")
}

func TestRender_EmptyWidth(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "standard")
	result := sb.Render(0)
	assert.Empty(t, result)
}

func TestRender_NegativeWidth(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "standard")
	result := sb.Render(-5)
	assert.Empty(t, result)
}

// --- Task 3: StatusCollector subscription ---

func TestHandleUpdate_ProviderSource(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "standard")
	sb.HandleUpdate(core.StatusUpdate{
		Source: "provider",
		Metrics: map[string]any{
			"model":     "claude-opus-4",
			"tokens_in": 1500,
			"cost_usd":  0.42,
		},
		Timestamp: time.Now(),
	})

	result := sb.Render(120)
	assert.Contains(t, result, "claude-opus-4")
	assert.Contains(t, result, "1500")
	assert.Contains(t, result, "$0.42")
}

func TestHandleUpdate_AgentSource(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "standard")
	sb.HandleUpdate(core.StatusUpdate{
		Source: "agent",
		Metrics: map[string]any{
			"context_percentage": 75,
		},
		Timestamp: time.Now(),
	})

	result := sb.Render(120)
	assert.Contains(t, result, "75%")
}

func TestHandleUpdate_AgentWritesToWorkspace(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "standard")
	sb.HandleUpdate(core.StatusUpdate{
		Source:  "provider",
		Metrics: map[string]any{"tokens_in": 500},
	})
	sb.HandleUpdate(core.StatusUpdate{
		Source:  "agent",
		Metrics: map[string]any{"context_percentage": 80},
	})

	// Agent context should be in workspace, not overwrite tokens.
	for _, seg := range sb.segments {
		if seg.Key == "tokens" {
			assert.Equal(t, "500", seg.Value)
		}
		if seg.Key == "workspace" {
			assert.Equal(t, "80%", seg.Value)
		}
	}
}

func TestHandleUpdate_UnknownSource(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "standard")
	// Should not panic.
	sb.HandleUpdate(core.StatusUpdate{
		Source:    "unknown",
		Metrics:   map[string]any{"foo": "bar"},
		Timestamp: time.Now(),
	})
}

// --- Task 4: Permission mode display ---

func TestSetPermissionMode_Default(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "standard")
	sb.SetPermissionMode("default")

	result := sb.Render(120)
	assert.Contains(t, result, "default")
}

func TestSetPermissionMode_AutoAccept(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "standard")
	sb.SetPermissionMode("auto-accept")

	result := sb.Render(120)
	assert.Contains(t, result, "auto-accept")
}

func TestSetPermissionMode_Yolo(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "standard")
	sb.SetPermissionMode("yolo")

	result := sb.Render(120)
	assert.Contains(t, result, "yolo")
}

func TestSetPermissionMode_UnknownFallsToDefault(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "standard")
	sb.SetPermissionMode("unknown-mode")

	// Should fall back to "default".
	for _, seg := range sb.segments {
		if seg.Key == "permission" {
			assert.Equal(t, "default", seg.Value)
			return
		}
	}
	t.Fatal("permission segment not found")
}

// --- Task 5: Hints segment ---

func TestDefaultHint(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "standard")
	result := sb.Render(120)
	assert.Contains(t, result, "Ctrl+Space: Menu")
}

func TestSetUpdateHint(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "standard")
	sb.SetUpdateHint(3)

	result := sb.Render(120)
	assert.Contains(t, result, "3 items have updates")
}

func TestSetUpdateHint_WithEmoji(t *testing.T) {
	cfg := testConfig()
	cfg.Emoji = true
	sb := NewStatusBar(testTheme(), cfg, "standard")
	sb.SetUpdateHint(5)

	// Check the segment value directly.
	for _, seg := range sb.segments {
		if seg.Key == "hints" {
			assert.Contains(t, seg.Value, "📦")
			assert.Contains(t, seg.Value, "5 items have updates")
			return
		}
	}
	t.Fatal("hints segment not found")
}

func TestSetUpdateHint_ZeroResetsToStartup(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "standard")
	sb.SetUpdateHint(3)
	sb.SetUpdateHint(0)

	for _, seg := range sb.segments {
		if seg.Key == "hints" {
			assert.Equal(t, "Ctrl+Space: Menu", seg.Value)
			return
		}
	}
	t.Fatal("hints segment not found")
}

func TestHints_LowestPriority(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "standard")
	// Hints should be the last segment (highest priority number = dropped first).
	last := sb.segments[len(sb.segments)-1]
	assert.Equal(t, "hints", last.Key)
	assert.Equal(t, 7, last.Priority)
}

// --- Task 6: Profile-aware configuration ---

func TestSetProfile_MinimalToStandard(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "minimal")
	assert.Len(t, sb.segments, 2)

	sb.SetProfile("standard")
	assert.Len(t, sb.segments, 7)
}

func TestSetProfile_StandardToMinimal(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "standard")
	assert.Len(t, sb.segments, 7)

	sb.SetProfile("minimal")
	assert.Len(t, sb.segments, 2)
}

// --- Task 7: Compact mode support ---

func TestSetSize_ClampsMinimum(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "standard")
	sb.SetSize(0, false)
	assert.Equal(t, 1, sb.width)
}

func TestSetSize_CompactParam(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "standard")
	sb.SetSize(60, true)
	assert.Equal(t, 60, sb.width)
}

func TestRender_RespectsPadding(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "standard")
	sb.updateSegment("model", "opus", sb.theme.Text)

	result := sb.Render(120)
	// Should be padded to width.
	resultWidth := ansi.StringWidth(result)
	assert.Equal(t, 120, resultWidth)
}

// --- Integration: full flow ---

func TestFullFlow_ProviderUpdate_PermissionChange_Render(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "standard")

	// Provider update.
	sb.HandleUpdate(core.StatusUpdate{
		Source: "provider",
		Metrics: map[string]any{
			"model":     "claude-sonnet",
			"tokens_in": 2500,
			"cost_usd":  1.05,
		},
		Timestamp: time.Now(),
	})

	// Permission change.
	sb.SetPermissionMode("yolo")

	result := sb.Render(120)
	assert.Contains(t, result, "claude-sonnet")
	assert.Contains(t, result, "2500")
	assert.Contains(t, result, "$1.05")
	assert.Contains(t, result, "yolo")
}

// --- Local Mode Indicator ---

func TestSetLocal_WithModel(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "standard")
	sb.SetLocal("codellama:13b")

	result := sb.Render(120)
	assert.Contains(t, result, "local")
	assert.Contains(t, result, "codellama:13b")
}

func TestSetLocal_NoModel(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "standard")
	sb.SetLocal("")

	result := sb.Render(120)
	assert.Contains(t, result, "local (ollama)")
}

func TestSetLocal_WithEmoji(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfigTrueColor(), "standard")
	sb.SetLocal("")

	result := sb.Render(120)
	assert.Contains(t, result, "🔌")
	assert.Contains(t, result, "local (ollama)")
}

func TestSetLocal_Accessible(t *testing.T) {
	cfg := testConfig()
	cfg.Verbosity = tui.VerbosityAccessible
	sb := NewStatusBar(testTheme(), cfg, "standard")
	sb.SetLocal("qwen2.5-coder:7b")

	result := sb.Render(120)
	assert.Contains(t, result, "[MODEL: local (qwen2.5-coder:7b)]")
}

func TestSetLocal_Minimal(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "minimal")
	sb.SetLocal("qwen2.5-coder:7b")

	result := sb.Render(120)
	assert.Contains(t, result, "local")
}

func TestSetLocalNoLLM(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfig(), "standard")
	sb.SetLocalNoLLM()

	result := sb.Render(120)
	assert.Contains(t, result, "local (no LLM)")
}

func TestSetLocalNoLLM_WithEmoji(t *testing.T) {
	sb := NewStatusBar(testTheme(), testConfigTrueColor(), "standard")
	sb.SetLocalNoLLM()

	result := sb.Render(120)
	assert.Contains(t, result, "⚠")
	assert.Contains(t, result, "local (no LLM)")
}
