package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileWrite_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.txt")

	tool := &FileWriteTool{}
	input, _ := json.Marshal(fileWriteInput{Path: path, Content: "hello"})

	output, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, output, "5 bytes")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))
}

func TestFileWrite_OverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exist.txt")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0644))

	tool := &FileWriteTool{}
	input, _ := json.Marshal(fileWriteInput{Path: path, Content: "new"})

	_, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "new", string(data))
}

func TestFileWrite_CreateParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "dir", "file.txt")

	tool := &FileWriteTool{}
	input, _ := json.Marshal(fileWriteInput{Path: path, Content: "nested"})

	_, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "nested", string(data))
}

func TestFileWrite_EmptyPath(t *testing.T) {
	tool := &FileWriteTool{}
	input, _ := json.Marshal(fileWriteInput{Path: "", Content: "x"})

	_, err := tool.Execute(context.Background(), input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path is required")
}

func TestFileWrite_Properties(t *testing.T) {
	tool := &FileWriteTool{}
	assert.Equal(t, "file_write", tool.Name())
	assert.True(t, tool.Destructive())
}
