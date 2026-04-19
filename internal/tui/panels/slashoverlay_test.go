// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package panels

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"siply.dev/siply/internal/skills"
	"siply.dev/siply/internal/tui"
)

func newTestOverlay() *SlashOverlay {
	theme := tui.DefaultTheme()
	config := tui.RenderConfig{
		Borders: tui.BorderUnicode,
		Color:   tui.ColorNone,
	}
	o := NewSlashOverlay(theme, config)
	o.SetSize(60, 10)
	return o
}

func TestSlashOverlay_ShowHide(t *testing.T) {
	o := newTestOverlay()
	assert.False(t, o.IsVisible(), "overlay should start hidden")

	o.Show()
	assert.True(t, o.IsVisible())

	o.Hide()
	assert.False(t, o.IsVisible())
}

func TestSlashOverlay_SetItems(t *testing.T) {
	o := newTestOverlay()
	builtins := []BuiltinCommand{
		{Name: "help", Description: "Show help"},
		{Name: "code", Description: "Switch to code mode"},
	}
	skillList := []skills.Skill{
		{Name: "code-review", Description: "Review code"},
	}
	o.SetItems(builtins, skillList)
	assert.Len(t, o.allItems, 3)
}

func TestSlashOverlay_Filter(t *testing.T) {
	o := newTestOverlay()
	builtins := []BuiltinCommand{
		{Name: "help", Description: "Show help"},
		{Name: "code", Description: "Switch to code mode"},
		{Name: "chat", Description: "Switch to chat mode"},
	}
	o.SetItems(builtins, nil)

	// Filter by "co" should match "code" only.
	o.Filter("co")
	assert.Equal(t, "code", o.SelectedName())

	// Filter by "ch" should match "chat" only.
	o.Filter("ch")
	assert.Equal(t, "chat", o.SelectedName())

	// Filter by "" should show all.
	o.Filter("")
	// First item should be "help".
	assert.Equal(t, "help", o.SelectedName())

	// Filter with no matches.
	o.Filter("zzz")
	assert.Equal(t, "", o.SelectedName())
}

func TestSlashOverlay_HandleKey_Tab(t *testing.T) {
	o := newTestOverlay()
	o.SetItems([]BuiltinCommand{
		{Name: "help", Description: "Show help"},
	}, nil)
	o.Show()

	selected, closed := o.HandleKey("tab")
	assert.Equal(t, "help", selected)
	assert.False(t, closed)
	assert.False(t, o.IsVisible(), "overlay should hide after selection")
}

func TestSlashOverlay_HandleKey_Enter_NoSelection(t *testing.T) {
	o := newTestOverlay()
	o.SetItems([]BuiltinCommand{
		{Name: "help", Description: "Show help"},
	}, nil)
	o.Show()

	// Enter should not select from the overlay (returns empty).
	selected, closed := o.HandleKey("enter")
	assert.Equal(t, "", selected)
	assert.False(t, closed)
	assert.True(t, o.IsVisible(), "overlay should stay visible on enter")
}

func TestSlashOverlay_HandleKey_Escape(t *testing.T) {
	o := newTestOverlay()
	o.Show()

	selected, closed := o.HandleKey("esc")
	assert.Equal(t, "", selected)
	assert.True(t, closed)
	assert.False(t, o.IsVisible(), "overlay should hide after escape")
}

func TestSlashOverlay_HandleKey_Navigation(t *testing.T) {
	o := newTestOverlay()
	o.SetItems([]BuiltinCommand{
		{Name: "help", Description: "Show help"},
		{Name: "code", Description: "Switch to code mode"},
		{Name: "chat", Description: "Switch to chat mode"},
	}, nil)
	o.Show()

	// Initially selected: "help" (first item).
	assert.Equal(t, "help", o.SelectedName())

	// Down -> "code".
	o.HandleKey("down")
	assert.Equal(t, "code", o.SelectedName())

	// Down -> "chat".
	o.HandleKey("down")
	assert.Equal(t, "chat", o.SelectedName())

	// Up -> "code".
	o.HandleKey("up")
	assert.Equal(t, "code", o.SelectedName())
}

func TestSlashOverlay_View_HiddenReturnsEmpty(t *testing.T) {
	o := newTestOverlay()
	o.Hide()
	assert.Equal(t, "", o.View())
}

func TestSlashOverlay_View_VisibleContainsCommands(t *testing.T) {
	o := newTestOverlay()
	o.SetItems([]BuiltinCommand{
		{Name: "help", Description: "Show help"},
	}, nil)
	o.Show()

	view := o.View()
	stripped := ansi.Strip(view)
	assert.Contains(t, stripped, "/help")
	assert.Contains(t, stripped, "Show help")
}

func TestSlashOverlay_SetSize_Clamping(t *testing.T) {
	o := newTestOverlay()

	o.SetSize(5, 1)
	assert.Equal(t, 20, o.width, "width should be clamped to minimum 20")
	assert.Equal(t, 3, o.height, "height should be clamped to minimum 3")
}

func TestSlashOverlay_FilterCaseInsensitive(t *testing.T) {
	o := newTestOverlay()
	o.SetItems([]BuiltinCommand{
		{Name: "Help", Description: "Show help"},
	}, nil)

	o.Filter("hel")
	require.NotEmpty(t, o.SelectedName())
}

func TestSlashOverlay_SkillItems(t *testing.T) {
	o := newTestOverlay()
	o.SetItems(nil, []skills.Skill{
		{Name: "code-review", Description: "Review code"},
		{Name: "test-gen", Description: "Generate tests"},
	})
	assert.Len(t, o.allItems, 2)

	o.Filter("code")
	assert.Equal(t, "code-review", o.SelectedName())
}
