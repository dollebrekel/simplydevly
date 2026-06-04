// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package components

import (
	"context"
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/tui"
)

var _ tui.SettingsOverlay = (*SettingsOverlay)(nil)

// providerRow is one entry in the "Providers & Keys" tab.
type providerRow struct {
	key     string // provider key, e.g. "anthropic"
	display string // human-readable, e.g. "Anthropic"
	keyless bool   // true for local providers (Ollama) that need no API key
}

// SettingsOverlay is the tabbed Settings screen reached via menu → "Settings".
// Tab 0 lists providers and lets the user enter a masked API key; tab 1 reuses
// the model picker. It replaces the previous direct jump to the model picker.
type SettingsOverlay struct {
	theme        tui.Theme
	renderConfig tui.RenderConfig
	width        int
	height       int
	open         bool

	tabs   []string
	active int // 0 = providers & keys, 1 = model

	providers []providerRow
	cursor    int
	editing   bool
	input     textinput.Model
	keyset    map[string]bool // provider key -> has a stored key
	saveErr   string

	store  core.CredentialStore
	picker *ModelPicker
}

// NewSettingsOverlay builds the overlay. cloudProviders is the ordered list of
// cloud provider keys to offer key entry for; an Ollama (local, keyless) row is
// appended automatically. store may be nil (then all providers read as missing).
func NewSettingsOverlay(theme tui.Theme, rc tui.RenderConfig, store core.CredentialStore, cloudProviders []string) *SettingsOverlay {
	in := textinput.New()
	in.Prompt = "Key: "
	in.EchoMode = textinput.EchoPassword
	in.EchoCharacter = '•'

	rows := make([]providerRow, 0, len(cloudProviders)+1)
	for _, p := range cloudProviders {
		rows = append(rows, providerRow{key: p, display: providerDisplayName(p)})
	}
	rows = append(rows, providerRow{key: "ollama", display: providerDisplayName("ollama"), keyless: true})

	return &SettingsOverlay{
		theme:        theme,
		renderConfig: rc,
		width:        48,
		height:       16,
		tabs:         []string{"Providers & Keys", "Model"},
		providers:    rows,
		input:        in,
		keyset:       map[string]bool{},
		store:        store,
		picker:       NewModelPicker(theme, rc),
	}
}

func (s *SettingsOverlay) IsOpen() bool { return s.open }

func (s *SettingsOverlay) Open() {
	s.open = true
	s.active = 0
	s.cursor = 0
	s.editing = false
	s.saveErr = ""
	s.input.SetValue("")
	s.refreshStatus()
	s.picker.OpenLoading()
	w, h := s.modelTabSize()
	s.picker.SetSize(w, h)
}

func (s *SettingsOverlay) Close() {
	s.open = false
	s.editing = false
	s.input.Blur()
	s.picker.Close()
}

// Init starts the masked-input cursor blink.
func (s *SettingsOverlay) Init() tea.Cmd { return textinput.Blink }

func (s *SettingsOverlay) SetSize(width, height int) {
	s.width = max(width, 1)
	s.height = max(height, 1)
	// Bound the masked input so a long API key scrolls inside the field instead
	// of wrapping and overflowing the box height.
	s.input.SetWidth(max(s.width-14, 8))
	w, h := s.modelTabSize()
	s.picker.SetSize(w, h)
}

// modelTabSize is the body area available to the embedded picker (the overlay
// reserves one row for the tab strip).
func (s *SettingsOverlay) modelTabSize() (int, int) {
	return s.width, max(s.height-1, 1)
}

// SetModelOptions feeds the Model tab's embedded picker.
func (s *SettingsOverlay) SetModelOptions(options []tui.ModelOption, err error) {
	s.picker.SetOptions(options, err)
}

func (s *SettingsOverlay) refreshStatus() {
	s.keyset = map[string]bool{}
	if s.store == nil {
		return
	}
	for _, row := range s.providers {
		if row.keyless {
			continue
		}
		cred, err := s.store.GetProvider(context.Background(), row.key)
		s.keyset[row.key] = err == nil && cred.Value != ""
	}
}

// Update handles a key (and, while editing, drives the masked input).
func (s *SettingsOverlay) Update(msg tea.Msg) tea.Cmd {
	if !s.open {
		return nil
	}
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	key := keyMsg.String()

	// While editing, the masked input captures most keys.
	if s.editing {
		switch key {
		case "esc":
			s.editing = false
			s.input.Blur()
			s.input.SetValue("")
			return nil
		case "enter":
			return s.saveKey()
		default:
			var cmd tea.Cmd
			s.input, cmd = s.input.Update(msg)
			return cmd
		}
	}

	switch key {
	case "esc":
		s.Close()
		return nil
	case "tab", "right":
		s.active = (s.active + 1) % len(s.tabs)
		return nil
	case "shift+tab", "left":
		s.active = (s.active - 1 + len(s.tabs)) % len(s.tabs)
		return nil
	}

	if s.active == 1 {
		// Model tab: forward navigation/selection to the embedded picker.
		if result := s.picker.HandleKey(key); result != nil {
			return func() tea.Msg { return result }
		}
		return nil
	}

	// Providers & Keys tab.
	switch key {
	case "up", "k":
		if s.cursor > 0 {
			s.cursor--
		}
	case "down", "j":
		if s.cursor < len(s.providers)-1 {
			s.cursor++
		}
	case "enter":
		if row := s.providers[s.cursor]; !row.keyless {
			s.editing = true
			s.saveErr = ""
			s.input.SetValue("")
			s.input.Focus()
			return textinput.Blink
		}
	}
	return nil
}

