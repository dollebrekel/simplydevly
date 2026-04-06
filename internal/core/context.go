// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package core

import "context"

// Message represents a conversation message.
type Message struct {
	Role        string       // "user", "assistant", "system"; tool results use role "user" with ToolResults populated
	Content     string       // text content
	ToolID      string       // correlation ID for tool results
	ToolCalls   []ToolCall   // tool calls requested by assistant
	ToolResults []ToolResult // tool results attached to this message
}

// ToolCall represents a tool invocation requested by the assistant.
type ToolCall struct {
	ToolID   string
	ToolName string
	Input    []byte // JSON-encoded parameters
}

// ToolResult holds the result of a single tool execution.
type ToolResult struct {
	ToolID  string
	Content string
	IsError bool
}

// ContextManager handles conversation context compaction.
type ContextManager interface {
	Lifecycle
	ShouldCompact(messages []Message, limit int) bool
	Compact(ctx context.Context, messages []Message) ([]Message, error)
}
