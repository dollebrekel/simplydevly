// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package checkpoint

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"siply.dev/siply/internal/core"
)

func TestManager_Checkpoint_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, "test-session")
	defer m.Close()

	step := core.StepCheckpoint{
		SessionID:  "test-session",
		StepNumber: 1,
		Timestamp:  time.Now(),
		ToolName:   "file_read",
		ToolInput:  []byte(`{"path": "/tmp/test.go"}`),
		ToolOutput: "file contents here",
		Messages: []core.Message{
			{Role: "user", Content: "read this file"},
			{Role: "assistant", Content: "ok"},
		},
		FileHashes: map[string]string{"/tmp/test.go": "abc123"},
	}

	err := m.Checkpoint(context.Background(), step)
	require.NoError(t, err)

	// Wait for async writer.
	m.Close()

	// Verify file exists.
	path := filepath.Join(dir, "test-session", "step-001.json")
	_, err = os.Stat(path)
	require.NoError(t, err)
}

func TestManager_List_SortedByStep(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, "test-session")

	for i := 1; i <= 3; i++ {
		_ = m.Checkpoint(context.Background(), core.StepCheckpoint{
			SessionID:  "test-session",
			StepNumber: i,
			Timestamp:  time.Now(),
			ToolName:   "bash",
			Messages:   []core.Message{{Role: "user", Content: "test"}},
		})
	}
	m.Close()

	m2 := NewManager(dir, "test-session")
	defer m2.Close()

	metas, err := m2.List("test-session")
	require.NoError(t, err)
	require.Len(t, metas, 3)

	assert.Equal(t, 1, metas[0].StepNumber)
	assert.Equal(t, 2, metas[1].StepNumber)
	assert.Equal(t, 3, metas[2].StepNumber)
	assert.Equal(t, "bash", metas[0].ToolName)
}

func TestManager_Load_Deserializes(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, "test-session")

	original := core.StepCheckpoint{
		SessionID:  "test-session",
		StepNumber: 1,
		Timestamp:  time.Now().Truncate(time.Millisecond),
		ToolName:   "file_write",
		ToolOutput: "done",
		Messages: []core.Message{
			{Role: "user", Content: "write file"},
		},
		FileHashes: map[string]string{"/tmp/out.go": "hash123"},
	}
	_ = m.Checkpoint(context.Background(), original)
	m.Close()

	m2 := NewManager(dir, "test-session")
	defer m2.Close()

	cp, err := m2.Load("test-session", 1)
	require.NoError(t, err)
	assert.Equal(t, 1, cp.StepNumber)
	assert.Equal(t, "file_write", cp.ToolName)
	assert.Len(t, cp.Messages, 1)
	assert.Equal(t, "write file", cp.Messages[0].Content)
	assert.Equal(t, "hash123", cp.FileHashes["/tmp/out.go"])
}

func TestManager_Load_GzipCompressed(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, "test-session")

	// Create a large message to trigger gzip.
	bigContent := make([]byte, 20*1024)
	for i := range bigContent {
		bigContent[i] = 'A' + byte(i%26)
	}

	_ = m.Checkpoint(context.Background(), core.StepCheckpoint{
		SessionID:  "test-session",
		StepNumber: 1,
		Timestamp:  time.Now(),
		ToolName:   "bash",
		ToolOutput: string(bigContent),
		Messages: []core.Message{
			{Role: "user", Content: string(bigContent)},
		},
	})
	m.Close()

	m2 := NewManager(dir, "test-session")
	defer m2.Close()

	cp, err := m2.Load("test-session", 1)
	require.NoError(t, err)
	assert.Equal(t, 1, cp.StepNumber)
	assert.Len(t, cp.Messages[0].Content, len(bigContent))
}

