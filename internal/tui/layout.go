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
	// Multi-panel width allocation (set by PanelManager).
	LeftPanelWidth    int
	RightPanelWidth   int
	BottomPanelHeight int
	CenterWidth       int
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

	// Default center width equals full content width (no side panels active yet).
	lc.CenterWidth = lc.MaxContentWidth

	return lc
}

// CalculateLayoutWithPanels extends CalculateLayout with explicit panel widths.
// It computes CenterWidth as remaining space after subtracting side panels.
// Auto-collapse rules: Compact and UltraCompact force zero side-panel widths.
func CalculateLayoutWithPanels(width, height, leftWidth, rightWidth, bottomHeight int) LayoutConstraints {
	lc := CalculateLayout(width, height)

	switch lc.Mode {
	case UltraCompact, Compact:
		// Auto-collapse all side panels — not enough space.
		lc.LeftPanelWidth = 0
		lc.RightPanelWidth = 0
		lc.BottomPanelHeight = 0
		lc.CenterWidth = lc.MaxContentWidth
	case Standard:
		// Allow one side panel at most.
		maxPanel := lc.MaxContentWidth - 40
		if maxPanel < 0 {
			maxPanel = 0
		}
		if leftWidth > 0 {
			lc.LeftPanelWidth = clampInt(leftWidth, 0, maxPanel)
			lc.RightPanelWidth = 0
		} else {
			lc.LeftPanelWidth = 0
			lc.RightPanelWidth = clampInt(rightWidth, 0, maxPanel)
		}
		lc.BottomPanelHeight = clampInt(bottomHeight, 0, lc.MaxContentHeight/3)
		lc.CenterWidth = lc.MaxContentWidth - lc.LeftPanelWidth - lc.RightPanelWidth
		if lc.CenterWidth < 40 && lc.CenterWidth < lc.MaxContentWidth {
			lc.LeftPanelWidth = 0
			lc.RightPanelWidth = 0
			lc.CenterWidth = lc.MaxContentWidth
		}
	default: // SplitAvailable
		lc.LeftPanelWidth = clampInt(leftWidth, 0, lc.MaxContentWidth/3)
		lc.RightPanelWidth = clampInt(rightWidth, 0, lc.MaxContentWidth/3)
		lc.BottomPanelHeight = clampInt(bottomHeight, 0, lc.MaxContentHeight/3)
		center := lc.MaxContentWidth - lc.LeftPanelWidth - lc.RightPanelWidth
		if center < 40 && center < lc.MaxContentWidth {
			// Panels consumed too much; collapse and give full width to center.
			lc.LeftPanelWidth = 0
			lc.RightPanelWidth = 0
			center = lc.MaxContentWidth
		}
		lc.CenterWidth = center
	}

	return lc
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
