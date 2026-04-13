// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package openrouter

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/providers"
)

const maxToolJSONSize = 10 * 1024 * 1024 // 10MB

// toolAccumulator tracks the state of an active tool call being streamed.
type toolAccumulator struct {
	ID      string
	Name    string
	JSONBuf strings.Builder
}

// streamParser reads SSE events from an OpenRouter streaming response.
// OpenRouter uses OpenAI-compatible streaming format.
type streamParser struct {
	scanner *bufio.Scanner

	activeTools map[int]*toolAccumulator
	pending     []core.StreamEvent
	model       string
}

func newStreamParser(r io.Reader) *streamParser {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 64*1024), 1024*1024) // 1MB max line size
	return &streamParser{
		scanner:     s,
		activeTools: make(map[int]*toolAccumulator),
	}
}

// chunkResponse represents a single SSE chunk (OpenAI-compatible format).
type chunkResponse struct {
	ID      string        `json:"id"`
	Model   string        `json:"model"`
	Choices []chunkChoice `json:"choices"`
	Usage   *chunkUsage   `json:"usage,omitempty"`
}

type chunkChoice struct {
	Index        int        `json:"index"`
	Delta        chunkDelta `json:"delta"`
	FinishReason *string    `json:"finish_reason"`
}

type chunkDelta struct {
	Content   string          `json:"content,omitempty"`
	ToolCalls []deltaToolCall `json:"tool_calls,omitempty"`
}

type deltaToolCall struct {
	Index    int             `json:"index"`
	ID       string          `json:"id,omitempty"`
	Type     string          `json:"type,omitempty"`
	Function deltaToolCallFn `json:"function,omitempty"`
}

type deltaToolCallFn struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type promptTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

type chunkUsage struct {
	PromptTokens        int                  `json:"prompt_tokens"`
	CompletionTokens    int                  `json:"completion_tokens"`
	TotalTokens         int                  `json:"total_tokens"`
	PromptTokensDetails *promptTokensDetails `json:"prompt_tokens_details,omitempty"`
}

// next reads the next meaningful StreamEvent from the SSE stream.
func (p *streamParser) next() (core.StreamEvent, error) {
	if len(p.pending) > 0 {
		event := p.pending[0]
		p.pending = p.pending[1:]
		return event, nil
	}

	for {
		line, err := p.readDataLine()
		if err != nil {
			return nil, err
		}

		if line == "[DONE]" {
			return &providers.DoneEvent{}, nil
		}

		event, err := p.handleChunk(line)
		if err != nil {
			return nil, err
		}
		if event != nil {
			return event, nil
		}
	}
}

// readDataLine reads SSE lines until it finds a "data: " line.
func (p *streamParser) readDataLine() (string, error) {
	for p.scanner.Scan() {
		line := p.scanner.Text()

		if strings.HasPrefix(line, "data: ") {
			return strings.TrimPrefix(line, "data: "), nil
		}
	}

	if err := p.scanner.Err(); err != nil {
		return "", fmt.Errorf("openrouter: scanner error: %w", err)
	}
	return "", io.EOF
}

// handleChunk processes a parsed JSON chunk.
func (p *streamParser) handleChunk(data string) (core.StreamEvent, error) {
	var chunk chunkResponse
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return nil, fmt.Errorf("openrouter: failed to parse chunk: %w", err)
	}

	if p.model == "" && chunk.Model != "" {
		p.model = chunk.Model
	}

	if chunk.Usage != nil {
		usage := core.TokenUsage{
			InputTokens:  chunk.Usage.PromptTokens,
			OutputTokens: chunk.Usage.CompletionTokens,
		}
		if chunk.Usage.PromptTokensDetails != nil {
			usage.CacheReadInputTokens = chunk.Usage.PromptTokensDetails.CachedTokens
		}
		return &providers.UsageEvent{
			Usage: usage,
			Model: p.model,
		}, nil
	}

	if len(chunk.Choices) == 0 {
		return nil, nil
	}

	choice := chunk.Choices[0]

	if len(choice.Delta.ToolCalls) > 0 {
		for _, tc := range choice.Delta.ToolCalls {
			acc, exists := p.activeTools[tc.Index]
			if !exists {
				acc = &toolAccumulator{
					ID:   tc.ID,
					Name: tc.Function.Name,
				}
				p.activeTools[tc.Index] = acc
			}
			if tc.Function.Arguments != "" {
				if acc.JSONBuf.Len()+len(tc.Function.Arguments) > maxToolJSONSize {
					return nil, fmt.Errorf("openrouter: tool call input exceeds maximum size (%d bytes)", maxToolJSONSize)
				}
				acc.JSONBuf.WriteString(tc.Function.Arguments)
			}
		}
	}

	if choice.Delta.Content != "" {
		return &providers.TextChunkEvent{Text: choice.Delta.Content}, nil
	}

	if choice.FinishReason != nil && *choice.FinishReason == "tool_calls" {
		return p.emitToolCalls()
	}

	return nil, nil
}

// emitToolCalls validates and emits all accumulated tool calls.
func (p *streamParser) emitToolCalls() (core.StreamEvent, error) {
	// Sort keys to emit tool calls in the order they appeared in the API response.
	keys := make([]int, 0, len(p.activeTools))
	for k := range p.activeTools {
		keys = append(keys, k)
	}
	sort.Ints(keys)

	var events []core.StreamEvent
	for _, k := range keys {
		acc := p.activeTools[k]
		inputJSON := acc.JSONBuf.String()
		if !json.Valid([]byte(inputJSON)) {
			return nil, fmt.Errorf("openrouter: tool call %q produced invalid JSON input", acc.Name)
		}
		events = append(events, &providers.ToolCallEvent{
			ToolName: acc.Name,
			ToolID:   acc.ID,
			Input:    json.RawMessage(inputJSON),
		})
	}
	p.activeTools = make(map[int]*toolAccumulator)

	if len(events) == 0 {
		return nil, nil
	}
	if len(events) > 1 {
		p.pending = append(p.pending, events[1:]...)
	}
	return events[0], nil
}
