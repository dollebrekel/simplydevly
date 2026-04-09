// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package components

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"siply.dev/siply/internal/tui"
)

// DiffLineType identifies the kind of diff line.
type DiffLineType int

const (
	DiffLineContext DiffLineType = iota
	DiffLineAdded
	DiffLineRemoved
	DiffLineHunkHeader
)

// DiffLine represents a single line in a unified diff.
type DiffLine struct {
	Type       DiffLineType
	Content    string
	OldLineNum int // 0 means not applicable (e.g., added line)
	NewLineNum int // 0 means not applicable (e.g., removed line)
}

// DiffData holds the parsed diff information for display.
type DiffData struct {
	FilePath   string
	OldContent string
	NewContent string
	Lines      []DiffLine
}

// contextLines is the number of context lines around each change hunk.
const contextLines = 3

// GenerateDiff produces a slice of DiffLines from old and new content
// using a longest common subsequence (LCS) algorithm.
func GenerateDiff(oldContent, newContent string) []DiffLine {
	oldLines := splitLines(oldContent)
	newLines := splitLines(newContent)

	lcs := computeLCS(oldLines, newLines)

	// Walk both sequences to produce raw diff lines.
	raw := buildRawDiff(oldLines, newLines, lcs)

	// Add context: keep only lines near changes.
	return filterContext(raw)
}

// splitLines splits content into lines, handling empty content.
// A trailing newline is stripped to avoid a phantom blank line.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// computeLCS returns the longest common subsequence table.
func computeLCS(a, b []string) [][]int {
	m, n := len(a), len(b)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				dp[i][j] = max(dp[i-1][j], dp[i][j-1])
			}
		}
	}
	return dp
}

// buildRawDiff walks the LCS table to produce all diff lines with line numbers.
func buildRawDiff(oldLines, newLines []string, dp [][]int) []DiffLine {
	var result []DiffLine
	i, j := len(oldLines), len(newLines)

	// Backtrack through LCS table.
	var stack []DiffLine
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && oldLines[i-1] == newLines[j-1] {
			stack = append(stack, DiffLine{
				Type:       DiffLineContext,
				Content:    oldLines[i-1],
				OldLineNum: i,
				NewLineNum: j,
			})
			i--
			j--
		} else if j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]) {
			stack = append(stack, DiffLine{
				Type:       DiffLineAdded,
				Content:    newLines[j-1],
				NewLineNum: j,
			})
			j--
		} else {
			stack = append(stack, DiffLine{
				Type:       DiffLineRemoved,
				Content:    oldLines[i-1],
				OldLineNum: i,
			})
			i--
		}
	}

	// Reverse the stack to get correct order.
	for k := len(stack) - 1; k >= 0; k-- {
		result = append(result, stack[k])
	}
	return result
}

// filterContext keeps only changed lines and up to contextLines of surrounding context.
// It also inserts hunk headers at boundaries.
func filterContext(raw []DiffLine) []DiffLine {
	if len(raw) == 0 {
		return nil
	}

	// Mark which lines to include (changed lines + context around them).
	include := make([]bool, len(raw))
	for i, line := range raw {
		if line.Type != DiffLineContext {
			// Include this changed line and context around it.
			start := max(i-contextLines, 0)
			end := min(i+contextLines+1, len(raw))
			for k := start; k < end; k++ {
				include[k] = true
			}
		}
	}

	// If no changes, return nil.
	hasChanges := false
	for _, inc := range include {
		if inc {
			hasChanges = true
			break
		}
	}
	if !hasChanges {
		return nil
	}

	var result []DiffLine
	lastIncluded := -1
	for i, line := range raw {
		if !include[i] {
			continue
		}
		// Insert hunk header at gaps between included regions.
		if lastIncluded >= 0 && i > lastIncluded+1 {
			oldStart := line.OldLineNum
			newStart := line.NewLineNum
			if oldStart == 0 && line.Type == DiffLineAdded {
				// Use previous context's old line number.
				for k := i - 1; k >= 0; k-- {
					if raw[k].OldLineNum > 0 {
						oldStart = raw[k].OldLineNum
						break
					}
				}
			}
			if newStart == 0 && line.Type == DiffLineRemoved {
				for k := i - 1; k >= 0; k-- {
					if raw[k].NewLineNum > 0 {
						newStart = raw[k].NewLineNum
						break
					}
				}
			}
			result = append(result, DiffLine{
				Type:    DiffLineHunkHeader,
				Content: fmt.Sprintf("@@ -%d +%d @@", oldStart, newStart),
			})
		}
		lastIncluded = i
		result = append(result, line)
	}
	return result
}

// DiffView is a pure rendering component for inline diff display.
// It does NOT implement tea.Model — App.View() calls Render() directly.
type DiffView struct {
	theme          tui.Theme
	renderConfig   tui.RenderConfig
	diffData       *DiffData
	state          tui.DiffViewState
	scrollOffset   int
	width          int
	height         int
	gutterOldWidth int
	gutterNewWidth int
}