// saveKey persists the typed key for the selected provider. An empty value
// cancels without clearing an existing key (Ask First default).
func (s *SettingsOverlay) saveKey() tea.Cmd {
	val := strings.TrimSpace(s.input.Value())
	row := s.providers[s.cursor]
	s.editing = false
	s.input.Blur()
	s.input.SetValue("")
	if val == "" {
		return nil
	}
	if s.store == nil {
		s.saveErr = "No key storage available"
		return nil
	}
	if err := s.store.SetProvider(context.Background(), row.key, core.Credential{Value: val}); err != nil {
		s.saveErr = "Save failed: " + err.Error()
		return nil
	}
	s.saveErr = ""
	s.refreshStatus()
	// Tell the app to reload model options so the new provider appears.
	return func() tea.Msg { return tui.SettingsKeySavedMsg{} }
}

// HandleMouse consumes clicks while the overlay is open so they don't leak to
// the panels below. Tab switching via mouse is left for a future iteration.
func (s *SettingsOverlay) HandleMouse(_ tea.Msg) tea.Cmd { return nil }

func (s *SettingsOverlay) Render(width, height int) string {
	if !s.open || width < 1 || height < 1 {
		return ""
	}
	strip := s.renderTabStrip(width)
	bodyHeight := max(height-1, 1)

	var body string
	if s.active == 1 {
		body = s.picker.Render(width, bodyHeight)
	} else {
		body = s.renderProviders(width, bodyHeight)
	}
	return strip + "\n" + body
}

func (s *SettingsOverlay) renderTabStrip(width int) string {
	cs := s.renderConfig.Color

	if s.renderConfig.Verbosity == tui.VerbosityAccessible {
		parts := make([]string, len(s.tabs))
		for i, name := range s.tabs {
			if i == s.active {
				parts[i] = "[" + name + "]"
			} else {
				parts[i] = name
			}
		}
		return "Tabs: " + strings.Join(parts, " | ")
	}

	var b strings.Builder
	for i, name := range s.tabs {
		seg := " " + name + " "
		switch {
		case i == s.active && cs == tui.ColorNone:
			b.WriteString(lipgloss.NewStyle().Reverse(true).Render(seg))
		case i == s.active:
			b.WriteString(s.theme.Heading.Resolve(cs).Bold(true).Render(seg))
		default:
			b.WriteString(s.theme.Muted.Resolve(cs).Render(seg))
		}
	}
	line := b.String()
	if ansi.StringWidth(line) > width {
		line = ansi.Truncate(line, width, "")
	}
	return line
}

func (s *SettingsOverlay) renderProviders(width, height int) string {
	var lines []string
	for i, row := range s.providers {
		prefix := "  "
		if i == s.cursor {
			prefix = "> "
		}
		var status string
		switch {
		case row.keyless:
			status = "(local, no key)"
		case s.keyset[row.key]:
			status = "✓ key set"
		default:
			status = "✗ missing"
		}
		line := ansi.Truncate(prefix+padRight(row.display, 12)+status, max(width-4, 1), "...")
		lines = append(lines, s.styleRow(i, row, line))
	}

	innerW := max(width-4, 1)
	lines = append(lines, "")
	if s.editing {
		lines = append(lines, ansi.Truncate(s.input.View(), innerW, ""))
	} else {
		lines = append(lines, s.muted(ansi.Truncate("enter add key   tab switch tab   esc close", innerW, "…")))
	}
	if s.saveErr != "" {
		lines = append(lines, s.errorText(ansi.Truncate(s.saveErr, innerW, "…")))
	}

	// Clamp to the border's body budget so the box never overflows its height.
	if budget := height - 2; budget >= 1 && len(lines) > budget {
		lines = lines[:budget]
	}
	return tui.RenderBorder("Providers & Keys", strings.Join(lines, "\n"), s.renderConfig, s.theme, width)
}

func (s *SettingsOverlay) styleRow(index int, row providerRow, line string) string {
	cs := s.renderConfig.Color
	if index == s.cursor {
		if cs == tui.ColorNone {
			return lipgloss.NewStyle().Reverse(true).Render(line)
		}
		return s.theme.Primary.Resolve(cs).Bold(true).Render(line)
	}
	if !row.keyless && s.keyset[row.key] {
		return s.theme.Success.Resolve(cs).Render(line)
	}
	return s.theme.Text.Resolve(cs).Render(line)
}

func (s *SettingsOverlay) muted(text string) string {
	return s.theme.Muted.Resolve(s.renderConfig.Color).Render(text)
}

func (s *SettingsOverlay) errorText(text string) string {
	return s.theme.Error.Resolve(s.renderConfig.Color).Render(text)
}

func padRight(s string, n int) string {
	if w := utf8.RuneCountInString(s); w < n {
		return s + strings.Repeat(" ", n-w)
	}
	return s
}
