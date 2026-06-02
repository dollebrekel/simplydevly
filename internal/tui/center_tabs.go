// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tui

import (
	"context"
	"strconv"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

// tabBarSegment is a clickable region of the rendered center tab bar. start and
// end are x-offsets (inclusive/exclusive) relative to the center origin.
type tabBarSegment struct {
	start  int
	end    int
	index  int  // target tab index for a switch segment
	newTab bool // true for the "+" affordance
}

// centerWidth returns the width available to the center window content.
func (a *App) centerWidth() int {
	w := a.width
	if a.panelManager != nil {
		w = a.layout.CenterWidth
		if w == 0 {
			w = a.width
		}
	}
	return w
}

// resizeTabs propagates the current center width and content height (minus the
// tab bar, when shown) to every tab's REPL panel. Sizing all tabs — not just the
// active one — keeps inactive tabs ready to render correctly on switch.
func (a *App) resizeTabs() {
	w := a.centerWidth()
	h := max(a.layout.MaxContentHeight-a.tabBarHeight(), 1)
	for _, t := range a.tabs {
		if t.repl != nil {
			t.repl.SetSize(w, h)
		}
	}
}

// createTab builds a new center tab via the injected factory, makes it active,
// and returns the new REPL's Init command. The toolset selects the new agent's
// tool registry (AC 6). Side-panel plugins are never reloaded — the factory
// shares the plugin layer (E1).
func (a *App) createTab(toolset ToolsetChoice) tea.Cmd {
	if a.tabFactory == nil {
		return func() tea.Msg {
			return FeedbackMsg{Level: LevelWarning, Summary: "Multi-tab is unavailable"}
		}
	}
	id := a.nextTabID
	repl, agent, err := a.tabFactory.NewTab(id, toolset)
	if err != nil {
		return func() tea.Msg {
			return FeedbackMsg{Level: LevelError, Summary: "Could not open new tab", Detail: err.Error()}
		}
	}
	a.nextTabID++
	t := &centerTab{id: id, repl: repl, agent: agent, toolset: toolset, title: strconv.Itoa(len(a.tabs) + 1)}
	if repl != nil {
		repl.SetBordered(a.layout.ShowBorders)
	}
	a.tabs = append(a.tabs, t)
	a.activeTab = len(a.tabs) - 1
	a.resizeTabs()
	if repl != nil {
		return repl.Init()
	}
	return nil
}

// closeActiveTab closes the focused tab, releasing its agent's provider
// resources via AgentCloser.Close, then focuses a neighbor (AC 7). The last tab
// can never be closed (user decision, 2026-06-02).
func (a *App) closeActiveTab() tea.Cmd {
	if len(a.tabs) <= 1 {
		return func() tea.Msg {
			return FeedbackMsg{Level: LevelInfo, Summary: "The last tab can't be closed"}
		}
	}
	t := a.tabs[a.activeTab]
	if closer, ok := t.agent.(AgentCloser); ok {
		_ = closer.Close(context.Background())
	}
	a.tabs = append(a.tabs[:a.activeTab], a.tabs[a.activeTab+1:]...)
	if a.activeTab >= len(a.tabs) {
		a.activeTab = len(a.tabs) - 1
	}
	a.resizeTabs()
	return nil
}

// switchTabBy moves focus by delta tabs, wrapping around (ctrl+pgup/pgdn).
func (a *App) switchTabBy(delta int) {
	n := len(a.tabs)
	if n <= 1 {
		return
	}
	a.activeTab = ((a.activeTab+delta)%n + n) % n
}

// switchTabTo focuses the tab at index, ignoring out-of-range requests
// (alt+1..9 / tab-bar clicks).
func (a *App) switchTabTo(index int) {
	if index < 0 || index >= len(a.tabs) {
		return
	}
	a.activeTab = index
}

// toggleTabBar flips the tab bar between shown and hidden and resizes tabs so
// the freed/consumed row is accounted for (AC 5).
func (a *App) toggleTabBar() {
	v := !a.tabBarVisible()
	a.tabBarOverride = &v
	a.resizeTabs()
}

// handleTabBarClick maps a click on the center tab bar's row to a tab switch or
// a new-tab request. Returns handled=false for clicks outside the bar so they
// fall through to the REPL. Best-effort hit-testing: the bar is the top row of
// the center region, offset by the left panel width.
func (a *App) handleTabBarClick(msg tea.MouseClickMsg) (tea.Cmd, bool) {
	if !a.tabBarVisible() || msg.Y != 0 {
		return nil, false
	}
	originX := 0
	if a.panelManager != nil {
		originX = a.panelManager.LeftPanelWidth()
	}
	rel := msg.X - originX
	for _, seg := range a.tabBarSegments {
		if rel >= seg.start && rel < seg.end {
			if seg.newTab {
				if a.tabFactory != nil {
					a.tabChooserOpen = true
				}
				return nil, true
			}
			a.switchTabTo(seg.index)
			return nil, true
		}
	}
	return nil, false
}

// tabDisplayLabel renders a tab's label: its 1-based position plus a marker for
// the read-only toolset.
func tabDisplayLabel(t *centerTab, index int) string {
	label := strconv.Itoa(index + 1)
	if t.toolset == ToolsetReadOnly {
		label += "·R" // "·R"
	}
	return label
}

// renderCenterTabBar renders the single-row tab bar and records the clickable
// segment ranges for mouse hit-testing. The active tab is highlighted; a "+"
// affordance opens a new tab.
func (a *App) renderCenterTabBar() string {
	cs := a.renderConfig.Color
	activeStyle := a.theme.Heading.Resolve(cs)
	mutedStyle := a.theme.Muted.Resolve(cs)
	plusStyle := a.theme.Accent.Resolve(cs)

	a.tabBarSegments = a.tabBarSegments[:0]
	var out strings.Builder
	x := 0
	for i, t := range a.tabs {
		seg := " " + tabDisplayLabel(t, i) + " "
		w := utf8.RuneCountInString(seg)
		if i == a.activeTab {
			out.WriteString(activeStyle.Render(seg))
		} else {
			out.WriteString(mutedStyle.Render(seg))
		}
		a.tabBarSegments = append(a.tabBarSegments, tabBarSegment{start: x, end: x + w, index: i})
		x += w
	}
	// "+" affordance.
	plus := " + "
	pw := utf8.RuneCountInString(plus)
	out.WriteString(plusStyle.Render(plus))
	a.tabBarSegments = append(a.tabBarSegments, tabBarSegment{start: x, end: x + pw, newTab: true})
	x += pw

	// Pad to the full center width so JoinVertical aligns cleanly. Truncation
	// is avoided by keeping labels compact; overflow simply runs to the edge.
	if width := a.centerWidth(); x < width {
		out.WriteString(strings.Repeat(" ", width-x))
	}
	return out.String()
}

// renderTabChooser draws the toolset chooser as a centered modal over the
// center area while the user picks a toolset for a new tab (AC 6).
func (a *App) renderTabChooser() string {
	cs := a.renderConfig.Color
	heading := a.theme.Heading.Resolve(cs)
	muted := a.theme.Muted.Resolve(cs)

	lines := heading.Render("New tab — choose toolset") + "\n\n" +
		"  [1] Full — all tools (read, write, edit, bash)\n" +
		"  [2] Read-only / Research — no file changes\n\n" +
		muted.Render("  [esc] cancel")

	box := lipgloss.NewStyle().Padding(1, 2)
	if a.caps.Unicode {
		box = box.Border(lipgloss.RoundedBorder())
	} else {
		box = box.Border(lipgloss.NormalBorder())
	}

	w := a.centerWidth()
	h := max(a.layout.MaxContentHeight-a.tabBarHeight(), 1)
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box.Render(lines))
}
