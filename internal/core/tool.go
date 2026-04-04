package core

import (
	"context"
	"encoding/json"
	"time"
)

// ToolRequest describes a tool invocation request.
type ToolRequest struct {
	Name   string          // tool name (e.g., "file_read")
	Input  json.RawMessage // JSON-encoded tool-specific parameters
	Source string          // who invoked: "agent", "plugin:xxx", "user"
}

// ToolResponse holds the result of a tool execution.
type ToolResponse struct {
	Output   string        // tool output text
	IsError  bool          // true when the tool reports a failure
	Duration time.Duration // wall-clock execution time
}

// ToolExecutor manages tool execution.
type ToolExecutor interface {
	Lifecycle
	Execute(ctx context.Context, req ToolRequest) (ToolResponse, error)
	ListTools() []ToolDefinition
	GetTool(name string) (ToolDefinition, error)
}
