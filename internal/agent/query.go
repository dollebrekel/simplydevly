// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package agent

import "siply.dev/siply/internal/core"

const (
	defaultSystemPrompt = "You are a coding assistant. You have access to tools for file operations, code search, bash commands, and web fetching. Use them to help the developer."
	defaultMaxTokens    = 4096
)

// buildQueryRequest assembles a QueryRequest from conversation state.
// hints is optional metadata the router can inspect for routing decisions.
func buildQueryRequest(messages []core.Message, systemPrompt string, tools []core.ToolDefinition, hints map[string]string) core.QueryRequest {
	if systemPrompt == "" {
		systemPrompt = defaultSystemPrompt
	}

	maxTokens := defaultMaxTokens

	return core.QueryRequest{
		Messages:     messages,
		SystemPrompt: systemPrompt,
		Tools:        tools,
		MaxTokens:    maxTokens,
		Hints:        hints,
	}
}
