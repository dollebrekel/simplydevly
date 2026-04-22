// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package agent

import (
	"context"
	"errors"
	"os"
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

	// Create maxToolIterations+1 responses, all returning tool calls.
	// Agent should stop at maxToolIterations.
	var responses [][]core.StreamEvent
	for range maxToolIterations + 1 {
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

func TestAgent_FileContextRejectsOutOfBounds(t *testing.T) {
	deps, provider, _, _, _, _ := newTestDeps()

	projectDir := t.TempDir()
	deps.Perm = &mockPermissionEvaluator{Verdict: core.Allow}

	provider.Responses = [][]core.StreamEvent{{
		&providers.TextChunkEvent{Text: "response"},
		&providers.DoneEvent{},
	}}

	agent := NewAgent(deps, AgentConfig{ProjectDir: projectDir})
	require.NoError(t, agent.Init(context.Background()))

	// Inject out-of-bounds path.
	agent.filesMu.Lock()
	agent.pendingContextFiles = []string{"/etc/passwd"}
	agent.filesMu.Unlock()

	err := agent.Run(context.Background(), "test")
	require.NoError(t, err)

	// The out-of-bounds file should NOT appear in the message.
	require.Len(t, agent.history, 2)
	assert.NotContains(t, agent.history[0].Content, "/etc/passwd")
	assert.NotContains(t, agent.history[0].Content, "File context")
}

func TestAgent_FileContextAcceptsWorkspaceFiles(t *testing.T) {
	deps, provider, _, _, _, _ := newTestDeps()

	projectDir := t.TempDir()

	// Create a file inside the workspace.
	testFile := projectDir + "/test.txt"
	require.NoError(t, os.WriteFile(testFile, []byte("workspace content"), 0o644))

	provider.Responses = [][]core.StreamEvent{{
		&providers.TextChunkEvent{Text: "response"},
		&providers.DoneEvent{},
	}}

	agent := NewAgent(deps, AgentConfig{ProjectDir: projectDir})
	require.NoError(t, agent.Init(context.Background()))

	// Inject valid workspace file.
	agent.filesMu.Lock()
	agent.pendingContextFiles = []string{testFile}
	agent.filesMu.Unlock()

	err := agent.Run(context.Background(), "test")
	require.NoError(t, err)

	// The workspace file should appear in the message.
	require.Len(t, agent.history, 2)
	assert.Contains(t, agent.history[0].Content, "workspace content")
}

func TestAgent_Run_NilTelemetry(t *testing.T) {
	// AC#6: Agent loop without Telemetry (nil) works unchanged.
	deps, provider, _, _, _, _ := newTestDeps()
	// deps.Telemetry is nil by default.

	provider.Responses = [][]core.StreamEvent{{
		&providers.TextChunkEvent{Text: "Hello"},
		&providers.UsageEvent{Usage: core.TokenUsage{InputTokens: 10, OutputTokens: 5}},
		&providers.DoneEvent{},
	}}

	agent := NewAgent(deps)
	err := agent.Run(context.Background(), "hi")
	require.NoError(t, err)
	assert.Len(t, agent.history, 2)
}

func TestAgent_Run_RecordsQueryTelemetry(t *testing.T) {
	// AC#1: After provider query, RecordStep is called with StepType="query".
	deps, provider, _, _, _, _ := newTestDeps()
	tel := &mockTelemetryCollector{}
	deps.Telemetry = tel

	provider.Responses = [][]core.StreamEvent{{
		&providers.TextChunkEvent{Text: "response"},
		&providers.UsageEvent{Usage: core.TokenUsage{InputTokens: 100, OutputTokens: 50}},
		&providers.DoneEvent{},
	}}

	agent := NewAgent(deps)
	err := agent.Run(context.Background(), "test")
	require.NoError(t, err)

	require.Len(t, tel.Steps, 1)
	step := tel.Steps[0]
	assert.Equal(t, "query", step.StepType)
	assert.Equal(t, 100, step.TokensIn)
	assert.Equal(t, 50, step.TokensOut)
	assert.True(t, step.LatencyMS >= 0)
	assert.Nil(t, step.ToolCalls)
}

func TestAgent_Run_RecordsToolTelemetry(t *testing.T) {
	// AC#2: After tool execution, RecordStep is called with StepType="tool-execution".
	deps, provider, tools, _, _, _ := newTestDeps()
	tel := &mockTelemetryCollector{}
	deps.Telemetry = tel

	tools.Responses["file_read"] = mockToolResult{
		Response: core.ToolResponse{Output: "contents"},
	}

	provider.Responses = [][]core.StreamEvent{
		{
			&providers.ToolCallEvent{
				ToolName: "file_read",
				ToolID:   "call-1",
				Input:    jsonRaw(map[string]string{"path": "/tmp/test.go"}),
			},
			&providers.UsageEvent{Usage: core.TokenUsage{InputTokens: 80, OutputTokens: 20}},
			&providers.DoneEvent{},
		},
		{
			&providers.TextChunkEvent{Text: "Done."},
			&providers.UsageEvent{Usage: core.TokenUsage{InputTokens: 200, OutputTokens: 30}},
			&providers.DoneEvent{},
		},
	}

	agent := NewAgent(deps)
	err := agent.Run(context.Background(), "read file")
	require.NoError(t, err)

	// Expect: query step, tool-execution step, query step (second provider call).
	require.Len(t, tel.Steps, 3)

	// First step: query.
	assert.Equal(t, "query", tel.Steps[0].StepType)

	// Second step: tool-execution.
	assert.Equal(t, "tool-execution", tel.Steps[1].StepType)
	assert.Equal(t, []string{"file_read"}, tel.Steps[1].ToolCalls)
	assert.True(t, tel.Steps[1].LatencyMS >= 0)

	// Third step: second query.
	assert.Equal(t, "query", tel.Steps[2].StepType)
}
