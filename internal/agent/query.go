// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package agent

import (
	"time"

	"siply.dev/siply/internal/core"
)

const (
	defaultSystemPrompt = "You are a coding assistant. You have access to tools for file operations, code search, bash commands, and web fetching. Use them to help the developer."
	defaultMaxTokens    = 4096
)

// buildQueryRequest assembles a QueryRequest from conversation state.
// taskStartTime should be frozen at the start of a user turn so all provider
// calls within that turn share the same timestamp (for prefix-cache stability).
// hints is optional metadata the router can inspect for routing decisions.
func buildQueryRequest(messages []core.Message, systemPrompt string, tools []core.ToolDefinition, hints map[string]string, taskStartTime time.Time) core.QueryRequest {
	if systemPrompt == "" {
		systemPrompt = defaultSystemPrompt
	}

	maxTokens := defaultMaxTokens

	return core.QueryRequest{
		Messages:      messages,
		SystemPrompt:  systemPrompt,
		Tools:         tools,
		MaxTokens:     maxTokens,
		Hints:         hints,
		TaskStartTime: taskStartTime,
	}
}
