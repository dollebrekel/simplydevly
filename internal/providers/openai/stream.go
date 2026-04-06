// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package openai

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

// streamParser reads SSE events from an OpenAI streaming response.
type streamParser struct {
	scanner *bufio.Scanner

	// activeTools tracks tool calls by index within delta.tool_calls array.
	activeTools map[int]*toolAccumulator

	// pending holds events waiting to be returned by next().
	pending []core.StreamEvent

	// model stores the model name from the first chunk.
	model string
}

func newStreamParser(r io.Reader) *streamParser {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 64*1024), 1024*1024) // 1MB max line size
	return &streamParser{
		scanner:     s,
		activeTools: make(map[int]*toolAccumulator),
	}
}

// chunkResponse represents a single SSE chunk from OpenAI's streaming API.
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

type chunkUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// next reads the next meaningful StreamEvent from the SSE stream.
// Returns io.EOF when the stream ends.
func (p *streamParser) next() (core.StreamEvent, error) {
	// Return pending events first (from multi-tool-call emit).
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

		// data: [DONE] signals stream end.
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

// readDataLine reads SSE lines until it finds a "data: " line and returns the data payload.
// Returns io.EOF when the stream ends.
func (p *streamParser) readDataLine() (string, error) {
	for p.scanner.Scan() {
		line := p.scanner.Text()

		if strings.HasPrefix(line, "data: ") {
			return strings.TrimPrefix(line, "data: "), nil
		}
	}

	if err := p.scanner.Err(); err != nil {
		return "", fmt.Errorf("openai: scanner error: %w", err)
	}
	return "", io.EOF
}

// handleChunk processes a parsed JSON chunk and returns a StreamEvent or nil.
func (p *streamParser) handleChunk(data string) (core.StreamEvent, error) {
	var chunk chunkResponse
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return nil, fmt.Errorf("openai: failed to parse chunk: %w", err)
	}

	// Track model from first chunk.
	if p.model == "" && chunk.Model != "" {
		p.model = chunk.Model
	}

	// Usage chunk (when stream_options.include_usage is true).
	if chunk.Usage != nil {
		return &providers.UsageEvent{
			Usage: core.TokenUsage{
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
			},
			Model: p.model,
		}, nil
	}

	if len(chunk.Choices) == 0 {
		return nil, nil
	}

	choice := chunk.Choices[0]

	// Handle tool calls in delta.
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
					return nil, fmt.Errorf("openai: tool call input exceeds maximum size (%d bytes)", maxToolJSONSize)
				}
				acc.JSONBuf.WriteString(tc.Function.Arguments)
			}
		}
	}

	// Handle text content.
	if choice.Delta.Content != "" {
		return &providers.TextChunkEvent{Text: choice.Delta.Content}, nil
	}

	// Check finish_reason to emit accumulated tool calls.
	if choice.FinishReason != nil && *choice.FinishReason == "tool_calls" {
		return p.emitToolCalls()
	}

	return nil, nil
}

// emitToolCalls validates and emits all accumulated tool calls.
// Returns the first one directly; queues the rest in pending.
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
			return nil, fmt.Errorf("openai: tool call %q produced invalid JSON input", acc.Name)
		}
		events = append(events, &providers.ToolCallEvent{
			ToolName: acc.Name,
			ToolID:   acc.ID,
			Input:    json.RawMessage(inputJSON),
		})
	}
	// Clear active tools.
	p.activeTools = make(map[int]*toolAccumulator)

	if len(events) == 0 {
		return nil, nil
	}
	if len(events) > 1 {
		p.pending = append(p.pending, events[1:]...)
	}
	return events[0], nil
}
