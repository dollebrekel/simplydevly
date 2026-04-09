// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package components

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/tui"
)

// --- Helpers ---

func diffTestConfig() tui.RenderConfig {
	return tui.RenderConfig{
		Color:     tui.ColorNone,
		Emoji:     false,
		Borders:   tui.BorderUnicode,
		Motion:    tui.MotionSpinner,
		Verbosity: tui.VerbosityFull,
	}
}

func diffTestConfigAccessible() tui.RenderConfig {
	return tui.RenderConfig{
		Color:     tui.ColorNone,
		Emoji:     false,
		Borders:   tui.BorderNone,
		Motion:    tui.MotionStatic,
		Verbosity: tui.VerbosityAccessible,
	}
}

func diffTestConfigColor() tui.RenderConfig {
	return tui.RenderConfig{
		Color:     tui.ColorTrueColor,
		Emoji:     false,
		Borders:   tui.BorderUnicode,
		Motion:    tui.MotionSpinner,
		Verbosity: tui.VerbosityFull,
	}
}

func newTestDiffView() *DiffView {
	return NewDiffView(testTheme(), diffTestConfig())
}

func sampleDiffData() DiffData {
	return DiffData{
		FilePath:   "src/handlers/auth.go",
		OldContent: "func handleAuth() {\n    token := getToken()\n    validate(token)\n}",
		NewContent: "func handleAuth() {\n    token, err := extractToken()\n    if err != nil {\n        return err\n    }\n    validate(token)\n}",
		Lines: GenerateDiff(
			"func handleAuth() {\n    token := getToken()\n    validate(token)\n}",
			"func handleAuth() {\n    token, err := extractToken()\n    if err != nil {\n        return err\n    }\n    validate(token)\n}",
		),
	}
}

// --- Task 6.7: Interface compliance ---

func TestDiffView_ImplementsDiffViewRenderer(t *testing.T) {
	var _ tui.DiffViewRenderer = (*DiffView)(nil)
}

// --- Constructor ---

func TestNewDiffView(t *testing.T) {
	dv := newTestDiffView()
	require.NotNil(t, dv)
	assert.Equal(t, 80, dv.width)
	assert.Equal(t, 20, dv.height)
	assert.Equal(t, tui.DiffViewing, dv.state)
	assert.Nil(t, dv.diffData)
}

// --- Task 6.1: Diff generation tests ---

func TestGenerateDiff_Additions(t *testing.T) {
	old := "line1\nline2"
	new := "line1\nline2\nline3\nline4"
	lines := GenerateDiff(old, new)

	require.NotEmpty(t, lines)
	addedCount := 0
	for _, l := range lines {
		if l.Type == DiffLineAdded {
			addedCount++
		}
	}
	assert.Equal(t, 2, addedCount, "Should have 2 added lines")
}

func TestGenerateDiff_Deletions(t *testing.T) {
	old := "line1\nline2\nline3\nline4"
	new := "line1\nline4"
	lines := GenerateDiff(old, new)

	require.NotEmpty(t, lines)
	removedCount := 0
	for _, l := range lines {
		if l.Type == DiffLineRemoved {
			removedCount++
		}
	}
	assert.Equal(t, 2, removedCount, "Should have 2 removed lines")
}

func TestGenerateDiff_MixedChanges(t *testing.T) {
	old := "func main() {\n    fmt.Println(\"hello\")\n}"
	new := "func main() {\n    fmt.Println(\"world\")\n    fmt.Println(\"!\")\n}"
	lines := GenerateDiff(old, new)

	require.NotEmpty(t, lines)

	hasAdded := false
	hasRemoved := false
	hasContext := false
	for _, l := range lines {
		switch l.Type {
		case DiffLineAdded:
			hasAdded = true
		case DiffLineRemoved:
			hasRemoved = true
		case DiffLineContext:
			hasContext = true
		}
	}
	assert.True(t, hasAdded, "Should have added lines")
	assert.True(t, hasRemoved, "Should have removed lines")
	assert.True(t, hasContext, "Should have context lines")
}

func TestGenerateDiff_ContextLines(t *testing.T) {
	// Create a file with many lines, change one in the middle.
	var oldLines, newLines []string
	for i := range 20 {
		oldLines = append(oldLines, "unchanged line "+string(rune('A'+i)))
		newLines = append(newLines, "unchanged line "+string(rune('A'+i)))
	}
	oldLines[10] = "old line"
	newLines[10] = "new line"

	lines := GenerateDiff(strings.Join(oldLines, "\n"), strings.Join(newLines, "\n"))
	require.NotEmpty(t, lines)

	// Should have context lines around the change but not ALL unchanged lines.
	// Max context = 3 on each side + 2 changed = 8 lines max.
	assert.LessOrEqual(t, len(lines), 9, "Should limit context lines")
}

