// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package kimi

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"unicode/utf8"

	"siply.dev/siply/internal/core"
)

// apiRequest is the Kimi Chat Completions API request body (OpenAI-compatible).
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

// apiMessage is a single message in the Kimi API format.
// Content may be a plain string or a content-block array (for cache references).
type apiMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// cacheContentBlock is a content block that references a Kimi context cache.
type cacheContentBlock struct {
	Type    string `json:"type"`
	CacheID string `json:"cache_id"`
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

// cacheCreateRequest is the payload for POST /v1/caching.
type cacheCreateRequest struct {
	Model    string       `json:"model"`
	Messages []apiMessage `json:"messages"`
	TTL      int          `json:"ttl,omitempty"`
}

// cacheCreateResponse is the response from POST /v1/caching.
type cacheCreateResponse struct {
	CacheID  string `json:"cache_id"`
	ExpireAt int64  `json:"expire_at"`
	Tokens   int    `json:"tokens"`
	Status   string `json:"status"`
}

// buildAPITools converts core.ToolDefinition slice to apiTool slice, sorted
// alphabetically by name for deterministic ordering (enables prefix caching).
func buildAPITools(tools []core.ToolDefinition) []apiTool {
	result := make([]apiTool, 0, len(tools))
	for _, t := range tools {
		result = append(result, apiTool{
			Type: "function",
			Function: apiFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Function.Name < result[j].Function.Name
	})
	return result
}

// toAPIRequest converts the internal QueryRequest to the Kimi API format.
// When cacheID is non-empty, the system prompt is replaced by a cache reference
// message (role "cache") so the content is not repeated in full (AC #3).
func toAPIRequest(req core.QueryRequest, apiTools []apiTool, cacheID string) apiRequest {
	var msgs []apiMessage

	if cacheID != "" {
		// With active cache: inject cache reference instead of full system prompt.
		msgs = append(msgs, apiMessage{
			Role: "cache",
			Content: []cacheContentBlock{
				{Type: "cache", CacheID: cacheID},
			},
		})
	} else if req.SystemPrompt != "" {
		// Without cache: send system prompt inline.
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

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	model := req.Model
	if model == "" {
		if envModel := os.Getenv("SIPLY_MODEL"); envModel != "" {
			model = envModel
		} else {
			model = "moonshot-v1-128k"
		}
	}

	return apiRequest{
		Model:         model,
		Stream:        true,
		StreamOptions: &streamOpts{IncludeUsage: true},
		Messages:      msgs,
		Tools:         apiTools,
		MaxTokens:     maxTokens,
		Temperature:   req.Temperature,
	}
}

// buildCacheRequest prepares the payload for POST /v1/caching.
// It includes the system prompt and serialised tool definitions as the
// stable, cacheable context.
func buildCacheRequest(model, systemPrompt string, tools []apiTool) (cacheCreateRequest, error) {
	var msgs []apiMessage

	if systemPrompt != "" {
		msgs = append(msgs, apiMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	// Include tool definitions as a "user" turn so they are part of the cached
	// context. This matches the pattern documented in the Kimi API reference.
	if len(tools) > 0 {
		toolsJSON, err := json.Marshal(tools)
		if err != nil {
			return cacheCreateRequest{}, fmt.Errorf("kimi cache: marshal tools: %w", err)
		}
		msgs = append(msgs, apiMessage{
			Role:    "user",
			Content: string(toolsJSON),
		})
	}

	if model == "" {
		model = "moonshot-v1-128k"
	}

	return cacheCreateRequest{
		Model:    model,
		Messages: msgs,
		TTL:      3600, // 1 hour default
	}, nil
}

// estimateTokens provides a rough token estimate for the system prompt and
// tool definitions combined. Uses a rune-based heuristic (4 runes per token on
// average) to decide whether to attempt context caching. Rune count is used
// instead of byte length so that multibyte UTF-8 text (e.g. Chinese, Japanese)
// is not over-counted.
func estimateTokens(systemPrompt string, tools []apiTool) int {
	chars := utf8.RuneCountInString(systemPrompt)
	for _, t := range tools {
		chars += utf8.RuneCountInString(t.Function.Name) + utf8.RuneCountInString(t.Function.Description)
		if t.Function.Parameters != nil {
			chars += utf8.RuneCount(t.Function.Parameters)
		}
	}
	return chars / 4
}
