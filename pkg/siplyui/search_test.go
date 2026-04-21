// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package siplyui_test

import (
	"strings"
	"testing"

	"siply.dev/siply/pkg/siplyui"
)

func TestSearchField_InitialEmpty(t *testing.T) {
	theme, cfg := defaultTestSetup()
	sf := siplyui.NewSearchField("Search...", theme, cfg)
	if sf.Query() != "" {
		t.Errorf("expected empty query, got %q", sf.Query())
	}
}

func TestSearchField_RenderPlaceholder(t *testing.T) {
	theme, cfg := defaultTestSetup()
	sf := siplyui.NewSearchField("Search...", theme, cfg)
	out := sf.Render(40)
	if !strings.Contains(out, "Search...") {
		t.Errorf("expected placeholder in render, got %q", out)
	}
}

func TestSearchField_InputHandling(t *testing.T) {
	theme, cfg := defaultTestSetup()
	sf := siplyui.NewSearchField("", theme, cfg)
	sf.HandleKey("h")
	sf.HandleKey("e")
	sf.HandleKey("l")
	sf.HandleKey("l")
	sf.HandleKey("o")
	if sf.Query() != "hello" {
		t.Errorf("expected query='hello', got %q", sf.Query())
	}
}

func TestSearchField_Backspace(t *testing.T) {
	theme, cfg := defaultTestSetup()
	sf := siplyui.NewSearchField("", theme, cfg)
	sf.HandleKey("h")
	sf.HandleKey("i")
	sf.HandleKey("backspace")
	if sf.Query() != "h" {
		t.Errorf("expected 'h' after backspace, got %q", sf.Query())
	}
}

func TestSearchField_CtrlUClear(t *testing.T) {
	theme, cfg := defaultTestSetup()
	sf := siplyui.NewSearchField("", theme, cfg)
	sf.SetQuery("hello world")
	sf.HandleKey("ctrl+u")
	if sf.Query() != "" {
		t.Errorf("expected empty after ctrl+u, got %q", sf.Query())
	}
}

func TestSearchField_LeftRightCursor(t *testing.T) {
	theme, cfg := defaultTestSetup()
	sf := siplyui.NewSearchField("", theme, cfg)
	sf.SetQuery("ab")
	sf.HandleKey("left")
	sf.HandleKey("h") // insert at position 1
	if sf.Query() != "ahb" {
		t.Errorf("expected 'ahb', got %q", sf.Query())
	}
}

func TestSearchField_Clear(t *testing.T) {
	theme, cfg := defaultTestSetup()
	sf := siplyui.NewSearchField("", theme, cfg)
	sf.SetQuery("test")
	sf.Clear()
	if sf.Query() != "" {
		t.Errorf("expected empty after Clear(), got %q", sf.Query())
	}
}

func TestSearchField_Match(t *testing.T) {
	theme, cfg := defaultTestSetup()
	sf := siplyui.NewSearchField("", theme, cfg)
	sf.SetQuery("go")
	items := []string{"golang", "python", "Go lang", "ruby"}
	matches := sf.Match(items)
	if len(matches) != 2 {
		t.Errorf("expected 2 matches for 'go', got %d: %v", len(matches), matches)
	}
}

func TestSearchField_MatchEmpty(t *testing.T) {
	theme, cfg := defaultTestSetup()
	sf := siplyui.NewSearchField("", theme, cfg)
	items := []string{"a", "b", "c"}
	matches := sf.Match(items)
	if len(matches) != 3 {
		t.Errorf("expected all 3 items when query is empty, got %d", len(matches))
	}
}