func TestGenerateDiff_EmptyInputs(t *testing.T) {
	tests := []struct {
		name string
		old  string
		new  string
	}{
		{"Both empty", "", ""},
		{"Old empty", "", "new content"},
		{"New empty", "old content", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := GenerateDiff(tt.old, tt.new)
			if tt.old == "" && tt.new == "" {
				assert.Empty(t, lines)
			} else {
				// At least one change should be present.
				assert.NotEmpty(t, lines)
			}
		})
	}
}

func TestGenerateDiff_IdenticalContent(t *testing.T) {
	content := "line1\nline2\nline3"
	lines := GenerateDiff(content, content)
	assert.Empty(t, lines, "Identical content should produce no diff lines")
}

// --- Task 6.2: Rendering tests ---

func TestRender_StandardMode(t *testing.T) {
	dv := newTestDiffView()
	data := sampleDiffData()
	dv.SetDiff(data)

	result := dv.Render(120, 20)
	stripped := ansi.Strip(result)

	assert.Contains(t, stripped, "--- a/src/handlers/auth.go")
	assert.Contains(t, stripped, "+++ b/src/handlers/auth.go")
	assert.Contains(t, stripped, "+", "Should contain + prefix for added lines")
	assert.Contains(t, stripped, "-", "Should contain - prefix for removed lines")
}

func TestRender_AddedLinesHavePlusPrefix(t *testing.T) {
	dv := newTestDiffView()
	dv.SetDiff(DiffData{
		FilePath:   "test.go",
		OldContent: "a",
		NewContent: "a\nb",
		Lines:      GenerateDiff("a", "a\nb"),
	})

	result := dv.Render(80, 20)
	stripped := ansi.Strip(result)
	lines := strings.Split(stripped, "\n")

	foundPlus := false
	for _, line := range lines {
		if strings.Contains(line, "+") && strings.Contains(line, "b") {
			foundPlus = true
			break
		}
	}
	assert.True(t, foundPlus, "Added lines should have + prefix")
}

func TestRender_RemovedLinesHaveMinusPrefix(t *testing.T) {
	dv := newTestDiffView()
	dv.SetDiff(DiffData{
		FilePath:   "test.go",
		OldContent: "a\nb",
		NewContent: "a",
		Lines:      GenerateDiff("a\nb", "a"),
	})

	result := dv.Render(80, 20)
	stripped := ansi.Strip(result)
	lines := strings.Split(stripped, "\n")

	foundMinus := false
	for _, line := range lines {
		if strings.Contains(line, "-") && strings.Contains(line, "b") {
			foundMinus = true
			break
		}
	}
	assert.True(t, foundMinus, "Removed lines should have - prefix")
}

func TestRender_NoColorMode(t *testing.T) {
	dv := NewDiffView(testTheme(), diffTestConfig())
	dv.SetDiff(sampleDiffData())

	result := dv.Render(120, 20)
	stripped := ansi.Strip(result)

	// In NoColor mode, + and - prefixes should still be present.
	assert.Contains(t, stripped, "+", "NoColor mode should still have + prefix")
	assert.Contains(t, stripped, "-", "NoColor mode should still have - prefix")

	// Raw output must contain zero ANSI escape sequences in no-color mode.
	assert.Equal(t, stripped, result, "NoColor mode should produce zero ANSI escape codes")
}

func TestRender_AccessibleMode(t *testing.T) {
	dv := NewDiffView(testTheme(), diffTestConfigAccessible())
	data := sampleDiffData()
	dv.SetDiff(data)

	result := dv.Render(120, 20)

	assert.Contains(t, result, "[ADD]", "Accessible mode should have [ADD] tags")
	assert.Contains(t, result, "[DEL]", "Accessible mode should have [DEL] tags")
	assert.Contains(t, result, "[CTX]", "Accessible mode should have [CTX] tags")
	assert.Contains(t, result, "[Tab=Accept]", "Accessible mode should have text action bar")
	assert.Contains(t, result, "[Esc=Reject]")
	assert.Contains(t, result, "[e=Edit]")
}

