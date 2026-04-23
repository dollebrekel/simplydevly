// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

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
//
// Execute returns (response, error) where:
//   - error is non-nil for executor/transport-level failures (tool not found, permission denied, marshaling errors).
//     The caller should treat these as infrastructure failures.
//   - ToolResponse.IsError indicates tool-level (domain) failure — the tool ran but reported an error.
//     The output still contains useful information (e.g., error message, partial output).
//   - Both can be set simultaneously: error wraps the tool error for programmatic handling,
//     while ToolResponse.Output preserves the tool's output text for the agent.
type ToolExecutor interface {
	Lifecycle
	Execute(ctx context.Context, req ToolRequest) (ToolResponse, error)
	ListTools() []ToolDefinition
	GetTool(name string) (ToolDefinition, error)
}
