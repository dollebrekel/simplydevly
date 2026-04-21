// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package siplyui_test

import (
	"strings"
	"testing"
	"time"

	"siply.dev/siply/pkg/siplyui"
)

func TestToastManager_Push(t *testing.T) {
	theme, cfg := defaultTestSetup()
	tm := siplyui.NewToastManager(theme, cfg)
	tm.Push("Hello", siplyui.LevelInfo, time.Minute)
	if tm.Count() != 1 {
		t.Errorf("expected 1 toast, got %d", tm.Count())
	}
}

func TestToastManager_Render(t *testing.T) {
	theme, cfg := defaultTestSetup()
	tm := siplyui.NewToastManager(theme, cfg)
	tm.Push("test message", siplyui.LevelSuccess, time.Minute)
	out := tm.Render(80)
	if !strings.Contains(out, "test message") {
		t.Errorf("expected message in render, got: %q", out)
	}
}

func TestToastManager_MaxVisible(t *testing.T) {
	theme, cfg := defaultTestSetup()
	tm := siplyui.NewToastManager(theme, cfg)
	for i := 0; i < 5; i++ {
		tm.Push("msg", siplyui.LevelInfo, time.Minute)
	}
	out := tm.Render(80)
	// At most 3 visible lines.
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) > 3 {
		t.Errorf("expected at most 3 visible toasts, got %d lines", len(lines))
	}
}

func TestToastManager_Tick_Expiry(t *testing.T) {
	theme, cfg := defaultTestSetup()
	tm := siplyui.NewToastManager(theme, cfg)
	tm.Push("expired", siplyui.LevelInfo, time.Nanosecond)
	time.Sleep(time.Millisecond)
	changed := tm.Tick()
	if !changed {
		t.Error("expected Tick to report change after toast expired")
	}
	if tm.Count() != 0 {
		t.Errorf("expected 0 toasts after expiry, got %d", tm.Count())
	}
}

func TestToastManager_NoColor_Labels(t *testing.T) {
	theme, cfg := defaultTestSetup()
	cfg.Color = siplyui.ColorNone
	tm := siplyui.NewToastManager(theme, cfg)
	tm.Push("problem", siplyui.LevelError, time.Minute)
	out := tm.Render(80)
	if !strings.Contains(out, "[ERR]") {
		t.Errorf("expected [ERR] label in no-color mode, got: %q", out)
	}
}

func TestToastManager_EmptyRender(t *testing.T) {
	theme, cfg := defaultTestSetup()
	tm := siplyui.NewToastManager(theme, cfg)
	if out := tm.Render(80); out != "" {
		t.Errorf("expected empty render with no toasts, got %q", out)
	}
}
