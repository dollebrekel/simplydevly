// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package panels

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"siply.dev/siply/internal/tui"
)

func TestBuiltinCommands_NotEmpty(t *testing.T) {
	cmds := BuiltinCommands()
	assert.NotEmpty(t, cmds, "BuiltinCommands should return at least one command")
}

func TestBuiltinCommands_AllHaveNameAndDescription(t *testing.T) {
	cmds := BuiltinCommands()
	for _, cmd := range cmds {
		assert.NotEmpty(t, cmd.Name, "every built-in command must have a name")
		assert.NotEmpty(t, cmd.Description, "command %q must have a description", cmd.Name)
	}
}

func TestBuiltinCommands_ContainsExpectedCommands(t *testing.T) {
	cmds := BuiltinCommands()
	names := make(map[string]bool, len(cmds))
	for _, cmd := range cmds {
		names[cmd.Name] = true
	}

	expected := []string{"help", "yolo", "auto-accept", "default", "code", "chat", "plan", "research", "marketplace"}
	for _, name := range expected {
		assert.True(t, names[name], "expected built-in command %q not found", name)
	}
}

func TestBuiltinCommands_MarketplaceHasHandler(t *testing.T) {
	cmds := BuiltinCommands()
	for _, cmd := range cmds {
		if cmd.Name == "marketplace" {
			require.NotNil(t, cmd.Handler, "marketplace command must have a handler")
			teaCmd := cmd.Handler()
			require.NotNil(t, teaCmd, "marketplace handler must return a tea.Cmd")
			msg := teaCmd()
			_, ok := msg.(tui.MarketplaceOpenMsg)
			assert.True(t, ok, "marketplace handler should return MarketplaceOpenMsg")
			return
		}
	}
	t.Fatal("marketplace command not found in BuiltinCommands")
}

func TestBuiltinCommandMap_AllPresent(t *testing.T) {
	m := builtinCommandMap()
	cmds := BuiltinCommands()
	assert.Equal(t, len(cmds), len(m), "map should have same number of entries as slice")
	for _, cmd := range cmds {
		_, ok := m[cmd.Name]
		assert.True(t, ok, "command %q should be in map", cmd.Name)
	}
}
