// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package panels

import (
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"siply.dev/siply/internal/skills"
	"siply.dev/siply/internal/tui"
)

// slashItem implements list.Item for the slash command overlay.
type slashItem struct {
	name        string
	description string
}

func (i slashItem) Title() string       { return "/" + i.name }
func (i slashItem) Description() string { return i.description }
func (i slashItem) FilterValue() string { return i.name }

// slashItemDelegate renders slash command items using theme tokens.
type slashItemDelegate struct {
	theme        tui.Theme
	renderConfig tui.RenderConfig
}

func (d slashItemDelegate) Height() int                             { return 1 }
func (d slashItemDelegate) Spacing() int                            { return 0 }
func (d slashItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d slashItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i, ok := item.(slashItem)
	if !ok {
		return
	}

	cs := d.renderConfig.Color
	isSelected := index == m.Index()

	var line string

	if d.renderConfig.Verbosity == tui.VerbosityAccessible {
		if isSelected {
			line = fmt.Sprintf("> /%s — %s", i.name, i.description)
		} else {
			line = fmt.Sprintf("  /%s — %s", i.name, i.description)
		}
		fmt.Fprint(w, line)
		return
	}

	if cs == tui.ColorNone {
		if isSelected {
			style := lipgloss.NewStyle().Reverse(true)
			line = style.Render(fmt.Sprintf("> /%s — %s", i.name, i.description))
		} else {
			nameStr := fmt.Sprintf("  /%s", i.name)
			descStr := lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf(" — %s", i.description))
			line = nameStr + descStr
		}
		fmt.Fprint(w, line)
		return
	}

	if isSelected {
		primaryStyle := d.theme.Primary.Resolve(cs).Bold(true)
		descStyle := d.theme.Muted.Resolve(cs)
		line = primaryStyle.Render(fmt.Sprintf("> /%s", i.name)) + descStyle.Render(fmt.Sprintf(" — %s", i.description))
	} else {
		textStyle := d.theme.Text.Resolve(cs)
		descStyle := d.theme.Muted.Resolve(cs)
		line = textStyle.Render(fmt.Sprintf("  /%s", i.name)) + descStyle.Render(fmt.Sprintf(" — %s", i.description))
	}
	fmt.Fprint(w, line)
}

// SlashOverlay displays a filterable list of available slash commands.
type SlashOverlay struct {
	list         list.Model
	theme        tui.Theme
	renderConfig tui.RenderConfig
	visible      bool
	allItems     []list.Item // unfiltered items for re-filtering
	width        int
	height       int
}

// NewSlashOverlay creates a new slash command overlay.
func NewSlashOverlay(theme tui.Theme, config tui.RenderConfig) *SlashOverlay {
	delegate := slashItemDelegate{theme: theme, renderConfig: config}
	l := list.New(nil, delegate, 40, 10)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowTitle(true)
	l.Title = "Commands"

	cs := config.Color
	if config.Verbosity == tui.VerbosityAccessible {
		l.Title = "[COMMANDS]"
	} else if cs != tui.ColorNone {
		headingStyle := theme.Heading.Resolve(cs)
		l.Styles.Title = headingStyle
		l.Styles.TitleBar = lipgloss.NewStyle()
	}

	if cs == tui.ColorNone {
		l.Styles.ActivePaginationDot = lipgloss.NewStyle()
		l.Styles.InactivePaginationDot = lipgloss.NewStyle()
		l.Styles.Title = lipgloss.NewStyle().Bold(true)
		l.Styles.TitleBar = lipgloss.NewStyle()
		l.Styles.PaginationStyle = lipgloss.NewStyle()
		l.Styles.DividerDot = lipgloss.NewStyle()
		l.Paginator.ActiveDot = "*"
		l.Paginator.InactiveDot = "."
	}

	l.DisableQuitKeybindings()

	return &SlashOverlay{
		list:         l,
		theme:        theme,
		renderConfig: config,
		width:        40,
		height:       10,
	}
}

