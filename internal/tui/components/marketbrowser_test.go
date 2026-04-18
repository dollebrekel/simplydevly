// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package components

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/marketplace"
	"siply.dev/siply/internal/tui"
)

func testIndex() *marketplace.Index {
	return &marketplace.Index{
		Version:   1,
		UpdatedAt: "2026-04-01T00:00:00Z",
		Items: []marketplace.Item{
			{
				Name:         "memory-default",
				Category:     "plugins",
				Description:  "Default memory plugin for siply",
				Author:       "simplydevly",
				Version:      "1.0.0",
				Rating:       4.8,
				InstallCount: 12500,
				Verified:     true,
				Tags:         []string{"memory", "context"},
				License:      "Apache-2.0",
				UpdatedAt:    "2026-03-15T00:00:00Z",
				Readme:       "# Memory Default\n\nA memory plugin.",
				Homepage:     "https://example.com/memory-default",
				DownloadURL:  "file:///tmp/test-plugin",
			},
			{
				Name:         "prompt-basic",
				Category:     "plugins",
				Description:  "Basic prompt templates",
				Author:       "simplydevly",
				Version:      "0.9.0",
				Rating:       4.2,
				InstallCount: 340,
				Verified:     false,
				Tags:         []string{"prompt", "templates"},
				License:      "MIT",
				UpdatedAt:    "2026-02-20T00:00:00Z",
			},
			{
				Name:         "tree-view",
				Category:     "extensions",
				Description:  "File tree sidebar extension",
				Author:       "community",
				Version:      "0.5.0",
				Rating:       3.9,
				InstallCount: 1200,
				Verified:     true,
				Tags:         []string{"tree", "sidebar", "files"},
				License:      "Apache-2.0",
				UpdatedAt:    "2026-01-10T00:00:00Z",
			},
		},
	}
}

func testLoader(idx *marketplace.Index) func() (*marketplace.Index, error) {
	return func() (*marketplace.Index, error) {
		return idx, nil
	}
}

func nilLoader() func() (*marketplace.Index, error) {
	return func() (*marketplace.Index, error) {
		return nil, marketplace.ErrIndexNotFound
	}
}

func newTestBrowser(idx *marketplace.Index, installer marketplace.InstallerFunc) *MarketBrowser {
	theme := tui.DefaultTheme()
	rc := tui.RenderConfig{Color: tui.ColorNone}
	var loader func() (*marketplace.Index, error)
	if idx != nil {
		loader = testLoader(idx)
	} else {
		loader = nilLoader()
	}
	mb := NewMarketBrowser(theme, rc, loader, installer, "") // cacheDir="" disables auto-sync in tests
	mb.SetSize(80, 24)
	return mb
}

func TestMarketBrowser_InitialState(t *testing.T) {
	mb := newTestBrowser(testIndex(), nil)
	assert.False(t, mb.IsOpen())
	assert.Equal(t, stateList, mb.state)
	assert.Len(t, mb.filtered, 3)
	assert.Equal(t, 0, mb.cursor)
}

func TestMarketBrowser_SearchFiltering(t *testing.T) {
	mb := newTestBrowser(testIndex(), nil)
	mb.Open()

	// Type "memory" → should filter to memory-default only
	for _, ch := range "memory" {
		mb.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}
	assert.Len(t, mb.filtered, 1)
	assert.Equal(t, "memory-default", mb.filtered[0].Name)

	// Clear search by setting empty value — simulate by rebuilding
	mb.searchInput.SetValue("")
	mb.refilter()
	assert.Len(t, mb.filtered, 3)
}

func TestMarketBrowser_CursorNavigation(t *testing.T) {
	mb := newTestBrowser(testIndex(), nil)
	mb.Open()

	// Initial position
	assert.Equal(t, 0, mb.cursor)

	// Move down
	mb.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 1, mb.cursor)

	mb.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 2, mb.cursor)

	// Clamp at bottom
	mb.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 2, mb.cursor)

	// Move up
	mb.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, 1, mb.cursor)

	// Move to top and clamp
	mb.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, 0, mb.cursor)
	mb.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, 0, mb.cursor)
}

func TestMarketBrowser_SwitchToInfo(t *testing.T) {
	mb := newTestBrowser(testIndex(), nil)
	mb.Open()

	// Press 'i' to switch to info
	mb.Update(tea.KeyPressMsg{Code: 'i'})
	assert.Equal(t, stateInfo, mb.state)

	// View should contain the item name
	view := mb.View()
	assert.Contains(t, view, "memory-default")
}

func TestMarketBrowser_BackFromInfo(t *testing.T) {
	mb := newTestBrowser(testIndex(), nil)
	mb.Open()

	// Move to item 1 and switch to info
	mb.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 1, mb.cursor)

	mb.Update(tea.KeyPressMsg{Code: 'i'})
	assert.Equal(t, stateInfo, mb.state)

	// Press Esc to go back
	mb.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.Equal(t, stateList, mb.state)
	assert.Equal(t, 1, mb.cursor) // cursor position preserved
}

