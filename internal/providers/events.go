package providers

import (
	"encoding/json"

	"siply.dev/siply/internal/core"
)

// Compile-time checks that all event types satisfy core.StreamEvent.
var (
	_ core.StreamEvent = (*TextChunkEvent)(nil)
	_ core.StreamEvent = (*ToolCallEvent)(nil)
	_ core.StreamEvent = (*ThinkingEvent)(nil)
	_ core.StreamEvent = (*ErrorEvent)(nil)
	_ core.StreamEvent = (*DoneEvent)(nil)
	_ core.StreamEvent = (*UsageEvent)(nil)
)

// TextChunkEvent carries a chunk of streamed text content.
type TextChunkEvent struct {
	Text string
}

// ToolCallEvent carries a complete tool invocation.
type ToolCallEvent struct {
	ToolName string
	ToolID   string
	Input    json.RawMessage
}

// ThinkingEvent carries thinking/reasoning text from the model.
type ThinkingEvent struct {
	Thinking string
}

// ErrorEvent carries a provider error during streaming.
type ErrorEvent struct {
	Err error
}

func (e *ErrorEvent) Error() string {
	if e.Err == nil {
		return "unknown error"
	}
	return e.Err.Error()
}

func (e *ErrorEvent) Unwrap() error {
	return e.Err
}

// DoneEvent signals that the stream is complete.
type DoneEvent struct{}

// UsageEvent carries token usage information.
type UsageEvent struct {
	Usage core.TokenUsage
	Model string
}
