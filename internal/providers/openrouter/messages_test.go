// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package openrouter

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"siply.dev/siply/internal/core"
)

func TestToAPIRequest_KimiModel(t *testing.T) {
	req := core.QueryRequest{
		Model:    "moonshotai/kimi-k2",
		Messages: []core.Message{{Role: "user", Content: "hello"}},
	}

	got := toAPIRequest(req)

	if got.Model != "moonshotai/kimi-k2" {
		t.Errorf("expected model %q, got %q", "moonshotai/kimi-k2", got.Model)
	}
}

func TestToAPIRequest_DefaultModel(t *testing.T) {
	t.Setenv("SIPLY_MODEL", "")
	req := core.QueryRequest{
		Messages: []core.Message{{Role: "user", Content: "hello"}},
	}

	got := toAPIRequest(req)

	if got.Model != "" {
		t.Errorf("expected empty model (no hardcoded default), got %q", got.Model)
	}
}

func TestToAPIRequest_SIPLYMODELEnvVar(t *testing.T) {
	t.Setenv("SIPLY_MODEL", "moonshotai/moonshot-v1-128k")
	req := core.QueryRequest{
		Messages: []core.Message{{Role: "user", Content: "hello"}},
	}

	got := toAPIRequest(req)

	if got.Model != "moonshotai/moonshot-v1-128k" {
		t.Errorf("expected env model, got %q", got.Model)
	}
}

func TestToAPIRequest_ExplicitModelOverridesEnv(t *testing.T) {
	t.Setenv("SIPLY_MODEL", "moonshotai/moonshot-v1-8k")
	req := core.QueryRequest{
		Model:    "moonshotai/kimi-k2",
		Messages: []core.Message{{Role: "user", Content: "hello"}},
	}

	got := toAPIRequest(req)

	if got.Model != "moonshotai/kimi-k2" {
		t.Errorf("explicit model should override env, got %q", got.Model)
	}
}

func TestToAPIRequest_ToolsSortedAlphabetically(t *testing.T) {
	req := core.QueryRequest{
		Messages: []core.Message{{Role: "user", Content: "hello"}},
		Tools: []core.ToolDefinition{
			{Name: "read_file", Description: "Read a file", InputSchema: json.RawMessage(`{}`)},
			{Name: "bash", Description: "Run bash", InputSchema: json.RawMessage(`{}`)},
			{Name: "write_file", Description: "Write a file", InputSchema: json.RawMessage(`{}`)},
		},
	}

	got := toAPIRequest(req)

	if len(got.Tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(got.Tools))
	}

	names := []string{got.Tools[0].Function.Name, got.Tools[1].Function.Name, got.Tools[2].Function.Name}
	expected := []string{"bash", "read_file", "write_file"}
	for i, want := range expected {
		if names[i] != want {
			t.Errorf("tools[%d]: expected %q, got %q", i, want, names[i])
		}
	}
}

func TestToAPIRequest_EmptyTools_NoSort(t *testing.T) {
	req := core.QueryRequest{
		Messages: []core.Message{{Role: "user", Content: "hello"}},
	}

	got := toAPIRequest(req)

	if len(got.Tools) != 0 {
		t.Errorf("expected no tools, got %d", len(got.Tools))
	}
}

func TestToAPIRequest_SystemPromptIncluded(t *testing.T) {
	req := core.QueryRequest{
		Messages:     []core.Message{{Role: "user", Content: "hello"}},
		SystemPrompt: "You are a coding assistant.",
	}

	got := toAPIRequest(req)

	if len(got.Messages) < 2 {
		t.Fatalf("expected system + user messages, got %d", len(got.Messages))
	}
	if got.Messages[0].Role != "system" {
		t.Errorf("expected first message to be system, got %q", got.Messages[0].Role)
	}
	if got.Messages[0].Content != "You are a coding assistant." {
		t.Errorf("unexpected system message content: %q", got.Messages[0].Content)
	}
}

func TestToAPIRequest_StreamOptionsIncludeUsage(t *testing.T) {
	req := core.QueryRequest{
		Messages: []core.Message{{Role: "user", Content: "hello"}},
	}

	got := toAPIRequest(req)

	if got.StreamOptions == nil {
		t.Fatal("expected StreamOptions to be set")
	}
	if !got.StreamOptions.IncludeUsage {
		t.Error("expected IncludeUsage to be true")
	}
}

