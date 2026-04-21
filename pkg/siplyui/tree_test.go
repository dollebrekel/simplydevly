// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package siplyui_test

import (
	"strings"
	"testing"

	"siply.dev/siply/pkg/siplyui"
)

func defaultTestSetup() (siplyui.Theme, siplyui.RenderConfig) {
	return siplyui.DefaultTheme(), siplyui.RenderConfig{
		Color:     siplyui.ColorNone,
		Emoji:     false,
		Borders:   siplyui.BorderNone,
		Verbosity: siplyui.VerbosityFull,
	}
}

func TestTree_RenderEmpty(t *testing.T) {
	theme, cfg := defaultTestSetup()
	tree := siplyui.NewTree(nil, theme, cfg)
	got := tree.Render(80, 24)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestTree_RenderBasicNodes(t *testing.T) {
	theme, cfg := defaultTestSetup()
	nodes := []siplyui.TreeNode{
		{Label: "root", Children: []siplyui.TreeNode{{Label: "child"}}},
		{Label: "leaf"},
	}
	tree := siplyui.NewTree(nodes, theme, cfg)
	out := tree.Render(80, 24)
	if !strings.Contains(out, "root") || !strings.Contains(out, "leaf") {
		t.Errorf("expected root and leaf in output, got: %q", out)
	}
	// child not shown because root is collapsed by default.
	if strings.Contains(out, "child") {
		t.Errorf("expected child to be hidden (collapsed), got: %q", out)
	}
}

func TestTree_ExpandCollapse(t *testing.T) {
	theme, cfg := defaultTestSetup()
	nodes := []siplyui.TreeNode{
		{Label: "root", Children: []siplyui.TreeNode{{Label: "child"}}},
	}
	tree := siplyui.NewTree(nodes, theme, cfg)

	// Press enter to expand root (cursor is at 0 = root).
	changed := tree.HandleKey("enter")
	if !changed {
		t.Error("expected changed=true after enter on collapsed node")
	}
	out := tree.Render(80, 24)
	if !strings.Contains(out, "child") {
		t.Errorf("expected child visible after expand, got: %q", out)
	}

	// Press enter again to collapse.
	tree.HandleKey("enter")
	out = tree.Render(80, 24)
	if strings.Contains(out, "child") {
		t.Errorf("expected child hidden after collapse, got: %q", out)
	}
}

func TestTree_Navigation(t *testing.T) {
	theme, cfg := defaultTestSetup()
	nodes := []siplyui.TreeNode{
		{Label: "A"},
		{Label: "B"},
		{Label: "C"},
	}
	tree := siplyui.NewTree(nodes, theme, cfg)
	if tree.CursorIndex() != 0 {
		t.Error("expected initial cursor at 0")
	}
	tree.HandleKey("down")
	if tree.CursorIndex() != 1 {
		t.Errorf("expected cursor at 1, got %d", tree.CursorIndex())
	}
	tree.HandleKey("up")
	if tree.CursorIndex() != 0 {
		t.Errorf("expected cursor back at 0, got %d", tree.CursorIndex())
	}
	// Can't go above 0.
	changed := tree.HandleKey("up")
	if changed {
		t.Error("expected no change at top boundary")
	}
}

func TestTree_Selection(t *testing.T) {
	theme, cfg := defaultTestSetup()
	nodes := []siplyui.TreeNode{{Label: "item"}}
	tree := siplyui.NewTree(nodes, theme, cfg)
	// Space to toggle selection.
	tree.HandleKey(" ")
	if !tree.Nodes[0].Selected {
		t.Error("expected node to be selected after space")
	}
	tree.HandleKey(" ")
	if tree.Nodes[0].Selected {
		t.Error("expected node to be deselected after second space")
	}
}

func TestTree_ScrollViewport(t *testing.T) {
	theme, cfg := defaultTestSetup()
	nodes := make([]siplyui.TreeNode, 20)
	for i := range nodes {
		nodes[i] = siplyui.TreeNode{Label: strings.Repeat("x", i+1)}
	}
	tree := siplyui.NewTree(nodes, theme, cfg)
	// Render with small height.
	out := tree.Render(80, 5)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) > 5 {
		t.Errorf("expected at most 5 lines for height=5, got %d", len(lines))
	}
}

func TestTree_NoColorIndicators(t *testing.T) {
	theme, cfg := defaultTestSetup()
	cfg.Color = siplyui.ColorNone
	nodes := []siplyui.TreeNode{
		{Label: "root", Children: []siplyui.TreeNode{{Label: "child"}}},
	}
	tree := siplyui.NewTree(nodes, theme, cfg)
	out := tree.Render(80, 24)
	// No-color mode uses + for collapsed.
	if !strings.Contains(out, "+") {
		t.Errorf("expected '+' indicator in no-color mode, got: %q", out)
	}
}