func TestRender_ColorMode(t *testing.T) {
	dv := NewDiffView(testTheme(), diffTestConfigColor())
	data := sampleDiffData()
	dv.SetDiff(data)

	result := dv.Render(120, 20)
	stripped := ansi.Strip(result)

	// Raw result should contain ANSI codes (longer than stripped).
	assert.Greater(t, len(result), len(stripped), "Color mode should include ANSI escape codes")
	// Stripped should still have the content.
	assert.Contains(t, stripped, "src/handlers/auth.go")
}

// --- Task 6.3: State transition tests ---

func TestHandleKey_TabAccepts(t *testing.T) {
	dv := newTestDiffView()
	dv.SetDiff(sampleDiffData())

	msg := dv.HandleKey("tab")
	require.NotNil(t, msg)

	acceptMsg, ok := msg.(tui.DiffAcceptedMsg)
	require.True(t, ok, "Should return DiffAcceptedMsg")
	assert.Equal(t, "src/handlers/auth.go", acceptMsg.FilePath)
	assert.Equal(t, tui.DiffAccepted, dv.State())
}

func TestHandleKey_EscRejects(t *testing.T) {
	dv := newTestDiffView()
	dv.SetDiff(sampleDiffData())

	msg := dv.HandleKey("esc")
	require.NotNil(t, msg)

	rejectMsg, ok := msg.(tui.DiffRejectedMsg)
	require.True(t, ok, "Should return DiffRejectedMsg")
	assert.Equal(t, "src/handlers/auth.go", rejectMsg.FilePath)
	assert.Equal(t, tui.DiffRejected, dv.State())
}

func TestHandleKey_EEditing(t *testing.T) {
	dv := newTestDiffView()
	dv.SetDiff(sampleDiffData())

	msg := dv.HandleKey("e")
	assert.Nil(t, msg, "Editing should not emit a message")
	assert.Equal(t, tui.DiffEditing, dv.State())
}

func TestHandleKey_EditingEscReturnsToViewing(t *testing.T) {
	dv := newTestDiffView()
	dv.SetDiff(sampleDiffData())
	dv.HandleKey("e") // Enter editing state.
	assert.Equal(t, tui.DiffEditing, dv.State())

	msg := dv.HandleKey("esc")
	assert.Nil(t, msg)
	assert.Equal(t, tui.DiffViewing, dv.State(), "Esc in editing should return to viewing")
}

func TestHandleKey_NoDataReturnsNil(t *testing.T) {
	dv := newTestDiffView()
	msg := dv.HandleKey("tab")
	assert.Nil(t, msg)
}

func TestHandleKey_NonViewingStateIgnored(t *testing.T) {
	dv := newTestDiffView()
	dv.SetDiff(sampleDiffData())
	dv.HandleKey("tab") // Move to Accepted state.

	// Further keys should be ignored in Accepted state.
	msg := dv.HandleKey("esc")
	assert.Nil(t, msg)
	assert.Equal(t, tui.DiffAccepted, dv.State(), "State should remain Accepted")
}

func TestSetDiff_ResetsState(t *testing.T) {
	dv := newTestDiffView()
	dv.SetDiff(sampleDiffData())
	dv.HandleKey("tab") // Move to Accepted.
	assert.Equal(t, tui.DiffAccepted, dv.State())

	// SetDiff should reset to Viewing.
	dv.SetDiff(sampleDiffData())
	assert.Equal(t, tui.DiffViewing, dv.State())
	assert.Equal(t, 0, dv.scrollOffset)
}

func TestClear_ResetsToEmpty(t *testing.T) {
	dv := newTestDiffView()
	dv.SetDiff(sampleDiffData())
	dv.HandleKey("tab")

	dv.Clear()
	assert.Nil(t, dv.diffData)
	assert.Equal(t, 0, dv.scrollOffset)
	assert.Equal(t, tui.DiffViewing, dv.state)
}

// --- Task 6.4: Scroll tests ---

