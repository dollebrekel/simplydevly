// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package ollama

import (
	"io"
	"strings"
	"testing"

	"siply.dev/siply/internal/providers"
)

func TestStreamParserTextChunks(t *testing.T) {
	ndjson := `{"model":"llama3.2","created_at":"2024-01-01T00:00:00Z","message":{"role":"assistant","content":"Hello"},"done":false}
{"model":"llama3.2","created_at":"2024-01-01T00:00:01Z","message":{"role":"assistant","content":" world"},"done":false}
{"model":"llama3.2","created_at":"2024-01-01T00:00:02Z","message":{"role":"assistant","content":""},"done":true,"prompt_eval_count":10,"eval_count":5}
`
	parser := newStreamParser(strings.NewReader(ndjson))

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

	// Third: UsageEvent from done=true
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
	if ue.Usage.OutputTokens != 5 {
		t.Fatalf("expected 5 output tokens, got %d", ue.Usage.OutputTokens)
	}
	if ue.Model != "llama3.2" {
		t.Fatalf("expected model 'llama3.2', got %q", ue.Model)
	}

	// Fourth: DoneEvent (pending from done=true handling)
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

func TestStreamParserErrorResponse(t *testing.T) {
	ndjson := `{"error":"model 'nonexistent' not found"}
`
	parser := newStreamParser(strings.NewReader(ndjson))

	ev, err := parser.next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ee, ok := ev.(*providers.ErrorEvent)
	if !ok {
		t.Fatalf("expected ErrorEvent, got %T", ev)
	}
	if !strings.Contains(ee.Error(), "not found") {
		t.Fatalf("expected 'not found' in error, got %q", ee.Error())
	}
}

func TestStreamParserEmptyStream(t *testing.T) {
	parser := newStreamParser(strings.NewReader(""))

	_, err := parser.next()
	if err != io.EOF {
		t.Fatalf("expected io.EOF for empty stream, got %v", err)
	}
}

func TestStreamParserInvalidJSON(t *testing.T) {
	ndjson := `{not valid json}
`
	parser := newStreamParser(strings.NewReader(ndjson))

	_, err := parser.next()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "failed to parse") {
		t.Fatalf("expected parse error, got: %v", err)
	}
}

func TestStreamParserDoneWithNoContent(t *testing.T) {
	// Done signal without prior content — still emits usage + done
	ndjson := `{"model":"llama3.2","message":{"role":"assistant","content":""},"done":true,"prompt_eval_count":5,"eval_count":0}
`
	parser := newStreamParser(strings.NewReader(ndjson))

	// UsageEvent
	ev, err := parser.next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ue, ok := ev.(*providers.UsageEvent)
	if !ok {
		t.Fatalf("expected UsageEvent, got %T", ev)
	}
	if ue.Usage.InputTokens != 5 {
		t.Errorf("expected 5 input tokens, got %d", ue.Usage.InputTokens)
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
