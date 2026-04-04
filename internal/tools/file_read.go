package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"siply.dev/siply/internal/core"
)

const maxFileSize = 10 * 1024 * 1024 // 10 MB

// FileReadTool reads file contents.
type FileReadTool struct{}

type fileReadInput struct {
	Path   string `json:"path"`
	Offset *int   `json:"offset,omitempty"`
	Limit  *int   `json:"limit,omitempty"`
}

func (t *FileReadTool) Name() string        { return "file_read" }
func (t *FileReadTool) Description() string { return "Read file contents" }
func (t *FileReadTool) Destructive() bool   { return false }
func (t *FileReadTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Absolute or relative file path"},"offset":{"type":"integer","description":"Line number to start reading (0-based)"},"limit":{"type":"integer","description":"Max lines to read"}},"required":["path"]}`)
}

func (t *FileReadTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var params fileReadInput
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("file_read: invalid input: %w", err)
	}
	if params.Path == "" {
		return "", fmt.Errorf("file_read: path is required")
	}

	info, err := os.Stat(params.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("file_read: %w: %s", core.ErrNotFound, params.Path)
		}
		return "", fmt.Errorf("file_read: %w", err)
	}
	if info.Size() > maxFileSize {
		return "", fmt.Errorf("file_read: file too large (%d bytes, max %d)", info.Size(), maxFileSize)
	}

	if params.Offset == nil && params.Limit == nil {
		data, err := os.ReadFile(params.Path)
		if err != nil {
			return "", fmt.Errorf("file_read: %w", err)
		}
		return string(data), nil
	}

	f, err := os.Open(params.Path)
	if err != nil {
		return "", fmt.Errorf("file_read: %w", err)
	}
	defer f.Close()

	offset := 0
	if params.Offset != nil {
		offset = *params.Offset
	}
	limit := -1
	if params.Limit != nil {
		limit = *params.Limit
	}

	scanner := bufio.NewScanner(f)
	var result []byte
	lineNum := 0
	collected := 0
	for scanner.Scan() {
		if lineNum >= offset {
			if limit >= 0 && collected >= limit {
				break
			}
			if collected > 0 {
				result = append(result, '\n')
			}
			result = append(result, scanner.Bytes()...)
			collected++
		}
		lineNum++
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("file_read: %w", err)
	}

	return string(result), nil
}