func TestScroll_DownAndUp(t *testing.T) {
	dv := newTestDiffView()
	// Create a diff with many lines.
	var oldLines, newLines []string
	for i := range 30 {
		oldLines = append(oldLines, "old "+string(rune('A'+i%26)))
		newLines = append(newLines, "new "+string(rune('A'+i%26)))
	}
	dv.SetDiff(DiffData{
		FilePath:   "big.go",
		OldContent: strings.Join(oldLines, "\n"),
		NewContent: strings.Join(newLines, "\n"),
		Lines:      GenerateDiff(strings.Join(oldLines, "\n"), strings.Join(newLines, "\n")),
	})
	dv.SetSize(80, 10) // Small viewport.

	// Scroll down.
	dv.HandleKey("down")
	assert.Equal(t, 1, dv.scrollOffset)

	dv.HandleKey("j")
	assert.Equal(t, 2, dv.scrollOffset)

	// Scroll up.
	dv.HandleKey("up")
	assert.Equal(t, 1, dv.scrollOffset)

	dv.HandleKey("k")
	assert.Equal(t, 0, dv.scrollOffset)
}

func TestScroll_BoundaryClamping(t *testing.T) {
	dv := newTestDiffView()
	dv.SetDiff(DiffData{
		FilePath:   "small.go",
		OldContent: "a",
		NewContent: "b",
		Lines:      GenerateDiff("a", "b"),
	})
	dv.SetSize(80, 20) // Viewport larger than content.

	// Scroll up past 0.
	dv.HandleKey("up")
	assert.Equal(t, 0, dv.scrollOffset, "Should clamp at 0")

	// Scroll down past max.
	for range 100 {
		dv.HandleKey("down")
	}
	assert.LessOrEqual(t, dv.scrollOffset, dv.maxScrollOffset(), "Should clamp at max")
}

func TestScroll_ContentLargerThanViewport(t *testing.T) {
	dv := newTestDiffView()
	var oldLines, newLines []string
	for i := range 50 {
		oldLines = append(oldLines, "line "+string(rune('A'+i%26)))
		newLines = append(newLines, "changed "+string(rune('A'+i%26)))
	}
	dv.SetDiff(DiffData{
		FilePath:   "large.go",
		OldContent: strings.Join(oldLines, "\n"),
		NewContent: strings.Join(newLines, "\n"),
		Lines:      GenerateDiff(strings.Join(oldLines, "\n"), strings.Join(newLines, "\n")),
	})
	dv.SetSize(80, 10)

	result1 := dv.Render(80, 10)

	// Scroll down and render again — content should differ.
	dv.HandleKey("down")
	dv.HandleKey("down")
	dv.HandleKey("down")
	result2 := dv.Render(80, 10)

	assert.NotEqual(t, result1, result2, "Scrolling should change rendered content")
}

// --- Task 6.5: Width adaptation tests ---

func TestDiffView_Render_NarrowTerminal(t *testing.T) {
	dv := newTestDiffView()
	dv.SetDiff(sampleDiffData())

	result := dv.Render(30, 20)
	assert.NotEmpty(t, result)
	for _, line := range strings.Split(result, "\n") {
		assert.LessOrEqual(t, ansi.StringWidth(line), 30, "Line should not exceed terminal width")
	}
}

func TestDiffView_Render_MediumTerminal(t *testing.T) {
	dv := newTestDiffView()
	dv.SetDiff(sampleDiffData())

	result := dv.Render(80, 20)
	stripped := ansi.Strip(result)
	assert.Contains(t, stripped, "src/handlers/auth.go")
}

func TestDiffView_Render_WideTerminal(t *testing.T) {
	dv := newTestDiffView()
	dv.SetDiff(sampleDiffData())

	result := dv.Render(200, 20)
	stripped := ansi.Strip(result)
	assert.Contains(t, stripped, "src/handlers/auth.go")
}

// --- Task 6.6: Line number gutter tests ---

func TestRender_LineNumberGutter(t *testing.T) {
	dv := newTestDiffView()
	dv.SetDiff(DiffData{
		FilePath:   "test.go",
		OldContent: "line1\nline2\nline3",
		NewContent: "line1\nnewline\nline3",
		Lines:      GenerateDiff("line1\nline2\nline3", "line1\nnewline\nline3"),
	})

	result := dv.Render(120, 20)
	stripped := ansi.Strip(result)

	// Should contain gutter separators.
	assert.Contains(t, stripped, "|", "Should contain gutter separator")
	assert.Contains(t, stripped, ":", "Should contain line number separator")
}

// --- Edge cases ---

func TestRender_EmptyDiffView(t *testing.T) {
	dv := newTestDiffView()
	result := dv.Render(80, 20)
	assert.Empty(t, result, "Empty diff view should render nothing")
}

func TestDiffView_Render_ZeroWidth(t *testing.T) {
	dv := newTestDiffView()
	dv.SetDiff(sampleDiffData())
	result := dv.Render(0, 20)
	assert.Empty(t, result)
}

