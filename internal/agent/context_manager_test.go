package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/core"
)

func TestTruncationCompactor_ShouldCompact_UnderThreshold(t *testing.T) {
	c := NewTruncationCompactor()

	// 100 chars = ~25 tokens. Limit 100 tokens. 25/100 = 25% — no compaction.
	messages := []core.Message{
		{Role: "user", Content: strings.Repeat("a", 100)},
	}

	assert.False(t, c.ShouldCompact(messages, 100))
}

func TestTruncationCompactor_ShouldCompact_OverThreshold(t *testing.T) {
	c := NewTruncationCompactor()

	// 400 chars = ~100 tokens. Limit 100 tokens. 100/100 = 100% — compact.
	messages := []core.Message{
		{Role: "user", Content: strings.Repeat("a", 400)},
	}

	assert.True(t, c.ShouldCompact(messages, 100))
}

func TestTruncationCompactor_ShouldCompact_AtThreshold(t *testing.T) {
	c := NewTruncationCompactor()

	// 320 chars = 80 tokens. Limit 100 tokens. 80/100 = 80% — exactly at threshold.
	// Should NOT compact (> 80%, not >=).
	messages := []core.Message{
		{Role: "user", Content: strings.Repeat("a", 320)},
	}

	assert.False(t, c.ShouldCompact(messages, 100))
}

func TestTruncationCompactor_ShouldCompact_JustOverThreshold(t *testing.T) {
	c := NewTruncationCompactor()

	// 324 chars = 81 tokens. Limit 100. 81/100 = 81% — over threshold.
	messages := []core.Message{
		{Role: "user", Content: strings.Repeat("a", 324)},
	}

	assert.True(t, c.ShouldCompact(messages, 100))
}

func TestTruncationCompactor_ShouldCompact_ZeroLimit(t *testing.T) {
	c := NewTruncationCompactor()
	assert.False(t, c.ShouldCompact(nil, 0))
}

func TestTruncationCompactor_Compact_PreservesFirstAndLast(t *testing.T) {
	c := NewTruncationCompactor()

	// Create 15 messages: system + 14 conversation messages.
	messages := make([]core.Message, 15)
	messages[0] = core.Message{Role: "system", Content: "system prompt"}
	for i := 1; i < 15; i++ {
		messages[i] = core.Message{Role: "user", Content: strings.Repeat("x", 10)}
	}

	result, err := c.Compact(context.Background(), messages)
	require.NoError(t, err)

	// Should keep first message + last 10 = 11 total.
	assert.Len(t, result, 11)
	assert.Equal(t, "system prompt", result[0].Content)

	// Last 10 should be the tail.
	for i := 1; i < 11; i++ {
		assert.Equal(t, messages[5+i-1].Content, result[i].Content)
	}
}

func TestTruncationCompactor_Compact_FewMessages(t *testing.T) {
	c := NewTruncationCompactor()

	// 5 messages — fewer than preserveRecentCount+1 (11). No compaction.
	messages := make([]core.Message, 5)
	for i := range messages {
		messages[i] = core.Message{Role: "user", Content: "msg"}
	}

	result, err := c.Compact(context.Background(), messages)
	require.NoError(t, err)
	assert.Len(t, result, 5)
}

func TestTruncationCompactor_Compact_ExactlyThresholdMessages(t *testing.T) {
	c := NewTruncationCompactor()

	// 11 messages = preserveRecentCount + 1. No compaction needed.
	messages := make([]core.Message, 11)
	for i := range messages {
		messages[i] = core.Message{Role: "user", Content: "msg"}
	}

	result, err := c.Compact(context.Background(), messages)
	require.NoError(t, err)
	assert.Len(t, result, 11)
}

func TestEstimateTokens(t *testing.T) {
	messages := []core.Message{
		{Content: strings.Repeat("a", 40)}, // 40 chars = 10 tokens
		{Content: strings.Repeat("b", 20)}, // 20 chars = 5 tokens
	}

	assert.Equal(t, 15, estimateTokens(messages))
}

func TestEstimateTokens_IncludesToolResults(t *testing.T) {
	messages := []core.Message{
		{
			Content: strings.Repeat("a", 40), // 10 tokens
			ToolResults: []core.ToolResult{
				{Content: strings.Repeat("c", 20)}, // 5 tokens
			},
		},
	}

	assert.Equal(t, 15, estimateTokens(messages))
}
