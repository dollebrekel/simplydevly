package anthropic

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/providers"
)

const maxToolJSONSize = 10 * 1024 * 1024 // 10MB

// toolBlock tracks the state of an active tool_use content block.
type toolBlock struct {
	ID      string
	Name    string
	JSONBuf strings.Builder
}

// streamParser reads SSE events from an Anthropic streaming response.
type streamParser struct {
	scanner *bufio.Scanner

	// Per-block tool state, keyed by content block index.
	activeTools map[int]*toolBlock

	// Track block types by index.
	blockTypes map[int]string

	// model stores the model name from message_start for usage events.
	model string
}

func newStreamParser(r io.Reader) *streamParser {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 64*1024), 1024*1024) // 1MB max line size
	return &streamParser{
		scanner:     s,
		blockTypes:  make(map[int]string),
		activeTools: make(map[int]*toolBlock),
	}
}

// sseEvent is a raw server-sent event.
type sseEvent struct {
	Event string
	Data  string
}

// next reads the next meaningful StreamEvent from the SSE stream.
// Returns io.EOF when the stream ends.
func (p *streamParser) next() (core.StreamEvent, error) {
	for {
		sse, err := p.readSSE()
		if err != nil {
			return nil, err
		}

		event, err := p.handleSSE(sse)
		if err != nil {
			return nil, err
		}
		if event != nil {
			return event, nil
		}
		// nil event means internal bookkeeping, keep reading.
	}
}

// readSSE reads one complete SSE event (event: + data: lines).
// Per the SSE spec, multiple data: lines are concatenated with newlines.
func (p *streamParser) readSSE() (sseEvent, error) {
	var event string
	var dataLines []string

	for p.scanner.Scan() {
		line := p.scanner.Text()

		if line == "" {
			// Empty line = end of SSE event.
			if len(dataLines) > 0 || event != "" {
				return sseEvent{Event: event, Data: strings.Join(dataLines, "\n")}, nil
			}
			continue
		}

		if strings.HasPrefix(line, "event: ") {
			event = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		}
		// Ignore other lines (comments, etc.)
	}

	if err := p.scanner.Err(); err != nil {
		return sseEvent{}, fmt.Errorf("anthropic: scanner error: %w", err)
	}
	return sseEvent{}, io.EOF
}

// handleSSE processes a raw SSE event and returns a StreamEvent or nil.
func (p *streamParser) handleSSE(sse sseEvent) (core.StreamEvent, error) {
	switch sse.Event {
	case "message_start":
		return p.handleMessageStart(sse.Data)

	case "content_block_start":
		return p.handleContentBlockStart(sse.Data)

	case "content_block_delta":
		return p.handleContentBlockDelta(sse.Data)

	case "content_block_stop":
		return p.handleContentBlockStop(sse.Data)

	case "message_delta":
		return p.handleMessageDelta(sse.Data)

	case "message_stop":
		return &providers.DoneEvent{}, nil

	case "ping":
		return nil, nil

	case "error":
		return p.handleError(sse.Data)

	default:
		// Unknown event types are silently skipped.
		return nil, nil
	}
}

