// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package anthropic

import (
	"encoding/json"
	"os"
	"sort"
	"strings"

	"siply.dev/siply/internal/core"
)

// Minimum token thresholds for Anthropic prompt caching.
// Prompts below these thresholds are sent as plain strings.
const (
	cacheMinTokensSonnet = 1024
	cacheMinTokensOpus   = 2048
)

// estimateTokens provides a rough token count estimate.
// Anthropic tokenizers average ~4 characters per token for English text.
func estimateTokens(text string) int {
	return len(text) / 4
}

// isOpusModel returns true if the model name indicates an Opus-class model.
func isOpusModel(model string) bool {
	return strings.Contains(model, "claude-opus")
}

// buildSystemField constructs the system field for the API request.
// For prompts above the caching threshold, it returns []apiSystemBlock with
// cache_control. For shorter prompts, it returns the plain string.
func buildSystemField(prompt string, model string) any {
	if prompt == "" {
		return nil
	}

	tokens := estimateTokens(prompt)
	threshold := cacheMinTokensSonnet
	if isOpusModel(model) {
		threshold = cacheMinTokensOpus
	}

	if tokens < threshold {
		return prompt
	}

	return []apiSystemBlock{{
		Type: "text",
		Text: prompt,
		CacheControl: &apiCacheControl{
			Type: "ephemeral",
		},
	}}
}

// apiRequest is the Anthropic Messages API request body.
type apiRequest struct {
	Model       string       `json:"model"`
	MaxTokens   int          `json:"max_tokens"`
	Stream      bool         `json:"stream"`
	System      any          `json:"system,omitempty"` // string or []apiSystemBlock
	Messages    []apiMessage `json:"messages"`
	Tools       []apiTool    `json:"tools,omitempty"`
	Temperature *float64     `json:"temperature,omitempty"`
}

// apiCacheControl represents the cache_control field for Anthropic prompt caching.
type apiCacheControl struct {
	Type string `json:"type"`
}

// apiSystemBlock is a content block in the system prompt array format,
// used when prompt caching is enabled.
type apiSystemBlock struct {
	Type         string           `json:"type"`
	Text         string           `json:"text"`
	CacheControl *apiCacheControl `json:"cache_control,omitempty"`
}

// apiMessage is a single message in the Anthropic format.
// Content is any because Anthropic accepts either a plain string
// or an array of content blocks (for tool_use / tool_result).
type apiMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// apiContentBlock represents a structured content block in the Anthropic format.
type apiContentBlock struct {
	Type         string           `json:"type"`
	Text         string           `json:"text,omitempty"`
	ID           string           `json:"id,omitempty"`
	Name         string           `json:"name,omitempty"`
	Input        json.RawMessage  `json:"input,omitempty"`
	ToolUseID    string           `json:"tool_use_id,omitempty"`
	Content      string           `json:"content,omitempty"`
	IsError      bool             `json:"is_error,omitempty"`
	CacheControl *apiCacheControl `json:"cache_control,omitempty"`
}

// apiTool is a tool definition in the Anthropic format.
type apiTool struct {
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	InputSchema  json.RawMessage  `json:"input_schema"`
	CacheControl *apiCacheControl `json:"cache_control,omitempty"`
}

// toAPIRequest converts the internal QueryRequest to the Anthropic API format.
func toAPIRequest(req core.QueryRequest) apiRequest {
	msgs := make([]apiMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = convertMessage(m)
	}

	tools := make([]apiTool, len(req.Tools))
	for i, t := range req.Tools {
		tools[i] = apiTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
	}

	// Sort tools alphabetically for deterministic ordering (enables prefix caching).
	// TODO(PC-2.1/AC-3): When MCP/plugin tools are merged into ListTools(),
	// partition static tools first (sorted, cached) and dynamic tools after
	// (outside cached prefix) before applying cache_control.
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name < tools[j].Name
	})

	// Apply cache_control on the last sorted tool to mark the end of the
	// cacheable prefix (system prompt + tool definitions).
	if len(tools) > 0 {
		tools[len(tools)-1].CacheControl = &apiCacheControl{Type: "ephemeral"}
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	model := req.Model
	if model == "" {
		if envModel := os.Getenv("SIPLY_MODEL"); envModel != "" {
			model = envModel
		} else {
			model = "claude-sonnet-4-20250514"
		}
	}

	return apiRequest{
		Model:       model,
		MaxTokens:   maxTokens,
		Stream:      true,
		System:      buildSystemField(req.SystemPrompt, model),
		Messages:    msgs,
		Tools:       tools,
		Temperature: req.Temperature,
	}
}

// convertMessage converts a core.Message to an apiMessage, handling both
// plain text messages and messages with tool calls or tool results.
func convertMessage(m core.Message) apiMessage {
	// Assistant message with tool calls → content block array
	if len(m.ToolCalls) > 0 {
		var blocks []apiContentBlock
		if m.Content != "" {
			blocks = append(blocks, apiContentBlock{
				Type: "text",
				Text: m.Content,
			})
		}
		for _, tc := range m.ToolCalls {
			input := tc.Input
			if input == nil {
				input = json.RawMessage("{}")
			}
			blocks = append(blocks, apiContentBlock{
				Type:  "tool_use",
				ID:    tc.ToolID,
				Name:  tc.ToolName,
				Input: input,
			})
		}
		return apiMessage{Role: m.Role, Content: blocks}
	}

	// User message with tool results → content block array
	if len(m.ToolResults) > 0 {
		blocks := make([]apiContentBlock, len(m.ToolResults))
		for i, tr := range m.ToolResults {
			blocks[i] = apiContentBlock{
				Type:      "tool_result",
				ToolUseID: tr.ToolID,
				Content:   tr.Content,
				IsError:   tr.IsError,
			}
		}
		return apiMessage{Role: m.Role, Content: blocks}
	}

	// Plain text message
	return apiMessage{Role: m.Role, Content: m.Content}
}
