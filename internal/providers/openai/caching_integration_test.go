// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

// Package openai — caching integration test.
// Verifies that the relocation trick and tool sorting produce byte-identical
// system messages across consecutive turns (the prerequisite for OpenAI prefix caching).
// No API key or network connection required.

package openai

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"siply.dev/siply/internal/core"
)

// simulateTurn builds an apiRequest for a single turn of an agent loop.
// Messages grow with each turn (simulates sliding-window conversation history).
func simulateTurn(systemPrompt string, messages []core.Message, tools []core.ToolDefinition, taskStart time.Time) apiRequest {
	return toAPIRequest(core.QueryRequest{
		SystemPrompt:  systemPrompt,
		Messages:      messages,
		Tools:         tools,
		TaskStartTime: taskStart,
	})
}

// extractSystemMessage returns the content of the first system-role message.
func extractSystemMessage(req apiRequest) string {
	for _, m := range req.Messages {
		if m.Role == "system" {
			return m.Content
		}
	}
	return ""
}

// extractFirstUserMessage returns the content of the first user-role message.
func extractFirstUserMessage(req apiRequest) string {
	for _, m := range req.Messages {
		if m.Role == "user" {
			return m.Content
		}
	}
	return ""
}

// extractToolNames returns the list of tool function names in order.
func extractToolNames(req apiRequest) []string {
	names := make([]string, len(req.Tools))
	for i, t := range req.Tools {
		names[i] = t.Function.Name
	}
	return names
}

// TestCachingRelocationTrick_SystemMessageByteIdenticalAcrossTurns is the core
// caching correctness test. It simulates 3 consecutive turns of a task and
// asserts that the system message bytes are identical in all turns.
func TestCachingRelocationTrick_SystemMessageByteIdenticalAcrossTurns(t *testing.T) {
	// Frozen at task start — this is what agent.go does in Run().
	taskStart := time.Date(2026, 4, 14, 9, 0, 0, 0, time.UTC)

	// System prompt with a dynamic timestamp — simulates a host that injects the
	// current datetime. After relocation trick: timestamp stripped, date-only remains.
	systemPrompt := "You are a coding assistant. Session started 2026-04-14 09:00:00 UTC. Be helpful."

	tools := []core.ToolDefinition{
		{Name: "write_file", Description: "Write a file", InputSchema: json.RawMessage(`{}`)},
		{Name: "bash", Description: "Run bash", InputSchema: json.RawMessage(`{}`)},
		{Name: "read_file", Description: "Read a file", InputSchema: json.RawMessage(`{}`)},
	}

	// Turn 1: initial user message.
	msgs1 := []core.Message{
		{Role: "user", Content: "Fix the bug in main.go"},
	}
	// Turn 2: user message + assistant response (history grows).
	msgs2 := []core.Message{
		{Role: "user", Content: "Fix the bug in main.go"},
		{Role: "assistant", Content: "I'll fix the bug now."},
		{Role: "user", Content: "Also add tests"},
	}
	// Turn 3: further grown history.
	msgs3 := []core.Message{
		{Role: "user", Content: "Fix the bug in main.go"},
		{Role: "assistant", Content: "I'll fix the bug now."},
		{Role: "user", Content: "Also add tests"},
		{Role: "assistant", Content: "Adding tests next."},
		{Role: "user", Content: "Run the tests"},
	}

	req1 := simulateTurn(systemPrompt, msgs1, tools, taskStart)
	req2 := simulateTurn(systemPrompt, msgs2, tools, taskStart)
	req3 := simulateTurn(systemPrompt, msgs3, tools, taskStart)

	sys1 := extractSystemMessage(req1)
	sys2 := extractSystemMessage(req2)
	sys3 := extractSystemMessage(req3)

	// CRITICAL: system message must be byte-identical across all turns.
	if sys1 != sys2 {
		t.Errorf("system message differs between turn 1 and turn 2:\n  turn1: %q\n  turn2: %q", sys1, sys2)
	}
	if sys2 != sys3 {
		t.Errorf("system message differs between turn 2 and turn 3:\n  turn2: %q\n  turn3: %q", sys2, sys3)
	}

	// System message must NOT contain the time component (stripped by relocation trick).
	if containsSubstr(sys1, "09:00:00") {
		t.Errorf("system message contains dynamic time '09:00:00' — relocation trick failed: %q", sys1)
	}

	// System message must still contain the date (kept for context).
	if !containsSubstr(sys1, "2026-04-14") {
		t.Errorf("system message lost the date '2026-04-14': %q", sys1)
	}

	t.Logf("✅ system message is byte-identical across 3 turns: %q", sys1)
}