// SetItems populates the overlay with built-in commands and installed skills.
func (s *SlashOverlay) SetItems(builtins []BuiltinCommand, skillList []skills.Skill) {
	items := make([]list.Item, 0, len(builtins)+len(skillList))
	for _, b := range builtins {
		items = append(items, slashItem{name: b.Name, description: b.Description})
	}
	for _, sk := range skillList {
		items = append(items, slashItem{name: sk.Name, description: sk.Description})
	}
	s.allItems = items
	s.list.SetItems(items)
}

// Show makes the overlay visible.
func (s *SlashOverlay) Show() { s.visible = true }

// Hide makes the overlay invisible.
func (s *SlashOverlay) Hide() { s.visible = false }

// IsVisible returns whether the overlay is currently shown.
func (s *SlashOverlay) IsVisible() bool { return s.visible }

// Filter filters the overlay items by prefix (text after "/").
func (s *SlashOverlay) Filter(prefix string) {
	prefix = strings.ToLower(prefix)
	if prefix == "" {
		s.list.SetItems(s.allItems)
		return
	}
	filtered := make([]list.Item, 0, len(s.allItems))
	for _, item := range s.allItems {
		si := item.(slashItem)
		if strings.HasPrefix(strings.ToLower(si.name), prefix) {
			filtered = append(filtered, item)
		}
	}
	s.list.SetItems(filtered)
}

// SelectedName returns the name of the currently selected command (without "/").
// Returns empty string if no item is selected.
func (s *SlashOverlay) SelectedName() string {
	item := s.list.SelectedItem()
	if item == nil {
		return ""
	}
	if si, ok := item.(slashItem); ok {
		return si.name
	}
	return ""
}

// HandleKey processes a key event for the overlay.
// Returns a command name if Tab was pressed (for insertion), or empty string.
// Returns ("", true) if Escape was pressed (close overlay).
func (s *SlashOverlay) HandleKey(key string) (selected string, closed bool) {
	switch key {
	case "tab":
		name := s.SelectedName()
		if name != "" {
			s.visible = false
			return name, false
		}
		return "", false
	case "esc":
		s.visible = false
		return "", true
	case "up", "k":
		s.list.CursorUp()
		return "", false
	case "down", "j":
		s.list.CursorDown()
		return "", false
	default:
		return "", false
	}
}

// SetSubcommandItems populates the overlay with subcommand items and shows it.
func (s *SlashOverlay) SetSubcommandItems(subcmds []BuiltinCommand) {
	items := make([]list.Item, 0, len(subcmds))
	for _, sc := range subcmds {
		items = append(items, slashItem{name: sc.Name, description: sc.Description})
	}
	s.allItems = items
	s.list.SetItems(items)
	s.visible = true
}

// HandleMouse routes a mouse event to the embedded list model.
func (s *SlashOverlay) HandleMouse(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return cmd
}

// SelectIndex selects an item by index in the list.
func (s *SlashOverlay) SelectIndex(index int) {
	items := s.list.Items()
	if index >= 0 && index < len(items) {
		s.list.Select(index)
	}
}

// ItemCount returns the number of currently visible items.
func (s *SlashOverlay) ItemCount() int {
	return len(s.list.Items())
}

// SetSize updates the overlay dimensions.
func (s *SlashOverlay) SetSize(width, height int) {
	if width < 20 {
		width = 20
	}
	if height < 3 {
		height = 3
	}
	s.width = width
	s.height = height
	s.list.SetSize(width-4, height-2)
}

// View renders the overlay.
func (s *SlashOverlay) View() string {
	if !s.visible {
		return ""
	}

	listView := s.list.View()

	title := "[COMMANDS]"
	if s.renderConfig.Verbosity != tui.VerbosityAccessible {
		cs := s.renderConfig.Color
		if cs != tui.ColorNone {
			headingStyle := s.theme.Heading.Resolve(cs)
			title = headingStyle.Render("Commands")
		} else {
			title = "Commands"
		}
	}

	bordered := tui.RenderBorder(title, listView, s.renderConfig, s.theme, s.width)

	// Truncate lines that exceed width.
	var b strings.Builder
	for i, line := range strings.Split(bordered, "\n") {
		if i > 0 {
			b.WriteByte('\n')
		}
		lineWidth := ansi.StringWidth(line)
		if lineWidth > s.width {
			b.WriteString(ansi.Truncate(line, s.width, ""))
		} else {
			b.WriteString(line)
		}
	}

	return b.String()
}
