// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearch_BasicPattern(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.go"), []byte("func main() {\n\tfmt.Println(\"hello\")\n}"), 0644))
	orig, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(orig) }()

	tool := &SearchTool{}
	input, _ := json.Marshal(searchInput{Pattern: "Println"})

	output, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, output, "Println")
}

func TestSearch_NoMatches(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.txt"), []byte("nothing here"), 0644))
	orig, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(orig) }()

	tool := &SearchTool{}
	input, _ := json.Marshal(searchInput{Pattern: "zzzznotfound"})

	output, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "No matches found", output)
}

func TestSearch_WithIncludeGlob(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.go"), []byte("match"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("match"), 0644))
	orig, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(orig) }()

	tool := &SearchTool{}
	input, _ := json.Marshal(searchInput{Pattern: "match", Include: "*.go"})

	output, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, output, "file.go")
	assert.NotContains(t, output, "file.txt")
}

func TestSearch_EmptyPattern(t *testing.T) {
	tool := &SearchTool{}
	input, _ := json.Marshal(searchInput{Pattern: ""})

	_, err := tool.Execute(context.Background(), input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pattern is required")
}

func TestSearch_Properties(t *testing.T) {
	tool := &SearchTool{}
	assert.Equal(t, "search", tool.Name())
	assert.False(t, tool.Destructive())
}

func TestSearch_Truncation(t *testing.T) {
	dir := t.TempDir()

	// Create a file with many matching lines.
	var content string
	for i := 0; i < 150; i++ {
		content += "matchline\n"
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "many.txt"), []byte(content), 0644))
	orig, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(orig) }()

	tool := &SearchTool{}
	input, _ := json.Marshal(searchInput{Pattern: "matchline"})

	output, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.NotEmpty(t, output)
	// Verify output is bounded: either truncated indicator or line count <= maxSearchResults.
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	assert.LessOrEqual(t, len(lines), maxSearchResults+1) // +1 for possible truncation message
}

func TestSearch_BlocksParentEscape(t *testing.T) {
	ws := t.TempDir()
	orig, _ := os.Getwd()
	require.NoError(t, os.Chdir(ws))
	defer func() { _ = os.Chdir(orig) }()

	tool := &SearchTool{}
	input, _ := json.Marshal(searchInput{Pattern: "x", Path: ".."})
	_, err := tool.Execute(context.Background(), input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "escapes workspace root")
}

func TestSearch_BlocksSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "inside")
	outside := t.TempDir()
	link := filepath.Join(inside, "escape")
	require.NoError(t, os.MkdirAll(inside, 0o755))
	require.NoError(t, os.Symlink(outside, link))

	orig, _ := os.Getwd()
	require.NoError(t, os.Chdir(inside))
	defer func() { _ = os.Chdir(orig) }()

	tool := &SearchTool{}
	input, _ := json.Marshal(searchInput{Pattern: "x", Path: "escape"})
	_, err := tool.Execute(context.Background(), input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "escapes workspace root")
}

func TestSearch_ExplicitFallbackOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "inside")
	outside := filepath.Join(root, "outside")
	require.NoError(t, os.MkdirAll(inside, 0o755))
	require.NoError(t, os.MkdirAll(outside, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "hit.txt"), []byte("needle"), 0o644))

	orig, _ := os.Getwd()
	require.NoError(t, os.Chdir(inside))
	defer func() { _ = os.Chdir(orig) }()

	tool := &SearchTool{}
	input, _ := json.Marshal(searchInput{
		Pattern:               "needle",
		Path:                  inside,
		AllowOutsideWorkspace: true,
		FallbackPath:          outside,
	})
	out, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, out, "[expanded scope]")
	assert.Contains(t, out, "hit.txt")
}

func TestSearch_FallbackErrorIsReturned(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "inside")
	require.NoError(t, os.MkdirAll(inside, 0o755))

	orig, _ := os.Getwd()
	require.NoError(t, os.Chdir(inside))
	defer func() { _ = os.Chdir(orig) }()

	tool := &SearchTool{}
	input, _ := json.Marshal(searchInput{
		Pattern:               "needle",
		AllowOutsideWorkspace: true,
		FallbackPath:          filepath.Join(root, "does-not-exist"),
	})
	_, err := tool.Execute(context.Background(), input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expanded scope fallback failed")
}
