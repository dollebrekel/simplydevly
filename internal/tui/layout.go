// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tui

// LayoutMode represents width-based layout categories (UX-DR4).
type LayoutMode int

const (
	// UltraCompact for terminals narrower than 60 columns.
	UltraCompact LayoutMode = iota
	// Compact for terminals 60-79 columns wide.
	Compact
	// Standard for terminals 80-119 columns wide.
	Standard
	// SplitAvailable for terminals 120+ columns wide.
	SplitAvailable
)

// String returns a human-readable name for the layout mode.
func (l LayoutMode) String() string {
	switch l {
	case UltraCompact:
		return "ultra-compact"
	case Compact:
		return "compact"
	case Standard:
		return "standard"
	case SplitAvailable:
		return "split-available"
	default:
		return "unknown"
	}
}

// LayoutConstraints describes layout properties derived from terminal dimensions.
type LayoutConstraints struct {
	Mode             LayoutMode
	ShowStatusBar    bool
	CompactStatusBar bool
	ShowBorders      bool
	MaxContentWidth  int
	MaxContentHeight int
}

// CalculateLayout computes the layout mode and constraints from terminal dimensions.
func CalculateLayout(width, height int) LayoutConstraints {
	// Clamp to safe minimums to prevent negative dimensions downstream.
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}

	lc := LayoutConstraints{
		MaxContentWidth:  width,
		MaxContentHeight: height,
	}

	// Width-based layout mode (UX-DR4).
	switch {
	case width < 60:
		lc.Mode = UltraCompact
		lc.ShowBorders = false
	case width < 80:
		lc.Mode = Compact
		lc.ShowBorders = true
	case width < 120:
		lc.Mode = Standard
		lc.ShowBorders = true
	default:
		lc.Mode = SplitAvailable
		lc.ShowBorders = true
	}

	// Height-based status bar visibility (UX-DR5).
	switch {
	case height < 15:
		lc.ShowStatusBar = false
		lc.CompactStatusBar = false
	case height < 25:
		lc.ShowStatusBar = true
		lc.CompactStatusBar = true
	default:
		lc.ShowStatusBar = true
		lc.CompactStatusBar = false
	}

	// Adjust content height for status bar.
	if lc.ShowStatusBar {
		if lc.CompactStatusBar {
			lc.MaxContentHeight = height - 1
		} else {
			lc.MaxContentHeight = height - 2
		}
	}

	// Adjust content width for borders.
	if lc.ShowBorders {
		lc.MaxContentWidth = width - 2
	}

	return lc
}
