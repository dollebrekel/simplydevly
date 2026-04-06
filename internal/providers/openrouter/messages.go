// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package openrouter

import (
	"encoding/json"

	"siply.dev/siply/internal/core"
)

// apiRequest is the OpenRouter API request body (OpenAI-compatible format).
type apiRequest struct {
	Model         string       `json:"model"`
	Stream        bool         `json:"stream"`
	StreamOptions *streamOpts  `json:"stream_options,omitempty"`
	Messages      []apiMessage `json:"messages"`
	Tools         []apiTool    `json:"tools,omitempty"`
	MaxTokens     int          `json:"max_tokens,omitempty"`
	Temperature   *float64     `json:"temperature,omitempty"`
}

// streamOpts configures streaming behavior.
type streamOpts struct {
	IncludeUsage bool `json:"include_usage"`
}

// apiMessage is a single message in the OpenAI-compatible format.
type apiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// apiTool is a tool definition in the OpenAI-compatible format.
type apiTool struct {
	Type     string      `json:"type"`
	Function apiFunction `json:"function"`
}

// apiFunction is the function definition within a tool.
type apiFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// toAPIRequest converts the internal QueryRequest to the OpenRouter API format.
func toAPIRequest(req core.QueryRequest) apiRequest {
	var msgs []apiMessage

	if req.SystemPrompt != "" {
		msgs = append(msgs, apiMessage{
			Role:    "system",
			Content: req.SystemPrompt,
		})
	}

	for _, m := range req.Messages {
		msgs = append(msgs, apiMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	var tools []apiTool
	for _, t := range req.Tools {
		tools = append(tools, apiTool{
			Type: "function",
			Function: apiFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	model := req.Model
	if model == "" {
		model = "anthropic/claude-sonnet-4-20250514"
	}

	return apiRequest{
		Model:         model,
		Stream:        true,
		StreamOptions: &streamOpts{IncludeUsage: true},
		Messages:      msgs,
		Tools:         tools,
		MaxTokens:     maxTokens,
		Temperature:   req.Temperature,
	}
}
