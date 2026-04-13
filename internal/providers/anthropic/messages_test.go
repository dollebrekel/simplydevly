// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	"siply.dev/siply/internal/core"
)

func TestBuildSystemField_ShortPrompt_ReturnsString(t *testing.T) {
	prompt := "You are a helpful assistant."
	result := buildSystemField(prompt, "claude-sonnet-4-20250514")

	str, ok := result.(string)
	if !ok {
		t.Fatalf("expected string for short prompt, got %T", result)
	}
	if str != prompt {
		t.Fatalf("expected %q, got %q", prompt, str)
	}
}

func TestBuildSystemField_LongPrompt_ReturnsBlocks(t *testing.T) {
	// Create a prompt that exceeds 1024 tokens (~4096 chars for Sonnet)
	prompt := strings.Repeat("a", 5000)
	result := buildSystemField(prompt, "claude-sonnet-4-20250514")

	blocks, ok := result.([]apiSystemBlock)
	if !ok {
		t.Fatalf("expected []apiSystemBlock for long prompt, got %T", result)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Type != "text" {
		t.Fatalf("expected type 'text', got %q", blocks[0].Type)
	}
	if blocks[0].CacheControl == nil {
		t.Fatal("expected cache_control to be set")
	}
	if blocks[0].CacheControl.Type != "ephemeral" {
		t.Fatalf("expected cache_control type 'ephemeral', got %q", blocks[0].CacheControl.Type)
	}
}

func TestBuildSystemField_OpusThreshold(t *testing.T) {
	// 6000 chars ≈ 1500 tokens — above Sonnet threshold (1024) but below Opus (2048)
	prompt := strings.Repeat("b", 6000)
	result := buildSystemField(prompt, "claude-opus-4-6-20250514")

	_, ok := result.(string)
	if !ok {
		t.Fatalf("expected string for Opus prompt below 2048 token threshold, got %T", result)
	}

	// 10000 chars ≈ 2500 tokens — above Opus threshold
	prompt = strings.Repeat("b", 10000)
	result = buildSystemField(prompt, "claude-opus-4-6-20250514")

	blocks, ok := result.([]apiSystemBlock)
	if !ok {
		t.Fatalf("expected []apiSystemBlock for Opus prompt above threshold, got %T", result)
	}
	if blocks[0].CacheControl == nil {
		t.Fatal("expected cache_control for Opus prompt above threshold")
	}
}

func TestBuildSystemField_EmptyPrompt(t *testing.T) {
	result := buildSystemField("", "claude-sonnet-4")
	if result != nil {
		t.Fatalf("expected nil for empty prompt, got %v", result)
	}
}

func TestAPIRequest_SystemJSON_PlainString(t *testing.T) {
	req := apiRequest{
		Model:     "claude-sonnet-4",
		MaxTokens: 4096,
		Stream:    true,
		System:    "You are helpful.",
		Messages:  []apiMessage{{Role: "user", Content: "Hi"}},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// System should be a plain string
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	sysStr := string(raw["system"])
	if !strings.HasPrefix(sysStr, `"`) {
		t.Fatalf("expected system as JSON string, got %s", sysStr)
	}
}

func TestAPIRequest_SystemJSON_CacheBlocks(t *testing.T) {
	req := apiRequest{
		Model:     "claude-sonnet-4",
		MaxTokens: 4096,
		Stream:    true,
		System: []apiSystemBlock{{
			Type: "text",
			Text: "You are helpful.",
			CacheControl: &apiCacheControl{
				Type: "ephemeral",
			},
		}},
		Messages: []apiMessage{{Role: "user", Content: "Hi"}},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// System should be an array
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	sysStr := string(raw["system"])
	if !strings.HasPrefix(sysStr, `[`) {
		t.Fatalf("expected system as JSON array, got %s", sysStr)
	}
	if !strings.Contains(sysStr, `"cache_control"`) {
		t.Fatalf("expected cache_control in system array, got %s", sysStr)
	}
	if !strings.Contains(sysStr, `"ephemeral"`) {
		t.Fatalf("expected ephemeral cache type in system, got %s", sysStr)
	}
}

func TestToAPIRequest_ShortSystemPrompt(t *testing.T) {
	req := core.QueryRequest{
		SystemPrompt: "Short prompt.",
		Messages:     []core.Message{{Role: "user", Content: "Hello"}},
	}

	apiReq := toAPIRequest(req)

	// Short prompt should be a plain string
	_, ok := apiReq.System.(string)
	if !ok {
		t.Fatalf("expected string system for short prompt, got %T", apiReq.System)
	}
}

func TestToAPIRequest_ToolsSortedAlphabetically(t *testing.T) {
	req := core.QueryRequest{
		SystemPrompt: "Test.",
		Messages:     []core.Message{{Role: "user", Content: "Hi"}},
		Tools: []core.ToolDefinition{
			{Name: "web", Description: "web tool", InputSchema: json.RawMessage(`{}`)},
			{Name: "bash", Description: "bash tool", InputSchema: json.RawMessage(`{}`)},
			{Name: "file_read", Description: "read tool", InputSchema: json.RawMessage(`{}`)},
			{Name: "search", Description: "search tool", InputSchema: json.RawMessage(`{}`)},
		},
	}

	apiReq := toAPIRequest(req)

	expected := []string{"bash", "file_read", "search", "web"}
	for i, tool := range apiReq.Tools {
		if tool.Name != expected[i] {
			t.Fatalf("tool[%d]: expected %q, got %q", i, expected[i], tool.Name)
		}
	}
}

func TestToAPIRequest_LastToolHasCacheControl(t *testing.T) {
	req := core.QueryRequest{
		SystemPrompt: "Test.",
		Messages:     []core.Message{{Role: "user", Content: "Hi"}},
		Tools: []core.ToolDefinition{
			{Name: "bash", Description: "bash", InputSchema: json.RawMessage(`{}`)},
			{Name: "web", Description: "web", InputSchema: json.RawMessage(`{}`)},
			{Name: "search", Description: "search", InputSchema: json.RawMessage(`{}`)},
		},
	}

	apiReq := toAPIRequest(req)

	// After sort: bash, search, web — only "web" (last) should have cache_control
	for i, tool := range apiReq.Tools {
		if i < len(apiReq.Tools)-1 {
			if tool.CacheControl != nil {
				t.Fatalf("tool[%d] %q should NOT have cache_control", i, tool.Name)
			}
		} else {
			if tool.CacheControl == nil {
				t.Fatalf("last tool %q should have cache_control", tool.Name)
			}
			if tool.CacheControl.Type != "ephemeral" {
				t.Fatalf("expected cache_control type 'ephemeral', got %q", tool.CacheControl.Type)
			}
		}
	}
}

func TestToAPIRequest_EmptyTools_NoCacheControl(t *testing.T) {
	req := core.QueryRequest{
		SystemPrompt: "Test.",
		Messages:     []core.Message{{Role: "user", Content: "Hi"}},
		Tools:        []core.ToolDefinition{},
	}

	apiReq := toAPIRequest(req)

	if len(apiReq.Tools) != 0 {
		t.Fatalf("expected 0 tools, got %d", len(apiReq.Tools))
	}
}

func TestToAPIRequest_SingleTool_HasCacheControl(t *testing.T) {
	req := core.QueryRequest{
		SystemPrompt: "Test.",
		Messages:     []core.Message{{Role: "user", Content: "Hi"}},
		Tools: []core.ToolDefinition{
			{Name: "bash", Description: "bash", InputSchema: json.RawMessage(`{}`)},
		},
	}

	apiReq := toAPIRequest(req)

	if len(apiReq.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(apiReq.Tools))
	}
	if apiReq.Tools[0].CacheControl == nil {
		t.Fatal("single tool should have cache_control")
	}
	if apiReq.Tools[0].CacheControl.Type != "ephemeral" {
		t.Fatalf("expected 'ephemeral', got %q", apiReq.Tools[0].CacheControl.Type)
	}
}

func TestToAPIRequest_CacheControlJSON(t *testing.T) {
	req := core.QueryRequest{
		SystemPrompt: "Test.",
		Messages:     []core.Message{{Role: "user", Content: "Hi"}},
		Tools: []core.ToolDefinition{
			{Name: "bash", Description: "bash", InputSchema: json.RawMessage(`{}`)},
			{Name: "web", Description: "web", InputSchema: json.RawMessage(`{}`)},
		},
	}

	apiReq := toAPIRequest(req)
	data, err := json.Marshal(apiReq)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	jsonStr := string(data)

	// "bash" tool (first, not last) should NOT have cache_control in JSON
	// "web" tool (last) should have cache_control
	if !strings.Contains(jsonStr, `"cache_control":{"type":"ephemeral"}`) {
		t.Fatal("expected cache_control in JSON output")
	}

	// Verify only the last tool has it by unmarshaling back
	var raw struct {
		Tools []json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// First tool should not contain cache_control
	if strings.Contains(string(raw.Tools[0]), "cache_control") {
		t.Fatal("first tool should not have cache_control in JSON")
	}
	// Last tool should contain cache_control
	if !strings.Contains(string(raw.Tools[1]), "cache_control") {
		t.Fatal("last tool should have cache_control in JSON")
	}
}

func TestToAPIRequest_LongSystemPrompt(t *testing.T) {
	req := core.QueryRequest{
		SystemPrompt: strings.Repeat("x", 5000), // >1024 estimated tokens
		Messages:     []core.Message{{Role: "user", Content: "Hello"}},
	}

	apiReq := toAPIRequest(req)

	// Long prompt should be []apiSystemBlock with cache_control
	blocks, ok := apiReq.System.([]apiSystemBlock)
	if !ok {
		t.Fatalf("expected []apiSystemBlock for long prompt, got %T", apiReq.System)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].CacheControl == nil || blocks[0].CacheControl.Type != "ephemeral" {
		t.Fatal("expected ephemeral cache_control on long system prompt")
	}
}