// NewDiffView creates a DiffView configured with the given theme and config.
func NewDiffView(theme tui.Theme, config tui.RenderConfig) *DiffView {
	return &DiffView{
		theme:        theme,
		renderConfig: config,
		width:        80,
		height:       20,
	}
}

// SetDiff loads diff data, resets scroll, and sets state to Viewing.
func (dv *DiffView) SetDiff(data DiffData) {
	dv.diffData = &data
	dv.scrollOffset = 0
	dv.state = tui.DiffViewing
	dv.computeGutterWidth()
}

// LoadDiff generates diff lines from old/new content and loads them.
// This is the interface-compatible entry point (no components.DiffData dependency).
func (dv *DiffView) LoadDiff(filePath, oldContent, newContent string) {
	lines := GenerateDiff(oldContent, newContent)
	dv.SetDiff(DiffData{
		FilePath:   filePath,
		OldContent: oldContent,
		NewContent: newContent,
		Lines:      lines,
	})
}

// IsActive returns true when the DiffView has diff data loaded.
func (dv *DiffView) IsActive() bool {
	return dv.diffData != nil
}

// computeGutterWidth determines the gutter column widths from max line numbers.
func (dv *DiffView) computeGutterWidth() {
	maxOld, maxNew := 0, 0
	for _, l := range dv.diffData.Lines {
		if l.OldLineNum > maxOld {
			maxOld = l.OldLineNum
		}
		if l.NewLineNum > maxNew {
			maxNew = l.NewLineNum
		}
	}
	dv.gutterOldWidth = len(fmt.Sprintf("%d", max(maxOld, 1)))
	dv.gutterNewWidth = len(fmt.Sprintf("%d", max(maxNew, 1)))
	if dv.gutterOldWidth < 3 {
		dv.gutterOldWidth = 3
	}
	if dv.gutterNewWidth < 3 {
		dv.gutterNewWidth = 3
	}
}

// SetSize updates the view dimensions. Width and height are clamped to minimum 1.
func (dv *DiffView) SetSize(width, height int) {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	dv.width = width
	dv.height = height
}

// State returns the current DiffViewState.
func (dv *DiffView) State() tui.DiffViewState {
	return dv.state
}

// Clear resets the DiffView to an empty state.
func (dv *DiffView) Clear() {
	dv.diffData = nil
	dv.scrollOffset = 0
	dv.state = tui.DiffViewing
}

// HandleKey processes a key press and returns a tea.Msg if a state transition occurs.
// Returns nil if the key is not handled or state is not Viewing.
func (dv *DiffView) HandleKey(key string) tea.Msg {
	if dv.diffData == nil {
		return nil
	}

	switch dv.state {
	case tui.DiffViewing:
		switch key {
		case "tab":
			dv.state = tui.DiffAccepted
			return tui.DiffAcceptedMsg{
				FilePath:   dv.diffData.FilePath,
				NewContent: dv.diffData.NewContent,
			}
		case "esc":
			dv.state = tui.DiffRejected
			return tui.DiffRejectedMsg{
				FilePath: dv.diffData.FilePath,
			}
		case "e":
			dv.state = tui.DiffEditing
			return nil
		case "up", "k":
			dv.scroll(-1)
			return nil
		case "down", "j":
			dv.scroll(1)
			return nil
		}
	case tui.DiffEditing:
		// Esc exits editing placeholder back to viewing.
		if key == "esc" {
			dv.state = tui.DiffViewing
			return nil
		}
	}
	return nil
}

// scroll adjusts the scroll offset with boundary clamping.
func (dv *DiffView) scroll(direction int) {
	dv.scrollOffset += direction
	if dv.scrollOffset < 0 {
		dv.scrollOffset = 0
	}
	maxOff := dv.maxScrollOffset()
	if dv.scrollOffset > maxOff {
		dv.scrollOffset = maxOff
	}
}

// maxScrollOffset returns the maximum valid scroll offset.
// File header (2 lines) and action bar (1 line) are always visible.
func (dv *DiffView) maxScrollOffset() int {
	if dv.diffData == nil {
		return 0
	}
	viewportHeight := dv.diffLineViewportHeight()
	maxOff := len(dv.diffData.Lines) - viewportHeight
	if maxOff < 0 {
		return 0
	}
	return maxOff
}

// diffLineViewportHeight returns lines available for diff content.
// Total height minus 2 for file header and 1 for action bar.
func (dv *DiffView) diffLineViewportHeight() int {
	h := dv.height - 3 // 2 header + 1 action bar
	if h < 1 {
		return 1
	}
	return h
}

