// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package kimi

import (
	"strings"
	"testing"

	"siply.dev/siply/internal/providers"
)

// sseFixture constructs an SSE stream string from data lines.
func sseFixture(lines ...string) string {
	var b strings.Builder
	for _, l := range lines {
		b.WriteString("data: ")
		b.WriteString(l)
		b.WriteString("\n\n")
	}
	return b.String()
}

// TestKimiStreamParser_CacheTokens verifies that cached_tokens from
// prompt_tokens_details are mapped to CacheReadInputTokens.
func TestKimiStreamParser_CacheTokens(t *testing.T) {
	usageChunk := `{"id":"x","model":"moonshot-v1-128k","choices":[],"usage":{"prompt_tokens":1000,"completion_tokens":50,"total_tokens":1050,"prompt_tokens_details":{"cached_tokens":800}}}`
	fixture := sseFixture(usageChunk, "[DONE]")

	parser := newStreamParser(strings.NewReader(fixture))

	// First event should be UsageEvent.
	ev, err := parser.next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	usageEv, ok := ev.(*providers.UsageEvent)
	if !ok {
		t.Fatalf("expected *providers.UsageEvent, got %T", ev)
	}
	if usageEv.Usage.InputTokens != 1000 {
		t.Errorf("InputTokens: got %d, want 1000", usageEv.Usage.InputTokens)
	}
	if usageEv.Usage.CacheReadInputTokens != 800 {
		t.Errorf("CacheReadInputTokens: got %d, want 800", usageEv.Usage.CacheReadInputTokens)
	}
	if usageEv.Usage.OutputTokens != 50 {
		t.Errorf("OutputTokens: got %d, want 50", usageEv.Usage.OutputTokens)
	}

	// Second event should be DoneEvent.
	ev, err = parser.next()
	if err != nil {
		t.Fatalf("unexpected error after usage: %v", err)
	}
	if _, ok := ev.(*providers.DoneEvent); !ok {
		t.Errorf("expected *providers.DoneEvent, got %T", ev)
	}
}

// TestKimiStreamParser_CacheCreationTokens verifies that cache_creation_input_tokens
// is mapped to CacheCreationInputTokens.
func TestKimiStreamParser_CacheCreationTokens(t *testing.T) {
	usageChunk := `{"id":"x","model":"moonshot-v1-128k","choices":[],"usage":{"prompt_tokens":5000,"completion_tokens":50,"total_tokens":5050,"cache_creation_input_tokens":5000}}`
	fixture := sseFixture(usageChunk, "[DONE]")

	parser := newStreamParser(strings.NewReader(fixture))
	ev, err := parser.next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	usageEv, ok := ev.(*providers.UsageEvent)
	if !ok {
		t.Fatalf("expected *providers.UsageEvent, got %T", ev)
	}
	if usageEv.Usage.CacheCreationInputTokens != 5000 {
		t.Errorf("CacheCreationInputTokens: got %d, want 5000", usageEv.Usage.CacheCreationInputTokens)
	}
}

// TestKimiStreamParser_TextContent verifies that text delta events are emitted.
func TestKimiStreamParser_TextContent(t *testing.T) {
	chunk1 := `{"id":"x","model":"moonshot-v1-128k","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`
	chunk2 := `{"id":"x","model":"moonshot-v1-128k","choices":[{"index":0,"delta":{"content":" World"},"finish_reason":"stop"}]}`
	fixture := sseFixture(chunk1, chunk2, "[DONE]")

	parser := newStreamParser(strings.NewReader(fixture))

	ev, err := parser.next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text, ok := ev.(*providers.TextChunkEvent)
	if !ok {
		t.Fatalf("expected *providers.TextChunkEvent, got %T", ev)
	}
	if text.Text != "Hello" {
		t.Errorf("got text %q, want %q", text.Text, "Hello")
	}
}
