package core

import "context"

// Message represents a conversation message.
type Message struct {
	Role    string
	Content string
}

// ContextManager handles conversation context compaction.
type ContextManager interface {
	Lifecycle
	ShouldCompact(messages []Message, limit int) bool
	Compact(ctx context.Context, messages []Message) ([]Message, error)
}
