package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// FileWriteTool creates or overwrites files.
type FileWriteTool struct{}

type fileWriteInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (t *FileWriteTool) Name() string        { return "file_write" }
func (t *FileWriteTool) Description() string { return "Create or overwrite a file" }
func (t *FileWriteTool) Destructive() bool   { return true }
func (t *FileWriteTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"File path to write"},"content":{"type":"string","description":"Content to write"}},"required":["path","content"]}`)
}

func (t *FileWriteTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var params fileWriteInput
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("file_write: invalid input: %w", err)
	}
	if params.Path == "" {
		return "", fmt.Errorf("file_write: path is required")
	}

	dir := filepath.Dir(params.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("file_write: creating directories: %w", err)
	}

	data := []byte(params.Content)
	if err := os.WriteFile(params.Path, data, 0644); err != nil {
		return "", fmt.Errorf("file_write: %w", err)
	}

	return fmt.Sprintf("Wrote %d bytes to %s", len(data), params.Path), nil
}
