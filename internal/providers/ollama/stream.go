package ollama

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/providers"
)

// streamResponse represents a single NDJSON line from Ollama's streaming API.
type streamResponse struct {
	Model           string        `json:"model"`
	CreatedAt       string        `json:"created_at"`
	Message         streamMessage `json:"message"`
	Done            bool          `json:"done"`
	Error           string        `json:"error,omitempty"`
	PromptEvalCount int           `json:"prompt_eval_count,omitempty"`
	EvalCount       int           `json:"eval_count,omitempty"`
}

type streamMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// streamParser reads NDJSON events from an Ollama streaming response.
type streamParser struct {
	scanner *bufio.Scanner
	model   string
	pending core.StreamEvent
}

func newStreamParser(r io.Reader) *streamParser {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 64*1024), 1024*1024) // 1MB max line size
	return &streamParser{
		scanner: s,
	}
}

// next reads the next meaningful StreamEvent from the NDJSON stream.
// Returns io.EOF when the stream ends.
func (p *streamParser) next() (core.StreamEvent, error) {
	// Return pending event (DoneEvent after UsageEvent).
	if p.pending != nil {
		event := p.pending
		p.pending = nil
		return event, nil
	}

	for p.scanner.Scan() {
		line := p.scanner.Text()
		if line == "" {
			continue
		}

		var resp streamResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			return nil, fmt.Errorf("ollama: failed to parse response: %w", err)
		}

		// Track model.
		if p.model == "" && resp.Model != "" {
			p.model = resp.Model
		}

		// Error response.
		if resp.Error != "" {
			return &providers.ErrorEvent{
				Err: fmt.Errorf("ollama: %s", resp.Error),
			}, nil
		}

		// Done signal — emit UsageEvent now, queue DoneEvent for next call.
		if resp.Done {
			p.pending = &providers.DoneEvent{}
			return &providers.UsageEvent{
				Usage: core.TokenUsage{
					InputTokens:  resp.PromptEvalCount,
					OutputTokens: resp.EvalCount,
				},
				Model: p.model,
			}, nil
		}

		// Text content.
		if resp.Message.Content != "" {
			return &providers.TextChunkEvent{Text: resp.Message.Content}, nil
		}
	}

	if err := p.scanner.Err(); err != nil {
		return nil, fmt.Errorf("ollama: scanner error: %w", err)
	}
	return nil, io.EOF
}
