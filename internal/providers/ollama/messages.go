// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package ollama

import (
	"siply.dev/siply/internal/core"
)

// apiRequest is the Ollama Chat API request body.
type apiRequest struct {
	Model    string       `json:"model"`
	Stream   bool         `json:"stream"`
	Messages []apiMessage `json:"messages"`
}

// apiMessage is a single message in the Ollama format.
type apiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// toAPIRequest converts the internal QueryRequest to the Ollama API format.
func toAPIRequest(req core.QueryRequest) apiRequest {
	var msgs []apiMessage

	// System prompt goes as a "system" role message.
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

	model := req.Model
	if model == "" {
		model = "llama3.2"
	}

	return apiRequest{
		Model:    model,
		Stream:   true,
		Messages: msgs,
	}
}
