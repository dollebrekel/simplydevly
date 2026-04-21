// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package siplyui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
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
	OldLineNum int
	NewLineNum int
}

// DiffData holds the parsed diff for display.
type DiffData struct {
	FilePath   string
	OldContent string
	NewContent string
	Lines      []DiffLine
}

// DiffMode controls rendering mode.
type DiffMode int

const (
	DiffInline DiffMode = iota
	DiffSideBySide
)

const diffContextLines = 3

// GenerateDiff produces DiffLines from old and new content via LCS.
func GenerateDiff(oldContent, newContent string) []DiffLine {
	old := splitDiffLines(oldContent)
	nw := splitDiffLines(newContent)
	lcs := computeDiffLCS(old, nw)
	raw := buildDiffRaw(old, nw, lcs)
	return filterDiffContext(raw)
}

func splitDiffLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

func computeDiffLCS(a, b []string) [][]int {
	m, n := len(a), len(b)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}
	return dp
}

func buildDiffRaw(old, nw []string, dp [][]int) []DiffLine {
	var stack []DiffLine
	i, j := len(old), len(nw)
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && old[i-1] == nw[j-1] {
			stack = append(stack, DiffLine{Type: DiffLineContext, Content: old[i-1], OldLineNum: i, NewLineNum: j})
			i--
			j--
		} else if j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]) {
			stack = append(stack, DiffLine{Type: DiffLineAdded, Content: nw[j-1], NewLineNum: j})
			j--
		} else {
			stack = append(stack, DiffLine{Type: DiffLineRemoved, Content: old[i-1], OldLineNum: i})
			i--
		}
	}
	for l, r := 0, len(stack)-1; l < r; l, r = l+1, r-1 {
		stack[l], stack[r] = stack[r], stack[l]
	}
	return stack
}

func filterDiffContext(raw []DiffLine) []DiffLine {
	if len(raw) == 0 {
		return nil
	}
	include := make([]bool, len(raw))
	hasChanges := false
	for i, line := range raw {
		if line.Type != DiffLineContext {
			hasChanges = true
			start := i - diffContextLines
			if start < 0 {
				start = 0
			}
			end := i + diffContextLines + 1
			if end > len(raw) {
				end = len(raw)
			}
			for k := start; k < end; k++ {
				include[k] = true
			}
		}
	}
	if !hasChanges {
		return nil
	}
	var result []DiffLine
	last := -1
	for i, line := range raw {
		if !include[i] {
			continue
		}
		if last >= 0 && i > last+1 {
			oldStart, newStart := line.OldLineNum, line.NewLineNum
			result = append(result, DiffLine{
				Type:    DiffLineHunkHeader,
				Content: fmt.Sprintf("@@ -%d +%d @@", oldStart, newStart),
			})
		}
		last = i
		result = append(result, line)
	}
	return result
}

// DiffView is a pure renderer for diffs with inline and side-by-side modes.
type DiffView struct {
	data         *DiffData
	mode         DiffMode
	theme        Theme
	renderConfig RenderConfig
	width        int
	height       int
	scrollOffset int
	gutterOldW   int
	gutterNewW   int
}

// NewDiffView creates a DiffView with the given theme and render config.
func NewDiffView(theme Theme, config RenderConfig) *DiffView {
	return &DiffView{theme: theme, renderConfig: config, width: 80, height: 20}
}

// SetDiff loads diff data and resets scroll.
func (dv *DiffView) SetDiff(data DiffData) {
	dv.data = &data
	dv.scrollOffset = 0
	dv.computeGutter()
}

// LoadDiff generates and loads a diff from old/new content.
func (dv *DiffView) LoadDiff(filePath, oldContent, newContent string) {
	dv.SetDiff(DiffData{
		FilePath:   filePath,
		OldContent: oldContent,
		NewContent: newContent,
		Lines:      GenerateDiff(oldContent, newContent),
	})
}

// IsActive returns true when diff data is loaded.
func (dv *DiffView) IsActive() bool { return dv.data != nil }

// Clear resets the DiffView.
func (dv *DiffView) Clear() { dv.data = nil; dv.scrollOffset = 0 }

// SetSize updates dimensions (clamped to ≥1).
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

