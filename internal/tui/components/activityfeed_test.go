// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package components

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func testConfigEmoji() tui.RenderConfig {
	return tui.RenderConfig{
		Color:     tui.ColorNone,
		Emoji:     true,
		Borders:   tui.BorderUnicode,
		Motion:    tui.MotionSpinner,
		Verbosity: tui.VerbosityFull,
	}
}

func testConfigAccessible() tui.RenderConfig {
	return tui.RenderConfig{
		Color:     tui.ColorNone,
		Emoji:     false,
		Borders:   tui.BorderNone,
		Motion:    tui.MotionStatic,
		Verbosity: tui.VerbosityAccessible,
	}
}

func testEntry(entryType EntryType, label string, duration time.Duration) FeedEntry {
	return FeedEntry{
		Type:      entryType,
		Label:     label,
		Duration:  duration,
		Timestamp: time.Now(),
	}
}

// --- Interface compliance ---

func TestActivityFeed_ImplementsActivityFeedRenderer(t *testing.T) {
	var _ tui.ActivityFeedRenderer = (*ActivityFeed)(nil)
}

// --- Constructor ---

func TestNewActivityFeed(t *testing.T) {
	af := NewActivityFeed(testTheme(), testConfig())
	require.NotNil(t, af)
	assert.Equal(t, 80, af.width)
	assert.Equal(t, 10, af.height)
	assert.Equal(t, FeedIdle, af.state)
	assert.Empty(t, af.entries)
}

// --- Task 5.1: Entry rendering tests ---

func TestRenderEntry_AllTypes_NoEmoji(t *testing.T) {
	af := NewActivityFeed(testTheme(), testConfig())

	tests := []struct {
		name     string
		typ      EntryType
		label    string
		contains string
	}{
		{"Read", EntryRead, "src/main.go", "Reading"},
		{"Edit", EntryEdit, "src/handler.go", "Editing"},
		{"Search", EntrySearch, "func.*", "Searching"},
		{"Bash", EntryBash, "ls -la", "Bash"},
		{"Web", EntryWeb, "https://api.example.com", "Web"},
		{"Tool", EntryTool, "custom_tool", "Tool"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			af.entries = nil
			af.state = FeedIdle
			af.scrollOffset = 0
			af.AddEntry(testEntry(tt.typ, tt.label, 50*time.Millisecond))
			result := af.Render(120, 5)
			stripped := ansi.Strip(result)
			assert.Contains(t, stripped, tt.contains, "Should contain type label")
			assert.Contains(t, stripped, tt.label, "Should contain label")
			assert.Contains(t, stripped, "50ms", "Should contain duration")
		})
	}
}

func TestRenderEntry_AllTypes_Emoji(t *testing.T) {
	af := NewActivityFeed(testTheme(), testConfigEmoji())

	tests := []struct {
		name     string
		typ      EntryType
		label    string
		emoji    string
	}{
		{"Read", EntryRead, "src/main.go", "\U0001F4D6"},
		{"Edit", EntryEdit, "src/handler.go", "\u270F\uFE0F"},
		{"Search", EntrySearch, "func.*", "\U0001F50D"},
		{"Bash", EntryBash, "ls -la", "\u26A1"},
		{"Web", EntryWeb, "https://api.example.com", "\U0001F310"},
		{"Tool", EntryTool, "custom_tool", "\U0001F527"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			af.entries = nil
			af.state = FeedIdle
			af.scrollOffset = 0
			af.AddEntry(testEntry(tt.typ, tt.label, 100*time.Millisecond))
			result := af.Render(120, 5)
			stripped := ansi.Strip(result)
			assert.Contains(t, stripped, tt.emoji, "Should contain emoji")
			assert.Contains(t, stripped, tt.label, "Should contain label")
		})
	}
}

func TestRenderEntry_AccessibleMode(t *testing.T) {
	af := NewActivityFeed(testTheme(), testConfigAccessible())

	tests := []struct {
		name string
		typ  EntryType
		tag  string
	}{
		{"Read", EntryRead, "[READ]"},
		{"Edit", EntryEdit, "[EDIT]"},
		{"Search", EntrySearch, "[SEARCH]"},
		{"Bash", EntryBash, "[BASH]"},
		{"Web", EntryWeb, "[WEB]"},
		{"Tool", EntryTool, "[TOOL]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			af.entries = nil
			af.state = FeedIdle
			af.scrollOffset = 0
			af.AddEntry(testEntry(tt.typ, "some/path.go", 25*time.Millisecond))
			result := af.Render(120, 5)
			assert.Contains(t, result, tt.tag)
			assert.Contains(t, result, "some/path.go")
			assert.Contains(t, result, "25ms")
		})
	}
}

// --- Task 5.2: Auto-scroll behavior tests ---