func TestDiffView_Render_ZeroHeight(t *testing.T) {
	dv := newTestDiffView()
	dv.SetDiff(sampleDiffData())
	assert.Empty(t, dv.Render(80, 0))
	assert.Empty(t, dv.Render(80, 3), "Height < 4 should return empty")
}

func TestDiffView_SetSize_ClampsMinimum(t *testing.T) {
	dv := newTestDiffView()
	dv.SetSize(0, 0)
	assert.Equal(t, 1, dv.width)
	assert.Equal(t, 1, dv.height)
}

func TestDiffView_SetSize_NegativeValues(t *testing.T) {
	dv := newTestDiffView()
	dv.SetSize(-5, -10)
	assert.Equal(t, 1, dv.width)
	assert.Equal(t, 1, dv.height)
}

func TestRender_ActionBar_ViewingState(t *testing.T) {
	dv := newTestDiffView()
	dv.SetDiff(sampleDiffData())

	result := dv.Render(120, 20)
	stripped := ansi.Strip(result)
	assert.Contains(t, stripped, "Accept")
	assert.Contains(t, stripped, "Reject")
	assert.Contains(t, stripped, "Edit")
}

func TestRender_ActionBar_AcceptedState(t *testing.T) {
	dv := newTestDiffView()
	dv.SetDiff(sampleDiffData())
	dv.HandleKey("tab")

	result := dv.Render(120, 20)
	stripped := ansi.Strip(result)
	assert.Contains(t, stripped, "Accepted")
}

func TestRender_ActionBar_RejectedState(t *testing.T) {
	dv := newTestDiffView()
	dv.SetDiff(sampleDiffData())
	dv.HandleKey("esc")

	result := dv.Render(120, 20)
	stripped := ansi.Strip(result)
	assert.Contains(t, stripped, "Rejected")
}

func TestRender_ActionBar_EditingState(t *testing.T) {
	dv := newTestDiffView()
	dv.SetDiff(sampleDiffData())
	dv.HandleKey("e")

	result := dv.Render(120, 20)
	stripped := ansi.Strip(result)
	assert.Contains(t, stripped, "Editing...")
	assert.Contains(t, stripped, "[Esc=Back]")
}

func TestRender_AccessibleActionBar(t *testing.T) {
	dv := NewDiffView(testTheme(), diffTestConfigAccessible())
	dv.SetDiff(sampleDiffData())

	result := dv.Render(120, 20)
	assert.Contains(t, result, "[Tab=Accept]")
	assert.Contains(t, result, "[Esc=Reject]")
	assert.Contains(t, result, "[e=Edit]")
}

func TestFormatGutter(t *testing.T) {
	dv := newTestDiffView()

	tests := []struct {
		name     string
		line     DiffLine
		contains string
	}{
		{
			"Context line",
			DiffLine{Type: DiffLineContext, OldLineNum: 5, NewLineNum: 5},
			"5:5",
		},
		{
			"Added line",
			DiffLine{Type: DiffLineAdded, NewLineNum: 10},
			":10",
		},
		{
			"Removed line",
			DiffLine{Type: DiffLineRemoved, OldLineNum: 7},
			"7:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gutter := dv.formatGutter(tt.line)
			assert.Contains(t, gutter, tt.contains)
			assert.Contains(t, gutter, "|", "Gutter should contain separator")
		})
	}
}

// --- LoadDiff and IsActive tests ---

func TestLoadDiff_GeneratesDiffAndLoads(t *testing.T) {
	dv := newTestDiffView()
	dv.LoadDiff("test.go", "line1\nline2", "line1\nline3")

	assert.True(t, dv.IsActive())
	assert.Equal(t, tui.DiffViewing, dv.State())
	assert.NotNil(t, dv.diffData)
	assert.Equal(t, "test.go", dv.diffData.FilePath)
	assert.NotEmpty(t, dv.diffData.Lines)
}

func TestIsActive_FalseWhenEmpty(t *testing.T) {
	dv := newTestDiffView()
	assert.False(t, dv.IsActive())
}

func TestIsActive_TrueAfterSetDiff(t *testing.T) {
	dv := newTestDiffView()
	dv.SetDiff(sampleDiffData())
	assert.True(t, dv.IsActive())
}