// AutoMode returns SideBySide when width >= 120, else Inline.
func AutoMode(width int) DiffMode {
	if width >= 120 {
		return DiffSideBySide
	}
	return DiffInline
}

// HandleKey processes scroll and mode-toggle keys.
func (dv *DiffView) HandleKey(key string) bool {
	if dv.data == nil {
		return false
	}
	switch key {
	case "up", "k":
		if dv.scrollOffset > 0 {
			dv.scrollOffset--
			return true
		}
	case "down", "j":
		max := dv.maxScroll()
		if dv.scrollOffset < max {
			dv.scrollOffset++
			return true
		}
	case "s":
		if dv.mode == DiffInline {
			dv.mode = DiffSideBySide
		} else {
			dv.mode = DiffInline
		}
		return true
	}
	return false
}

// Render renders the diff view with the configured or automatic mode.
func (dv *DiffView) Render(width, height int) string {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	dv.width = width
	dv.height = height

	if dv.data == nil {
		return ""
	}

	mode := dv.mode
	if mode == DiffSideBySide && width < 120 {
		mode = DiffInline
	}

	if mode == DiffSideBySide {
		return dv.RenderSideBySide(width, height)
	}
	return dv.RenderInline(width, height)
}

// RenderInline renders a unified diff format.
func (dv *DiffView) RenderInline(width, height int) string {
	if width < 1 {
		width = 1
	}
	if height < 4 || dv.data == nil {
		return ""
	}
	cs := dv.renderConfig.Color

	var b strings.Builder
	// File header.
	fp := dv.data.FilePath
	b.WriteString(ansi.Truncate("--- a/"+fp, width, "") + "\n")
	b.WriteString(ansi.Truncate("+++ b/"+fp, width, "") + "\n")

	viewH := height - 3
	if viewH < 1 {
		viewH = 1
	}

	lines := dv.data.Lines
	start := dv.scrollOffset
	end := start + viewH
	if end > len(lines) {
		end = len(lines)
	}
	if start >= len(lines) {
		start = 0
		end = 0
	}

	for i := start; i < end; i++ {
		b.WriteString(dv.renderInlineLine(lines[i], width, cs) + "\n")
	}
	for range viewH - (end - start) {
		b.WriteByte('\n')
	}
	b.WriteString(ansi.Truncate("[↑↓=scroll s=side-by-side]", width, ""))
	return b.String()
}

func (dv *DiffView) renderInlineLine(line DiffLine, width int, cs ColorSetting) string {
	if line.Type == DiffLineHunkHeader {
		style := dv.theme.TextMuted.Resolve(cs)
		return ansi.Truncate(style.Render(line.Content), width, "")
	}
	gutter := dv.formatGutter(line)
	content := gutter
	switch line.Type {
	case DiffLineAdded:
		content += "+" + line.Content
		if cs != ColorNone {
			return ansi.Truncate(dv.theme.Secondary.Resolve(cs).Render(content), width, "")
		}
	case DiffLineRemoved:
		content += "-" + line.Content
		if cs != ColorNone {
			return ansi.Truncate(dv.theme.Error.Resolve(cs).Render(content), width, "")
		}
	default:
		content += " " + line.Content
		if cs != ColorNone {
			return ansi.Truncate(dv.theme.TextMuted.Resolve(cs).Render(content), width, "")
		}
	}
	return ansi.Truncate(content, width, "")
}

