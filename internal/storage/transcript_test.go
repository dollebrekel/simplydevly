// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package storage_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"siply.dev/siply/internal/storage"
)

// AC#2: JSONL transcript append + read roundtrip
func TestTranscriptWriter_AppendAndRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.jsonl")

	type Entry struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	w, err := storage.NewTranscriptWriter(path)
	require.NoError(t, err)

	entries := []Entry{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
		{Role: "user", Content: "bye"},
	}

	for _, e := range entries {
		require.NoError(t, w.Append(e))
	}
	require.NoError(t, w.Close())

	// Read back.
	got, err := storage.ReadTranscript(path)
	require.NoError(t, err)
	require.Len(t, got, 3)

	for i, raw := range got {
		var e Entry
		require.NoError(t, json.Unmarshal(raw, &e))
		assert.Equal(t, entries[i], e)
	}
}

// AC#2: crash safety — partial write doesn't corrupt previous entries
func TestTranscriptWriter_CrashSafety(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crash.jsonl")

	type Entry struct {
		ID int `json:"id"`
	}

	// Write 3 valid entries.
	w, err := storage.NewTranscriptWriter(path)
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		require.NoError(t, w.Append(Entry{ID: i}))
	}
	require.NoError(t, w.Close())

	// Simulate a partial/corrupt write by appending raw garbage.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	require.NoError(t, err)
	_, err = f.Write([]byte(`{"id": broken`))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// ReadTranscript should return the 3 valid entries and skip the corrupt line.
	got, err := storage.ReadTranscript(path)
	require.NoError(t, err)
	assert.Len(t, got, 3, "corrupt last line should be skipped, 3 valid entries remain")
}

// AC#2: empty JSONL file returns empty slice
func TestTranscriptWriter_EmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.jsonl")
	require.NoError(t, os.WriteFile(path, []byte{}, 0644))

	got, err := storage.ReadTranscript(path)
	require.NoError(t, err)
	require.NotNil(t, got, "ReadTranscript must return empty slice, not nil")
	assert.Empty(t, got)
}

// AC#2: JSONL with blank lines skips them
func TestTranscriptWriter_SkipsBlankLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "blanks.jsonl")
	content := "{\"a\":1}\n\n{\"b\":2}\n\n\n{\"c\":3}\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	got, err := storage.ReadTranscript(path)
	require.NoError(t, err)
	assert.Len(t, got, 3)
}
