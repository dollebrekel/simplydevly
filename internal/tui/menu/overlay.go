// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package menu

import (
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"siply.dev/siply/internal/tui"
)

// menuItem implements list.Item and list.DefaultItem for the menu.
type menuItem struct {
	title       string
	description string
}

func (i menuItem) Title() string       { return i.title }
func (i menuItem) Description() string { return i.description }
func (i menuItem) FilterValue() string { return i.title }

// menuItems returns the 11 menu items in order (FR43).
func menuItems() []list.Item {
	return []list.Item{
		menuItem{"Workspaces", "Switch or manage workspaces"},
		menuItem{"Extensions", "Browse installed extensions"},
		menuItem{"Marketplace", "Discover and install plugins"},
		menuItem{"Learn", "Keybindings and shortcuts"},
		menuItem{"Triggers", "Manage automation triggers"},
		menuItem{"Theme", "Change color theme"},
		menuItem{"Settings", "Configure siply"},
		menuItem{"Install", "Install a plugin"},
		menuItem{"Remove", "Remove a plugin"},
		menuItem{"Update", "Update plugins"},
		menuItem{"About", "Version and system info"},
	}
}

// itemDelegate renders menu items using theme tokens.
type itemDelegate struct {
	theme        tui.Theme
	renderConfig tui.RenderConfig
}

func (d itemDelegate) Height() int                          { return 2 }
func (d itemDelegate) Spacing() int                         { return 0 }
func (d itemDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }

func (d itemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i, ok := item.(menuItem)
	if !ok {
		return
	}

	cs := d.renderConfig.Color
	isSelected := index == m.Index()

	var title, desc string

	if d.renderConfig.Verbosity == tui.VerbosityAccessible {
		// Accessible mode: numbered items.
		if isSelected {
			title = fmt.Sprintf("> [%d] %s", index+1, i.title)
		} else {
			title = fmt.Sprintf("  [%d] %s", index+1, i.title)
		}
		desc = fmt.Sprintf("      %s", i.description)
		fmt.Fprint(w, title+"\n"+desc)
		return
	}

	if cs == tui.ColorNone {
		// No-color mode: Reverse for selected, plain for normal.
		if isSelected {
			style := lipgloss.NewStyle().Reverse(true)
			title = style.Render(fmt.Sprintf("> %s", i.title))
			desc = fmt.Sprintf("  %s", i.description)
		} else {
			title = fmt.Sprintf("  %s", i.title)
			desc = lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf("  %s", i.description))
		}
		fmt.Fprint(w, title+"\n"+desc)
		return
	}

	// Color mode: use theme tokens.
	if isSelected {
		primaryStyle := d.theme.Primary.Resolve(cs).Bold(true)
		title = primaryStyle.Render(fmt.Sprintf("> %s", i.title))
		descStyle := d.theme.Muted.Resolve(cs)
		desc = descStyle.Render(fmt.Sprintf("  %s", i.description))
	} else {
		textStyle := d.theme.Text.Resolve(cs)
		title = textStyle.Render(fmt.Sprintf("  %s", i.title))
		descStyle := d.theme.Muted.Resolve(cs)
		desc = descStyle.Render(fmt.Sprintf("  %s", i.description))
	}
	fmt.Fprint(w, title+"\n"+desc)
}

// Overlay is the Phase 1 menu overlay using bubbles/list.
type Overlay struct {
	list         list.Model
	theme        tui.Theme
	renderConfig tui.RenderConfig
	open         bool
	learnView    *LearnView
	learnOpen    bool
	width        int
	height       int
}

// NewOverlay creates a new menu overlay with an optional MarkdownRenderer for the Learn view.
func NewOverlay(theme tui.Theme, renderConfig tui.RenderConfig, markdownView ...tui.MarkdownRenderer) *Overlay {
	delegate := itemDelegate{theme: theme, renderConfig: renderConfig}
	items := menuItems()
	l := list.New(items, delegate, 40, 20)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowTitle(true)
	l.Title = "Menu"

	// Style the title using theme tokens.
	cs := renderConfig.Color
	if renderConfig.Verbosity == tui.VerbosityAccessible {
		l.Title = "[MENU]"
	} else {
		headingStyle := theme.Heading.Resolve(cs)
		l.Styles.Title = headingStyle
		l.Styles.TitleBar = lipgloss.NewStyle()
	}

	l.DisableQuitKeybindings()

	// In no-color mode, override all list styles that would emit color codes.
	if cs == tui.ColorNone {
		l.Styles.ActivePaginationDot = lipgloss.NewStyle()
		l.Styles.InactivePaginationDot = lipgloss.NewStyle()
		l.Styles.Title = lipgloss.NewStyle().Bold(true)
		l.Styles.TitleBar = lipgloss.NewStyle()
		l.Styles.PaginationStyle = lipgloss.NewStyle()
		l.Styles.DividerDot = lipgloss.NewStyle()
		// Override paginator dot strings to plain characters.
		l.Paginator.ActiveDot = "*"
		l.Paginator.InactiveDot = "."
	}

	o := &Overlay{
		list:         l,
		theme:        theme,
		renderConfig: renderConfig,
		width:        40,
		height:       20,
	}

	if len(markdownView) > 0 && markdownView[0] != nil {
		o.learnView = NewLearnView(theme, renderConfig, markdownView[0])
	}

	return o
}