// RenderSideBySide renders old content on the left, new on the right.
func (dv *DiffView) RenderSideBySide(width, height int) string {
	if width < 10 || height < 4 || dv.data == nil {
		return dv.RenderInline(width, height)
	}
	cs := dv.renderConfig.Color
	half := width / 2

	var b strings.Builder
	fp := dv.data.FilePath
	b.WriteString(ansi.Truncate("--- a/"+fp, half, "") + " " + ansi.Truncate("+++ b/"+fp, half, "") + "\n")

	viewH := height - 3
	if viewH < 1 {
		viewH = 1
	}

	// Build left/right line pairs from diff data.
	type pair struct{ left, right string }
	var pairs []pair

	lines := dv.data.Lines
	i := 0
	for i < len(lines) {
		l := lines[i]
		if l.Type == DiffLineHunkHeader {
			pairs = append(pairs, pair{l.Content, l.Content})
			i++
			continue
		}
		if l.Type == DiffLineContext {
			gutter := fmt.Sprintf("%3d", l.OldLineNum)
			pairs = append(pairs, pair{gutter + " " + l.Content, gutter + " " + l.Content})
			i++
			continue
		}
		// Pair removed + added lines together.
		var removed, added []DiffLine
		for i < len(lines) && lines[i].Type == DiffLineRemoved {
			removed = append(removed, lines[i])
			i++
		}
		for i < len(lines) && lines[i].Type == DiffLineAdded {
			added = append(added, lines[i])
			i++
		}
		maxPairs := len(removed)
		if len(added) > maxPairs {
			maxPairs = len(added)
		}
		for p := 0; p < maxPairs; p++ {
			left, right := "", ""
			if p < len(removed) {
				gutter := fmt.Sprintf("%3d", removed[p].OldLineNum)
				left = gutter + " -" + removed[p].Content
			}
			if p < len(added) {
				gutter := fmt.Sprintf("%3d", added[p].NewLineNum)
				right = gutter + " +" + added[p].Content
			}
			pairs = append(pairs, pair{left, right})
		}
	}

	start := dv.scrollOffset
	end := start + viewH
	if end > len(pairs) {
		end = len(pairs)
	}
	if start >= len(pairs) {
		start = 0
		end = 0
	}

	removedStyle := dv.theme.Error.Resolve(cs)
	addedStyle := dv.theme.Secondary.Resolve(cs)
	sepStyle := dv.theme.Border.Resolve(cs)
	sep := sepStyle.Render("│")

	for _, p := range pairs[start:end] {
		leftStyled := p.left
		rightStyled := p.right
		if cs != ColorNone {
			if strings.HasPrefix(strings.TrimLeft(p.left, "0123456789 "), "-") {
				leftStyled = removedStyle.Render(ansi.Strip(p.left))
			}
			if strings.HasPrefix(strings.TrimLeft(p.right, "0123456789 "), "+") {
				rightStyled = addedStyle.Render(ansi.Strip(p.right))
			}
		}
		left := ansi.Truncate(leftStyled, half-1, "…")
		leftW := ansi.StringWidth(ansi.Strip(left))
		leftPad := half - 1 - leftW
		if leftPad < 0 {
			leftPad = 0
		}
		right := ansi.Truncate(rightStyled, half-1, "…")
		b.WriteString(left + strings.Repeat(" ", leftPad) + sep + right + "\n")
	}
	for range viewH - (end - start) {
		b.WriteByte('\n')
	}
	b.WriteString(ansi.Truncate("[↑↓=scroll s=inline]", width, ""))
	return b.String()
}

func (dv *DiffView) computeGutter() {
	maxOld, maxNew := 0, 0
	for _, l := range dv.data.Lines {
		if l.OldLineNum > maxOld {
			maxOld = l.OldLineNum
		}
		if l.NewLineNum > maxNew {
			maxNew = l.NewLineNum
		}
	}
	dv.gutterOldW = len(fmt.Sprintf("%d", maxOld))
	dv.gutterNewW = len(fmt.Sprintf("%d", maxNew))
	if dv.gutterOldW < 3 {
		dv.gutterOldW = 3
	}
	if dv.gutterNewW < 3 {
		dv.gutterNewW = 3
	}
}

func (dv *DiffView) formatGutter(line DiffLine) string {
	var old, nw string
	if line.OldLineNum > 0 {
		old = fmt.Sprintf("%*d", dv.gutterOldW, line.OldLineNum)
	} else {
		old = strings.Repeat(" ", dv.gutterOldW)
	}
	if line.NewLineNum > 0 {
		nw = fmt.Sprintf("%*d", dv.gutterNewW, line.NewLineNum)
	} else {
		nw = strings.Repeat(" ", dv.gutterNewW)
	}
	return old + ":" + nw + " | "
}

func (dv *DiffView) maxScroll() int {
	if dv.data == nil {
		return 0
	}
	viewH := dv.height - 3
	if viewH < 1 {
		viewH = 1
	}
	max := len(dv.data.Lines) - viewH
	if max < 0 {
		return 0
	}
	return max
}
