// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package openai

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"

	"siply.dev/siply/internal/core"
)

// apiRequest is the OpenAI Chat Completions API request body.
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

// apiMessage is a single message in the OpenAI format.
type apiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// apiTool is a tool definition in the OpenAI format.
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

// sortByFunctionName implements sort.Interface for []apiTool ordered by Function.Name.
type sortByFunctionName []apiTool

func (s sortByFunctionName) Len() int           { return len(s) }
func (s sortByFunctionName) Less(i, j int) bool { return s[i].Function.Name < s[j].Function.Name }
func (s sortByFunctionName) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// dateTimePattern matches "YYYY-MM-DD HH:MM" or "YYYY-MM-DD HH:MM:SS" with optional
// trailing timezone: abbreviated (e.g., "UTC", "EST", "CEST") or numeric offset (e.g., "+0200", "-05:00").
var dateTimePattern = regexp.MustCompile(`\b(\d{4}-\d{2}-\d{2})\s+\d{2}:\d{2}(?::\d{2})?(?:\s+(?:[A-Z]{2,5}|[+-]\d{2}:?\d{2}))?\b`)

// uuidPattern matches UUID-like session identifiers (case-insensitive).
var uuidPattern = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)

// buildStableSystemMessage strips known dynamic patterns from systemPrompt.
// Full datetime strings (YYYY-MM-DD HH:MM[:SS] [TZ]) are replaced with date-only
// strings (YYYY-MM-DD). UUID-like session identifiers are replaced with "<id>".
// Returns the stable portion suitable for OpenAI prefix caching.
func buildStableSystemMessage(systemPrompt string) string {
	// Replace "YYYY-MM-DD HH:MM:SS [TZ]" → "YYYY-MM-DD" (keep date, drop time+tz).
	s := dateTimePattern.ReplaceAllString(systemPrompt, "$1")
	// Replace UUID-like session identifiers with stable sentinel.
	s = uuidPattern.ReplaceAllString(s, "<id>")
	return strings.TrimSpace(s)
}

// toAPIRequest converts the internal QueryRequest to the OpenAI API format.
func toAPIRequest(req core.QueryRequest) apiRequest {
	var msgs []apiMessage

	// Compute frozen date for <system-reminder>. Use TaskStartTime if set
	// (frozen once per task at agent loop level), otherwise fall back to today's date.
	// Date-only format ensures stability across all turns of a single task.
	frozenTime := req.TaskStartTime
	if frozenTime.IsZero() {
		frozenTime = time.Now()
	}
	frozenDate := frozenTime.Format("2006-01-02")

	// Apply relocation trick: strip dynamic patterns from system prompt so the
	// system message is byte-identical across turns (enables OpenAI prefix caching).
	// If the conversation has no user message, keep the system prompt intact so
	// the model still receives date context (relocation requires a user turn to land in).
	hasUserMessage := false
	for _, m := range req.Messages {
		if m.Role == "user" {
			hasUserMessage = true
			break
		}
	}
	if req.SystemPrompt != "" {
		var systemContent string
		if hasUserMessage {
			systemContent = buildStableSystemMessage(req.SystemPrompt)
		} else {
			systemContent = req.SystemPrompt
		}
		msgs = append(msgs, apiMessage{
			Role:    "system",
			Content: systemContent,
		})
	}

	// Append <system-reminder> with frozen date to the first user message.
	// This relocates dynamic date context out of the system message.
	reminder := "\n\n<system-reminder>Current date: " + frozenDate + "</system-reminder>"
	firstUserFound := false
	for _, m := range req.Messages {
		msg := apiMessage{Role: m.Role, Content: m.Content}
		if !firstUserFound && m.Role == "user" {
			msg.Content += reminder
			firstUserFound = true
		}
		msgs = append(msgs, msg)
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

	// Sort tools alphabetically for deterministic serialization (enables OpenAI prefix caching).
	// Tool names are unique by OpenAI spec, so stable and unstable sort produce identical output;
	// sort.Stable is used for snapshot-test determinism and defensive correctness.
	sort.Stable(sortByFunctionName(tools))

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	model := req.Model
	if model == "" {
		model = "gpt-4o"
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
