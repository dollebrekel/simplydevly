// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"siply.dev/siply/internal/fileutil"
)

const (
	filePermissions = 0644
	dirPermissions  = 0700
	storeFileName   = "memory.json"
)

// KVStore implements core.MemoryBackend with an in-memory cache backed by
// a JSON file for persistence. Each workspace gets its own memory store
// scoped to its project directory.
type KVStore struct {
	baseDir     string
	mu          sync.RWMutex
	data        map[string][]byte
	initialized bool
}

// NewKVStore creates a new KVStore that persists data under baseDir.
// No I/O is performed until Init is called.
func NewKVStore(baseDir string) *KVStore {
	return &KVStore{
		baseDir: baseDir,
		data:    make(map[string][]byte),
	}
}

// Init creates the storage directory and loads any existing data from disk.
func (s *KVStore) Init(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.baseDir, dirPermissions); err != nil {
		return fmt.Errorf("memory: failed to create base dir: %w", err)
	}

	if err := s.loadFromDisk(); err != nil {
		slog.Warn("memory: failed to load existing data, starting fresh", "err", err)
		s.data = make(map[string][]byte)
	}

	s.initialized = true
	return nil
}

// Start is a no-op for KVStore.
func (s *KVStore) Start(_ context.Context) error { return nil }

// Stop flushes the in-memory cache to disk.
func (s *KVStore) Stop(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.initialized {
		return nil
	}
	return s.flushToDisk()
}

// Health returns an error if the store has not been initialized.
func (s *KVStore) Health() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.initialized {
		return fmt.Errorf("memory: not initialized")
	}
	return nil
}

// Remember stores a value under the given key and persists to disk.
func (s *KVStore) Remember(_ context.Context, key string, value []byte) error {
	if err := validateKey(key); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.initialized {
		return fmt.Errorf("memory: not initialized")
	}

	// Copy value to prevent caller mutation.
	copied := make([]byte, len(value))
	copy(copied, value)
	s.data[key] = copied

	return s.flushToDisk()
}

// Recall retrieves a value by key. Returns os.ErrNotExist if the key is absent.
func (s *KVStore) Recall(_ context.Context, key string) ([]byte, error) {
	if err := validateKey(key); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.initialized {
		return nil, fmt.Errorf("memory: not initialized")
	}

	val, ok := s.data[key]
	if !ok {
		return nil, os.ErrNotExist
	}

	// Return a copy to prevent caller mutation.
	result := make([]byte, len(val))
	copy(result, val)
	return result, nil
}

// Forget removes a key and persists the change. Returns nil if the key is absent.
func (s *KVStore) Forget(_ context.Context, key string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.initialized {
		return fmt.Errorf("memory: not initialized")
	}

	delete(s.data, key)
	return s.flushToDisk()
}

// Search returns all values whose keys contain the query substring.
// An empty query returns all values.
func (s *KVStore) Search(_ context.Context, query string) ([][]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.initialized {
		return nil, fmt.Errorf("memory: not initialized")
	}

	var results [][]byte
	for key, val := range s.data {
		if query == "" || strings.Contains(key, query) {
			copied := make([]byte, len(val))
			copy(copied, val)
			results = append(results, copied)
		}
	}

	if results == nil {
		results = [][]byte{}
	}
	return results, nil
}

// storeData is the JSON-serializable format for the KV store on disk.
type storeData struct {
	Version int               `json:"version"`
	Items   map[string][]byte `json:"items"`
}

// flushToDisk writes the current in-memory data to the JSON file atomically.
// Caller must hold s.mu write lock.
func (s *KVStore) flushToDisk() error {
	sd := storeData{
		Version: 1,
		Items:   s.data,
	}
	raw, err := json.MarshalIndent(sd, "", "  ")
	if err != nil {
		return fmt.Errorf("memory: failed to marshal data: %w", err)
	}

	path := filepath.Join(s.baseDir, storeFileName)
	if err := fileutil.AtomicWriteFile(path, raw, filePermissions); err != nil {
		return fmt.Errorf("memory: failed to write store file: %w", err)
	}
	return nil
}

// loadFromDisk reads the JSON file and populates the in-memory cache.
// Caller must hold s.mu write lock.
func (s *KVStore) loadFromDisk() error {
	path := filepath.Join(s.baseDir, storeFileName)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Fresh store, no file yet.
		}
		return fmt.Errorf("memory: failed to read store file: %w", err)
	}

	var sd storeData
	if err := json.Unmarshal(raw, &sd); err != nil {
		return fmt.Errorf("memory: failed to unmarshal store file: %w", err)
	}

	if sd.Items == nil {
		sd.Items = make(map[string][]byte)
	}
	s.data = sd.Items
	return nil
}

// validateKey rejects empty keys and keys with path traversal attempts.
func validateKey(key string) error {
	if key == "" {
		return fmt.Errorf("memory: empty key")
	}
	if strings.Contains(key, "..") {
		return fmt.Errorf("memory: key contains path traversal")
	}
	return nil
}
