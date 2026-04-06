// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package anthropic

import (
	"encoding/json"

	"siply.dev/siply/internal/core"
)

// apiRequest is the Anthropic Messages API request body.
type apiRequest struct {
	Model       string       `json:"model"`
	MaxTokens   int          `json:"max_tokens"`
	Stream      bool         `json:"stream"`
	System      string       `json:"system,omitempty"`
	Messages    []apiMessage `json:"messages"`
	Tools       []apiTool    `json:"tools,omitempty"`
	Temperature *float64     `json:"temperature,omitempty"`
}

// apiMessage is a single message in the Anthropic format.
type apiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// apiTool is a tool definition in the Anthropic format.
type apiTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// toAPIRequest converts the internal QueryRequest to the Anthropic API format.
func toAPIRequest(req core.QueryRequest) apiRequest {
	msgs := make([]apiMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = apiMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	tools := make([]apiTool, len(req.Tools))
	for i, t := range req.Tools {
		tools[i] = apiTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	model := req.Model
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}

	return apiRequest{
		Model:       model,
		MaxTokens:   maxTokens,
		Stream:      true,
		System:      req.SystemPrompt,
		Messages:    msgs,
		Tools:       tools,
		Temperature: req.Temperature,
	}
}
