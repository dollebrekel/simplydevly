// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package kimi

import (
	"encoding/json"
	"testing"

	"siply.dev/siply/internal/core"
)

// TestKimiToAPIRequest_WithCache_NoSystemPromptRepeated verifies that when a
// cache_id is provided the system prompt is NOT included in the request messages.
func TestKimiToAPIRequest_WithCache_NoSystemPromptRepeated(t *testing.T) {
	req := core.QueryRequest{
		SystemPrompt: "you are a helpful assistant with a very long system prompt",
		Model:        "moonshot-v1-128k",
		Messages:     []core.Message{{Role: "user", Content: "hello"}},
	}

	apiReq := toAPIRequest(req, nil, "cache_abc123")

	// First message must be role "cache", not "system".
	if len(apiReq.Messages) == 0 {
		t.Fatal("expected at least one message")
	}
	first := apiReq.Messages[0]
	if first.Role != "cache" {
		t.Errorf("expected role %q, got %q", "cache", first.Role)
	}
	// Content must be the cache content blocks, not the system prompt string.
	blocks, ok := first.Content.([]cacheContentBlock)
	if !ok {
		t.Fatalf("expected content to be []cacheContentBlock, got %T", first.Content)
	}
	if len(blocks) != 1 || blocks[0].CacheID != "cache_abc123" {
		t.Errorf("unexpected cache content: %+v", blocks)
	}

	// Verify that the system prompt text is NOT present in any message.
	for _, m := range apiReq.Messages {
		if s, ok := m.Content.(string); ok && s == req.SystemPrompt {
			t.Error("system prompt should NOT be repeated when cache_id is set")
		}
	}
}

// TestKimiToAPIRequest_NoCacheID_SystemPromptPresent verifies that when no
// cache_id is given the system prompt is sent as a regular system message.
func TestKimiToAPIRequest_NoCacheID_SystemPromptPresent(t *testing.T) {
	req := core.QueryRequest{
		SystemPrompt: "you are a helpful assistant",
		Model:        "moonshot-v1-128k",
		Messages:     []core.Message{{Role: "user", Content: "hello"}},
	}

	apiReq := toAPIRequest(req, nil, "")

	if len(apiReq.Messages) == 0 {
		t.Fatal("expected messages")
	}
	first := apiReq.Messages[0]
	if first.Role != "system" {
		t.Errorf("expected role %q without cache, got %q", "system", first.Role)
	}
	if s, ok := first.Content.(string); !ok || s != req.SystemPrompt {
		t.Errorf("expected system prompt content, got %v", first.Content)
	}
}

// TestKimiToAPIRequest_ToolsSorted verifies that tools are sorted alphabetically.
func TestKimiToAPIRequest_ToolsSorted(t *testing.T) {
	tools := []core.ToolDefinition{
		{Name: "zebra", Description: "z tool"},
		{Name: "apple", Description: "a tool"},
		{Name: "mango", Description: "m tool"},
	}
	req := core.QueryRequest{
		Model:    "moonshot-v1-128k",
		Messages: []core.Message{{Role: "user", Content: "hi"}},
		Tools:    tools,
	}

	apiTools := buildAPITools(req.Tools)
	apiReq := toAPIRequest(req, apiTools, "")

	names := make([]string, len(apiReq.Tools))
	for i, t := range apiReq.Tools {
		names[i] = t.Function.Name
	}

	want := []string{"apple", "mango", "zebra"}
	for i, got := range names {
		if got != want[i] {
			t.Errorf("tool[%d]: got %q, want %q", i, got, want[i])
		}
	}
}

// TestBuildCacheRequest verifies that the cache request includes the system
// prompt and tools in the expected format.
func TestBuildCacheRequest(t *testing.T) {
	tools := []apiTool{
		{Type: "function", Function: apiFunction{Name: "read_file", Description: "reads a file"}},
	}
	req := buildCacheRequest("moonshot-v1-128k", "system prompt text", tools)

	if req.Model != "moonshot-v1-128k" {
		t.Errorf("got model %q, want %q", req.Model, "moonshot-v1-128k")
	}
	if len(req.Messages) < 1 {
		t.Fatal("expected at least one message in cache request")
	}
	if req.Messages[0].Role != "system" {
		t.Errorf("first message role: got %q, want %q", req.Messages[0].Role, "system")
	}

	// Tools should be serialised in the second message.
	if len(req.Messages) < 2 {
		t.Fatal("expected tools message in cache request")
	}
	toolsMsg := req.Messages[1]
	if toolsMsg.Role != "user" {
		t.Errorf("tools message role: got %q, want %q", toolsMsg.Role, "user")
	}
	// Content should be valid JSON.
	var toolsPayload []apiTool
	content, ok := toolsMsg.Content.(string)
	if !ok {
		t.Fatalf("tools content should be string, got %T", toolsMsg.Content)
	}
	if err := json.Unmarshal([]byte(content), &toolsPayload); err != nil {
		t.Errorf("tools content is not valid JSON: %v", err)
	}
}

// TestEstimateTokens verifies that the token estimator returns a positive value
// for a non-empty system prompt.
func TestEstimateTokens(t *testing.T) {
	// 4000 tokens * 4 chars/token = 16000 chars; use 20000 to exceed threshold.
	longPrompt := make([]byte, 20000)
	for i := range longPrompt {
		longPrompt[i] = 'a'
	}
	n := estimateTokens(string(longPrompt), nil)
	if n < cacheTokenThreshold {
		t.Errorf("expected estimate >= %d, got %d", cacheTokenThreshold, n)
	}
}

// TestEstimateTokens_BelowThreshold verifies a short prompt stays below the threshold.
func TestEstimateTokens_BelowThreshold(t *testing.T) {
	n := estimateTokens("short prompt", nil)
	if n >= cacheTokenThreshold {
		t.Errorf("expected estimate < %d, got %d", cacheTokenThreshold, n)
	}
}
