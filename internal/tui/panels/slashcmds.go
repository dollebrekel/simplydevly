// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package panels

import (
	tea "charm.land/bubbletea/v2"
	"siply.dev/siply/internal/tui"
)

// BuiltinCommand represents a built-in slash command with its handler.
type BuiltinCommand struct {
	Name        string
	Description string
	Handler     func() tea.Cmd
}

// BuiltinCommands returns the list of built-in slash commands in display order.
func BuiltinCommands() []BuiltinCommand {
	return []BuiltinCommand{
		{
			Name:        "help",
			Description: "Show help and keybindings",
			Handler:     nil, // Stub — future story
		},
		{
			Name:        "yolo",
			Description: "Enable auto-accept mode",
			Handler:     nil, // Stub — future story
		},
		{
			Name:        "auto-accept",
			Description: "Enable auto-accept mode",
			Handler:     nil, // Stub — future story
		},
		{
			Name:        "default",
			Description: "Reset to default permissions",
			Handler:     nil, // Stub — future story
		},
		{
			Name:        "code",
			Description: "Switch to code mode",
			Handler:     nil, // Stub — future story
		},
		{
			Name:        "chat",
			Description: "Switch to chat mode",
			Handler:     nil, // Stub — future story
		},
		{
			Name:        "plan",
			Description: "Switch to plan mode",
			Handler:     nil, // Stub — future story
		},
		{
			Name:        "research",
			Description: "Switch to research mode",
			Handler:     nil, // Stub — future story
		},
		{
			Name:        "marketplace",
			Description: "Open marketplace browser",
			Handler: func() tea.Cmd {
				return func() tea.Msg {
					return tui.MarketplaceOpenMsg{}
				}
			},
		},
	}
}

// builtinCommandMap returns a map of built-in command name to BuiltinCommand
// for fast lookup during submission.
func builtinCommandMap() map[string]BuiltinCommand {
	cmds := BuiltinCommands()
	m := make(map[string]BuiltinCommand, len(cmds))
	for _, c := range cmds {
		m[c.Name] = c
	}
	return m
}
