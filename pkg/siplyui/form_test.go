// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package siplyui_test

import (
	"testing"

	"siply.dev/siply/pkg/siplyui"
)

func TestForm_Navigation(t *testing.T) {
	theme, cfg := defaultTestSetup()
	fields := []siplyui.FormField{
		siplyui.NewTextField("Name", "Enter name", nil),
		siplyui.NewTextField("Email", "Enter email", nil),
	}
	form := siplyui.NewForm(fields, theme, cfg)
	if form.FocusedIndex() != 0 {
		t.Error("expected initial focus at 0")
	}
	form.HandleKey("tab")
	if form.FocusedIndex() != 1 {
		t.Errorf("expected focus at 1 after tab, got %d", form.FocusedIndex())
	}
	form.HandleKey("tab") // wraps around
	if form.FocusedIndex() != 0 {
		t.Errorf("expected focus wrap to 0, got %d", form.FocusedIndex())
	}
	form.HandleKey("shift+tab")
	if form.FocusedIndex() != 1 {
		t.Errorf("expected focus at 1 after shift+tab, got %d", form.FocusedIndex())
	}
}

func TestForm_Values(t *testing.T) {
	theme, cfg := defaultTestSetup()
	tf := siplyui.NewTextField("Name", "", nil)
	form := siplyui.NewForm([]siplyui.FormField{tf}, theme, cfg)
	form.HandleKey("A")
	form.HandleKey("l")
	form.HandleKey("i")
	form.HandleKey("c")
	form.HandleKey("e")
	vals := form.Values()
	if vals["Name"] != "Alice" {
		t.Errorf("expected 'Alice', got %q", vals["Name"])
	}
}

func TestCheckboxField_Toggle(t *testing.T) {
	theme, cfg := defaultTestSetup()
	cb := siplyui.NewCheckboxField("Enable", false)
	form := siplyui.NewForm([]siplyui.FormField{cb}, theme, cfg)
	form.HandleKey(" ")
	if form.Values()["Enable"] != "true" {
		t.Errorf("expected checked=true after space, got %q", form.Values()["Enable"])
	}
}

func TestSelectField_Navigation(t *testing.T) {
	theme, cfg := defaultTestSetup()
	sf := siplyui.NewSelectField("Color", []string{"Red", "Green", "Blue"})
	form := siplyui.NewForm([]siplyui.FormField{sf}, theme, cfg)
	form.HandleKey("right")
	if form.Values()["Color"] != "Green" {
		t.Errorf("expected 'Green', got %q", form.Values()["Color"])
	}
	form.HandleKey("left")
	if form.Values()["Color"] != "Red" {
		t.Errorf("expected 'Red', got %q", form.Values()["Color"])
	}
}

func TestForm_Render(t *testing.T) {
	theme, cfg := defaultTestSetup()
	fields := []siplyui.FormField{
		siplyui.NewTextField("Name", "placeholder", nil),
	}
	form := siplyui.NewForm(fields, theme, cfg)
	out := form.Render(60)
	if out == "" {
		t.Error("expected non-empty render output")
	}
}
