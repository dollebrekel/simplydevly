// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/storage"
)

// TestStorage_FullLifecycle tests the complete storage flow:
// Init → Put → Get → List → Delete → Health
// AC#1, AC#6
func TestStorage_FullLifecycle(t *testing.T) {
	dir := t.TempDir()
	s := storage.NewFileStorage(dir)
	ctx := context.Background()

	// Health should fail before Init.
	require.Error(t, s.Health())

	// Init creates base dir.
	require.NoError(t, s.Init(ctx))
	require.NoError(t, s.Health())

	// Start/Stop are no-ops but should not error.
	require.NoError(t, s.Start(ctx))

	// Put data.
	require.NoError(t, s.Put(ctx, "sessions/2026-04-06/main.jsonl", []byte("line1\n")))
	require.NoError(t, s.Put(ctx, "sessions/2026-04-06/side.jsonl", []byte("line2\n")))
	require.NoError(t, s.Put(ctx, "cache/marketplace/plugins.json", []byte(`{"count":1}`)))

	// Get data back.
	got, err := s.Get(ctx, "sessions/2026-04-06/main.jsonl")
	require.NoError(t, err)
	assert.Equal(t, []byte("line1\n"), got)

	// List with prefix.
	list, err := s.List(ctx, "sessions/2026-04-06")
	require.NoError(t, err)
	assert.Len(t, list, 2)

	// List all.
	all, err := s.List(ctx, "")
	require.NoError(t, err)
	assert.Len(t, all, 3)

	// Delete.
	require.NoError(t, s.Delete(ctx, "cache/marketplace/plugins.json"))
	_, err = s.Get(ctx, "cache/marketplace/plugins.json")
	assert.True(t, os.IsNotExist(err))

	// Stop.
	require.NoError(t, s.Stop(ctx))

	// Health should still work after Stop (FileStorage doesn't change state on Stop).
	require.NoError(t, s.Health())
}

// TestStorage_TranscriptBulk writes 100 transcript entries and reads them all back.
// AC#2
func TestStorage_TranscriptBulk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.jsonl")

	type Entry struct {
		ID      int    `json:"id"`
		Content string `json:"content"`
	}

	w, err := storage.NewTranscriptWriter(path)
	require.NoError(t, err)

	// Write 100 entries.
	for i := range 100 {
		require.NoError(t, w.Append(Entry{ID: i, Content: fmt.Sprintf("entry-%d", i)}))
	}
	require.NoError(t, w.Close())

	// Read all back.
	entries, err := storage.ReadTranscript(path)
	require.NoError(t, err)
	require.Len(t, entries, 100)

	// Verify order.
	for i, raw := range entries {
		var e Entry
		require.NoError(t, json.Unmarshal(raw, &e))
		assert.Equal(t, i, e.ID)
		assert.Equal(t, fmt.Sprintf("entry-%d", i), e.Content)
	}
}

// TestStorage_CorruptionRecoveryE2E tests Put → corrupt file → Get recovers from .bak
// AC#5
func TestStorage_CorruptionRecoveryE2E(t *testing.T) {
	dir := t.TempDir()
	s := storage.NewFileStorage(dir)
	ctx := context.Background()
	require.NoError(t, s.Init(ctx))

	original := []byte(`{"config": "original"}`)
	updated := []byte(`{"config": "updated"}`)

	// Put original.
	require.NoError(t, s.Put(ctx, "settings.json", original))

	// Put updated — this creates .bak with original content.
	require.NoError(t, s.Put(ctx, "settings.json", updated))

	// Verify .bak exists with original content.
	bakPath := filepath.Join(dir, "settings.json.bak")
	bakData, err := os.ReadFile(bakPath)
	require.NoError(t, err)
	assert.Equal(t, original, bakData)

	// Corrupt primary file by making it unreadable.
	fullPath := filepath.Join(dir, "settings.json")
	require.NoError(t, os.Chmod(fullPath, 0000))

	// Get should recover from .bak (original content).
	got, err := s.Get(ctx, "settings.json")
	require.NoError(t, err)
	assert.Equal(t, original, got)

	// Restore permissions for cleanup.
	_ = os.Chmod(fullPath, 0644)
}
