// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package components

import (
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

func TestModelPicker_RendersCloudAndLocalSections(t *testing.T) {
	p := newTestModelPicker()
	p.OpenLoading()
	p.SetOptions([]tui.ModelOption{
		{Kind: "cloud", Provider: "kimi", Model: "kimi-k2.6", Active: true},
		{Kind: "local", Provider: "ollama", Model: "qwen3:32b"},
	}, nil)

	view := ansi.Strip(p.Render(80, 20))
	assert.Contains(t, view, "Cloud")
	assert.Contains(t, view, "kimi/kimi-k2.6 (active)")
	assert.Contains(t, view, "Local")
	assert.Contains(t, view, "qwen3:32b")
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
