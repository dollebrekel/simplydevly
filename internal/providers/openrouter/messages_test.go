// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package openrouter

import (
	"encoding/json"
	"testing"

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

	if got.Model != "anthropic/claude-sonnet-4-20250514" {
		t.Errorf("expected default model, got %q", got.Model)
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