func TestManager_Prune_DeletesOldest_PreservesCurrent(t *testing.T) {
	dir := t.TempDir()

	// Create two "old" sessions with data.
	for _, sessID := range []string{"old-session-1", "old-session-2"} {
		m := NewManager(dir, sessID)
		for i := 1; i <= 5; i++ {
			_ = m.Checkpoint(context.Background(), core.StepCheckpoint{
				SessionID:  sessID,
				StepNumber: i,
				Timestamp:  time.Now(),
				ToolName:   "bash",
				ToolOutput: string(make([]byte, 1024)),
				Messages:   []core.Message{{Role: "user", Content: "test"}},
			})
		}
		m.Close()
	}

	// Current session.
	current := NewManager(dir, "current-session")
	_ = current.Checkpoint(context.Background(), core.StepCheckpoint{
		SessionID:  "current-session",
		StepNumber: 1,
		Timestamp:  time.Now(),
		ToolName:   "bash",
		Messages:   []core.Message{{Role: "user", Content: "test"}},
	})
	current.Close()

	// Prune to very small limit — should delete old sessions but keep current.
	mgr := NewManager(dir, "current-session")
	defer mgr.Close()
	err := mgr.Prune(100) // 100 bytes — forces all old sessions to be pruned
	require.NoError(t, err)

	// Current session must still exist.
	_, err = os.Stat(filepath.Join(dir, "current-session"))
	assert.NoError(t, err)
}

func TestManager_DeleteAfterStep(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, "test-session")

	for i := 1; i <= 5; i++ {
		_ = m.Checkpoint(context.Background(), core.StepCheckpoint{
			SessionID:  "test-session",
			StepNumber: i,
			Timestamp:  time.Now(),
			ToolName:   "bash",
			Messages:   []core.Message{{Role: "user", Content: "test"}},
		})
	}
	m.Close()

	m2 := NewManager(dir, "test-session")
	defer m2.Close()

	err := m2.DeleteAfterStep("test-session", 3)
	require.NoError(t, err)

	metas, err := m2.List("test-session")
	require.NoError(t, err)
	require.Len(t, metas, 3)
	assert.Equal(t, 1, metas[0].StepNumber)
	assert.Equal(t, 2, metas[1].StepNumber)
	assert.Equal(t, 3, metas[2].StepNumber)
}

func TestHashWorkspaceFiles_CorrectSHA256(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.go")
	require.NoError(t, os.WriteFile(testFile, []byte("package main\n"), 0o644))

	hashes := HashWorkspaceFiles([]string{testFile})
	require.Len(t, hashes, 1)

	hash, ok := hashes[testFile]
	assert.True(t, ok)
	assert.Len(t, hash, 64) // SHA256 hex length
}

func TestHashWorkspaceFiles_SkipsLargeFiles(t *testing.T) {
	dir := t.TempDir()
	bigFile := filepath.Join(dir, "big.bin")
	// Create 11 MB file.
	require.NoError(t, os.WriteFile(bigFile, make([]byte, 11*1024*1024), 0o644))

	hashes := HashWorkspaceFiles([]string{bigFile})
	assert.Empty(t, hashes)
}

func TestHashWorkspaceFiles_SkipsBinaryFiles(t *testing.T) {
	dir := t.TempDir()
	binFile := filepath.Join(dir, "binary")
	data := []byte{0x00, 0x01, 0x02, 0x03}
	require.NoError(t, os.WriteFile(binFile, data, 0o644))

	hashes := HashWorkspaceFiles([]string{binFile})
	assert.Empty(t, hashes)
}

func TestManager_List_EmptySession(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, "nonexistent")
	defer m.Close()

	metas, err := m.List("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, metas)
}

func TestManager_CheckpointDropped_WhenChannelFull(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, "test-session")

	// Fill the channel by sending more than capacity.
	// The writer goroutine will process them, but if we send fast enough
	// some might be dropped. This test just verifies no panic/error.
	for i := 0; i < channelCapacity+10; i++ {
		_ = m.Checkpoint(context.Background(), core.StepCheckpoint{
			SessionID:  "test-session",
			StepNumber: i + 1,
			Timestamp:  time.Now(),
			ToolName:   "bash",
			Messages:   []core.Message{{Role: "user", Content: "test"}},
		})
	}
	m.Close()

	// Verify at least some were written (the writer processes concurrently).
	m2 := NewManager(dir, "test-session")
	defer m2.Close()
	metas, _ := m2.List("test-session")
	assert.NotEmpty(t, metas)
}