func TestToAPIRequest_SystemPromptStable_NoTimestamp(t *testing.T) {
	req := core.QueryRequest{
		Messages:     []core.Message{{Role: "user", Content: "hello"}},
		SystemPrompt: "You are an assistant. Current time: 2026-04-14 15:30:22. Be helpful.",
	}

	got := toAPIRequest(req)

	if len(got.Messages) == 0 {
		t.Fatal("expected at least one message")
	}
	systemMsg := got.Messages[0]
	if systemMsg.Role != "system" {
		t.Fatalf("expected first message to be system, got %q", systemMsg.Role)
	}
	if strings.Contains(systemMsg.Content, "15:30:22") {
		t.Errorf("system message must not contain time component '15:30:22': %q", systemMsg.Content)
	}
	if !strings.Contains(systemMsg.Content, "2026-04-14") {
		t.Errorf("system message should retain date component '2026-04-14': %q", systemMsg.Content)
	}
}

func TestToAPIRequest_DynamicContent_MovedToFirstUserMessage(t *testing.T) {
	fixedDate := time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC)
	req := core.QueryRequest{
		Messages:      []core.Message{{Role: "user", Content: "hello"}},
		SystemPrompt:  "You are a coding assistant.",
		TaskStartTime: fixedDate,
	}

	got := toAPIRequest(req)

	var userContent string
	for _, m := range got.Messages {
		if m.Role == "user" {
			userContent = m.Content
			break
		}
	}

	if userContent == "" {
		t.Fatal("no user message found in output")
	}
	if !strings.Contains(userContent, "<system-reminder>") {
		t.Errorf("expected <system-reminder> in first user message, got: %q", userContent)
	}
	if !strings.Contains(userContent, "2026-04-14") {
		t.Errorf("expected frozen date '2026-04-14' in <system-reminder>, got: %q", userContent)
	}
}

func TestToAPIRequest_NoUserMessages_SystemPromptKeptIntact(t *testing.T) {
	// When there are no user messages, the system prompt must NOT be stripped
	// (relocation trick requires a user turn to land in).
	withTime := "You are an assistant. Current time: 2026-04-14 15:30:22."
	req := core.QueryRequest{
		Messages:     []core.Message{{Role: "assistant", Content: "previous assistant turn"}},
		SystemPrompt: withTime,
	}

	got := toAPIRequest(req)

	var systemContent string
	for _, m := range got.Messages {
		if m.Role == "system" {
			systemContent = m.Content
			break
		}
	}
	if systemContent == "" {
		t.Fatal("expected system message in output")
	}
	// System prompt should be kept intact — time component must be preserved.
	if !strings.Contains(systemContent, "15:30:22") {
		t.Errorf("system prompt should be intact when no user message; want '15:30:22' in %q", systemContent)
	}
	// The reminder must NOT appear anywhere in the output (no user message to land in).
	for _, m := range got.Messages {
		if strings.Contains(m.Content, "<system-reminder>") {
			t.Errorf("reminder must not be injected when there is no user message, found in role=%q content=%q", m.Role, m.Content)
		}
	}
	// The assistant message must be unchanged.
	var assistantContent string
	for _, m := range got.Messages {
		if m.Role == "assistant" {
			assistantContent = m.Content
			break
		}
	}
	if assistantContent != "previous assistant turn" {
		t.Errorf("assistant message must be unchanged, got %q", assistantContent)
	}
}

func TestBuildStableSystemMessage_Idempotent(t *testing.T) {
	// Applying buildStableSystemMessage twice must produce the same result as once.
	inputs := []string{
		"You are helpful. Current time: 2026-04-14 15:30:22 UTC.",
		"Session abc12345-0000-0000-0000-000000000000 active.",
		"Plain system prompt with no dynamic content.",
		"Date: 2026-04-14 09:00 +0200. Task: review.",
	}
	for _, input := range inputs {
		once := buildStableSystemMessage(input)
		twice := buildStableSystemMessage(once)
		if once != twice {
			t.Errorf("buildStableSystemMessage not idempotent for input %q:\n  once:  %q\n  twice: %q", input, once, twice)
		}
	}
}

func TestBuildStableSystemMessage_NumericOffset(t *testing.T) {
	// Numeric UTC offsets like +0200 or -05:00 must be consumed along with the time component.
	cases := []struct {
		input string
		want  string
	}{
		{"Current time: 2026-04-14 15:30:22 +0200.", "Current time: 2026-04-14."},
		{"Current time: 2026-04-14 15:30:22 -05:00.", "Current time: 2026-04-14."},
		{"Current time: 2026-04-14 15:30:22 UTC.", "Current time: 2026-04-14."},
		{"Current time: 2026-04-14 15:30:22.", "Current time: 2026-04-14."},
	}
	for _, tc := range cases {
		got := buildStableSystemMessage(tc.input)
		if got != tc.want {
			t.Errorf("input %q: got %q, want %q", tc.input, got, tc.want)
		}
	}
}
