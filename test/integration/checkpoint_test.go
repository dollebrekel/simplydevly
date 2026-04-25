// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/checkpoint"
	"siply.dev/siply/internal/core"
)

func TestCheckpoint_EndToEnd_CreateListLoad(t *testing.T) {
	dir := t.TempDir()
	sessionID := "e2e-session"
	mgr := checkpoint.NewManager(dir, sessionID)

	// Create 5 checkpoints.
	for i := 1; i <= 5; i++ {
		msgs := make([]core.Message, i)
		for j := range msgs {
			msgs[j] = core.Message{Role: "user", Content: "msg"}
		}
		err := mgr.Checkpoint(context.Background(), core.StepCheckpoint{
			SessionID:  sessionID,
			StepNumber: i,
			Timestamp:  time.Now(),
			ToolName:   "bash",
			ToolInput:  []byte(`{"cmd":"echo"}`),
			ToolOutput: "ok",
			Messages:   msgs,
		})
		require.NoError(t, err)
	}
	mgr.Close()

	// List should show 5 entries.
	mgr2 := checkpoint.NewManager(dir, sessionID)
	defer mgr2.Close()

	metas, err := mgr2.List(sessionID)
	require.NoError(t, err)
	require.Len(t, metas, 5)

	for i, m := range metas {
		assert.Equal(t, i+1, m.StepNumber)
		assert.Equal(t, "bash", m.ToolName)
		assert.Equal(t, i+1, m.MessageCount)
	}
}

func TestCheckpoint_Rewind_BranchTimeline(t *testing.T) {
	dir := t.TempDir()
	sessionID := "rewind-session"
	mgr := checkpoint.NewManager(dir, sessionID)

	// Create 5 checkpoints.
	for i := 1; i <= 5; i++ {
		_ = mgr.Checkpoint(context.Background(), core.StepCheckpoint{
			SessionID:  sessionID,
			StepNumber: i,
			Timestamp:  time.Now(),
			ToolName:   "bash",
			Messages:   []core.Message{{Role: "user", Content: "test"}},
		})
	}
	mgr.Close()

	// "Rewind" to step 3 by deleting after 3.
	mgr2 := checkpoint.NewManager(dir, sessionID)

	err := mgr2.DeleteAfterStep(sessionID, 3)
	require.NoError(t, err)

	// Add 2 new steps (the branched timeline).
	for i := 4; i <= 5; i++ {
		_ = mgr2.Checkpoint(context.Background(), core.StepCheckpoint{
			SessionID:  sessionID,
			StepNumber: i,
			Timestamp:  time.Now(),
			ToolName:   "file_write",
			Messages:   []core.Message{{Role: "user", Content: "branched"}},
		})
	}
	mgr2.Close()

	// Verify: steps 1-3 from original, 4-5 from branch.
	mgr3 := checkpoint.NewManager(dir, sessionID)
	defer mgr3.Close()

	metas, err := mgr3.List(sessionID)
	require.NoError(t, err)
	require.Len(t, metas, 5)

	// Steps 1-3 should be "bash", steps 4-5 should be "file_write".
	for _, m := range metas[:3] {
		assert.Equal(t, "bash", m.ToolName)
	}
	for _, m := range metas[3:] {
		assert.Equal(t, "file_write", m.ToolName)
	}
}

func TestCheckpoint_Pruning(t *testing.T) {
	dir := t.TempDir()

	// Create many sessions with data to exceed a small limit.
	for s := 0; s < 10; s++ {
		sessID := "prune-session-" + string(rune('A'+s))
		m := checkpoint.NewManager(dir, sessID)
		for i := 1; i <= 10; i++ {
			_ = m.Checkpoint(context.Background(), core.StepCheckpoint{
				SessionID:  sessID,
				StepNumber: i,
				Timestamp:  time.Now(),
				ToolName:   "bash",
				ToolOutput: string(make([]byte, 1024)),
				Messages:   []core.Message{{Role: "user", Content: "test data padding"}},
			})
		}
		m.Close()
	}

	// Prune to small limit.
	current := checkpoint.NewManager(dir, "prune-session-J")
	defer current.Close()
	err := current.Prune(5 * 1024) // 5 KB limit
	require.NoError(t, err)
}
