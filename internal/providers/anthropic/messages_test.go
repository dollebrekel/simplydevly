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

// --- PC-2.2: Conversation Sliding Window Cache Tests ---

func TestBreakpointBudget_Counting(t *testing.T) {
	// No system blocks, no tools → 4 remaining
	if got := breakpointBudget("plain string", 0); got != 4 {
		t.Fatalf("no system blocks, no tools: expected 4, got %d", got)
	}

	// System blocks, no tools → 3 remaining
	sys := []apiSystemBlock{{Type: "text", Text: "x", CacheControl: &apiCacheControl{Type: "ephemeral"}}}
	if got := breakpointBudget(sys, 0); got != 3 {
		t.Fatalf("system blocks, no tools: expected 3, got %d", got)
	}

	// No system blocks, with tools → 3 remaining
	if got := breakpointBudget("plain", 5); got != 3 {
		t.Fatalf("no system blocks, with tools: expected 3, got %d", got)
	}

	// System blocks + tools → 2 remaining
	if got := breakpointBudget(sys, 3); got != 2 {
		t.Fatalf("system blocks + tools: expected 2, got %d", got)
	}
}

func TestToAPIRequest_ConversationCacheControl_LastToolResult(t *testing.T) {
	req := core.QueryRequest{
		SystemPrompt: "Test.",
		Messages: []core.Message{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", ToolCalls: []core.ToolCall{
				{ToolID: "tc1", ToolName: "bash", Input: json.RawMessage(`{"cmd":"ls"}`)},
			}},
			{Role: "user", ToolResults: []core.ToolResult{
				{ToolID: "tc1", Content: "file1.go\nfile2.go"},
			}},
			{Role: "assistant", Content: "I see those files."},
		},
	}

	apiReq := toAPIRequest(req)

	// Message at index 2 is the tool_result message — its last block should have cache_control
	blocks, ok := apiReq.Messages[2].Content.([]apiContentBlock)
	if !ok {
		t.Fatalf("expected []apiContentBlock for tool result message, got %T", apiReq.Messages[2].Content)
	}
	lastBlock := blocks[len(blocks)-1]
	if lastBlock.CacheControl == nil {
		t.Fatal("expected cache_control on last block of last tool_result message")
	}
	if lastBlock.CacheControl.Type != "ephemeral" {
		t.Fatalf("expected cache_control type 'ephemeral', got %q", lastBlock.CacheControl.Type)
	}
}

func TestToAPIRequest_ConversationCacheControl_OnlyLastMessage(t *testing.T) {
	req := core.QueryRequest{
		SystemPrompt: "Test.",
		Messages: []core.Message{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", ToolCalls: []core.ToolCall{
				{ToolID: "tc1", ToolName: "bash", Input: json.RawMessage(`{}`)},
			}},
			{Role: "user", ToolResults: []core.ToolResult{
				{ToolID: "tc1", Content: "result1"},
			}},
			{Role: "assistant", ToolCalls: []core.ToolCall{
				{ToolID: "tc2", ToolName: "read", Input: json.RawMessage(`{}`)},
			}},
			{Role: "user", ToolResults: []core.ToolResult{
				{ToolID: "tc2", Content: "result2"},
			}},
			{Role: "assistant", Content: "Done."},
		},
	}

	apiReq := toAPIRequest(req)

	// First tool_result message (index 2) should NOT have cache_control
	blocks1, ok := apiReq.Messages[2].Content.([]apiContentBlock)
	if !ok {
		t.Fatalf("expected []apiContentBlock for first tool result, got %T", apiReq.Messages[2].Content)
	}
	if blocks1[len(blocks1)-1].CacheControl != nil {
		t.Fatal("first tool_result message should NOT have cache_control")
	}

	// Second tool_result message (index 4) SHOULD have cache_control
	blocks2, ok := apiReq.Messages[4].Content.([]apiContentBlock)
	if !ok {
		t.Fatalf("expected []apiContentBlock for second tool result, got %T", apiReq.Messages[4].Content)
	}
	if blocks2[len(blocks2)-1].CacheControl == nil {
		t.Fatal("last tool_result message should have cache_control")
	}
	if blocks2[len(blocks2)-1].CacheControl.Type != "ephemeral" {
		t.Fatalf("expected 'ephemeral', got %q", blocks2[len(blocks2)-1].CacheControl.Type)
	}
}

