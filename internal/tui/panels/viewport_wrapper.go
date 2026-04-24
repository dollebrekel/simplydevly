// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

// viewport_wrapper.go — Per-panel viewport met dirty-flag tracking en fout-afhandeling.
//
// Elke panelViewport omvat een bubbles/v2 viewport.Model die de inhoud clipt
// tot de toegewezen paneel-afmetingen en scroll-invoer (Up/Down/PgUp/PgDown)
// ondersteunt. De dirty-vlag voorkomt onnodige re-renders: alleen panelen
// waarvan de inhoud is gewijzigd worden opnieuw getekend.

package panels

import (
	"fmt"
	"strings"
	"sync"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

// panelViewport wraps a viewport.Model with dirty tracking and error state.
type panelViewport struct {
	mu sync.Mutex

	vp viewport.Model

	// content stores the last-set full content string for diffing.
	content string

	// dirty indicates the content has changed since the last render.
	dirty bool

	// errorMsg holds an error overlay message (e.g. "[plugin-name crashed]").
	// When non-empty, the viewport shows this instead of regular content.
	errorMsg string

	// pluginName identifies the owning plugin for error messages.
	pluginName string
}

// newPanelViewport creates a panelViewport for the given dimensions.
func newPanelViewport(width, height int, pluginName string) *panelViewport {
	vp := viewport.New(viewport.WithWidth(width), viewport.WithHeight(height))
	return &panelViewport{
		vp:         vp,
		pluginName: pluginName,
		dirty:      true, // eerste render is altijd nodig
	}
}

// SetContent updates the viewport content. If the new content differs from the
// previous content, the dirty flag is set. Thread-safe.
func (pv *panelViewport) SetContent(content string) {
	pv.mu.Lock()
	defer pv.mu.Unlock()

	// Reset fout-status wanneer nieuwe inhoud binnenkomt.
	pv.errorMsg = ""

	if content == pv.content {
		return
	}
	pv.content = content
	pv.vp.SetContent(content)
	pv.dirty = true
}

// SetContentLines sets content from a slice of lines. Convenience wrapper
// around SetContent for use with gRPC PanelContentUpdate.lines.
func (pv *panelViewport) SetContentLines(lines []string) {
	pv.SetContent(strings.Join(lines, "\n"))
}

// SetError puts the viewport into an error state with a message like
// "[plugin-name crashed]". The dirty flag is set. Thread-safe.
func (pv *panelViewport) SetError(err error) {
	pv.mu.Lock()
	defer pv.mu.Unlock()

	name := pv.pluginName
	if name == "" {
		name = "unknown"
	}
	pv.errorMsg = fmt.Sprintf("[%s crashed: %v]", name, err)
	pv.vp.SetContent(pv.errorMsg)
	pv.dirty = true
}

// SetErrorMsg puts the viewport into an error state with a pre-formatted string.
func (pv *panelViewport) SetErrorMsg(msg string) {
	pv.mu.Lock()
	defer pv.mu.Unlock()

	pv.errorMsg = msg
	pv.vp.SetContent(msg)
	pv.dirty = true
}

// ClearError removes the error state and restores previous content.
func (pv *panelViewport) ClearError() {
	pv.mu.Lock()
	defer pv.mu.Unlock()

	if pv.errorMsg == "" {
		return
	}
	pv.errorMsg = ""
	pv.vp.SetContent(pv.content)
	pv.dirty = true
}

// IsDirty returns whether content has changed since the last MarkClean call.
func (pv *panelViewport) IsDirty() bool {
	pv.mu.Lock()
	defer pv.mu.Unlock()
	return pv.dirty
}

// MarkClean clears the dirty flag after a successful render.
func (pv *panelViewport) MarkClean() {
	pv.mu.Lock()
	defer pv.mu.Unlock()
	pv.dirty = false
}

// HasError returns whether the viewport is in an error state.
func (pv *panelViewport) HasError() bool {
	pv.mu.Lock()
	defer pv.mu.Unlock()
	return pv.errorMsg != ""
}

// ErrorMsg returns the current error message, or empty if no error.
func (pv *panelViewport) ErrorMsg() string {
	pv.mu.Lock()
	defer pv.mu.Unlock()
	return pv.errorMsg
}

// View returns the rendered viewport string.
func (pv *panelViewport) View() string {
	pv.mu.Lock()
	defer pv.mu.Unlock()
	return pv.vp.View()
}

// Update delegates a tea.Msg to the inner viewport (scroll handling).
func (pv *panelViewport) Update(msg tea.Msg) tea.Cmd {
	pv.mu.Lock()
	defer pv.mu.Unlock()
	var cmd tea.Cmd
	pv.vp, cmd = pv.vp.Update(msg)
	return cmd
}

// SetSize resizes the viewport to new dimensions. Sets dirty.
func (pv *panelViewport) SetSize(width, height int) {
	pv.mu.Lock()
	defer pv.mu.Unlock()
	pv.vp.SetWidth(width)
	pv.vp.SetHeight(height)
	pv.dirty = true
}

// Content returns the current (non-error) content string.
func (pv *panelViewport) Content() string {
	pv.mu.Lock()
	defer pv.mu.Unlock()
	return pv.content
}
