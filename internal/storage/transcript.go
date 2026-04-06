// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package storage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// TranscriptWriter appends JSONL entries to a session transcript file.
// Each Append call writes one complete JSON line in a single Write syscall.
// Writes smaller than PIPE_BUF (4KB on Linux) are atomic; larger entries
// may be partially written on a crash. Durability is best-effort — data
// may remain in OS buffers until the next fsync or file close.
type TranscriptWriter struct {
	file *os.File
	mu   sync.Mutex
}

// NewTranscriptWriter opens (or creates) a JSONL transcript file for append-only writing.
func NewTranscriptWriter(path string) (*TranscriptWriter, error) {
	if err := os.MkdirAll(filepath.Dir(path), dirPermissions); err != nil {
		return nil, fmt.Errorf("storage: failed to create transcript directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, filePermissions)
	if err != nil {
		return nil, fmt.Errorf("storage: failed to open transcript: %w", err)
	}
	return &TranscriptWriter{file: f}, nil
}

// Append marshals entry as JSON and writes it as a single line followed by a newline.
func (w *TranscriptWriter) Append(entry any) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("storage: failed to marshal transcript entry: %w", err)
	}

	// Append newline for JSONL format.
	// Allocate new slice to avoid mutating json.Marshal's backing array.
	line := make([]byte, len(data)+1)
	copy(line, data)
	line[len(data)] = '\n'

	w.mu.Lock()
	defer w.mu.Unlock()

	// Single Write syscall — atomic for data < PIPE_BUF (4KB).
	if _, err := w.file.Write(line); err != nil {
		return fmt.Errorf("storage: failed to write transcript entry: %w", err)
	}
	return nil
}

// Close closes the underlying file handle.
// Acquires the mutex to ensure no concurrent Append is in progress.
func (w *TranscriptWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Close()
}

// ReadTranscript reads a JSONL file and returns each line as a json.RawMessage.
// Empty lines are skipped.
func ReadTranscript(path string) ([]json.RawMessage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("storage: failed to open transcript for reading: %w", err)
	}
	defer f.Close()

	var entries []json.RawMessage
	scanner := bufio.NewScanner(f)
	// Raise buffer limit from default 64KB to 10MB for large AI responses.
	scanner.Buffer(make([]byte, 1<<20), 10<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		// Validate JSON before accepting.
		if !json.Valid(line) {
			continue
		}
		entry := make(json.RawMessage, len(line))
		copy(entry, line)
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("storage: failed to read transcript: %w", err)
	}
	if entries == nil {
		entries = []json.RawMessage{}
	}
	return entries, nil
}
