// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package components

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/tui"
)

func newTestModelPicker() *ModelPicker {
	p := NewModelPicker(tui.DefaultTheme(), tui.RenderConfig{Color: tui.ColorNone})
	p.SetSize(80, 20)
	return p
}

func TestModelPicker_RendersPerProviderAndLocalSections(t *testing.T) {
	p := newTestModelPicker()
	p.OpenLoading()
	p.SetOptions([]tui.ModelOption{
		{Kind: "cloud", Provider: "anthropic", Model: "claude-opus-4-8", Active: true},
		{Kind: "cloud", Provider: "kimi", Model: "kimi-k2.6"},
		{Kind: "local", Provider: "ollama", Model: "qwen3:32b"},
	}, nil)

	view := ansi.Strip(p.Render(80, 20))
	// Cloud models are grouped under per-provider headings (not a single "Cloud").
	assert.Contains(t, view, "Anthropic")
	assert.Contains(t, view, "Kimi")
	// The provider is in the heading, so the row shows only the model name.
	assert.Contains(t, view, "claude-opus-4-8 (active)")
	assert.NotContains(t, view, "anthropic/claude-opus-4-8")
	assert.Contains(t, view, "Local")
	assert.Contains(t, view, "qwen3:32b")
}

func TestModelPicker_DisabledNoKeyEntryDimmedAndUnselectable(t *testing.T) {
	p := newTestModelPicker()
	p.OpenLoading()
	p.SetOptions([]tui.ModelOption{
		{Kind: "cloud", Provider: "anthropic", Model: "claude-opus-4-8"},
		{Kind: "cloud", Provider: "openai", Model: "gpt-4o", Disabled: true, Description: "(no API key)"},
	}, nil)

	view := ansi.Strip(p.Render(80, 20))
	// Disabled entries are still shown, with their "(no API key)" hint visible.
	assert.Contains(t, view, "OpenAI")
	assert.Contains(t, view, "gpt-4o")
	assert.Contains(t, view, "(no API key)")

	// Cursor starts on the first enabled option; moving down must skip the
	// disabled openai entry and wrap back to the anthropic model.
	p.HandleKey("down")
	msg := p.HandleKey("enter")
	require.IsType(t, tui.ModelSelectedMsg{}, msg)
	assert.Equal(t, "claude-opus-4-8", msg.(tui.ModelSelectedMsg).Option.Model)
}

func TestModelPicker_ManyModelsNoLayoutBreak(t *testing.T) {
	p := newTestModelPicker()
	p.OpenLoading()
	var opts []tui.ModelOption
	for _, prov := range []string{"anthropic", "kimi", "openai", "openrouter"} {
		for i := range 5 {
			opts = append(opts, tui.ModelOption{Kind: "cloud", Provider: prov, Model: prov + "-model-" + string(rune('a'+i))})
		}
	}
	p.SetOptions(opts, nil)

	const height = 12
	view := p.Render(80, height)
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	// Render must respect the height budget (border + truncation) without
	// overflowing — the ~26 untruncated lines are capped to the window height.
	assert.LessOrEqual(t, len(lines), height)
	assert.Greater(t, len(lines), 1)
}

func TestModelPicker_SelectSkipsDisabledOption(t *testing.T) {
	p := newTestModelPicker()
	p.OpenLoading()
	p.SetOptions([]tui.ModelOption{
		{Kind: "cloud", Provider: "kimi", Model: "kimi-k2.6"},
		{Kind: "local", Provider: "ollama", Model: "(Ollama not running)", Disabled: true},
	}, nil)

	p.HandleKey("down")
	msg := p.HandleKey("enter")

	require.IsType(t, tui.ModelSelectedMsg{}, msg)
	selected := msg.(tui.ModelSelectedMsg)
	assert.Equal(t, "kimi-k2.6", selected.Option.Model)
}
