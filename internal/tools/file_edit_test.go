// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/core"
)

func TestFileEdit_SingleMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	require.NoError(t, os.WriteFile(path, []byte("hello world"), 0644))

	tool := &FileEditTool{}
	input, _ := json.Marshal(fileEditInput{Path: path, OldString: "world", NewString: "Go"})

	output, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, output, "Edited")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "hello Go", string(data))
}

func TestFileEdit_ZeroMatches(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0644))

	tool := &FileEditTool{}
	input, _ := json.Marshal(fileEditInput{Path: path, OldString: "xyz", NewString: "abc"})

	_, err := tool.Execute(context.Background(), input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestFileEdit_MultipleMatches(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	require.NoError(t, os.WriteFile(path, []byte("aaa"), 0644))

	tool := &FileEditTool{}
	input, _ := json.Marshal(fileEditInput{Path: path, OldString: "a", NewString: "b"})

	_, err := tool.Execute(context.Background(), input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "3 times")
}

func TestFileEdit_FileNotFound(t *testing.T) {
	tool := &FileEditTool{}
	input, _ := json.Marshal(fileEditInput{Path: "/nonexistent.txt", OldString: "x", NewString: "y"})

	_, err := tool.Execute(context.Background(), input)
	require.Error(t, err)
	assert.ErrorIs(t, err, core.ErrNotFound)
}

func TestFileEdit_Properties(t *testing.T) {
	tool := &FileEditTool{}
	assert.Equal(t, "file_edit", tool.Name())
	assert.True(t, tool.Destructive())
}
