package agent

import "siply.dev/siply/internal/core"

const (
	defaultSystemPrompt = "You are a coding assistant. You have access to tools for file operations, code search, bash commands, and web fetching. Use them to help the developer."
	defaultMaxTokens    = 4096
)

// buildQueryRequest assembles a QueryRequest from conversation state.
func buildQueryRequest(messages []core.Message, systemPrompt string, tools []core.ToolDefinition) core.QueryRequest {
	if systemPrompt == "" {
		systemPrompt = defaultSystemPrompt
	}

	maxTokens := defaultMaxTokens

	return core.QueryRequest{
		Messages:     messages,
		SystemPrompt: systemPrompt,
		Tools:        tools,
		MaxTokens:    maxTokens,
	}
}
