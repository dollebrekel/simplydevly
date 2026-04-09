// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package menu

// KeyBinding represents a single keybinding entry.
type KeyBinding struct {
	Key      string
	Action   string
	Category string
}

// KeyBindingCategory groups related keybindings under a named section.
type KeyBindingCategory struct {
	Name     string
	Bindings []KeyBinding
}

// DefaultKeyBindings returns the 5 default keybinding categories (FR44).
func DefaultKeyBindings() []KeyBindingCategory {
	return []KeyBindingCategory{
		{
			Name: "Navigation",
			Bindings: []KeyBinding{
				{Key: "↑ / ↓", Action: "Move selection", Category: "Navigation"},
				{Key: "← / →", Action: "Switch tabs/panels", Category: "Navigation"},
				{Key: "Enter", Action: "Confirm/select", Category: "Navigation"},
				{Key: "Esc", Action: "Back/close", Category: "Navigation"},
				{Key: "Tab / Shift+Tab", Action: "Cycle focus", Category: "Navigation"},
			},
		},
		{
			Name: "AI Agent",
			Bindings: []KeyBinding{
				{Key: "Enter", Action: "Submit prompt", Category: "AI Agent"},
				{Key: "Ctrl+C", Action: "Cancel agent", Category: "AI Agent"},
				{Key: "Ctrl+L", Action: "Clear screen", Category: "AI Agent"},
			},
		},
		{
			Name: "Extensions",
			Bindings: []KeyBinding{
				{Key: "Ctrl+T", Action: "Toggle tree panel", Category: "Extensions"},
				{Key: "Ctrl+B", Action: "Toggle borders", Category: "Extensions"},
			},
		},
		{
			Name: "Git",
			Bindings: []KeyBinding{
				{Key: "—", Action: "Git keybindings (future)", Category: "Git"},
			},
		},
		{
			Name: "Terminal",
			Bindings: []KeyBinding{
				{Key: "Ctrl+A / Ctrl+E", Action: "Start/end of line", Category: "Terminal"},
				{Key: "Ctrl+W", Action: "Delete word", Category: "Terminal"},
				{Key: "Ctrl+U", Action: "Clear line", Category: "Terminal"},
				{Key: "Ctrl+Space", Action: "Open menu", Category: "Terminal"},
			},
		},
	}
}
