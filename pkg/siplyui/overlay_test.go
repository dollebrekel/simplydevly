// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package siplyui_test

import (
	"strings"
	"testing"

	"siply.dev/siply/pkg/siplyui"
)

func TestOverlay_InitiallyEmpty(t *testing.T) {
	theme, cfg := defaultTestSetup()
	o := siplyui.NewOverlay(theme, cfg)
	if o.IsOpen() {
		t.Error("expected overlay to be closed initially")
	}
	if o.LayerCount() != 0 {
		t.Errorf("expected 0 layers, got %d", o.LayerCount())
	}
}

func TestOverlay_PushPop(t *testing.T) {
	theme, cfg := defaultTestSetup()
	o := siplyui.NewOverlay(theme, cfg)
	o.Push(siplyui.OverlayLayer{Content: func(w, h int) string { return "layer1" }})
	if !o.IsOpen() {
		t.Error("expected overlay open after push")
	}
	if o.LayerCount() != 1 {
		t.Errorf("expected 1 layer, got %d", o.LayerCount())
	}
	o.Pop()
	if o.IsOpen() {
		t.Error("expected overlay closed after pop")
	}
}

func TestOverlay_MaxThreeLayers(t *testing.T) {
	theme, cfg := defaultTestSetup()
	o := siplyui.NewOverlay(theme, cfg)
	for i := 0; i < 5; i++ {
		o.Push(siplyui.OverlayLayer{Content: func(w, h int) string { return "x" }})
	}
	if o.LayerCount() > 3 {
		t.Errorf("expected max 3 layers, got %d", o.LayerCount())
	}
}

func TestOverlay_EscPopsTopLayer(t *testing.T) {
	theme, cfg := defaultTestSetup()
	o := siplyui.NewOverlay(theme, cfg)
	o.Push(siplyui.OverlayLayer{Content: func(w, h int) string { return "layer1" }})
	o.Push(siplyui.OverlayLayer{Content: func(w, h int) string { return "layer2" }})
	o.HandleKey("esc")
	if o.LayerCount() != 1 {
		t.Errorf("expected 1 layer after esc, got %d", o.LayerCount())
	}
}

func TestOverlay_FocusTrap(t *testing.T) {
	theme, cfg := defaultTestSetup()
	o := siplyui.NewOverlay(theme, cfg)
	consumed := false
	o.Push(siplyui.OverlayLayer{
		Content: func(w, h int) string { return "modal" },
		OnKey:   func(key string) bool { consumed = true; return true },
	})
	o.HandleKey("enter")
	if !consumed {
		t.Error("expected key to be routed to top layer's OnKey")
	}
}

func TestOverlay_Clear(t *testing.T) {
	theme, cfg := defaultTestSetup()
	o := siplyui.NewOverlay(theme, cfg)
	o.Push(siplyui.OverlayLayer{Content: func(w, h int) string { return "x" }})
	o.Push(siplyui.OverlayLayer{Content: func(w, h int) string { return "y" }})
	o.Clear()
	if o.IsOpen() {
		t.Error("expected overlay closed after clear")
	}
}

func TestOverlay_RenderNoLayers(t *testing.T) {
	theme, cfg := defaultTestSetup()
	o := siplyui.NewOverlay(theme, cfg)
	if out := o.Render(80, 24); out != "" {
		t.Errorf("expected empty render when no layers, got %q", out)
	}
}

func TestOverlay_RenderShowsContent(t *testing.T) {
	theme, cfg := defaultTestSetup()
	o := siplyui.NewOverlay(theme, cfg)
	o.Push(siplyui.OverlayLayer{Content: func(w, h int) string { return "hello world" }})
	out := o.Render(80, 24)
	if !strings.Contains(out, "hello world") {
		t.Errorf("expected 'hello world' in render output, got: %q", out)
	}
}
