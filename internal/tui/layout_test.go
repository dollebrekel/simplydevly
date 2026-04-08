// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalculateLayout_WidthBreakpoints(t *testing.T) {
	tests := []struct {
		name     string
		width    int
		height   int
		wantMode LayoutMode
		wantBorders bool
	}{
		{"ultra-compact at 40", 40, 30, UltraCompact, false},
		{"ultra-compact at 59", 59, 30, UltraCompact, false},
		{"compact at 60", 60, 30, Compact, true},
		{"compact at 79", 79, 30, Compact, true},
		{"standard at 80", 80, 30, Standard, true},
		{"standard at 119", 119, 30, Standard, true},
		{"split at 120", 120, 30, SplitAvailable, true},
		{"split at 200", 200, 30, SplitAvailable, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lc := CalculateLayout(tt.width, tt.height)
			assert.Equal(t, tt.wantMode, lc.Mode, "layout mode")
			assert.Equal(t, tt.wantBorders, lc.ShowBorders, "show borders")
		})
	}
}

func TestCalculateLayout_HeightBreakpoints(t *testing.T) {
	tests := []struct {
		name            string
		height          int
		wantStatusBar   bool
		wantCompact     bool
	}{
		{"hidden at 10", 10, false, false},
		{"hidden at 14", 14, false, false},
		{"compact at 15", 15, true, true},
		{"compact at 24", 24, true, true},
		{"full at 25", 25, true, false},
		{"full at 40", 40, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lc := CalculateLayout(80, tt.height)
			assert.Equal(t, tt.wantStatusBar, lc.ShowStatusBar, "show status bar")
			assert.Equal(t, tt.wantCompact, lc.CompactStatusBar, "compact status bar")
		})
	}
}

func TestCalculateLayout_ContentDimensions(t *testing.T) {
	// Standard layout with borders and full status bar.
	lc := CalculateLayout(100, 30)
	assert.Equal(t, 98, lc.MaxContentWidth, "content width with borders")
	assert.Equal(t, 28, lc.MaxContentHeight, "content height with full status bar")

	// Ultra-compact without borders, no status bar.
	lc = CalculateLayout(40, 10)
	assert.Equal(t, 40, lc.MaxContentWidth, "content width without borders")
	assert.Equal(t, 10, lc.MaxContentHeight, "content height without status bar")

	// Compact status bar.
	lc = CalculateLayout(80, 20)
	assert.Equal(t, 19, lc.MaxContentHeight, "content height with compact status bar")
}

func TestLayoutMode_String(t *testing.T) {
	assert.Equal(t, "ultra-compact", UltraCompact.String())
	assert.Equal(t, "compact", Compact.String())
	assert.Equal(t, "standard", Standard.String())
	assert.Equal(t, "split-available", SplitAvailable.String())
}
