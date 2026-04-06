// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package openrouter

import (
	"io"
	"strings"
	"testing"

	"siply.dev/siply/internal/providers"
)

func TestStreamParserTextDelta(t *testing.T) {
	sse := `data: {"id":"chatcmpl-1","model":"anthropic/claude-sonnet-4-20250514","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}

data: {"id":"chatcmpl-1","model":"anthropic/claude-sonnet-4-20250514","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}

data: {"id":"chatcmpl-1","model":"anthropic/claude-sonnet-4-20250514","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}

data: [DONE]

`
	parser := newStreamParser(strings.NewReader(sse))

	// TextChunkEvent "Hello"
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

	// TextChunkEvent " world"
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

	// UsageEvent
	ev, err = parser.next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ue, ok := ev.(*providers.UsageEvent)
	if !ok {
		t.Fatalf("expected UsageEvent, got %T", ev)
	}
	if ue.Usage.InputTokens != 10 || ue.Usage.OutputTokens != 2 {
		t.Fatalf("expected 10/2 tokens, got %d/%d", ue.Usage.InputTokens, ue.Usage.OutputTokens)
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

	// EOF
	_, err = parser.next()
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
}

func TestStreamParserToolCalls(t *testing.T) {
	sse := `data: {"id":"chatcmpl-1","model":"openai/gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read_file","arguments":""}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-1","model":"openai/gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":\"/test\"}"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-1","model":"openai/gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]

`
	parser := newStreamParser(strings.NewReader(sse))

	ev, err := parser.next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tc, ok := ev.(*providers.ToolCallEvent)
	if !ok {
		t.Fatalf("expected ToolCallEvent, got %T", ev)
	}
	if tc.ToolName != "read_file" {
		t.Fatalf("expected 'read_file', got %q", tc.ToolName)
	}
	if string(tc.Input) != `{"path":"/test"}` {
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

	// EOF
	_, err = parser.next()
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
}

func TestStreamParserEmptyStream(t *testing.T) {
	parser := newStreamParser(strings.NewReader(""))

	_, err := parser.next()
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
}

func TestStreamParserModelTracking(t *testing.T) {
	sse := `data: {"id":"chatcmpl-1","model":"meta-llama/llama-3.1-70b","choices":[{"index":0,"delta":{"content":"Hi"},"finish_reason":null}]}

data: {"id":"chatcmpl-1","model":"meta-llama/llama-3.1-70b","choices":[],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}

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
	if ue.Model != "meta-llama/llama-3.1-70b" {
		t.Fatalf("expected model 'meta-llama/llama-3.1-70b', got %q", ue.Model)
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

	// EOF
	_, err = parser.next()
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
}
