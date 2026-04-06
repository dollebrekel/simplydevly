// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package agent

import (
	"context"

	"siply.dev/siply/internal/core"
)

const (
	// charsPerToken is the heuristic for token estimation (4 chars ≈ 1 token).
	charsPerToken = 4
	// compactThreshold triggers compaction at 80% of the context limit.
	compactThreshold = 0.80
	// preserveRecentCount is the number of recent messages to always preserve.
	preserveRecentCount = 10
)

// TruncationCompactor implements core.ContextManager using simple truncation.
// It estimates tokens via a character-count heuristic and drops middle messages
// when the conversation exceeds the provider's context window.
type TruncationCompactor struct{}

// NewTruncationCompactor creates a TruncationCompactor.
func NewTruncationCompactor() *TruncationCompactor {
	return &TruncationCompactor{}
}

func (c *TruncationCompactor) Init(_ context.Context) error  { return nil }
func (c *TruncationCompactor) Start(_ context.Context) error { return nil }
func (c *TruncationCompactor) Stop(_ context.Context) error  { return nil }
func (c *TruncationCompactor) Health() error                 { return nil }

// ShouldCompact returns true when estimated tokens exceed 80% of limit.
func (c *TruncationCompactor) ShouldCompact(messages []core.Message, limit int) bool {
	if limit <= 0 {
		return false
	}
	est := estimateTokens(messages)
	return float64(est) > float64(limit)*compactThreshold
}

// Compact removes oldest messages from the middle, preserving the first message
// (system prompt context) and the last N messages (recent context).
func (c *TruncationCompactor) Compact(_ context.Context, messages []core.Message) ([]core.Message, error) {
	if len(messages) == 0 {
		return messages, nil
	}
	if len(messages) <= preserveRecentCount+1 {
		return messages, nil
	}

	// We don't know the limit here, so we remove middle messages aggressively.
	// The caller checks ShouldCompact first with the real limit, so if we get
	// here we know we need to shrink. Keep first + last N.
	first := messages[0]
	tail := messages[len(messages)-preserveRecentCount:]

	result := make([]core.Message, 0, 1+len(tail))
	result = append(result, first)
	result = append(result, tail...)
	return result, nil
}

// estimateTokens uses a simple heuristic: 4 characters ≈ 1 token.
func estimateTokens(messages []core.Message) int {
	total := 0
	for _, m := range messages {
		total += len(m.Content)
		for _, tc := range m.ToolCalls {
			total += len(tc.Input)
		}
		for _, tr := range m.ToolResults {
			total += len(tr.Content)
		}
	}
	return total / charsPerToken
}