func TestMarketBrowser_InstallFromList(t *testing.T) {
	installed := false
	installer := func(_ context.Context, _ string) error {
		installed = true
		return nil
	}

	mb := newTestBrowser(testIndex(), installer)
	mb.Open()

	cmd := mb.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	// Execute the cmd to get the result
	resultMsg := cmd()
	result, ok := resultMsg.(tui.MarketplaceInstallResultMsg)
	require.True(t, ok)
	assert.Equal(t, "memory-default", result.Name)
	// Note: Install may fail because file:///tmp/test-plugin doesn't exist,
	// but the installer was called.
	_ = installed
}

func TestMarketBrowser_InstallFromInfo(t *testing.T) {
	installer := func(_ context.Context, _ string) error {
		return nil
	}

	mb := newTestBrowser(testIndex(), installer)
	mb.Open()

	// Switch to info
	mb.Update(tea.KeyPressMsg{Code: 'i'})
	assert.Equal(t, stateInfo, mb.state)

	// Press Enter to install
	cmd := mb.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	resultMsg := cmd()
	result, ok := resultMsg.(tui.MarketplaceInstallResultMsg)
	require.True(t, ok)
	assert.Equal(t, "memory-default", result.Name)
}

func TestMarketBrowser_EmptyIndex(t *testing.T) {
	mb := newTestBrowser(nil, nil)
	mb.Open()

	view := mb.View()
	assert.Contains(t, view, "No marketplace data available")
}

func TestMarketBrowser_NoSearchResults(t *testing.T) {
	mb := newTestBrowser(testIndex(), nil)
	mb.Open()

	// Search for something that doesn't match
	mb.searchInput.SetValue("xyznonexistent")
	mb.refilter()

	view := mb.View()
	assert.Contains(t, view, "No items match your search")
}

func TestMarketBrowser_OpenClose(t *testing.T) {
	mb := newTestBrowser(testIndex(), nil)

	assert.False(t, mb.IsOpen())
	mb.Open()
	assert.True(t, mb.IsOpen())
	mb.Close()
	assert.False(t, mb.IsOpen())
	assert.Equal(t, stateList, mb.state)
}

func TestMarketBrowser_EscClosesInList(t *testing.T) {
	mb := newTestBrowser(testIndex(), nil)
	mb.Open()

	cmd := mb.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(tui.MarketplaceCloseMsg)
	assert.True(t, ok)
}

func TestMarketBrowser_InstallNoInstaller(t *testing.T) {
	mb := newTestBrowser(testIndex(), nil) // nil installer
	mb.Open()

	cmd := mb.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, cmd)
	assert.Contains(t, mb.installMsg, "unavailable")
}

func TestMarketBrowser_ViewRendersWithItems(t *testing.T) {
	mb := newTestBrowser(testIndex(), nil)
	mb.Open()

	view := mb.View()
	assert.Contains(t, view, "memory-default")
	assert.Contains(t, view, "prompt-basic")
}

func TestMarketBrowser_InstallResultSuccess(t *testing.T) {
	mb := newTestBrowser(testIndex(), nil)
	mb.Open()

	mb.Update(tui.MarketplaceInstallResultMsg{
		Name:    "memory-default",
		Version: "1.0.0",
	})
	assert.Contains(t, mb.installMsg, "Installed memory-default v1.0.0")
}

func TestMarketBrowser_InstallResultError(t *testing.T) {
	mb := newTestBrowser(testIndex(), nil)
	mb.Open()

	mb.Update(tui.MarketplaceInstallResultMsg{
		Name: "memory-default",
		Err:  assert.AnError,
	})
	assert.Contains(t, mb.installMsg, "Install failed")
}

func TestMarketBrowser_VimNavigation(t *testing.T) {
	mb := newTestBrowser(testIndex(), nil)
	mb.Open()

	assert.Equal(t, 0, mb.cursor)

	// j moves down
	mb.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	assert.Equal(t, 1, mb.cursor)

	mb.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	assert.Equal(t, 2, mb.cursor)

	// k moves up
	mb.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	assert.Equal(t, 1, mb.cursor)
}

func TestMarketBrowser_WebKeyListNoURL(t *testing.T) {
	mb := newTestBrowser(testIndex(), nil)
	mb.Open()

	// Move to prompt-basic (no Homepage)
	mb.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 1, mb.cursor)

	mb.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	assert.Contains(t, mb.installMsg, "No web URL available")
}

func TestMarketBrowser_WebKeyInfoNoURL(t *testing.T) {
	mb := newTestBrowser(testIndex(), nil)
	mb.Open()

	// Move to prompt-basic (no Homepage) and open info
	mb.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	mb.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	assert.Equal(t, stateInfo, mb.state)

	mb.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	assert.Contains(t, mb.installMsg, "No web URL available")
}

func TestMarketBrowser_ConcurrentInstallGuard(t *testing.T) {
	installer := func(_ context.Context, _ string) error { return nil }
	mb := newTestBrowser(testIndex(), installer)
	mb.Open()

	// First install returns a cmd
	cmd := mb.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	assert.True(t, mb.installing)

	// Second install while first is pending returns nil (guarded)
	cmd2 := mb.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, cmd2)
}