func TestAutoScroll_AtBottom(t *testing.T) {
	af := NewActivityFeed(testTheme(), testConfig())
	af.SetSize(80, 3)

	// Add 5 entries; feed height is 3, so should auto-scroll to show last 3.
	for i := range 5 {
		af.AddEntry(testEntry(EntryRead, "file"+string(rune('0'+i))+".go", time.Millisecond))
	}

	result := ansi.Strip(af.Render(80, 3))
	// Should show last entries (auto-scroll).
	assert.Contains(t, result, "file2.go")
	assert.Contains(t, result, "file3.go")
	assert.Contains(t, result, "file4.go")
}

func TestAutoScroll_ScrolledUp(t *testing.T) {
	af := NewActivityFeed(testTheme(), testConfig())
	af.SetSize(80, 3)

	// Add 5 entries.
	for i := range 5 {
		af.AddEntry(testEntry(EntryRead, "file"+string(rune('A'+i))+".go", time.Millisecond))
	}

	// Scroll up — should disable auto-scroll.
	af.HandleScroll(-1)

	// Add another entry.
	af.AddEntry(testEntry(EntryRead, "fileNEW.go", time.Millisecond))

	result := ansi.Strip(af.Render(80, 3))
	// Should NOT auto-scroll to show new entry because user scrolled up.
	assert.NotContains(t, result, "fileNEW.go")
}

func TestAutoScroll_ScrollBackToBottom(t *testing.T) {
	af := NewActivityFeed(testTheme(), testConfig())
	af.SetSize(80, 3)

	for i := range 5 {
		af.AddEntry(testEntry(EntryRead, "file"+string(rune('A'+i))+".go", time.Millisecond))
	}

	// Scroll up then back to bottom.
	af.HandleScroll(-1)
	af.HandleScroll(1)

	// Add another entry — should auto-scroll since we're at bottom again.
	af.AddEntry(testEntry(EntryRead, "fileNEW.go", time.Millisecond))

	result := ansi.Strip(af.Render(80, 3))
	assert.Contains(t, result, "fileNEW.go")
}

// --- Task 5.3: State transition tests ---

func TestState_IdleToStreaming(t *testing.T) {
	af := NewActivityFeed(testTheme(), testConfig())
	assert.Equal(t, FeedIdle, af.state)

	af.AddEntry(testEntry(EntryRead, "file.go", time.Millisecond))
	assert.Equal(t, FeedStreaming, af.state)
}

func TestState_StreamingToComplete(t *testing.T) {
	af := NewActivityFeed(testTheme(), testConfig())
	af.AddEntry(testEntry(EntryRead, "file.go", time.Millisecond))
	assert.Equal(t, FeedStreaming, af.state)

	af.SetState(FeedComplete)
	assert.Equal(t, FeedComplete, af.state)
}

func TestState_StreamingToCancelled(t *testing.T) {
	af := NewActivityFeed(testTheme(), testConfig())
	af.AddEntry(testEntry(EntryRead, "file.go", time.Millisecond))

	af.SetState(FeedCancelled)
	assert.Equal(t, FeedCancelled, af.state)
}

// --- Task 5.4: Width adaptation tests ---

func TestRender_NarrowTerminal(t *testing.T) {
	af := NewActivityFeed(testTheme(), testConfig())
	af.AddEntry(FeedEntry{
		Type:     EntryRead,
		Label:    "very/long/path/to/some/deeply/nested/file/structure/main.go",
		Duration: 5 * time.Millisecond,
	})

	result := af.Render(40, 5)
	assert.NotEmpty(t, result)
	// Each line should not exceed width.
	for _, line := range strings.Split(result, "\n") {
		assert.LessOrEqual(t, ansi.StringWidth(line), 40, "Line should not exceed terminal width")
	}
}

func TestRender_MediumTerminal(t *testing.T) {
	af := NewActivityFeed(testTheme(), testConfig())
	af.AddEntry(testEntry(EntrySearch, "func.*Handler", 12*time.Millisecond))

	result := af.Render(80, 5)
	assert.NotEmpty(t, result)
	stripped := ansi.Strip(result)
	assert.Contains(t, stripped, "func.*Handler")
}

func TestRender_WideTerminal(t *testing.T) {
	af := NewActivityFeed(testTheme(), testConfig())
	af.AddEntry(FeedEntry{
		Type:     EntryEdit,
		Label:    "src/handlers/auth.go",
		Detail:   "(lines 12-45)",
		Duration: 120 * time.Millisecond,
	})

	result := af.Render(200, 5)
	stripped := ansi.Strip(result)
	assert.Contains(t, stripped, "src/handlers/auth.go")
	assert.Contains(t, stripped, "(lines 12-45)")
	assert.Contains(t, stripped, "120ms")
}

// --- Task 5.5: Entry cap test ---

func TestEntryCapAt500(t *testing.T) {
	af := NewActivityFeed(testTheme(), testConfig())

	for i := range 600 {
		af.AddEntry(testEntry(EntryRead, "file.go", time.Duration(i)*time.Millisecond))
	}

	assert.Equal(t, 500, len(af.entries), "Should cap at 500 entries")
	// Oldest entries should be dropped — first entry should have duration 100ms (index 100).
	assert.Equal(t, 100*time.Millisecond, af.entries[0].Duration)
}

