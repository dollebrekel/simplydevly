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

func TestModelPicker_RendersFriendlyNameAndCategory(t *testing.T) {
	p := newTestModelPicker()
	p.OpenLoading()
	p.SetOptions([]tui.ModelOption{
		{Kind: "cloud", Provider: "anthropic", Model: "claude-opus-4-8", Name: "Claude Opus 4.8", Category: "krachtigst", Active: true},
		// No friendly name set: the row must fall back to the raw ID.
		{Kind: "cloud", Provider: "anthropic", Model: "claude-opus-4-8-20250101"},
	}, nil)

	view := ansi.Strip(p.Render(80, 20))
	// The human-readable name and its category are shown, not the cryptic API ID.
	assert.Contains(t, view, "Claude Opus 4.8")
	assert.Contains(t, view, "krachtigst")
	// The active flag stays adjacent to the name.
	assert.Contains(t, view, "Claude Opus 4.8 (active)")
	// An entry without a friendly name still renders via its raw ID fallback.
	assert.Contains(t, view, "claude-opus-4-8-20250101")
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

// manyProviderOptions builds 4 providers × 5 models — more than fits in a short
// window — for the scroll-viewport tests below.
func manyProviderOptions() []tui.ModelOption {
	var opts []tui.ModelOption
	for _, prov := range []string{"anthropic", "openai", "google", "xai"} {
		for i := range 5 {
			opts = append(opts, tui.ModelOption{
				Kind:     "cloud",
				Provider: prov,
				Model:    prov + "-m" + string(rune('a'+i)),
			})
		}
	}
	return opts
}

func TestModelPicker_CursorStaysVisibleWhenScrolled(t *testing.T) {
	p := newTestModelPicker()
	p.OpenLoading()
	opts := manyProviderOptions()
	p.SetOptions(opts, nil)

	// Navigate from the first option to the last one.
	for range len(opts) - 1 {
		p.HandleKey("down")
	}
	last := opts[len(opts)-1].Model

	const height = 10
	view := ansi.Strip(p.Render(80, height))
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	// The window must follow the cursor instead of truncating from the top, so
	// the selected (last) option stays on screen — within the height budget.
	assert.LessOrEqual(t, len(lines), height)
	assert.Contains(t, view, last)
}

func TestModelPicker_CursorAtTopShowsFirstRowAndClipsBottom(t *testing.T) {
	p := newTestModelPicker()
	p.OpenLoading()
	p.SetOptions(manyProviderOptions(), nil)

	// Cursor starts at the top: the first heading + option are visible and the
	// far-down content is clipped (proving it is not rendering everything).
	const height = 10
	view := ansi.Strip(p.Render(80, height))
	assert.Contains(t, view, "Anthropic")
	assert.Contains(t, view, "anthropic-ma")
	assert.NotContains(t, view, "xai-me")
}

func TestModelPicker_CursorWrapAroundResnapsToTop(t *testing.T) {
	p := newTestModelPicker()
	p.OpenLoading()
	opts := manyProviderOptions()
	p.SetOptions(opts, nil)

	// Moving down once per option wraps the cursor back to the first option.
	for range opts {
		p.HandleKey("down")
	}

	const height = 10
	view := ansi.Strip(p.Render(80, height))
	// The window must re-snap to the top: first option visible, bottom clipped.
	assert.Contains(t, view, "anthropic-ma")
	assert.NotContains(t, view, "xai-me")
}

func TestModelPicker_ScrollHintReflectsOverflow(t *testing.T) {
	p := newTestModelPicker()
	p.OpenLoading()

	// Everything fits: no scroll hint.
	p.SetOptions([]tui.ModelOption{
		{Kind: "cloud", Provider: "anthropic", Model: "claude-opus-4-8"},
	}, nil)
	assert.NotContains(t, ansi.Strip(p.Render(80, 20)), "↑↓ more")

	// Content overflows the short window: scroll hint appears.
	p.SetOptions(manyProviderOptions(), nil)
	assert.Contains(t, ansi.Strip(p.Render(80, 10)), "↑↓ more")
}
