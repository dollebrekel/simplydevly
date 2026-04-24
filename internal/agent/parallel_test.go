// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package agent

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/providers"
)

// newParallelAgent creates an agent with ParallelTools enabled.
func newParallelAgent(deps AgentDeps) *Agent {
	return NewAgent(deps, AgentConfig{ParallelTools: true})
}

func TestParallel_ResultsInOriginalOrder(t *testing.T) {
	deps, provider, tools, _, _, _ := newTestDeps()

	// Three tools with varying "execution times" via channel delays.
	tools.Responses["tool_a"] = mockToolResult{Response: core.ToolResponse{Output: "result_a"}}
	tools.Responses["tool_b"] = mockToolResult{Response: core.ToolResponse{Output: "result_b"}}
	tools.Responses["tool_c"] = mockToolResult{Response: core.ToolResponse{Output: "result_c"}}

	provider.Responses = [][]core.StreamEvent{
		{
			&providers.ToolCallEvent{ToolName: "tool_a", ToolID: "call-a", Input: jsonRaw(map[string]string{})},
			&providers.ToolCallEvent{ToolName: "tool_b", ToolID: "call-b", Input: jsonRaw(map[string]string{})},
			&providers.ToolCallEvent{ToolName: "tool_c", ToolID: "call-c", Input: jsonRaw(map[string]string{})},
			&providers.DoneEvent{},
		},
		{
			&providers.TextChunkEvent{Text: "All tools done."},
			&providers.DoneEvent{},
		},
	}

	agent := newParallelAgent(deps)
	err := agent.Run(context.Background(), "run three tools")
	require.NoError(t, err)

	// History: user → assistant (3 tool calls) → 3 tool results → assistant (text)
	require.Len(t, agent.history, 6)

	// Tool results should be in original order: a, b, c.
	assert.Equal(t, "call-a", agent.history[2].ToolResults[0].ToolID)
	assert.Equal(t, "result_a", agent.history[2].ToolResults[0].Content)
	assert.Equal(t, "call-b", agent.history[3].ToolResults[0].ToolID)
	assert.Equal(t, "result_b", agent.history[3].ToolResults[0].Content)
	assert.Equal(t, "call-c", agent.history[4].ToolResults[0].ToolID)
	assert.Equal(t, "result_c", agent.history[4].ToolResults[0].Content)
}

func TestParallel_PartialPermissionDenial(t *testing.T) {
	deps, provider, tools, _, _, _ := newTestDeps()

	tools.Responses["safe_tool"] = mockToolResult{Response: core.ToolResponse{Output: "safe_result"}}
	tools.Responses["dangerous_tool"] = mockToolResult{Err: core.ErrPermissionDenied}
	tools.Responses["another_safe"] = mockToolResult{Response: core.ToolResponse{Output: "another_result"}}

	provider.Responses = [][]core.StreamEvent{
		{
			&providers.ToolCallEvent{ToolName: "safe_tool", ToolID: "call-safe", Input: jsonRaw(map[string]string{})},
			&providers.ToolCallEvent{ToolName: "dangerous_tool", ToolID: "call-danger", Input: jsonRaw(map[string]string{})},
			&providers.ToolCallEvent{ToolName: "another_safe", ToolID: "call-safe2", Input: jsonRaw(map[string]string{})},
			&providers.DoneEvent{},
		},
		{
			&providers.TextChunkEvent{Text: "Handled denial."},
			&providers.DoneEvent{},
		},
	}

	agent := newParallelAgent(deps)
	err := agent.Run(context.Background(), "run mixed tools")
	require.NoError(t, err)

	// Verify safe tools succeeded.
	assert.False(t, agent.history[2].ToolResults[0].IsError)
	assert.Equal(t, "safe_result", agent.history[2].ToolResults[0].Content)

	// Verify dangerous tool was denied but didn't stop others.
	assert.True(t, agent.history[3].ToolResults[0].IsError)
	assert.Contains(t, agent.history[3].ToolResults[0].Content, "Permission denied")

	// Verify third tool succeeded.
	assert.False(t, agent.history[4].ToolResults[0].IsError)
	assert.Equal(t, "another_result", agent.history[4].ToolResults[0].Content)
}

func TestParallel_ContextCancellationMidExecution(t *testing.T) {
	deps, provider, tools, _, _, _ := newTestDeps()

	// Use a slow tool executor that blocks until context is canceled.
	slowTools := &slowToolExecutor{
		inner:    tools,
		blockFor: 5 * time.Second,
	}
	deps.Tools = slowTools

	tools.Responses["slow_tool"] = mockToolResult{Response: core.ToolResponse{Output: "slow_result"}}

	provider.Responses = [][]core.StreamEvent{
		{
			&providers.ToolCallEvent{ToolName: "slow_tool", ToolID: "call-1", Input: jsonRaw(map[string]string{})},
			&providers.ToolCallEvent{ToolName: "slow_tool", ToolID: "call-2", Input: jsonRaw(map[string]string{})},
			&providers.DoneEvent{},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	agent := newParallelAgent(deps)
	err := agent.Run(ctx, "slow tools")
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))

	// History must NOT be committed on cancellation — Run returns early.
	assert.Empty(t, agent.history, "history should not be committed after context cancellation")
}