// Render produces the diff view string for the given dimensions.
func (dv *DiffView) Render(width, height int) string {
	if width < 1 || height < 4 || dv.diffData == nil {
		return ""
	}

	// Sync stored dimensions so scroll clamping uses the same values.
	dv.width = width
	dv.height = height

	cs := dv.renderConfig.Color
	accessible := dv.renderConfig.Verbosity == tui.VerbosityAccessible

	var b strings.Builder

	// File header (2 lines).
	b.WriteString(dv.renderFileHeader(width, cs, accessible))

	// Diff lines: total height minus 2 header and 1 action bar.
	viewportHeight := height - 3

	lines := dv.diffData.Lines
	start := dv.scrollOffset
	end := start + viewportHeight
	if end > len(lines) {
		end = len(lines)
	}
	if start >= len(lines) {
		start = max(len(lines)-viewportHeight, 0)
		end = len(lines)
	}

	for i := start; i < end; i++ {
		line := dv.renderDiffLine(lines[i], width, cs, accessible)
		b.WriteString(line)
		b.WriteByte('\n')
	}

	// Pad remaining viewport lines if content is shorter.
	rendered := end - start
	for range viewportHeight - rendered {
		b.WriteByte('\n')
	}

	// Action bar (1 line).
	b.WriteString(dv.renderActionBar(width, cs, accessible))

	return b.String()
}

// renderFileHeader renders the --- a/ and +++ b/ header lines.
func (dv *DiffView) renderFileHeader(width int, cs tui.ColorSetting, accessible bool) string {
	fp := dv.diffData.FilePath
	oldHeader := "--- a/" + fp
	newHeader := "+++ b/" + fp

	if !accessible && cs != tui.ColorNone {
		style := dv.theme.Heading.Resolve(cs)
		oldHeader = style.Render(oldHeader)
		newHeader = style.Render(newHeader)
	}

	oldHeader = ansi.Truncate(oldHeader, width, "")
	newHeader = ansi.Truncate(newHeader, width, "")

	return oldHeader + "\n" + newHeader + "\n"
}

// renderDiffLine renders a single diff line with gutter and styling.
func (dv *DiffView) renderDiffLine(line DiffLine, width int, cs tui.ColorSetting, accessible bool) string {
	// Hunk headers are rendered as separators without gutter.
	if line.Type == DiffLineHunkHeader {
		header := line.Content
		if !accessible && cs != tui.ColorNone {
			style := dv.theme.TextMuted.Resolve(cs)
			header = style.Render(header)
		}
		return ansi.Truncate(header, width, "")
	}

	// Build gutter: "old:new | "
	gutter := dv.formatGutter(line)

	// Build content with prefix.
	var prefix string
	var accessibleTag string
	var style lipgloss.Style

	switch line.Type {
	case DiffLineAdded:
		prefix = "+"
		accessibleTag = "[ADD] "
		s := dv.theme.Secondary.Resolve(cs)
		style = s.Bold(true)
	case DiffLineRemoved:
		prefix = "-"
		accessibleTag = "[DEL] "
		s := dv.theme.Error.Resolve(cs)
		style = s.Faint(true)
	default:
		prefix = " "
		accessibleTag = "[CTX] "
		style = dv.theme.TextMuted.Resolve(cs)
	}

	var result string
	content := gutter + prefix + line.Content
	if accessible || cs == tui.ColorNone {
		// No styling in accessible or no-color mode — prefix is the only differentiator.
		if accessible {
			result = accessibleTag + content
		} else {
			result = content
		}
	} else {
		result = style.Render(content)
	}

	return ansi.Truncate(result, width, "")
}

// formatGutter formats the line number gutter as "old:new | ".
// Uses dynamic widths computed from max line numbers.
func (dv *DiffView) formatGutter(line DiffLine) string {
	ow := dv.gutterOldWidth
	nw := dv.gutterNewWidth
	var oldStr, newStr string
	if line.OldLineNum > 0 {
		oldStr = fmt.Sprintf("%*d", ow, line.OldLineNum)
	} else {
		oldStr = strings.Repeat(" ", ow)
	}
	if line.NewLineNum > 0 {
		newStr = fmt.Sprintf("%*d", nw, line.NewLineNum)
	} else {
		newStr = strings.Repeat(" ", nw)
	}
	return oldStr + ":" + newStr + " | "
}

// renderActionBar renders the action bar based on current state.
func (dv *DiffView) renderActionBar(width int, cs tui.ColorSetting, accessible bool) string {
	var bar string

	switch dv.state {
	case tui.DiffAccepted:
		if accessible || cs == tui.ColorNone {
			bar = "Accepted"
		} else {
			bar = dv.theme.Success.Resolve(cs).Render("Accepted")
		}
	case tui.DiffRejected:
		if accessible || cs == tui.ColorNone {
			bar = "Rejected"
		} else {
			bar = dv.theme.Warning.Resolve(cs).Render("Rejected")
		}
	case tui.DiffEditing:
		bar = "Editing... [Esc=Back]"
	default:
		if accessible || cs == tui.ColorNone {
			bar = "[Tab=Accept] [Esc=Reject] [e=Edit]"
		} else {
			keybindStyle := dv.theme.Keybind.Resolve(cs)
			tab := keybindStyle.Render("Tab")
			esc := keybindStyle.Render("Esc")
			e := keybindStyle.Render("e")
			bar = tab + " Accept  " + esc + " Reject  " + e + " Edit"
		}
	}

	return ansi.Truncate(bar, width, "")
}