// messageStart is the JSON payload for message_start events.
type messageStart struct {
	Message struct {
		Model string `json:"model"`
		Usage struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

func (p *streamParser) handleMessageStart(data string) (core.StreamEvent, error) {
	var ms messageStart
	if err := json.Unmarshal([]byte(data), &ms); err != nil {
		return nil, fmt.Errorf("anthropic: failed to parse message_start: %w", err)
	}
	p.model = ms.Message.Model
	if ms.Message.Usage.InputTokens > 0 {
		return &providers.UsageEvent{
			Usage: core.TokenUsage{
				InputTokens: ms.Message.Usage.InputTokens,
			},
			Model: p.model,
		}, nil
	}
	return nil, nil
}

// contentBlockStart is the JSON payload for content_block_start events.
type contentBlockStart struct {
	Index        int `json:"index"`
	ContentBlock struct {
		Type string `json:"type"`
		ID   string `json:"id,omitempty"`
		Name string `json:"name,omitempty"`
	} `json:"content_block"`
}

func (p *streamParser) handleContentBlockStart(data string) (core.StreamEvent, error) {
	var cbs contentBlockStart
	if err := json.Unmarshal([]byte(data), &cbs); err != nil {
		return nil, fmt.Errorf("anthropic: failed to parse content_block_start: %w", err)
	}

	p.blockTypes[cbs.Index] = cbs.ContentBlock.Type

	if cbs.ContentBlock.Type == "tool_use" {
		p.activeTools[cbs.Index] = &toolBlock{
			ID:   cbs.ContentBlock.ID,
			Name: cbs.ContentBlock.Name,
		}
	}

	return nil, nil
}

// contentBlockDelta is the JSON payload for content_block_delta events.
type contentBlockDelta struct {
	Index int `json:"index"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text,omitempty"`
		Thinking    string `json:"thinking,omitempty"`
		PartialJSON string `json:"partial_json,omitempty"`
	} `json:"delta"`
}

func (p *streamParser) handleContentBlockDelta(data string) (core.StreamEvent, error) {
	var cbd contentBlockDelta
	if err := json.Unmarshal([]byte(data), &cbd); err != nil {
		return nil, fmt.Errorf("anthropic: failed to parse content_block_delta: %w", err)
	}

	switch cbd.Delta.Type {
	case "text_delta":
		return &providers.TextChunkEvent{Text: cbd.Delta.Text}, nil

	case "thinking_delta":
		return &providers.ThinkingEvent{Thinking: cbd.Delta.Thinking}, nil

	case "input_json_delta":
		if tb, ok := p.activeTools[cbd.Index]; ok {
			if tb.JSONBuf.Len()+len(cbd.Delta.PartialJSON) > maxToolJSONSize {
				return nil, fmt.Errorf("anthropic: tool call input exceeds maximum size (%d bytes)", maxToolJSONSize)
			}
			tb.JSONBuf.WriteString(cbd.Delta.PartialJSON)
		}
		return nil, nil

	default:
		return nil, nil
	}
}

// contentBlockStop is the JSON payload for content_block_stop events.
type contentBlockStop struct {
	Index int `json:"index"`
}

func (p *streamParser) handleContentBlockStop(data string) (core.StreamEvent, error) {
	var cbs contentBlockStop
	if err := json.Unmarshal([]byte(data), &cbs); err != nil {
		return nil, fmt.Errorf("anthropic: failed to parse content_block_stop: %w", err)
	}

	blockType := p.blockTypes[cbs.Index]
	if blockType == "tool_use" {
		tb, ok := p.activeTools[cbs.Index]
		if !ok {
			return nil, nil
		}
		inputJSON := tb.JSONBuf.String()
		toolName := tb.Name
		toolID := tb.ID
		delete(p.activeTools, cbs.Index)

		if !json.Valid([]byte(inputJSON)) {
			return nil, fmt.Errorf("anthropic: tool call %q produced invalid JSON input", toolName)
		}
		return &providers.ToolCallEvent{
			ToolName: toolName,
			ToolID:   toolID,
			Input:    json.RawMessage(inputJSON),
		}, nil
	}

	return nil, nil
}

// messageDelta is the JSON payload for message_delta events.
type messageDelta struct {
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (p *streamParser) handleMessageDelta(data string) (core.StreamEvent, error) {
	var md messageDelta
	if err := json.Unmarshal([]byte(data), &md); err != nil {
		return nil, fmt.Errorf("anthropic: failed to parse message_delta: %w", err)
	}

	return &providers.UsageEvent{
		Usage: core.TokenUsage{
			OutputTokens: md.Usage.OutputTokens,
		},
		Model: p.model,
	}, nil
}

// apiError is the JSON payload for error events.
type apiError struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (p *streamParser) handleError(data string) (core.StreamEvent, error) {
	var ae apiError
	if err := json.Unmarshal([]byte(data), &ae); err != nil {
		return &providers.ErrorEvent{
			Err: fmt.Errorf("anthropic: stream error (unparseable): %s", data),
		}, nil
	}

	return &providers.ErrorEvent{
		Err: fmt.Errorf("anthropic: %s: %s", ae.Error.Type, ae.Error.Message),
	}, nil
}
