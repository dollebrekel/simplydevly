package core

import (
	"context"
	"encoding/json"
)

// ProviderCapabilities describes what an AI provider supports.
type ProviderCapabilities struct {
	SupportsToolCalls    bool
	SupportsThinking     bool
	SupportsStreaming    bool
	SupportsSystemPrompt bool
	SupportsVision       bool
	MaxContextTokens     int
}

// StreamEvent is a marker interface for streaming response events.
// Concrete types are defined in provider packages.
type StreamEvent interface{}

// ToolDefinition describes a tool that the AI model can invoke.
// InputSchema is optional JSON Schema for tool inputs; adapters may validate it.
type ToolDefinition struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

// QueryRequest holds parameters for a provider query.
// Zero-value semantics: Model="" means "use provider default", MaxTokens=0
// means "use provider default" (must be non-negative), Temperature=nil means
// "use provider default" (valid range [0.0, 2.0] when set), Tools=nil means
// no tools. Adapters MAY ignore unsupported fields but must not error on them.
type QueryRequest struct {
	Messages     []Message
	SystemPrompt string
	Tools        []ToolDefinition
	MaxTokens    int
	Model        string
	Temperature  *float64
}

// Provider defines the contract for AI provider adapters.
type Provider interface {
	Lifecycle
	Capabilities() ProviderCapabilities
	Query(ctx context.Context, req QueryRequest) (<-chan StreamEvent, error)
}
