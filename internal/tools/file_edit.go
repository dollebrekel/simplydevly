package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"siply.dev/siply/internal/core"
)

// FileEditTool performs string replacement edits on files.
type FileEditTool struct{}

type fileEditInput struct {
	Path      string `json:"path"`
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

func (t *FileEditTool) Name() string        { return "file_edit" }
func (t *FileEditTool) Description() string { return "Edit a file by replacing text" }
func (t *FileEditTool) Destructive() bool   { return true }
func (t *FileEditTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"File path to edit"},"old_string":{"type":"string","description":"Exact text to find"},"new_string":{"type":"string","description":"Replacement text"}},"required":["path","old_string","new_string"]}`)
}

func (t *FileEditTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var params fileEditInput
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("file_edit: invalid input: %w", err)
	}
	if params.Path == "" {
		return "", fmt.Errorf("file_edit: path is required")
	}
	if params.OldString == "" {
		return "", fmt.Errorf("file_edit: old_string must not be empty")
	}

	data, err := os.ReadFile(params.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("file_edit: %w: %s", core.ErrNotFound, params.Path)
		}
		return "", fmt.Errorf("file_edit: %w", err)
	}

	content := string(data)
	count := strings.Count(content, params.OldString)

	switch count {
	case 0:
		return "", fmt.Errorf("file_edit: old_string not found in %s", params.Path)
	case 1:
		// exactly one match — good
	default:
		return "", fmt.Errorf("file_edit: old_string found %d times in %s (expected exactly 1)", count, params.Path)
	}

	newContent := strings.Replace(content, params.OldString, params.NewString, 1)

	// Preserve existing file permissions.
	info, statErr := os.Stat(params.Path)
	perm := os.FileMode(0644)
	if statErr == nil {
		perm = info.Mode().Perm()
	}
	if err := os.WriteFile(params.Path, []byte(newContent), perm); err != nil {
		return "", fmt.Errorf("file_edit: %w", err)
	}

	oldLines := strings.Count(params.OldString, "\n") + 1
	newLines := strings.Count(params.NewString, "\n") + 1
	return fmt.Sprintf("Edited %s: replaced %d lines with %d lines", params.Path, oldLines, newLines), nil
}
