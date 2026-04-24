// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package panels

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── newPanelViewport ───────────────────────────────────────────────────────

func TestPanelViewport_New_IsDirty(t *testing.T) {
	vp := newPanelViewport(40, 10, "test-plugin")
	require.NotNil(t, vp)
	assert.True(t, vp.IsDirty(), "new viewport should start dirty")
}

// ─── SetContent / dirty flag ────────────────────────────────────────────────

func TestPanelViewport_SetContent_SetsDirty(t *testing.T) {
	vp := newPanelViewport(40, 10, "test-plugin")
	vp.MarkClean()
	assert.False(t, vp.IsDirty())

	vp.SetContent("hello world")
	assert.True(t, vp.IsDirty())
	assert.Equal(t, "hello world", vp.Content())
}

func TestPanelViewport_SetContent_SameContent_NoDirty(t *testing.T) {
	vp := newPanelViewport(40, 10, "test-plugin")
	vp.SetContent("hello")
	vp.MarkClean()
	assert.False(t, vp.IsDirty())

	// Same content should not set dirty.
	vp.SetContent("hello")
	assert.False(t, vp.IsDirty())
}

func TestPanelViewport_SetContent_DifferentContent_SetsDirty(t *testing.T) {
	vp := newPanelViewport(40, 10, "test-plugin")
	vp.SetContent("hello")
	vp.MarkClean()

	vp.SetContent("world")
	assert.True(t, vp.IsDirty())
	assert.Equal(t, "world", vp.Content())
}

func TestPanelViewport_SetContentLines(t *testing.T) {
	vp := newPanelViewport(40, 10, "test-plugin")
	vp.SetContentLines([]string{"line1", "line2", "line3"})
	assert.Equal(t, "line1\nline2\nline3", vp.Content())
	assert.True(t, vp.IsDirty())
}

// ─── MarkClean ──────────────────────────────────────────────────────────────

func TestPanelViewport_MarkClean(t *testing.T) {
	vp := newPanelViewport(40, 10, "test-plugin")
	assert.True(t, vp.IsDirty())

	vp.MarkClean()
	assert.False(t, vp.IsDirty())
}

// ─── Error state ────────────────────────────────────────────────────────────

func TestPanelViewport_SetError(t *testing.T) {
	vp := newPanelViewport(40, 10, "my-plugin")
	vp.SetContent("normal content")
	vp.MarkClean()

	vp.SetError(errors.New("connection lost"))
	assert.True(t, vp.HasError())
	assert.True(t, vp.IsDirty())
	assert.Contains(t, vp.ErrorMsg(), "my-plugin crashed")
	assert.Contains(t, vp.ErrorMsg(), "connection lost")
}

func TestPanelViewport_SetErrorMsg(t *testing.T) {
	vp := newPanelViewport(40, 10, "test-plugin")
	vp.SetErrorMsg("[test-plugin crashed]")
	assert.True(t, vp.HasError())
	assert.Equal(t, "[test-plugin crashed]", vp.ErrorMsg())
}

func TestPanelViewport_ClearError(t *testing.T) {
	vp := newPanelViewport(40, 10, "my-plugin")
	vp.SetContent("original content")
	vp.MarkClean()

	vp.SetError(errors.New("boom"))
	assert.True(t, vp.HasError())

	vp.ClearError()
	assert.False(t, vp.HasError())
	assert.True(t, vp.IsDirty())
	// Content should be restored to the non-error value.
	assert.Equal(t, "original content", vp.Content())
}

func TestPanelViewport_ClearError_NoOp_WhenNoError(t *testing.T) {
	vp := newPanelViewport(40, 10, "test-plugin")
	vp.SetContent("hello")
	vp.MarkClean()

	vp.ClearError()
	assert.False(t, vp.IsDirty(), "ClearError should not set dirty when no error")
}

func TestPanelViewport_SetContent_ClearsError(t *testing.T) {
	vp := newPanelViewport(40, 10, "my-plugin")
	vp.SetError(errors.New("old error"))
	assert.True(t, vp.HasError())

	vp.SetContent("new content")
	assert.False(t, vp.HasError())
	assert.Equal(t, "new content", vp.Content())
}

func TestPanelViewport_SetError_EmptyPluginName(t *testing.T) {
	vp := newPanelViewport(40, 10, "")
	vp.SetError(errors.New("fail"))
	assert.Contains(t, vp.ErrorMsg(), "unknown crashed")
}

// ─── View ───────────────────────────────────────────────────────────────────

func TestPanelViewport_View_ReturnsString(t *testing.T) {
	vp := newPanelViewport(40, 5, "test-plugin")
	vp.SetContent("line 1\nline 2\nline 3")
	view := vp.View()
	assert.NotEmpty(t, view)
}

// ─── SetSize ────────────────────────────────────────────────────────────────

func TestPanelViewport_SetSize_SetsDirty(t *testing.T) {
	vp := newPanelViewport(40, 10, "test-plugin")
	vp.MarkClean()

	vp.SetSize(60, 20)
	assert.True(t, vp.IsDirty())
}
