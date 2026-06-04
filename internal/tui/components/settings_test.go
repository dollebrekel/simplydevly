// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package components

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/tui"
)

// fakeCredStore is an in-memory core.CredentialStore for the settings tests.
type fakeCredStore struct {
	keys   map[string]string
	setErr error
}

func (f *fakeCredStore) Init(context.Context) error  { return nil }
func (f *fakeCredStore) Start(context.Context) error { return nil }
func (f *fakeCredStore) Stop(context.Context) error  { return nil }
func (f *fakeCredStore) Health() error               { return nil }

func (f *fakeCredStore) GetProvider(_ context.Context, provider string) (core.Credential, error) {
	if f.keys == nil {
		return core.Credential{}, nil
	}
	return core.Credential{Value: f.keys[provider]}, nil
}

func (f *fakeCredStore) SetProvider(_ context.Context, provider string, cred core.Credential) error {
	if f.setErr != nil {
		return f.setErr
	}
	if f.keys == nil {
		f.keys = map[string]string{}
	}
	f.keys[provider] = cred.Value
	return nil
}

func (f *fakeCredStore) GetPluginCredential(context.Context, string, string) (core.Credential, error) {
	return core.Credential{}, nil
}
func (f *fakeCredStore) SetPluginCredential(context.Context, string, string, core.Credential) error {
	return nil
}

func newTestSettings(store core.CredentialStore) *SettingsOverlay {
	s := NewSettingsOverlay(
		tui.DefaultTheme(),
		tui.RenderConfig{Color: tui.ColorNone},
		store,
		[]string{"anthropic", "openai", "kimi", "openrouter"},
	)
	s.SetSize(80, 20)
	s.Open()
	return s
}

func typeString(s *SettingsOverlay, text string) {
	for _, r := range text {
		s.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func TestSettingsOverlay_ShowsProviderKeyStatus(t *testing.T) {
	s := newTestSettings(&fakeCredStore{keys: map[string]string{"anthropic": "sk-live"}})

	view := ansi.Strip(s.Render(80, 20))
	assert.Contains(t, view, "Anthropic")
	assert.Contains(t, view, "✓ key set")
	assert.Contains(t, view, "OpenAI")
	assert.Contains(t, view, "✗ missing")
	// Ollama is shown as a keyless local row.
	assert.Contains(t, view, "Ollama")
	assert.Contains(t, view, "(local, no key)")
}

func TestSettingsOverlay_TabSwitchShowsModelPicker(t *testing.T) {
	s := newTestSettings(&fakeCredStore{})
	s.SetModelOptions([]tui.ModelOption{
		{Kind: "cloud", Provider: "anthropic", Model: "claude-opus-4-8", Name: "Claude Opus 4.8"},
	}, nil)

	// Providers tab first.
	assert.Contains(t, ansi.Strip(s.Render(80, 20)), "Anthropic")

	// Tab switches to the Model tab, which renders the embedded picker.
	s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	assert.Contains(t, ansi.Strip(s.Render(80, 20)), "Claude Opus 4.8")
}

func TestSettingsOverlay_EnterTypeSaveKey(t *testing.T) {
	store := &fakeCredStore{}
	s := newTestSettings(store)

	// Enter on the first provider (Anthropic) opens the masked input.
	s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.True(t, s.editing)

	typeString(s, "secret-key")
	// The raw key must never be shown (masked input).
	assert.NotContains(t, ansi.Strip(s.Render(80, 20)), "secret-key")

	// Enter saves and emits SettingsKeySavedMsg so the app reloads models.
	cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	_, ok := cmd().(tui.SettingsKeySavedMsg)
	assert.True(t, ok)

	assert.False(t, s.editing)
	assert.Equal(t, "secret-key", store.keys["anthropic"])
	// Status flips to set without a reload.
	assert.Contains(t, ansi.Strip(s.Render(80, 20)), "✓ key set")
}

func TestSettingsOverlay_EscCancelsEditingWithoutSaving(t *testing.T) {
	store := &fakeCredStore{}
	s := newTestSettings(store)

	s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	typeString(s, "abc")
	s.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	assert.False(t, s.editing)
	_, saved := store.keys["anthropic"]
	assert.False(t, saved, "esc during editing must not save")
	assert.True(t, s.IsOpen(), "esc during editing only cancels, does not close")
}

func TestSettingsOverlay_EscClosesWhenNotEditing(t *testing.T) {
	s := newTestSettings(&fakeCredStore{})
	s.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.False(t, s.IsOpen())
}

func TestSettingsOverlay_OllamaRowIsKeyless(t *testing.T) {
	s := newTestSettings(&fakeCredStore{})

	// Move the cursor to the last row (Ollama).
	for range len(s.providers) - 1 {
		s.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	// Enter on a keyless row must not open the input.
	s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.False(t, s.editing)
}

func TestSettingsOverlay_NilStoreReadsAsMissing(t *testing.T) {
	s := newTestSettings(nil)
	view := ansi.Strip(s.Render(80, 20))
	assert.Contains(t, view, "✗ missing")
	assert.NotContains(t, view, "✓ key set")
}

func TestSettingsOverlay_SaveErrorKeepsOverlayOpen(t *testing.T) {
	store := &fakeCredStore{setErr: errors.New("disk full")}
	s := newTestSettings(store)

	s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	typeString(s, "secret-key")
	cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	// On a save failure no reload message is emitted, the overlay stays open,
	// and an error line is shown.
	assert.Nil(t, cmd)
	assert.True(t, s.IsOpen())
	assert.False(t, s.editing)
	_, saved := store.keys["anthropic"]
	assert.False(t, saved)
	assert.Contains(t, ansi.Strip(s.Render(80, 20)), "Save failed")
}

func TestSettingsOverlay_AccessibleModeRendersTabsAndStatus(t *testing.T) {
	s := NewSettingsOverlay(
		tui.DefaultTheme(),
		tui.RenderConfig{Color: tui.ColorNone, Verbosity: tui.VerbosityAccessible},
		&fakeCredStore{keys: map[string]string{"anthropic": "sk-live"}},
		[]string{"anthropic", "openai", "kimi", "openrouter"},
	)
	s.SetSize(80, 20)
	s.Open()

	view := ansi.Strip(s.Render(80, 20))
	// Accessible tab strip is plain text with the active tab bracketed.
	assert.Contains(t, view, "Tabs:")
	assert.Contains(t, view, "[Providers & Keys]")
	assert.Contains(t, view, "✓ key set")
}