func TestToAPIRequest_ConversationCacheControl_NoToolResults(t *testing.T) {
	req := core.QueryRequest{
		SystemPrompt: "Test.",
		Messages: []core.Message{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there!"},
			{Role: "user", Content: "How are you?"},
		},
	}

	apiReq := toAPIRequest(req)

	// No tool results → no conversation cache_control on any message
	for i, msg := range apiReq.Messages {
		if blocks, ok := msg.Content.([]apiContentBlock); ok {
			for j, b := range blocks {
				if b.CacheControl != nil {
					t.Fatalf("message[%d] block[%d] should not have cache_control (no tool results)", i, j)
				}
			}
		}
	}
}

func TestToAPIRequest_ConversationCacheControl_SingleTurn(t *testing.T) {
	req := core.QueryRequest{
		SystemPrompt: "Test.",
		Messages: []core.Message{
			{Role: "user", Content: "Hello"},
		},
	}

	apiReq := toAPIRequest(req)

	if len(apiReq.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(apiReq.Messages))
	}

	// Single turn, plain text — no cache_control
	if _, ok := apiReq.Messages[0].Content.(string); !ok {
		t.Fatalf("expected string content for single user message, got %T", apiReq.Messages[0].Content)
	}
}

func TestToAPIRequest_BreakpointBudget_NeverExceedsFour(t *testing.T) {
	// Long system prompt (= 1 breakpoint) + tools (= 1 breakpoint) + tool results
	// Should still work: 2 used, 2 remaining, conversation gets 1 → total 3
	req := core.QueryRequest{
		SystemPrompt: strings.Repeat("x", 5000), // long prompt → system breakpoint
		Messages: []core.Message{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", ToolCalls: []core.ToolCall{
				{ToolID: "tc1", ToolName: "bash", Input: json.RawMessage(`{}`)},
			}},
			{Role: "user", ToolResults: []core.ToolResult{
				{ToolID: "tc1", Content: "output"},
			}},
		},
		Tools: []core.ToolDefinition{
			{Name: "bash", Description: "bash", InputSchema: json.RawMessage(`{}`)},
		},
	}

	apiReq := toAPIRequest(req)

	// Count all breakpoints
	total := 0

	// System
	if blocks, ok := apiReq.System.([]apiSystemBlock); ok {
		for _, b := range blocks {
			if b.CacheControl != nil {
				total++
			}
		}
	}

	// Tools
	for _, tool := range apiReq.Tools {
		if tool.CacheControl != nil {
			total++
		}
	}

	// Messages
	for _, msg := range apiReq.Messages {
		if blocks, ok := msg.Content.([]apiContentBlock); ok {
			for _, b := range blocks {
				if b.CacheControl != nil {
					total++
				}
			}
		}
	}

	if total > 4 {
		t.Fatalf("total breakpoints %d exceeds maximum of 4", total)
	}

	// In this case: system(1) + tools(1) + conversation(1) = 3
	if total != 3 {
		t.Fatalf("expected 3 total breakpoints (system+tools+conversation), got %d", total)
	}
}

// --- PC-2.3: Intermediate Breakpoints Tests ---

// makeToolSteps creates n tool-call/tool-result step pairs (assistant tool_use + user tool_result).
// Each step produces 2 messages and 2 content blocks.
func makeToolSteps(n int) []core.Message {
	msgs := []core.Message{{Role: "user", Content: "Hello"}} // initial user message
	for i := 0; i < n; i++ {
		id := "tc" + strings.Repeat("x", i) // unique IDs
		msgs = append(msgs, core.Message{
			Role: "assistant",
			ToolCalls: []core.ToolCall{
				{ToolID: id, ToolName: "bash", Input: json.RawMessage(`{}`)},
			},
		})
		msgs = append(msgs, core.Message{
			Role: "user",
			ToolResults: []core.ToolResult{
				{ToolID: id, Content: "output"},
			},
		})
	}
	return msgs
}

// countConversationBreakpoints counts cache_control breakpoints in message content blocks.
func countConversationBreakpoints(msgs []apiMessage) int {
	count := 0
	for _, msg := range msgs {
		if blocks, ok := msg.Content.([]apiContentBlock); ok {
			for _, b := range blocks {
				if b.CacheControl != nil {
					count++
				}
			}
		}
	}
	return count
}

