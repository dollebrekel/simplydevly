package core

import "context"

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

// QueryRequest holds parameters for a provider query.
// Placeholder — detailed in Story 2.1.
type QueryRequest struct{}

// Provider defines the contract for AI provider adapters.
type Provider interface {
	Lifecycle
	Capabilities() ProviderCapabilities
	Query(ctx context.Context, req QueryRequest) (<-chan StreamEvent, error)
}
