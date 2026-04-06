// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/providers"
)

func TestAgent_Init_ValidatesDeps(t *testing.T) {
	a := NewAgent(AgentDeps{})
	err := a.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider is required")
}

func TestAgent_Run_BasicTextResponse(t *testing.T) {
	deps, provider, _, events, _, _ := newTestDeps()

	provider.Responses = [][]core.StreamEvent{{
		&providers.TextChunkEvent{Text: "Hello "},
		&providers.TextChunkEvent{Text: "world"},
		&providers.UsageEvent{Usage: core.TokenUsage{InputTokens: 10, OutputTokens: 5}},
		&providers.DoneEvent{},
	}}

	agent := NewAgent(deps)
	err := agent.Run(context.Background(), "hi")
	require.NoError(t, err)

	// Verify conversation history.
	require.Len(t, agent.history, 2)
	assert.Equal(t, "user", agent.history[0].Role)
	assert.Equal(t, "hi", agent.history[0].Content)
	assert.Equal(t, "assistant", agent.history[1].Role)
	assert.Equal(t, "Hello world", agent.history[1].Content)

	// Verify stream.text events were published.
	textEvents := events.eventsOfType("stream.text")
	assert.Len(t, textEvents, 2)

	// Verify query lifecycle events.
	assert.Len(t, events.eventsOfType("agent.query_started"), 1)
	assert.Len(t, events.eventsOfType("agent.query_completed"), 1)
}

func TestAgent_Run_MultiTurnToolUse(t *testing.T) {
	deps, provider, tools, _, _, _ := newTestDeps()

	tools.Responses["file_read"] = mockToolResult{
		Response: core.ToolResponse{Output: "file contents here"},
	}

	// First call: provider requests a tool.
	// Second call: provider returns text after seeing tool result.
	provider.Responses = [][]core.StreamEvent{
		{
			&providers.ToolCallEvent{
				ToolName: "file_read",
				ToolID:   "call-1",
				Input:    jsonRaw(map[string]string{"path": "/tmp/test.go"}),
			},
			&providers.DoneEvent{},
		},
		{
			&providers.TextChunkEvent{Text: "I read the file."},
			&providers.DoneEvent{},
		},
	}

	agent := NewAgent(deps)
	err := agent.Run(context.Background(), "read /tmp/test.go")
	require.NoError(t, err)

	// History: user → assistant (tool call) → user (tool result) → assistant (text).
	require.Len(t, agent.history, 4)
	assert.Equal(t, "user", agent.history[0].Role)
	assert.Equal(t, "assistant", agent.history[1].Role)
	assert.Len(t, agent.history[1].ToolCalls, 1)
	assert.Equal(t, "user", agent.history[2].Role)
	assert.Len(t, agent.history[2].ToolResults, 1)
	assert.Equal(t, "file contents here", agent.history[2].ToolResults[0].Content)
	assert.Equal(t, "assistant", agent.history[3].Role)
	assert.Equal(t, "I read the file.", agent.history[3].Content)

	// Verify tool was actually called.
	require.Len(t, tools.Calls, 1)
	assert.Equal(t, "file_read", tools.Calls[0].Name)
}

func TestAgent_Run_Cancellation(t *testing.T) {
	deps, provider, _, _, _, _ := newTestDeps()

	// Provider that blocks — cancel before it finishes.
	ctx, cancel := context.WithCancel(context.Background())

	provider.Responses = [][]core.StreamEvent{{
		&providers.TextChunkEvent{Text: "partial"},
		// No DoneEvent — simulates a long stream.
	}}

	// Cancel immediately before Run processes.
	cancel()

	agent := NewAgent(deps)
	err := agent.Run(ctx, "test")
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
}

func TestAgent_Run_MaxIterations(t *testing.T) {
	deps, provider, tools, _, _, _ := newTestDeps()

	tools.Responses["always_tool"] = mockToolResult{
		Response: core.ToolResponse{Output: "result"},
	}

	// Create 11 responses, all returning tool calls. Agent should stop at 10.
	var responses [][]core.StreamEvent
	for range 11 {
		responses = append(responses, []core.StreamEvent{
			&providers.ToolCallEvent{
				ToolName: "always_tool",
				ToolID:   "call-loop",
				Input:    jsonRaw(map[string]string{}),
			},
			&providers.DoneEvent{},
		})
	}
	provider.Responses = responses

	agent := NewAgent(deps)
	err := agent.Run(context.Background(), "loop forever")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max tool iterations")
}

func TestAgent_Run_ProviderError(t *testing.T) {
	deps, provider, _, _, _, _ := newTestDeps()

	provider.Responses = [][]core.StreamEvent{{
		&providers.ErrorEvent{Err: errors.New("connection refused")},
	}}

	agent := NewAgent(deps)
	err := agent.Run(context.Background(), "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider stream")
	assert.Contains(t, err.Error(), "connection refused")
}

func TestAgent_Run_PermissionDenied(t *testing.T) {
	deps, provider, tools, _, _, _ := newTestDeps()

	tools.Responses["dangerous_tool"] = mockToolResult{
		Err: core.ErrPermissionDenied,
	}

	// Provider asks for dangerous tool, then returns text after seeing denial.
	provider.Responses = [][]core.StreamEvent{
		{
			&providers.ToolCallEvent{
				ToolName: "dangerous_tool",
				ToolID:   "call-danger",
				Input:    jsonRaw(map[string]string{}),
			},
			&providers.DoneEvent{},
		},
		{
			&providers.TextChunkEvent{Text: "OK, I won't do that."},
			&providers.DoneEvent{},
		},
	}

	agent := NewAgent(deps)
	err := agent.Run(context.Background(), "delete everything")
	require.NoError(t, err)

	// Verify denial was sent as tool result.
	require.Len(t, agent.history, 4)
	assert.True(t, agent.history[2].ToolResults[0].IsError)
	assert.Contains(t, agent.history[2].ToolResults[0].Content, "Permission denied")
}

func TestAgent_Run_ToolNotFound(t *testing.T) {
	deps, provider, tools, _, _, _ := newTestDeps()

	tools.Responses["nonexistent"] = mockToolResult{
		Err: core.ErrToolNotFound,
	}

	provider.Responses = [][]core.StreamEvent{
		{
			&providers.ToolCallEvent{
				ToolName: "nonexistent",
				ToolID:   "call-404",
				Input:    jsonRaw(map[string]string{}),
			},
			&providers.DoneEvent{},
		},
		{
			&providers.TextChunkEvent{Text: "That tool doesn't exist."},
			&providers.DoneEvent{},
		},
	}

	agent := NewAgent(deps)
	err := agent.Run(context.Background(), "use nonexistent tool")
	require.NoError(t, err)

	assert.Contains(t, agent.history[2].ToolResults[0].Content, "Tool not found")
}

func TestAgent_Run_ThinkingEvent(t *testing.T) {
	deps, provider, _, events, _, _ := newTestDeps()

	provider.Responses = [][]core.StreamEvent{{
		&providers.ThinkingEvent{Thinking: "Let me think..."},
		&providers.TextChunkEvent{Text: "Answer"},
		&providers.DoneEvent{},
	}}

	agent := NewAgent(deps)
	err := agent.Run(context.Background(), "hard question")
	require.NoError(t, err)

	thinkingEvents := events.eventsOfType("stream.thinking")
	assert.Len(t, thinkingEvents, 1)
}
