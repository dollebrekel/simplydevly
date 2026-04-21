// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package siplyui_test

import (
	"strings"
	"testing"

	"siply.dev/siply/pkg/siplyui"
)

func TestDiffView_AutoMode(t *testing.T) {
	if siplyui.AutoMode(119) != siplyui.DiffInline {
		t.Error("expected Inline for width < 120")
	}
	if siplyui.AutoMode(120) != siplyui.DiffSideBySide {
		t.Error("expected SideBySide for width >= 120")
	}
}

func TestDiffView_LoadDiff(t *testing.T) {
	theme, cfg := defaultTestSetup()
	dv := siplyui.NewDiffView(theme, cfg)
	dv.LoadDiff("test.txt", "hello\nworld\n", "hello\nearth\n")
	if !dv.IsActive() {
		t.Error("expected IsActive after LoadDiff")
	}
}

func TestDiffView_RenderInline(t *testing.T) {
	theme, cfg := defaultTestSetup()
	dv := siplyui.NewDiffView(theme, cfg)
	dv.LoadDiff("test.txt", "hello\n", "world\n")
	out := dv.RenderInline(80, 20)
	if out == "" {
		t.Error("expected non-empty render")
	}
	if !strings.Contains(out, "test.txt") {
		t.Errorf("expected filename in output, got: %q", out)
	}
}

func TestDiffView_RenderSideBySide(t *testing.T) {
	theme, cfg := defaultTestSetup()
	dv := siplyui.NewDiffView(theme, cfg)
	dv.LoadDiff("file.go", "old\n", "new\n")
	out := dv.RenderSideBySide(120, 20)
	if out == "" {
		t.Error("expected non-empty side-by-side render")
	}
}

func TestDiffView_Scroll(t *testing.T) {
	theme, cfg := defaultTestSetup()
	dv := siplyui.NewDiffView(theme, cfg)
	// Build a diff with many lines.
	var old, nw strings.Builder
	for i := 0; i < 30; i++ {
		old.WriteString("old line\n")
		nw.WriteString("new line\n")
	}
	dv.LoadDiff("big.txt", old.String(), nw.String())
	changed := dv.HandleKey("down")
	if !changed {
		t.Error("expected scroll down to return changed=true")
	}
}

func TestDiffView_ToggleMode(t *testing.T) {
	theme, cfg := defaultTestSetup()
	dv := siplyui.NewDiffView(theme, cfg)
	dv.LoadDiff("x.txt", "a\n", "b\n")
	dv.HandleKey("s")
	// After 's', mode should toggle (we can't inspect mode directly, but render should work).
	out := dv.Render(120, 20)
	if out == "" {
		t.Error("expected non-empty render after mode toggle")
	}
}

func TestDiffView_Clear(t *testing.T) {
	theme, cfg := defaultTestSetup()
	dv := siplyui.NewDiffView(theme, cfg)
	dv.LoadDiff("f", "a", "b")
	dv.Clear()
	if dv.IsActive() {
		t.Error("expected IsActive=false after Clear")
	}
}

func TestGenerateDiff_NoDiff(t *testing.T) {
	lines := siplyui.GenerateDiff("same\n", "same\n")
	if len(lines) != 0 {
		t.Errorf("expected no diff lines for identical content, got %d", len(lines))
	}
}

func TestGenerateDiff_HasChanges(t *testing.T) {
	lines := siplyui.GenerateDiff("old\n", "new\n")
	if len(lines) == 0 {
		t.Error("expected diff lines for different content")
	}
}
