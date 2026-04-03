package anthropic

import (
	"io"
	"strings"
	"testing"

	"siply.dev/siply/internal/providers"
)

func TestStreamParserTextDelta(t *testing.T) {
	sse := `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","model":"claude-sonnet-4-20250514"}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{},"usage":{"output_tokens":10}}

event: message_stop
data: {"type":"message_stop"}

`
	parser := newStreamParser(strings.NewReader(sse))

	// next() skips nil events (message_start, content_block_start are internal).
	// First non-nil: TextChunkEvent "Hello"
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

	// content_block_stop for text block → nil, skipped by next()
	// message_delta → UsageEvent
	ev, err = parser.next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ue, ok := ev.(*providers.UsageEvent)
	if !ok {
		t.Fatalf("expected UsageEvent, got %T", ev)
	}
	if ue.Usage.OutputTokens != 10 {
		t.Fatalf("expected 10 output tokens, got %d", ue.Usage.OutputTokens)
	}

	// message_stop → DoneEvent
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

func TestStreamParserThinkingDelta(t *testing.T) {
	sse := `event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Let me think about this..."}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_stop
data: {"type":"message_stop"}

`
	parser := newStreamParser(strings.NewReader(sse))

	// First non-nil: ThinkingEvent
	ev, err := parser.next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	te, ok := ev.(*providers.ThinkingEvent)
	if !ok {
		t.Fatalf("expected ThinkingEvent, got %T", ev)
	}
	if te.Thinking != "Let me think about this..." {
		t.Fatalf("unexpected thinking: %q", te.Thinking)
	}

	// content_block_stop (thinking) → nil, skipped
	// message_stop → DoneEvent
	ev, err = parser.next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, ok = ev.(*providers.DoneEvent)
	if !ok {
		t.Fatalf("expected DoneEvent, got %T", ev)
	}
}

func TestStreamParserToolUse(t *testing.T) {
	sse := `event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_123","name":"file_read"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\":"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"/tmp/test\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_stop
data: {"type":"message_stop"}

`
	parser := newStreamParser(strings.NewReader(sse))

	// content_block_start (tool_use) → nil, input_json_deltas → nil,
	// content_block_stop → ToolCallEvent (first non-nil)
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
	if tc.ToolID != "toolu_123" {
		t.Fatalf("expected tool ID 'toolu_123', got %q", tc.ToolID)
	}
	if string(tc.Input) != `{"path":"/tmp/test"}` {
		t.Fatalf("unexpected input: %s", string(tc.Input))
	}

	// message_stop → DoneEvent
	ev, err = parser.next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, ok = ev.(*providers.DoneEvent)
	if !ok {
		t.Fatalf("expected DoneEvent, got %T", ev)
	}
}

func TestStreamParserErrorEvent(t *testing.T) {
	sse := `event: error
data: {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}

`
	parser := newStreamParser(strings.NewReader(sse))

	ev, err := parser.next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ee, ok := ev.(*providers.ErrorEvent)
	if !ok {
		t.Fatalf("expected ErrorEvent, got %T", ev)
	}
	if !strings.Contains(ee.Error(), "overloaded_error") {
		t.Fatalf("expected overloaded_error in message, got %q", ee.Error())
	}
}

func TestStreamParserPingIgnored(t *testing.T) {
	sse := `event: ping
data: {}

event: message_stop
data: {"type":"message_stop"}

`
	parser := newStreamParser(strings.NewReader(sse))

	// ping → nil, skipped by next()
	// message_stop → DoneEvent
	ev, err := parser.next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, ok := ev.(*providers.DoneEvent)
	if !ok {
		t.Fatalf("expected DoneEvent, got %T", ev)
	}
}

func TestStreamParserEmptyStream(t *testing.T) {
	parser := newStreamParser(strings.NewReader(""))

	_, err := parser.next()
	if err != io.EOF {
		t.Fatalf("expected io.EOF for empty stream, got %v", err)
	}
}

func TestStreamParserMixedContent(t *testing.T) {
	// Thinking block followed by text block
	sse := `event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Reasoning..."}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Answer"}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: message_stop
data: {"type":"message_stop"}

`
	parser := newStreamParser(strings.NewReader(sse))

	// ThinkingEvent
	ev, err := parser.next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	te, ok := ev.(*providers.ThinkingEvent)
	if !ok {
		t.Fatalf("expected ThinkingEvent, got %T", ev)
	}
	if te.Thinking != "Reasoning..." {
		t.Fatalf("unexpected thinking: %q", te.Thinking)
	}

	// TextChunkEvent
	ev, err = parser.next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tc, ok := ev.(*providers.TextChunkEvent)
	if !ok {
		t.Fatalf("expected TextChunkEvent, got %T", ev)
	}
	if tc.Text != "Answer" {
		t.Fatalf("unexpected text: %q", tc.Text)
	}

	// DoneEvent
	ev, err = parser.next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, ok = ev.(*providers.DoneEvent)
	if !ok {
		t.Fatalf("expected DoneEvent, got %T", ev)
	}
}
