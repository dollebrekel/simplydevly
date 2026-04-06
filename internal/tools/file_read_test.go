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

	"siply.dev/siply/internal/core"
)

func TestFileRead_BasicRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	require.NoError(t, os.WriteFile(path, []byte("hello world"), 0644))

	tool := &FileReadTool{}
	input, _ := json.Marshal(fileReadInput{Path: path})

	output, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "hello world", output)
}

func TestFileRead_FileNotFound(t *testing.T) {
	tool := &FileReadTool{}
	input, _ := json.Marshal(fileReadInput{Path: "/nonexistent/file.txt"})

	_, err := tool.Execute(context.Background(), input)
	require.Error(t, err)
	assert.ErrorIs(t, err, core.ErrNotFound)
}

func TestFileRead_OffsetAndLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lines.txt")
	content := "line0\nline1\nline2\nline3\nline4"
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	tool := &FileReadTool{}

	offset := 1
	limit := 2
	input, _ := json.Marshal(fileReadInput{Path: path, Offset: &offset, Limit: &limit})

	output, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "line1\nline2", output)
}

func TestFileRead_FileTooLarge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.bin")

	// Create a file > 10MB using sparse file trick.
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(maxFileSize+1))
	f.Close()

	tool := &FileReadTool{}
	input, _ := json.Marshal(fileReadInput{Path: path})

	_, err = tool.Execute(context.Background(), input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too large")
}

func TestFileRead_EmptyPath(t *testing.T) {
	tool := &FileReadTool{}
	input, _ := json.Marshal(fileReadInput{Path: ""})

	_, err := tool.Execute(context.Background(), input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path is required")
}

func TestFileRead_Properties(t *testing.T) {
	tool := &FileReadTool{}
	assert.Equal(t, "file_read", tool.Name())
	assert.False(t, tool.Destructive())
	assert.True(t, strings.Contains(string(tool.InputSchema()), "path"))
}
