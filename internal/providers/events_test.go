package providers

import (
	"encoding/json"
	"errors"
	"testing"

	"siply.dev/siply/internal/core"
)

func TestTextChunkEventSatisfiesStreamEvent(t *testing.T) {
	var e core.StreamEvent = &TextChunkEvent{Text: "hello"}
	tc, ok := e.(*TextChunkEvent)
	if !ok {
		t.Fatal("TextChunkEvent should satisfy StreamEvent")
	}
	if tc.Text != "hello" {
		t.Fatalf("expected text 'hello', got %q", tc.Text)
	}
}

func TestToolCallEventSatisfiesStreamEvent(t *testing.T) {
	input := json.RawMessage(`{"path":"/tmp"}`)
	var e core.StreamEvent = &ToolCallEvent{
		ToolName: "file_read",
		ToolID:   "tool_123",
		Input:    input,
	}
	tc := e.(*ToolCallEvent)
	if tc.ToolName != "file_read" {
		t.Fatalf("expected tool name 'file_read', got %q", tc.ToolName)
	}
	if tc.ToolID != "tool_123" {
		t.Fatalf("expected tool ID 'tool_123', got %q", tc.ToolID)
	}
	if string(tc.Input) != `{"path":"/tmp"}` {
		t.Fatalf("unexpected input: %s", tc.Input)
	}
}

func TestThinkingEventSatisfiesStreamEvent(t *testing.T) {
	var e core.StreamEvent = &ThinkingEvent{Thinking: "let me think..."}
	tc := e.(*ThinkingEvent)
	if tc.Thinking != "let me think..." {
		t.Fatalf("expected thinking text, got %q", tc.Thinking)
	}
}

func TestErrorEventSatisfiesStreamEvent(t *testing.T) {
	var e core.StreamEvent = &ErrorEvent{Err: errors.New("something failed")}
	ee := e.(*ErrorEvent)
	if ee.Error() != "something failed" {
		t.Fatalf("expected error message, got %q", ee.Error())
	}
}

func TestErrorEventNilErr(t *testing.T) {
	ee := &ErrorEvent{}
	if ee.Error() != "unknown error" {
		t.Fatalf("expected 'unknown error' for nil Err, got %q", ee.Error())
	}
}

func TestErrorEventUnwrap(t *testing.T) {
	sentinel := errors.New("root cause")
	ee := &ErrorEvent{Err: sentinel}
	if !errors.Is(ee, sentinel) {
		t.Fatal("errors.Is should traverse ErrorEvent via Unwrap")
	}
}

func TestDoneEventSatisfiesStreamEvent(t *testing.T) {
	var e core.StreamEvent = &DoneEvent{}
	_, ok := e.(*DoneEvent)
	if !ok {
		t.Fatal("DoneEvent should satisfy StreamEvent")
	}
}

func TestUsageEventSatisfiesStreamEvent(t *testing.T) {
	var e core.StreamEvent = &UsageEvent{
		Usage: core.TokenUsage{InputTokens: 100, OutputTokens: 50},
		Model: "claude-sonnet-4-20250514",
	}
	ue := e.(*UsageEvent)
	if ue.Usage.InputTokens != 100 {
		t.Fatalf("expected 100 input tokens, got %d", ue.Usage.InputTokens)
	}
	if ue.Usage.OutputTokens != 50 {
		t.Fatalf("expected 50 output tokens, got %d", ue.Usage.OutputTokens)
	}
	if ue.Model != "claude-sonnet-4-20250514" {
		t.Fatalf("expected model name, got %q", ue.Model)
	}
}
