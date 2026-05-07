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
	Subcommands []BuiltinCommand
}

// BuiltinCommands returns the list of built-in slash commands in display order.
func BuiltinCommands() []BuiltinCommand {
	return []BuiltinCommand{
		// Mode switches
		{
			Name:        "help",
			Description: "Show help and keybindings",
		},
		{
			Name:        "yolo",
			Description: "Enable auto-accept mode",
		},
		{
			Name:        "auto-accept",
			Description: "Enable auto-accept mode",
		},
		{
			Name:        "default",
			Description: "Reset to default permissions",
		},
		{
			Name:        "code",
			Description: "Switch to code mode",
		},
		{
			Name:        "chat",
			Description: "Switch to chat mode",
		},
		{
			Name:        "plan",
			Description: "Switch to plan mode",
		},
		{
			Name:        "research",
			Description: "Switch to research mode",
		},
		// Marketplace
		{
			Name:        "marketplace",
			Description: "Browse and search the marketplace",
			Handler: func() tea.Cmd {
				return func() tea.Msg {
					return tui.MarketplaceOpenMsg{}
				}
			},
			Subcommands: []BuiltinCommand{
				{Name: "list", Description: "List marketplace items"},
				{Name: "search", Description: "Search marketplace items"},
				{Name: "info", Description: "Show item details"},
				{Name: "install", Description: "Install a marketplace item"},
				{Name: "publish", Description: "Publish to marketplace"},
				{Name: "sync", Description: "Sync marketplace data"},
				{Name: "update", Description: "Update a marketplace item"},
				{Name: "rate", Description: "Rate a marketplace item"},
				{Name: "review", Description: "Write a review"},
				{Name: "reviews", Description: "Show reviews for an item"},
				{Name: "report", Description: "Report a marketplace item"},
			},
		},
		// Auth
		{
			Name:        "auth",
			Description: "Sign in, check status, manage account",
			Subcommands: []BuiltinCommand{
				{Name: "login", Description: "Sign in to your account"},
				{Name: "logout", Description: "Sign out of your account"},
				{Name: "pro", Description: "Manage pro subscription"},
				{Name: "status", Description: "Show authentication status"},
			},
		},
		// Plugins
		{
			Name:        "plugins",
			Description: "Manage installed plugins",
			Subcommands: []BuiltinCommand{
				{Name: "list", Description: "List installed plugins"},
				{Name: "install", Description: "Install a plugin"},
				{Name: "remove", Description: "Remove a plugin"},
			},
		},
		// Workspaces
		{
			Name:        "workspaces",
			Description: "Switch or manage workspaces",
			Subcommands: []BuiltinCommand{
				{Name: "list", Description: "List available workspaces"},
				{Name: "switch", Description: "Switch to a workspace"},
			},
		},
		// Skills
		{
			Name:        "skills",
			Description: "Manage installed skills",
			Subcommands: []BuiltinCommand{
				{Name: "list", Description: "List installed skills"},
				{Name: "create", Description: "Scaffold a new custom skill"},
			},
		},
		// Agents
		{
			Name:        "agents",
			Description: "Manage agent configurations",
			Subcommands: []BuiltinCommand{
				{Name: "list", Description: "List agent configurations"},
				{Name: "create", Description: "Scaffold a new agent config"},
			},
		},
		// Profile
		{
			Name:        "profile",
			Description: "Manage install profiles",
			Subcommands: []BuiltinCommand{
				{Name: "list", Description: "List available profiles"},
				{Name: "save", Description: "Save current config as profile"},
				{Name: "share", Description: "Share a profile"},
				{Name: "install", Description: "Install a profile"},
			},
		},
		// Plugin lifecycle commands
		{
			Name:        "update",
			Description: "Update a plugin to latest version",
		},
		{
			Name:        "rollback",
			Description: "Rollback plugin to previous version",
		},
		{
			Name:        "pin",
			Description: "Pin a plugin to a specific version",
		},
		{
			Name:        "unpin",
			Description: "Unpin a plugin, allowing updates",
		},
		{
			Name:        "check",
			Description: "Check plugins for available updates",
		},
		{
			Name:        "install",
			Description: "Install from lockfile",
		},
		{
			Name:        "lock",
			Description: "Generate or verify lockfile",
		},
		// Layout
		{
			Name:        "layout",
			Description: "Manage panel layout",
			Subcommands: []BuiltinCommand{
				{
					Name:        "lock",
					Description: "Lock panel dividers (prevent drag)",
					Handler: func() tea.Cmd {
						return func() tea.Msg { return tui.LayoutLockMsg{Locked: true} }
					},
				},
				{
					Name:        "unlock",
					Description: "Unlock panel dividers (allow drag)",
					Handler: func() tea.Cmd {
						return func() tea.Msg { return tui.LayoutLockMsg{Locked: false} }
					},
				},
			},
		},
		// Task runner
		{
			Name:        "run",
			Description: "Run a one-shot task",
		},
		// Checkpoint & Rewind (Pro)
		{
			Name:        "rewind",
			Description: "Rewind to a previous checkpoint step (Pro)",
		},
		{
			Name:        "checkpoint",
			Description: "List, show, or export checkpoints (Pro)",
			Subcommands: []BuiltinCommand{
				{Name: "list", Description: "List all checkpoints"},
				{Name: "show", Description: "Show checkpoint details"},
				{Name: "export", Description: "Export checkpoint as JSON"},
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
