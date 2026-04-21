// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package siplyui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// TreeNode is a single node in a hierarchical tree.
type TreeNode struct {
	Label    string
	Icon     string // emoji icon shown when config.Emoji == true
	Children []TreeNode
	Expanded bool
	Selected bool
	Data     any
}

// Tree is a pure renderer for hierarchical data with expand/collapse, icons, and selection.
type Tree struct {
	Nodes        []TreeNode
	theme        Theme
	renderConfig RenderConfig
	cursor       int
	scrollOffset int
	viewHeight   int
}

// NewTree creates a Tree with the given nodes, theme, and render config.
func NewTree(nodes []TreeNode, theme Theme, config RenderConfig) *Tree {
	return &Tree{
		Nodes:        nodes,
		theme:        theme,
		renderConfig: config,
	}
}

// Render returns the tree as a string, clipped to width×height.
func (t *Tree) Render(width, height int) string {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}

	t.viewHeight = height
	flat := t.flatten(t.Nodes, 0)

	// Clamp scroll.
	maxOff := len(flat) - height
	if maxOff < 0 {
		maxOff = 0
	}
	if t.scrollOffset > maxOff {
		t.scrollOffset = maxOff
	}
	if t.scrollOffset < 0 {
		t.scrollOffset = 0
	}

	end := t.scrollOffset + height
	if end > len(flat) {
		end = len(flat)
	}
	visible := flat[t.scrollOffset:end]

	cs := t.renderConfig.Color
	noColor := cs == ColorNone
	noEmoji := !t.renderConfig.Emoji

	var b strings.Builder
	for i, entry := range visible {
		absIdx := t.scrollOffset + i
		selected := absIdx == t.cursor

		// Indent.
		indent := strings.Repeat("  ", entry.depth)

		// Expand/collapse icon.
		var expandIcon string
		if len(entry.node.Children) > 0 {
			if noColor {
				if entry.node.Expanded {
					expandIcon = "- "
				} else {
					expandIcon = "+ "
				}
			} else {
				if entry.node.Expanded {
					expandIcon = "▼ "
				} else {
					expandIcon = "▶ "
				}
			}
		} else {
			expandIcon = "  "
		}

		// Node icon.
		nodeIcon := ""
		if entry.node.Icon != "" && !noEmoji {
			nodeIcon = entry.node.Icon + " "
		}

		label := indent + expandIcon + nodeIcon + entry.node.Label
		label = ansi.Truncate(label, width, "…")

		if selected {
			style := t.theme.Highlight.Resolve(cs)
			label = style.Render(ansi.Strip(label))
		} else if entry.node.Selected {
			style := t.theme.Primary.Resolve(cs)
			label = style.Render(ansi.Strip(label))
		}

		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(label)
	}
	return b.String()
}

// HandleKey processes a key press. Returns true if the tree state changed.
func (t *Tree) HandleKey(key string) bool {
	flat := t.flatten(t.Nodes, 0)
	if len(flat) == 0 {
		return false
	}
	if t.cursor >= len(flat) {
		t.cursor = len(flat) - 1
	}
	switch key {
	case "up", "k":
		if t.cursor > 0 {
			t.cursor--
			t.adjustScroll(len(flat))
			return true
		}
	case "down", "j":
		if t.cursor < len(flat)-1 {
			t.cursor++
			t.adjustScroll(len(flat))
			return true
		}
	case "enter":
		entry := flat[t.cursor]
		if len(entry.node.Children) > 0 {
			entry.node.Expanded = !entry.node.Expanded
			t.setNode(t.Nodes, entry.path)
			return true
		}
	case " ":
		entry := flat[t.cursor]
		entry.node.Selected = !entry.node.Selected
		t.setNode(t.Nodes, entry.path)
		return true
	}
	return false
}

// flatEntry is a flattened representation of a tree node used for rendering.
type flatEntry struct {
	node  *TreeNode
	depth int
	path  []int
}

// flatten produces a depth-first ordered list of visible nodes.
func (t *Tree) flatten(nodes []TreeNode, depth int) []flatEntry {
	return t.flattenPath(nodes, depth, nil)
}

func (t *Tree) flattenPath(nodes []TreeNode, depth int, path []int) []flatEntry {
	var result []flatEntry
	for i := range nodes {
		p := append(append([]int{}, path...), i)
		result = append(result, flatEntry{node: &nodes[i], depth: depth, path: p})
		if nodes[i].Expanded {
			result = append(result, t.flattenPath(nodes[i].Children, depth+1, p)...)
		}
	}
	return result
}

// setNode updates a node in the tree using a path from flatten.
func (t *Tree) setNode(nodes []TreeNode, path []int) {
	if len(path) == 0 {
		return
	}
	flat := t.flatten(nodes, 0)
	for _, e := range flat {
		if pathEqual(e.path, path) {
			// Node already updated in place (pointer).
			return
		}
	}
}

func pathEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// adjustScroll ensures cursor is within the visible viewport.
func (t *Tree) adjustScroll(total int) {
	if t.cursor < t.scrollOffset {
		t.scrollOffset = t.cursor
	}
	viewH := t.viewHeight
	if viewH < 1 {
		viewH = 20
	}
	if t.cursor >= t.scrollOffset+viewH {
		t.scrollOffset = t.cursor - viewH + 1
	}
	maxOff := total - viewH
	if maxOff < 0 {
		maxOff = 0
	}
	if t.scrollOffset > maxOff {
		t.scrollOffset = maxOff
	}
}

// CursorIndex returns the current cursor position.
func (t *Tree) CursorIndex() int { return t.cursor }

// SetCursor sets the cursor to the given index (clamped).
func (t *Tree) SetCursor(idx int) {
	flat := t.flatten(t.Nodes, 0)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(flat) {
		idx = len(flat) - 1
	}
	t.cursor = idx
}