func TestPlaceConversationBreakpoints_ShortSession(t *testing.T) {
	// 10 steps = 20 blocks (well below threshold of 54)
	steps := makeToolSteps(10)
	msgs := make([]apiMessage, len(steps))
	for i, m := range steps {
		msgs[i] = convertMessage(m)
	}

	placed := placeConversationBreakpoints(msgs, 2)

	// Short session: should only place trailing breakpoint
	if placed != 1 {
		t.Fatalf("short session: expected 1 breakpoint placed, got %d", placed)
	}

	total := countConversationBreakpoints(msgs)
	if total != 1 {
		t.Fatalf("short session: expected 1 total conversation breakpoint, got %d", total)
	}

	// Verify it's on the last tool_result
	lastTRMsg := msgs[len(msgs)-1] // last message is a tool_result
	blocks := lastTRMsg.Content.([]apiContentBlock)
	if blocks[0].CacheControl == nil {
		t.Fatal("short session: expected breakpoint on last tool_result")
	}
}

func TestPlaceConversationBreakpoints_LongSession_IntermediateAdded(t *testing.T) {
	// 30 steps = 60+ blocks (above threshold of 54)
	steps := makeToolSteps(30)
	msgs := make([]apiMessage, len(steps))
	for i, m := range steps {
		msgs[i] = convertMessage(m)
	}

	placed := placeConversationBreakpoints(msgs, 2)

	// Budget=2: should place 1 intermediate + 1 trailing
	if placed != 2 {
		t.Fatalf("long session budget=2: expected 2 breakpoints placed, got %d", placed)
	}

	total := countConversationBreakpoints(msgs)
	if total != 2 {
		t.Fatalf("long session budget=2: expected 2 total conversation breakpoints, got %d", total)
	}

	// Verify the last tool_result always has a breakpoint (trailing)
	for i := len(msgs) - 1; i >= 0; i-- {
		blocks, ok := msgs[i].Content.([]apiContentBlock)
		if !ok {
			continue
		}
		for _, b := range blocks {
			if b.Type == "tool_result" {
				if b.CacheControl == nil {
					t.Fatal("trailing breakpoint missing on last tool_result")
				}
				goto foundTrailing
			}
		}
	}
	t.Fatal("no tool_result found in messages")
foundTrailing:

	// Verify an intermediate breakpoint exists before the trailing one
	intermediateFound := false
	for i := 0; i < len(msgs)-2; i++ { // exclude last 2 messages (last step)
		blocks, ok := msgs[i].Content.([]apiContentBlock)
		if !ok {
			continue
		}
		for _, b := range blocks {
			if b.CacheControl != nil {
				intermediateFound = true
			}
		}
	}
	if !intermediateFound {
		t.Fatal("expected at least one intermediate breakpoint before trailing")
	}
}

func TestPlaceConversationBreakpoints_BudgetRespected(t *testing.T) {
	// 30 steps, budget=1: only trailing
	steps := makeToolSteps(30)
	msgs1 := make([]apiMessage, len(steps))
	for i, m := range steps {
		msgs1[i] = convertMessage(m)
	}

	placed := placeConversationBreakpoints(msgs1, 1)
	if placed != 1 {
		t.Fatalf("budget=1: expected 1, got %d", placed)
	}
	if countConversationBreakpoints(msgs1) != 1 {
		t.Fatalf("budget=1: expected 1 total breakpoint, got %d", countConversationBreakpoints(msgs1))
	}

	// 30 steps, budget=2: 1 intermediate + 1 trailing
	msgs2 := make([]apiMessage, len(steps))
	for i, m := range steps {
		msgs2[i] = convertMessage(m)
	}

	placed = placeConversationBreakpoints(msgs2, 2)
	if placed != 2 {
		t.Fatalf("budget=2: expected 2, got %d", placed)
	}
	if countConversationBreakpoints(msgs2) != 2 {
		t.Fatalf("budget=2: expected 2 total breakpoints, got %d", countConversationBreakpoints(msgs2))
	}

	// 30 steps, budget=3: up to 2 intermediate + 1 trailing
	msgs3 := make([]apiMessage, len(steps))
	for i, m := range steps {
		msgs3[i] = convertMessage(m)
	}

	placed = placeConversationBreakpoints(msgs3, 3)
	if placed > 3 {
		t.Fatalf("budget=3: placed %d breakpoints, exceeds budget", placed)
	}
	if placed < 2 {
		t.Fatalf("budget=3: expected at least 2 breakpoints (1 intermediate + 1 trailing), got %d", placed)
	}
}