func TestParallel_SingleToolFallback(t *testing.T) {
	deps, provider, tools, _, _, _ := newTestDeps()

	tools.Responses["only_tool"] = mockToolResult{Response: core.ToolResponse{Output: "only_result"}}

	provider.Responses = [][]core.StreamEvent{
		{
			&providers.ToolCallEvent{ToolName: "only_tool", ToolID: "call-only", Input: jsonRaw(map[string]string{})},
			&providers.DoneEvent{},
		},
		{
			&providers.TextChunkEvent{Text: "Done."},
			&providers.DoneEvent{},
		},
	}

	agent := newParallelAgent(deps)
	err := agent.Run(context.Background(), "one tool")
	require.NoError(t, err)

	// Same behavior as sequential: single tool result.
	require.Len(t, agent.history, 4)
	assert.Equal(t, "only_result", agent.history[2].ToolResults[0].Content)
}

func TestParallel_GoroutineLeakFree(t *testing.T) {
	// Stabilize goroutine count before measurement.
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	before := runtime.NumGoroutine()

	deps, provider, tools, _, _, _ := newTestDeps()

	tools.Responses["tool_a"] = mockToolResult{Response: core.ToolResponse{Output: "a"}}
	tools.Responses["tool_b"] = mockToolResult{Response: core.ToolResponse{Output: "b"}}

	provider.Responses = [][]core.StreamEvent{
		{
			&providers.ToolCallEvent{ToolName: "tool_a", ToolID: "call-a", Input: jsonRaw(map[string]string{})},
			&providers.ToolCallEvent{ToolName: "tool_b", ToolID: "call-b", Input: jsonRaw(map[string]string{})},
			&providers.DoneEvent{},
		},
		{
			&providers.TextChunkEvent{Text: "OK"},
			&providers.DoneEvent{},
		},
	}

	agent := newParallelAgent(deps)
	err := agent.Run(context.Background(), "leak test")
	require.NoError(t, err)

	// Retry goroutine count check to reduce flakiness under CI load.
	var after int
	for range 10 {
		time.Sleep(50 * time.Millisecond)
		runtime.GC()
		after = runtime.NumGoroutine()
		if after <= before+2 {
			return // pass
		}
	}
	t.Errorf("goroutine leak detected: before=%d after=%d (tolerance +2)", before, after)
}

func TestParallel_TransparencyLoggerLogsAllTools(t *testing.T) {
	deps, provider, tools, events, _, _ := newTestDeps()

	tools.Responses["tool_x"] = mockToolResult{Response: core.ToolResponse{Output: "x"}}
	tools.Responses["tool_y"] = mockToolResult{Response: core.ToolResponse{Output: "y"}}

	provider.Responses = [][]core.StreamEvent{
		{
			&providers.ToolCallEvent{ToolName: "tool_x", ToolID: "call-x", Input: jsonRaw(map[string]string{})},
			&providers.ToolCallEvent{ToolName: "tool_y", ToolID: "call-y", Input: jsonRaw(map[string]string{})},
			&providers.DoneEvent{},
		},
		{
			&providers.TextChunkEvent{Text: "Done"},
			&providers.DoneEvent{},
		},
	}

	agent := newParallelAgent(deps)
	err := agent.Run(context.Background(), "log test")
	require.NoError(t, err)

	// Each tool execution should be logged.
	toolEvents := events.eventsOfType("tool.executed")
	assert.Len(t, toolEvents, 2)
}

func TestParallel_ToolsExecuteConcurrently(t *testing.T) {
	deps, provider, tools, _, _, _ := newTestDeps()

	// Track concurrent executions using an atomic counter.
	var maxConcurrent atomic.Int32
	var current atomic.Int32

	concurrentTools := &concurrencyTrackingExecutor{
		inner:         tools,
		current:       &current,
		maxConcurrent: &maxConcurrent,
		holdFor:       50 * time.Millisecond,
	}
	deps.Tools = concurrentTools

	tools.Responses["tool_a"] = mockToolResult{Response: core.ToolResponse{Output: "a"}}
	tools.Responses["tool_b"] = mockToolResult{Response: core.ToolResponse{Output: "b"}}
	tools.Responses["tool_c"] = mockToolResult{Response: core.ToolResponse{Output: "c"}}

	provider.Responses = [][]core.StreamEvent{
		{
			&providers.ToolCallEvent{ToolName: "tool_a", ToolID: "call-a", Input: jsonRaw(map[string]string{})},
			&providers.ToolCallEvent{ToolName: "tool_b", ToolID: "call-b", Input: jsonRaw(map[string]string{})},
			&providers.ToolCallEvent{ToolName: "tool_c", ToolID: "call-c", Input: jsonRaw(map[string]string{})},
			&providers.DoneEvent{},
		},
		{
			&providers.TextChunkEvent{Text: "Done"},
			&providers.DoneEvent{},
		},
	}

	agent := newParallelAgent(deps)
	err := agent.Run(context.Background(), "concurrent test")
	require.NoError(t, err)

	// At least 2 tools should have been executing simultaneously.
	assert.GreaterOrEqual(t, maxConcurrent.Load(), int32(2),
		"tools should execute concurrently in parallel mode")
}