// --- Edge cases ---

func TestRender_EmptyFeed(t *testing.T) {
	af := NewActivityFeed(testTheme(), testConfig())
	result := af.Render(80, 10)
	assert.Empty(t, result)
}

func TestRender_ZeroWidth(t *testing.T) {
	af := NewActivityFeed(testTheme(), testConfig())
	af.AddEntry(testEntry(EntryRead, "file.go", time.Millisecond))
	result := af.Render(0, 10)
	assert.Empty(t, result)
}

func TestRender_ZeroHeight(t *testing.T) {
	af := NewActivityFeed(testTheme(), testConfig())
	af.AddEntry(testEntry(EntryRead, "file.go", time.Millisecond))
	result := af.Render(80, 0)
	assert.Empty(t, result)
}

func TestSetSize_ClampsMinimum(t *testing.T) {
	af := NewActivityFeed(testTheme(), testConfig())
	af.SetSize(0, 0)
	assert.Equal(t, 1, af.width)
	assert.Equal(t, 1, af.height)
}

func TestSetSize_NegativeValues(t *testing.T) {
	af := NewActivityFeed(testTheme(), testConfig())
	af.SetSize(-5, -10)
	assert.Equal(t, 1, af.width)
	assert.Equal(t, 1, af.height)
}

func TestHandleScroll_ClampsBounds(t *testing.T) {
	af := NewActivityFeed(testTheme(), testConfig())
	af.SetSize(80, 5)

	// Scroll up when empty — should stay at 0.
	af.HandleScroll(-1)
	assert.Equal(t, 0, af.scrollOffset)

	// Add a few entries, scroll past bottom.
	for range 3 {
		af.AddEntry(testEntry(EntryRead, "file.go", time.Millisecond))
	}
	af.HandleScroll(100)
	assert.LessOrEqual(t, af.scrollOffset, af.maxScrollOffset())
}

func TestFormatDuration_Milliseconds(t *testing.T) {
	assert.Equal(t, "50ms", formatDuration(50*time.Millisecond))
	assert.Equal(t, "0ms", formatDuration(0))
	assert.Equal(t, "999ms", formatDuration(999*time.Millisecond))
}

func TestFormatDuration_Seconds(t *testing.T) {
	assert.Equal(t, "1.0s", formatDuration(time.Second))
	assert.Equal(t, "2.5s", formatDuration(2500*time.Millisecond))
}

func TestParseEntryType(t *testing.T) {
	tests := []struct {
		input    string
		expected EntryType
	}{
		{"file_read", EntryRead},
		{"file_edit", EntryEdit},
		{"file_write", EntryEdit},
		{"grep", EntrySearch},
		{"glob", EntrySearch},
		{"search", EntrySearch},
		{"bash", EntryBash},
		{"web_fetch", EntryWeb},
		{"web_search", EntryWeb},
		{"unknown_tool", EntryTool},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, ParseEntryType(tt.input))
		})
	}
}

func TestRenderEntry_ErrorEntry(t *testing.T) {
	af := NewActivityFeed(testTheme(), testConfig())
	af.AddEntry(FeedEntry{
		Type:     EntryBash,
		Label:    "exit 1",
		Duration: 50 * time.Millisecond,
		IsError:  true,
	})

	result := af.Render(120, 5)
	stripped := ansi.Strip(result)
	assert.Contains(t, stripped, "exit 1")
	assert.Contains(t, stripped, "50ms")
}

func TestRender_EntryWithDetail(t *testing.T) {
	af := NewActivityFeed(testTheme(), testConfig())
	af.AddEntry(FeedEntry{
		Type:     EntryEdit,
		Label:    "src/handler.go",
		Detail:   "(lines 12-45)",
		Duration: 120 * time.Millisecond,
	})

	result := af.Render(120, 5)
	stripped := ansi.Strip(result)
	assert.Contains(t, stripped, "src/handler.go")
	assert.Contains(t, stripped, "(lines 12-45)")
	assert.Contains(t, stripped, "120ms")
}

func TestRender_MultipleEntries(t *testing.T) {
	af := NewActivityFeed(testTheme(), testConfig())
	af.AddEntry(testEntry(EntryRead, "file1.go", time.Millisecond))
	af.AddEntry(testEntry(EntryEdit, "file2.go", 2*time.Millisecond))
	af.AddEntry(testEntry(EntrySearch, "pattern", 3*time.Millisecond))

	result := af.Render(120, 10)
	stripped := strings.TrimRight(ansi.Strip(result), "\n")
	lines := strings.Split(stripped, "\n")
	assert.Equal(t, 3, len(lines))
	assert.Contains(t, lines[0], "file1.go")
	assert.Contains(t, lines[1], "file2.go")
	assert.Contains(t, lines[2], "pattern")
}
