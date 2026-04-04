package openai

import (
	"io"
	"strings"
	"testing"

	"siply.dev/siply/internal/providers"
)

func TestStreamParserTextDelta(t *testing.T) {
	sse := `data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}

data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}

data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}

data: [DONE]

`
	parser := newStreamParser(strings.NewReader(sse))

	// First: TextChunkEvent "Hello"
	ev, err := parser.next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tc, ok := ev.(*providers.TextChunkEvent)
	if !ok {
		t.Fatalf("expected TextChunkEvent, got %T", ev)
	}
	if tc.Text != "Hello" {
		t.Fatalf("expected 'Hello', got %q", tc.Text)
	}

	// Second: TextChunkEvent " world"
	ev, err = parser.next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tc, ok = ev.(*providers.TextChunkEvent)
	if !ok {
		t.Fatalf("expected TextChunkEvent, got %T", ev)
	}
	if tc.Text != " world" {
		t.Fatalf("expected ' world', got %q", tc.Text)
	}

	// Third: UsageEvent (finish_reason=stop chunk has usage)
	ev, err = parser.next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ue, ok := ev.(*providers.UsageEvent)
	if !ok {
		t.Fatalf("expected UsageEvent, got %T", ev)
	}
	if ue.Usage.InputTokens != 10 {
		t.Fatalf("expected 10 input tokens, got %d", ue.Usage.InputTokens)
	}
	if ue.Usage.OutputTokens != 2 {
		t.Fatalf("expected 2 output tokens, got %d", ue.Usage.OutputTokens)
	}

	// Fourth: DoneEvent from [DONE]
	ev, err = parser.next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, ok = ev.(*providers.DoneEvent)
	if !ok {
		t.Fatalf("expected DoneEvent, got %T", ev)
	}

	// EOF
	_, err = parser.next()
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
}

func TestStreamParserToolCalls(t *testing.T) {
	sse := `data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"file_read","arguments":""}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"/tmp/test\"}"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]

`
	parser := newStreamParser(strings.NewReader(sse))

	// First non-nil: ToolCallEvent (accumulated, emitted on finish_reason=tool_calls)
	ev, err := parser.next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tc, ok := ev.(*providers.ToolCallEvent)
	if !ok {
		t.Fatalf("expected ToolCallEvent, got %T", ev)
	}
	if tc.ToolName != "file_read" {
		t.Fatalf("expected tool name 'file_read', got %q", tc.ToolName)
	}
	if tc.ToolID != "call_abc" {
		t.Fatalf("expected tool ID 'call_abc', got %q", tc.ToolID)
	}
	if string(tc.Input) != `{"path":"/tmp/test"}` {
		t.Fatalf("unexpected input: %s", string(tc.Input))
	}

	// DoneEvent from [DONE]
	ev, err = parser.next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, ok = ev.(*providers.DoneEvent)
	if !ok {
		t.Fatalf("expected DoneEvent, got %T", ev)
	}
}

func TestStreamParserUsageOnly(t *testing.T) {
	// Usage chunk with empty choices
	sse := `data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[],"usage":{"prompt_tokens":50,"completion_tokens":100,"total_tokens":150}}

data: [DONE]

`
	parser := newStreamParser(strings.NewReader(sse))

	ev, err := parser.next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ue, ok := ev.(*providers.UsageEvent)
	if !ok {
		t.Fatalf("expected UsageEvent, got %T", ev)
	}
	if ue.Usage.InputTokens != 50 {
		t.Errorf("expected 50 input tokens, got %d", ue.Usage.InputTokens)
	}
	if ue.Usage.OutputTokens != 100 {
		t.Errorf("expected 100 output tokens, got %d", ue.Usage.OutputTokens)
	}
	if ue.Model != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o', got %q", ue.Model)
	}
}

func TestStreamParserEmptyStream(t *testing.T) {
	parser := newStreamParser(strings.NewReader(""))

	_, err := parser.next()
	if err != io.EOF {
		t.Fatalf("expected io.EOF for empty stream, got %v", err)
	}
}

func TestStreamParserDoneOnly(t *testing.T) {
	sse := `data: [DONE]

`
	parser := newStreamParser(strings.NewReader(sse))

	ev, err := parser.next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, ok := ev.(*providers.DoneEvent)
	if !ok {
		t.Fatalf("expected DoneEvent, got %T", ev)
	}
}

func TestStreamParserModelTracking(t *testing.T) {
	sse := `data: {"id":"chatcmpl-1","model":"gpt-4o-2024-05-13","choices":[{"index":0,"delta":{"content":"Hi"},"finish_reason":null}]}

data: {"id":"chatcmpl-1","model":"gpt-4o-2024-05-13","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":1,"total_tokens":11}}

data: [DONE]

`
	parser := newStreamParser(strings.NewReader(sse))

	// Skip text chunk
	if _, err := parser.next(); err != nil {
		t.Fatalf("unexpected error on text chunk: %v", err)
	}

	// UsageEvent should have model
	ev, err := parser.next()
	if err != nil {
		t.Fatalf("unexpected error on usage chunk: %v", err)
	}
	ue, ok := ev.(*providers.UsageEvent)
	if !ok {
		t.Fatalf("expected UsageEvent, got %T", ev)
	}
	if ue.Model != "gpt-4o-2024-05-13" {
		t.Fatalf("expected model 'gpt-4o-2024-05-13', got %q", ue.Model)
	}
}

func TestStreamParserInvalidJSON(t *testing.T) {
	sse := `data: {invalid json}

`
	parser := newStreamParser(strings.NewReader(sse))

	_, err := parser.next()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "failed to parse") {
		t.Fatalf("expected parse error, got: %v", err)
	}
}