func TestPlaceConversationBreakpoints_NeverExceedsFour(t *testing.T) {
	// Full scenario: system(1) + tools(1) + conversation(budget=2)
	// Total must never exceed 4
	req := core.QueryRequest{
		SystemPrompt: strings.Repeat("x", 5000), // system breakpoint
		Messages:     makeToolSteps(30),          // long session
		Tools: []core.ToolDefinition{
			{Name: "bash", Description: "bash", InputSchema: json.RawMessage(`{}`)},
		},
	}

	apiReq := toAPIRequest(req)

	total := 0
	// System
	if blocks, ok := apiReq.System.([]apiSystemBlock); ok {
		for _, b := range blocks {
			if b.CacheControl != nil {
				total++
			}
		}
	}
	// Tools
	for _, tool := range apiReq.Tools {
		if tool.CacheControl != nil {
			total++
		}
	}
	// Messages
	total += countConversationBreakpoints(apiReq.Messages)

	if total > 4 {
		t.Fatalf("total breakpoints %d exceeds maximum of 4", total)
	}
}

func TestPlaceConversationBreakpoints_TrailingAlwaysLast(t *testing.T) {
	// 30 steps with budget=2
	steps := makeToolSteps(30)
	msgs := make([]apiMessage, len(steps))
	for i, m := range steps {
		msgs[i] = convertMessage(m)
	}

	placeConversationBreakpoints(msgs, 2)

	// Find the last tool_result in the entire message list
	lastTRMsgIdx := -1
	lastTRBlockIdx := -1
	for i := len(msgs) - 1; i >= 0; i-- {
		blocks, ok := msgs[i].Content.([]apiContentBlock)
		if !ok {
			continue
		}
		for j := len(blocks) - 1; j >= 0; j-- {
			if blocks[j].Type == "tool_result" {
				lastTRMsgIdx = i
				lastTRBlockIdx = j
				goto found
			}
		}
	}
found:
	if lastTRMsgIdx < 0 {
		t.Fatal("no tool_result found")
	}

	blocks := msgs[lastTRMsgIdx].Content.([]apiContentBlock)
	if blocks[lastTRBlockIdx].CacheControl == nil {
		t.Fatal("last tool_result must always have cache_control (trailing breakpoint)")
	}
}

func TestPlaceConversationBreakpoints_StablePositions(t *testing.T) {
	// Place breakpoints for 30 steps
	steps30 := makeToolSteps(30)
	msgs30 := make([]apiMessage, len(steps30))
	for i, m := range steps30 {
		msgs30[i] = convertMessage(m)
	}
	placeConversationBreakpoints(msgs30, 2)

	// Record intermediate breakpoint positions (excluding last tool_result = trailing)
	var intermediatePositions30 []int
	lastTRIdx30 := -1
	for i := len(msgs30) - 1; i >= 0; i-- {
		if blocks, ok := msgs30[i].Content.([]apiContentBlock); ok {
			for _, b := range blocks {
				if b.Type == "tool_result" {
					lastTRIdx30 = i
					goto foundLast30
				}
			}
		}
	}
foundLast30:
	for i, msg := range msgs30 {
		if i == lastTRIdx30 {
			continue // skip trailing
		}
		if blocks, ok := msg.Content.([]apiContentBlock); ok {
			for _, b := range blocks {
				if b.CacheControl != nil {
					intermediatePositions30 = append(intermediatePositions30, i)
				}
			}
		}
	}

	// Now add 1 more step (31 steps) and place breakpoints again
	steps31 := makeToolSteps(31)
	msgs31 := make([]apiMessage, len(steps31))
	for i, m := range steps31 {
		msgs31[i] = convertMessage(m)
	}
	placeConversationBreakpoints(msgs31, 2)

	// Record intermediate positions for 31 steps
	var intermediatePositions31 []int
	lastTRIdx31 := -1
	for i := len(msgs31) - 1; i >= 0; i-- {
		if blocks, ok := msgs31[i].Content.([]apiContentBlock); ok {
			for _, b := range blocks {
				if b.Type == "tool_result" {
					lastTRIdx31 = i
					goto foundLast31
				}
			}
		}
	}
foundLast31:
	for i, msg := range msgs31 {
		if i == lastTRIdx31 {
			continue
		}
		if blocks, ok := msg.Content.([]apiContentBlock); ok {
			for _, b := range blocks {
				if b.CacheControl != nil {
					intermediatePositions31 = append(intermediatePositions31, i)
				}
			}
		}
	}

	// Intermediate breakpoints should be at same positions (stability).
	// The first N-1 messages are identical, so intermediate positions should match.
	if len(intermediatePositions30) != len(intermediatePositions31) {
		t.Fatalf("intermediate count changed: %d vs %d (30 vs 31 steps)",
			len(intermediatePositions30), len(intermediatePositions31))
	}
	for i := range intermediatePositions30 {
		if intermediatePositions30[i] != intermediatePositions31[i] {
			t.Fatalf("intermediate position[%d] shifted: %d → %d (should be stable)",
				i, intermediatePositions30[i], intermediatePositions31[i])
		}
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
