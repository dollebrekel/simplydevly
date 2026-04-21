// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siplyv1 "siply.dev/siply/api/proto/gen/siply/v1"
	"siply.dev/siply/pkg/siplyui"
)

// TestHandleRender_NoFileSelected verifies placeholder text when no file is set.
func TestHandleRender_NoFileSelected(t *testing.T) {
	p := &markdownPlugin{}
	resp, err := p.handleRender(nil)
	require.NoError(t, err)
	assert.True(t, resp.GetSuccess())
	assert.Equal(t, placeholder, string(resp.GetResult()))
}

// TestHandleRender_WithMarkdownFile verifies .md content is rendered via MarkdownView.
func TestHandleRender_WithMarkdownFile(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Hello\n\nWorld"), 0o644))

	p := &markdownPlugin{selectedFile: mdFile}
	// payload: width 80 encoded as 2 big-endian bytes
	resp, err := p.handleRender([]byte{0x00, 0x50})
	require.NoError(t, err)
	assert.True(t, resp.GetSuccess())
	assert.Contains(t, string(resp.GetResult()), "Hello")
}

// TestHandleRender_MissingFile verifies error message for unreadable file.
func TestHandleRender_MissingFile(t *testing.T) {
	p := &markdownPlugin{selectedFile: "/nonexistent/path/to/file.md"}
	resp, err := p.handleRender(nil)
	require.NoError(t, err)
	assert.True(t, resp.GetSuccess())
	assert.Contains(t, string(resp.GetResult()), "Error reading")
}

// TestHandleEvent_MdFileUpdatesSelected verifies .md file selection is stored.
func TestHandleEvent_MdFileUpdatesSelected(t *testing.T) {
	p := &markdownPlugin{}
	_, err := p.HandleEvent(t.Context(), &siplyv1.HandleEventRequest{
		EventType: "file.selected",
		Payload:   []byte("/home/user/doc.md"),
	})
	require.NoError(t, err)
	p.mu.RLock()
	defer p.mu.RUnlock()
	assert.Equal(t, "/home/user/doc.md", p.selectedFile)
}

// TestHandleEvent_NonMdFileIgnored verifies non-.md files don't update selectedFile.
func TestHandleEvent_NonMdFileIgnored(t *testing.T) {
	p := &markdownPlugin{}
	_, err := p.HandleEvent(t.Context(), &siplyv1.HandleEventRequest{
		EventType: "file.selected",
		Payload:   []byte("/home/user/main.go"),
	})
	require.NoError(t, err)
	p.mu.RLock()
	defer p.mu.RUnlock()
	assert.Empty(t, p.selectedFile)
}

// TestHandleEvent_NonMdFileClearsPrevious verifies selecting a non-.md file clears a prior .md selection.
func TestHandleEvent_NonMdFileClearsPrevious(t *testing.T) {
	p := &markdownPlugin{selectedFile: "/home/user/old.md"}
	_, err := p.HandleEvent(t.Context(), &siplyv1.HandleEventRequest{
		EventType: "file.selected",
		Payload:   []byte("/home/user/main.go"),
	})
	require.NoError(t, err)
	p.mu.RLock()
	defer p.mu.RUnlock()
	assert.Empty(t, p.selectedFile, "non-.md selection should clear previous .md file")
}

// TestHandleEvent_WrongEventType verifies non-file-selected events are ignored.
func TestHandleEvent_WrongEventType(t *testing.T) {
	p := &markdownPlugin{selectedFile: "/existing.md"}
	_, err := p.HandleEvent(t.Context(), &siplyv1.HandleEventRequest{
		EventType: "plugin.loaded",
		Payload:   []byte("/new.md"),
	})
	require.NoError(t, err)
	p.mu.RLock()
	defer p.mu.RUnlock()
	assert.Equal(t, "/existing.md", p.selectedFile, "unrelated events must not change selectedFile")
}

// TestMarkdownViewDirectly verifies siplyui.MarkdownView renders headings.
func TestMarkdownViewDirectly(t *testing.T) {
	mv := siplyui.NewMarkdownView(siplyui.DefaultTheme(), siplyui.DefaultRenderConfig())
	output := mv.Render("# Hello World", 80)
	assert.Contains(t, output, "Hello World")
}

// TestMarkdownViewEmpty verifies empty input returns empty output.
func TestMarkdownViewEmpty(t *testing.T) {
	mv := siplyui.NewMarkdownView(siplyui.DefaultTheme(), siplyui.DefaultRenderConfig())
	assert.Empty(t, mv.Render("", 80))
}

// TestHandleRender_WidthFromPayload verifies the payload width decoding.
func TestHandleRender_WidthFromPayload(t *testing.T) {
	p := &markdownPlugin{}
	// width 120 = 0x00 0x78
	resp, err := p.handleRender([]byte{0x00, 0x78})
	require.NoError(t, err)
	assert.True(t, resp.GetSuccess())
	// No file selected → placeholder
	assert.Equal(t, placeholder, string(resp.GetResult()))
}
