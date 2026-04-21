// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package siplyui_test

import (
	"strings"
	"testing"

	"siply.dev/siply/pkg/siplyui"
)

func TestCard_RenderBasic(t *testing.T) {
	theme, cfg := defaultTestSetup()
	card := siplyui.Card{
		Title:       "My Plugin",
		Description: "Does something cool",
		Version:     "v1.0.0",
		Author:      "Alice",
	}
	out := siplyui.RenderCard(card, theme, cfg, 60)
	if !strings.Contains(out, "My Plugin") {
		t.Errorf("expected title in output, got: %q", out)
	}
	if !strings.Contains(out, "Does something cool") {
		t.Errorf("expected description in output, got: %q", out)
	}
}

func TestCard_RenderAllFields(t *testing.T) {
	theme, cfg := defaultTestSetup()
	card := siplyui.Card{
		Title:        "Tool",
		Description:  "desc",
		Version:      "v2.0",
		Author:       "Bob",
		License:      "MIT",
		Rating:       4.0,
		InstallCount: 1234,
		Badges:       []siplyui.Badge{{Label: "PRO", Style: siplyui.BadgeSuccess}},
	}
	out := siplyui.RenderCard(card, theme, cfg, 80)
	if !strings.Contains(out, "v2.0") {
		t.Errorf("expected version in output, got: %q", out)
	}
	if !strings.Contains(out, "1234 installs") {
		t.Errorf("expected install count in output, got: %q", out)
	}
}

func TestBadge_NoColor(t *testing.T) {
	theme, cfg := defaultTestSetup()
	cfg.Color = siplyui.ColorNone
	badge := siplyui.Badge{Label: "PRO", Style: siplyui.BadgeSuccess}
	out := siplyui.RenderBadge(badge, theme, cfg)
	if !strings.Contains(out, "[PRO]") {
		t.Errorf("expected [PRO] bracket in no-color mode, got: %q", out)
	}
}

func TestCard_Rating_NoColor(t *testing.T) {
	theme, cfg := defaultTestSetup()
	cfg.Color = siplyui.ColorNone
	card := siplyui.Card{Title: "x", Rating: 3.0}
	out := siplyui.RenderCard(card, theme, cfg, 40)
	if !strings.Contains(out, "***") {
		t.Errorf("expected text asterisks for rating in no-color, got: %q", out)
	}
}