func (o *Overlay) IsOpen() bool { return o.open }
func (o *Overlay) Open()        { o.open = true }
func (o *Overlay) Close()       { o.open = false; o.learnOpen = false }

func (o *Overlay) Toggle() {
	if o.open {
		o.open = false
		o.learnOpen = false
		return
	}
	o.open = true
}

// HandleKey processes a key event and returns a message or nil.
func (o *Overlay) HandleKey(key string) tea.Msg {
	// Self-healing: if learnOpen but no learnView, reset state.
	if o.learnOpen && o.learnView == nil {
		o.learnOpen = false
	}

	// When Learn view is open, route keys there.
	if o.learnOpen && o.learnView != nil {
		result := o.learnView.HandleKey(key)
		if _, ok := result.(tui.LearnCloseMsg); ok {
			o.learnOpen = false
			return nil
		}
		return nil
	}

	switch key {
	case "enter":
		if item := o.list.SelectedItem(); item != nil {
			if mi, ok := item.(menuItem); ok {
				// Intercept "Learn" to open the learn view.
				if mi.title == "Learn" && o.learnView != nil {
					o.learnOpen = true
					return nil
				}
				return tui.MenuItemSelectedMsg{Label: mi.title}
			}
		}
		return nil
	case "esc":
		o.open = false
		return nil
	case "up", "k":
		o.list.CursorUp()
		return nil
	case "down", "j":
		o.list.CursorDown()
		return nil
	default:
		// Ignore unknown/empty keys — pagination is handled by up/down.
		return nil
	}
}

// HandleMouse routes a mouse message to the internal list for click support.
func (o *Overlay) HandleMouse(msg tea.Msg) tea.Cmd {
	if o.learnOpen {
		return nil
	}
	var cmd tea.Cmd
	o.list, cmd = o.list.Update(msg)
	return cmd
}

// SetSize updates the overlay dimensions with minimum clamping.
func (o *Overlay) SetSize(width, height int) {
	if width < 20 {
		width = 20
	}
	if height < 5 {
		height = 5
	}
	o.width = width
	o.height = height
	o.list.SetSize(width-4, height-2) // Account for border
	if o.learnView != nil {
		o.learnView.SetSize(width, height)
	}
}

// Render returns the rendered menu overlay using stored dimensions.
// The width and height parameters are accepted for interface compliance
// but the overlay uses its internally stored dimensions (set via SetSize)
// to stay consistent with the list model's configured size.
func (o *Overlay) Render(width, height int) string {
	// Use stored dimensions to stay in sync with list.SetSize.
	width = o.width
	height = o.height

	// When Learn view is open, delegate rendering.
	if o.learnOpen && o.learnView != nil {
		return o.learnView.Render(width, height)
	}

	// Render list content.
	listView := o.list.View()

	// Build title based on mode.
	title := "[MENU]"
	if o.renderConfig.Verbosity != tui.VerbosityAccessible {
		cs := o.renderConfig.Color
		headingStyle := o.theme.Heading.Resolve(cs)
		title = headingStyle.Render("Menu")
	}

	bordered := tui.RenderBorder(title, listView, o.renderConfig, o.theme, width)

	// Pad to fill height.
	lines := strings.Split(bordered, "\n")
	if len(lines) < height {
		padding := strings.Repeat("\n", height-len(lines))
		bordered += padding
	}

	// Truncate lines that exceed width.
	var b strings.Builder
	for i, line := range strings.Split(bordered, "\n") {
		if i > 0 {
			b.WriteByte('\n')
		}
		lineWidth := ansi.StringWidth(line)
		if lineWidth > width {
			b.WriteString(ansi.Truncate(line, width, ""))
		} else {
			b.WriteString(line)
		}
	}

	return b.String()
}