func TestAgentConfig_EffectiveMaxIterations(t *testing.T) {
	tests := []struct {
		name     string
		config   AgentConfig
		expected int
	}{
		{"zero uses default", AgentConfig{}, maxToolIterations},
		{"negative uses default", AgentConfig{MaxIterations: -1}, maxToolIterations},
		{"positive uses value", AgentConfig{MaxIterations: 5}, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.config.effectiveMaxIterations())
		})
	}
}

func TestAgentConfig_CustomMaxIterations(t *testing.T) {
	deps, provider, tools, _, _, _ := newTestDeps()

	tools.Responses["loop_tool"] = mockToolResult{Response: core.ToolResponse{Output: "loop"}}

	// Provider always returns tool calls.
	var responses [][]core.StreamEvent
	for range 10 {
		responses = append(responses, []core.StreamEvent{
			&providers.ToolCallEvent{ToolName: "loop_tool", ToolID: "call-loop", Input: jsonRaw(map[string]string{})},
			&providers.DoneEvent{},
		})
	}
	provider.Responses = responses

	agent := NewAgent(deps, AgentConfig{MaxIterations: 3})
	err := agent.Run(context.Background(), "loop")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max tool iterations (3) reached")
}

func TestNewAgent_BackwardsCompatible(t *testing.T) {
	deps, _, _, _, _, _ := newTestDeps()

	// No config — should work like before (sequential, default iterations).
	agent := NewAgent(deps)
	assert.False(t, agent.config.ParallelTools)
	assert.Equal(t, maxToolIterations, agent.config.effectiveMaxIterations())
}

// --- Test helpers ---

// slowToolExecutor wraps a mockToolExecutor but blocks for a duration.
type slowToolExecutor struct {
	inner    *mockToolExecutor
	blockFor time.Duration
}

func (s *slowToolExecutor) Init(ctx context.Context) error   { return s.inner.Init(ctx) }
func (s *slowToolExecutor) Start(ctx context.Context) error  { return s.inner.Start(ctx) }
func (s *slowToolExecutor) Stop(ctx context.Context) error   { return s.inner.Stop(ctx) }
func (s *slowToolExecutor) Health() error                    { return s.inner.Health() }
func (s *slowToolExecutor) ListTools() []core.ToolDefinition { return s.inner.ListTools() }
func (s *slowToolExecutor) GetTool(name string) (core.ToolDefinition, error) {
	return s.inner.GetTool(name)
}

func (s *slowToolExecutor) Execute(ctx context.Context, req core.ToolRequest) (core.ToolResponse, error) {
	select {
	case <-time.After(s.blockFor):
	case <-ctx.Done():
		return core.ToolResponse{
			Output:  "Context canceled: " + ctx.Err().Error(),
			IsError: true,
		}, ctx.Err()
	}
	return s.inner.Execute(ctx, req)
}

// concurrencyTrackingExecutor wraps a mockToolExecutor and tracks concurrent
// execution count.
type concurrencyTrackingExecutor struct {
	inner         *mockToolExecutor
	current       *atomic.Int32
	maxConcurrent *atomic.Int32
	holdFor       time.Duration
}

func (c *concurrencyTrackingExecutor) Init(ctx context.Context) error   { return c.inner.Init(ctx) }
func (c *concurrencyTrackingExecutor) Start(ctx context.Context) error  { return c.inner.Start(ctx) }
func (c *concurrencyTrackingExecutor) Stop(ctx context.Context) error   { return c.inner.Stop(ctx) }
func (c *concurrencyTrackingExecutor) Health() error                    { return c.inner.Health() }
func (c *concurrencyTrackingExecutor) ListTools() []core.ToolDefinition { return c.inner.ListTools() }
func (c *concurrencyTrackingExecutor) GetTool(name string) (core.ToolDefinition, error) {
	return c.inner.GetTool(name)
}

func (c *concurrencyTrackingExecutor) Execute(ctx context.Context, req core.ToolRequest) (core.ToolResponse, error) {
	n := c.current.Add(1)
	// Update max if this is a new high.
	for {
		old := c.maxConcurrent.Load()
		if n <= old || c.maxConcurrent.CompareAndSwap(old, n) {
			break
		}
	}

	select {
	case <-time.After(c.holdFor):
	case <-ctx.Done():
		c.current.Add(-1)
		return core.ToolResponse{
			Output:  "Context canceled: " + ctx.Err().Error(),
			IsError: true,
		}, ctx.Err()
	}
	c.current.Add(-1)

	return c.inner.Execute(ctx, req)
}
