// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package components

import (
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/tui"
)

func TestNewProgressIndicator(t *testing.T) {
	theme := testTheme()
	rc := testConfig()
	p := NewProgressIndicator("Installing...", &theme, &rc)
	require.NotNil(t, p)
	assert.Equal(t, "Installing...", p.Label())
	assert.False(t, p.IsDone())
}

func TestProgressIndicator_StaticMode_InProgress(t *testing.T) {
	theme := testTheme()
	rc := tui.RenderConfig{
		Color:     tui.ColorNone,
		Motion:    tui.MotionStatic,
		Verbosity: tui.VerbosityFull,
	}
	p := NewProgressIndicator("Loading plugins", &theme, &rc)
	result := p.Render(80)
	assert.Contains(t, result, "[...]")
	assert.Contains(t, result, "Loading plugins")
}

func TestProgressIndicator_StaticMode_Complete(t *testing.T) {
	theme := testTheme()
	rc := tui.RenderConfig{
		Color:     tui.ColorNone,
		Motion:    tui.MotionStatic,
		Verbosity: tui.VerbosityFull,
	}
	p := NewProgressIndicator("Loading plugins", &theme, &rc)
	p.CompleteWithDuration("done", 1200*time.Millisecond)
	result := p.Render(80)
	stripped := ansi.Strip(result)
	assert.Contains(t, stripped, "OK:")
	assert.Contains(t, stripped, "Loading plugins")
	assert.Contains(t, stripped, "1.2s")
	assert.True(t, p.IsDone())
}

func TestProgressIndicator_SpinnerMode_InProgress(t *testing.T) {
	theme := testTheme()
	rc := tui.RenderConfig{
		Color:     tui.ColorNone,
		Motion:    tui.MotionSpinner,
		Verbosity: tui.VerbosityFull,
	}
	p := NewProgressIndicator("Processing", &theme, &rc)
	// Init should return a tick command.
	cmd := p.Init()
	assert.NotNil(t, cmd, "Spinner mode should return tick cmd from Init")
	result := p.Render(80)
	assert.Contains(t, result, "Processing")
}

func TestProgressIndicator_SpinnerMode_Complete(t *testing.T) {
	theme := testTheme()
	rc := tui.RenderConfig{
		Color:     tui.ColorNone,
		Emoji:     true,
		Motion:    tui.MotionSpinner,
		Verbosity: tui.VerbosityFull,
	}
	p := NewProgressIndicator("Processing", &theme, &rc)
	p.CompleteWithDuration("ok", 500*time.Millisecond)
	result := p.Render(80)
	stripped := ansi.Strip(result)
	assert.Contains(t, stripped, "\u2705")
	assert.Contains(t, stripped, "Processing")
	assert.Contains(t, stripped, "500ms")
}

func TestProgressIndicator_AccessibleMode_InProgress(t *testing.T) {
	theme := testTheme()
	rc := testConfigAccessible()
	p := NewProgressIndicator("Installing", &theme, &rc)
	result := p.Render(80)
	assert.Contains(t, result, "[...]")
	assert.Contains(t, result, "Installing")
	assert.Equal(t, result, ansi.Strip(result), "Accessible mode should have no ANSI codes")
}

func TestProgressIndicator_AccessibleMode_Complete(t *testing.T) {
	theme := testTheme()
	rc := testConfigAccessible()
	p := NewProgressIndicator("Installing", &theme, &rc)
	p.CompleteWithDuration("ok", 2500*time.Millisecond)
	result := p.Render(80)
	assert.Contains(t, result, "[DONE]")
	assert.Contains(t, result, "Installing")
	assert.Contains(t, result, "2.5s")
	assert.Equal(t, result, ansi.Strip(result))
}

func TestProgressIndicator_InitNoTick_StaticMode(t *testing.T) {
	theme := testTheme()
	rc := tui.RenderConfig{Motion: tui.MotionStatic}
	p := NewProgressIndicator("test", &theme, &rc)
	cmd := p.Init()
	assert.Nil(t, cmd, "Static mode should not return tick cmd")
}

func TestProgressIndicator_UpdateNoop_WhenDone(t *testing.T) {
	theme := testTheme()
	rc := tui.RenderConfig{Motion: tui.MotionSpinner}
	p := NewProgressIndicator("test", &theme, &rc)
	p.Complete("done")
	cmd := p.Update(nil)
	assert.Nil(t, cmd, "Update should be no-op when done")
}