// TestCachingRelocationTrick_FrozenDateInFirstUserMessage verifies that the
// frozen date from TaskStartTime appears in the first user message as a
// <system-reminder> tag — not in the system message.
func TestCachingRelocationTrick_FrozenDateInFirstUserMessage(t *testing.T) {
	taskStart := time.Date(2026, 4, 14, 9, 0, 0, 0, time.UTC)
	msgs := []core.Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there"},
		{Role: "user", Content: "How are you"},
	}

	req := simulateTurn("You are helpful.", msgs, nil, taskStart)

	firstUser := extractFirstUserMessage(req)
	sysMsg := extractSystemMessage(req)

	// Frozen date must appear in first user message.
	if !containsSubstr(firstUser, "2026-04-14") {
		t.Errorf("frozen date not found in first user message: %q", firstUser)
	}
	if !containsSubstr(firstUser, "<system-reminder>") {
		t.Errorf("<system-reminder> tag missing from first user message: %q", firstUser)
	}

	// System message must NOT contain the <system-reminder> tag.
	if containsSubstr(sysMsg, "<system-reminder>") {
		t.Errorf("system message must not contain <system-reminder>: %q", sysMsg)
	}

	// Only the FIRST user message should have the reminder — not subsequent ones.
	found := 0
	for _, m := range req.Messages {
		if m.Role == "user" && containsSubstr(m.Content, "<system-reminder>") {
			found++
		}
	}
	if found != 1 {
		t.Errorf("expected exactly 1 user message with <system-reminder>, got %d", found)
	}

	t.Logf("✅ frozen date %q in first user message, not in system message", "2026-04-14")
}

// TestCachingRelocationTrick_ToolOrderDeterministic verifies that tools are
// always sorted alphabetically regardless of registration order.
func TestCachingRelocationTrick_ToolOrderDeterministic(t *testing.T) {
	taskStart := time.Date(2026, 4, 14, 9, 0, 0, 0, time.UTC)
	msgs := []core.Message{{Role: "user", Content: "go"}}

	// Same tools in different orders across two turns.
	toolsOrderA := []core.ToolDefinition{
		{Name: "write_file", InputSchema: json.RawMessage(`{}`)},
		{Name: "bash", InputSchema: json.RawMessage(`{}`)},
		{Name: "read_file", InputSchema: json.RawMessage(`{}`)},
	}
	toolsOrderB := []core.ToolDefinition{
		{Name: "bash", InputSchema: json.RawMessage(`{}`)},
		{Name: "read_file", InputSchema: json.RawMessage(`{}`)},
		{Name: "write_file", InputSchema: json.RawMessage(`{}`)},
	}

	reqA := simulateTurn("system", msgs, toolsOrderA, taskStart)
	reqB := simulateTurn("system", msgs, toolsOrderB, taskStart)

	namesA := extractToolNames(reqA)
	namesB := extractToolNames(reqB)

	// Tool order must be identical regardless of input order.
	if len(namesA) != len(namesB) {
		t.Fatalf("tool count mismatch: %d vs %d", len(namesA), len(namesB))
	}
	for i := range namesA {
		if namesA[i] != namesB[i] {
			t.Errorf("tool[%d] differs: %q vs %q", i, namesA[i], namesB[i])
		}
	}

	// Must be alphabetically sorted.
	expected := []string{"bash", "read_file", "write_file"}
	for i, want := range expected {
		if namesA[i] != want {
			t.Errorf("tools[%d] = %q, want %q", i, namesA[i], want)
		}
	}

	t.Logf("✅ tool order deterministic across registration orders: %v", namesA)
}

// TestCachingRelocationTrick_ToolSerializationByteIdentical verifies that the
// JSON-serialized tool array is byte-identical across turns (the actual cache key).
func TestCachingRelocationTrick_ToolSerializationByteIdentical(t *testing.T) {
	taskStart := time.Date(2026, 4, 14, 9, 0, 0, 0, time.UTC)

	tools := []core.ToolDefinition{
		{Name: "write_file", Description: "Write", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "bash", Description: "Bash", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "read_file", Description: "Read", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}

	req1 := simulateTurn("prompt", []core.Message{{Role: "user", Content: "turn 1"}}, tools, taskStart)
	req2 := simulateTurn("prompt", []core.Message{{Role: "user", Content: "turn 2"}}, tools, taskStart)

	tools1, _ := json.Marshal(req1.Tools)
	tools2, _ := json.Marshal(req2.Tools)

	if string(tools1) != string(tools2) {
		t.Errorf("tool JSON differs between turns:\n  turn1: %s\n  turn2: %s", tools1, tools2)
	}

	t.Logf("✅ tool serialization byte-identical: %d bytes", len(tools1))
}

func containsSubstr(s, substr string) bool {
	return strings.Contains(s, substr)
}
