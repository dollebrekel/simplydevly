// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package agent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/checkpoint"
	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/providers"
)

func TestAgent_Rewind_ReplacesHistory(t *testing.T) {
	dir := t.TempDir()
	mgr := checkpoint.NewManager(dir, "test-session")
	defer mgr.Close()

	// Create a checkpoint with known state.
	msgs := []core.Message{
		{Role: "user", Content: "step one"},
		{Role: "assistant", Content: "response one"},
	}
	require.NoError(t, mgr.Checkpoint(context.Background(), core.StepCheckpoint{
		SessionID:  "test-session",
		StepNumber: 1,
		Timestamp:  time.Now(),
		ToolName:   "bash",
		Messages:   msgs,
	}))
	// Wait for async write.
	time.Sleep(50 * time.Millisecond)

	deps, _, _, events, _, _ := newTestDeps()
	deps.Checkpoint = mgr

	ag := NewAgent(deps)
	require.NoError(t, ag.Init(context.Background()))
	ag.checkpointSessionID = "test-session"

	// Set history to something different.
	ag.history = []core.Message{
		{Role: "user", Content: "step one"},
		{Role: "assistant", Content: "response one"},
		{Role: "user", Content: "step two"},
		{Role: "assistant", Content: "response two"},
	}
	ag.stepCounter.Store(2)

	// Rewind to step 1.
	err := ag.Rewind(1)
	require.NoError(t, err)

	// History should be the checkpoint state + system message.
	assert.Len(t, ag.history, 3) // 2 original + 1 rewind system msg
	assert.Equal(t, "user", ag.history[0].Role)
	assert.Equal(t, "step one", ag.history[0].Content)
	assert.Contains(t, ag.history[2].Content, "Rewound to step 1")

	// Step counter should be reset.
	assert.Equal(t, 1, int(ag.stepCounter.Load()))

	// Rewind event should be published.
	cpEvents := events.eventsOfType("checkpoint")
	assert.Len(t, cpEvents, 1)
	cpEvent := cpEvents[0].(*core.CheckpointEvent)
	assert.Equal(t, "rewind", cpEvent.Action)
	assert.Equal(t, 1, cpEvent.StepNumber)
}

func TestAgent_Rewind_BlockedDuringRun(t *testing.T) {
	dir := t.TempDir()
	mgr := checkpoint.NewManager(dir, "test-session")
	defer mgr.Close()

	deps, _, _, _, _, _ := newTestDeps()
	deps.Checkpoint = mgr

	ag := NewAgent(deps)
	require.NoError(t, ag.Init(context.Background()))

	// Simulate running state.
	ag.mu.Lock()
	ag.running = true
	ag.mu.Unlock()

	err := ag.Rewind(1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot rewind while agent is running")

	ag.mu.Lock()
	ag.running = false
	ag.mu.Unlock()
}

func TestAgent_Rewind_NoCheckpointManager(t *testing.T) {
	deps, _, _, _, _, _ := newTestDeps()
	ag := NewAgent(deps)
	require.NoError(t, ag.Init(context.Background()))

	err := ag.Rewind(1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "checkpoint manager not available")
}

func TestAgent_ListCheckpoints(t *testing.T) {
	dir := t.TempDir()
	mgr := checkpoint.NewManager(dir, "test-session")

	for i := 1; i <= 3; i++ {
		_ = mgr.Checkpoint(context.Background(), core.StepCheckpoint{
			SessionID:  "test-session",
			StepNumber: i,
			Timestamp:  time.Now(),
			ToolName:   "bash",
			Messages:   []core.Message{{Role: "user", Content: "test"}},
		})
	}
	mgr.Close()

	mgr2 := checkpoint.NewManager(dir, "test-session")
	defer mgr2.Close()

	deps, _, _, _, _, _ := newTestDeps()
	deps.Checkpoint = mgr2

	ag := NewAgent(deps)
	require.NoError(t, ag.Init(context.Background()))
	ag.checkpointSessionID = "test-session"

	metas, err := ag.ListCheckpoints()
	require.NoError(t, err)
	assert.Len(t, metas, 3)
}

func TestAgent_Run_CreatesCheckpoints(t *testing.T) {
	dir := t.TempDir()
	mgr := checkpoint.NewManager(dir, "test-session")
	defer mgr.Close()

	deps, provider, _, _, _, _ := newTestDeps()
	deps.Checkpoint = mgr

	// Set up a tool call response.
	provider.Responses = [][]core.StreamEvent{
		{
			&providers.ToolCallEvent{
				ToolID: "call1", ToolName: "bash", Input: []byte(`{"cmd":"ls"}`),
			},
			&providers.UsageEvent{Usage: core.TokenUsage{InputTokens: 10, OutputTokens: 5}},
			&providers.DoneEvent{},
		},
		{
			&providers.TextChunkEvent{Text: "Done"},
			&providers.UsageEvent{Usage: core.TokenUsage{InputTokens: 15, OutputTokens: 3}},
			&providers.DoneEvent{},
		},
	}

	ag := NewAgent(deps)
	require.NoError(t, ag.Init(context.Background()))
	ag.checkpointSessionID = "test-session"

	err := ag.Run(context.Background(), "list files")
	require.NoError(t, err)

	// Wait for async checkpoint write.
	time.Sleep(100 * time.Millisecond)

	metas, err := mgr.List("test-session")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(metas), 1)
}

func TestAgent_FeatureGate_FreeUser_NoCheckpoints(t *testing.T) {
	deps, provider, _, _, _, _ := newTestDeps()
	// No checkpoint manager = Free user.

	provider.Responses = [][]core.StreamEvent{{
		&providers.TextChunkEvent{Text: "Hello"},
		&providers.UsageEvent{Usage: core.TokenUsage{InputTokens: 10, OutputTokens: 5}},
		&providers.DoneEvent{},
	}}

	ag := NewAgent(deps)
	require.NoError(t, ag.Init(context.Background()))

	err := ag.Run(context.Background(), "hi")
	require.NoError(t, err)

	// Step counter should remain 0.
	assert.Equal(t, 0, int(ag.stepCounter.Load()))
}