func TestIsActive_FalseAfterClear(t *testing.T) {
	dv := newTestDiffView()
	dv.SetDiff(sampleDiffData())
	dv.Clear()
	assert.False(t, dv.IsActive())
}

// --- Hunk header tests ---

func TestGenerateDiff_HunkHeaders_DiscontiguousChanges(t *testing.T) {
	// Create two changes far apart to force a gap in context.
	var oldLines, newLines []string
	for i := range 30 {
		line := "unchanged " + string(rune('A'+i%26))
		oldLines = append(oldLines, line)
		newLines = append(newLines, line)
	}
	oldLines[5] = "old line A"
	newLines[5] = "new line A"
	oldLines[25] = "old line B"
	newLines[25] = "new line B"

	lines := GenerateDiff(strings.Join(oldLines, "\n"), strings.Join(newLines, "\n"))
	require.NotEmpty(t, lines)

	// Should contain at least one hunk header between the two changes.
	hunkCount := 0
	for _, l := range lines {
		if l.Type == DiffLineHunkHeader {
			hunkCount++
		}
	}
	assert.GreaterOrEqual(t, hunkCount, 1, "Discontiguous changes should produce hunk headers")
}

func TestRender_HunkHeadersVisible(t *testing.T) {
	dv := newTestDiffView()

	var oldLines, newLines []string
	for i := range 30 {
		line := "unchanged " + string(rune('A'+i%26))
		oldLines = append(oldLines, line)
		newLines = append(newLines, line)
	}
	oldLines[5] = "old line A"
	newLines[5] = "new line A"
	oldLines[25] = "old line B"
	newLines[25] = "new line B"

	dv.SetDiff(DiffData{
		FilePath:   "hunks.go",
		OldContent: strings.Join(oldLines, "\n"),
		NewContent: strings.Join(newLines, "\n"),
		Lines:      GenerateDiff(strings.Join(oldLines, "\n"), strings.Join(newLines, "\n")),
	})

	result := dv.Render(120, 40)
	stripped := ansi.Strip(result)
	assert.Contains(t, stripped, "@@", "Rendered output should contain hunk headers")
}

// --- Dynamic gutter width tests ---

func TestFormatGutter_LargeLineNumbers(t *testing.T) {
	dv := newTestDiffView()
	dv.SetDiff(DiffData{
		FilePath:   "big.go",
		OldContent: "",
		NewContent: "",
		Lines: []DiffLine{
			{Type: DiffLineContext, OldLineNum: 1234, NewLineNum: 5678, Content: "x"},
		},
	})

	gutter := dv.formatGutter(DiffLine{OldLineNum: 1234, NewLineNum: 5678})
	assert.Contains(t, gutter, "1234")
	assert.Contains(t, gutter, "5678")
	// Width should accommodate 4-digit numbers.
	assert.GreaterOrEqual(t, dv.gutterOldWidth, 4)
	assert.GreaterOrEqual(t, dv.gutterNewWidth, 4)
}

// --- Render height boundary ---

func TestRender_HeightTooSmall_ReturnsEmpty(t *testing.T) {
	dv := newTestDiffView()
	dv.SetDiff(sampleDiffData())
	// Height < 4 should return empty (need header + 1 line + action bar).
	assert.Empty(t, dv.Render(80, 3))
	assert.Empty(t, dv.Render(80, 2))
	assert.Empty(t, dv.Render(80, 1))
}

// --- splitLines trailing newline ---

func TestSplitLines_TrailingNewline(t *testing.T) {
	lines := splitLines("line1\nline2\n")
	assert.Equal(t, []string{"line1", "line2"}, lines, "Trailing newline should not produce phantom line")
}

// --- No-color accessible action bar ---

func TestRender_NoColor_AcceptedActionBar(t *testing.T) {
	dv := NewDiffView(testTheme(), diffTestConfig())
	dv.SetDiff(sampleDiffData())
	dv.HandleKey("tab")

	result := dv.Render(120, 20)
	assert.Equal(t, result, ansi.Strip(result), "NoColor accepted state should have zero ANSI")
	assert.Contains(t, result, "Accepted")
}

func TestRender_NoColor_RejectedActionBar(t *testing.T) {
	dv := NewDiffView(testTheme(), diffTestConfig())
	dv.SetDiff(sampleDiffData())
	dv.HandleKey("esc")

	result := dv.Render(120, 20)
	assert.Equal(t, result, ansi.Strip(result), "NoColor rejected state should have zero ANSI")
	assert.Contains(t, result, "Rejected")
}
