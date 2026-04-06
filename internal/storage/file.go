// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	filePermissions = 0644
	dirPermissions  = 0700
)

// FileStorage implements core.Storage with file-based persistence.
// Data is stored under a base directory with logical path keys mapped to filesystem paths.
type FileStorage struct {
	baseDir     string
	mu          sync.RWMutex
	initialized bool
}

// NewFileStorage creates a new FileStorage that persists data under baseDir.
// No I/O is performed until Init is called.
func NewFileStorage(baseDir string) *FileStorage {
	return &FileStorage{baseDir: baseDir}
}

// Init creates the base directory with 0700 permissions.
func (s *FileStorage) Init(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.baseDir, dirPermissions); err != nil {
		return fmt.Errorf("storage: failed to create base dir: %w", err)
	}
	s.initialized = true
	return nil
}

// Start is a no-op for FileStorage.
func (s *FileStorage) Start(_ context.Context) error { return nil }

// Stop is a no-op for FileStorage.
func (s *FileStorage) Stop(_ context.Context) error { return nil }

// Health returns an error if the store has not been initialized.
func (s *FileStorage) Health() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.initialized {
		return fmt.Errorf("storage: not initialized")
	}
	return nil
}

// validatePath rejects empty, absolute, and directory-traversal paths.
func validatePath(p string) error {
	if p == "" {
		return fmt.Errorf("storage: empty path")
	}
	if filepath.IsAbs(p) {
		return fmt.Errorf("storage: absolute path not allowed")
	}
	cleaned := filepath.Clean(p)
	if strings.Contains(cleaned, "..") {
		return fmt.Errorf("storage: path traversal not allowed")
	}
	return nil
}

// fullPath returns the absolute filesystem path for a logical key.
func (s *FileStorage) fullPath(p string) string {
	return filepath.Join(s.baseDir, filepath.Clean(p))
}

// checkInit returns an error if the store has not been initialized.
func (s *FileStorage) checkInit() error {
	if !s.initialized {
		return fmt.Errorf("storage: not initialized")
	}
	return nil
}

// Get reads the file at the given logical path.
// Returns os.ErrNotExist if the file does not exist.
// If the primary file is corrupt and a .bak exists, recovers from backup.
func (s *FileStorage) Get(_ context.Context, path string) ([]byte, error) {
	if err := validatePath(path); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := s.checkInit(); err != nil {
		return nil, err
	}

	full := s.fullPath(path)
	data, err := os.ReadFile(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, err
		}
		// Primary read failed — attempt .bak recovery.
		return s.recoverFromBackup(full, err)
	}
	return data, nil
}

// Put writes data to the given logical path, creating parent directories as needed.
// Before overwriting, copies the current file to <path>.bak for corruption recovery.
func (s *FileStorage) Put(_ context.Context, path string, data []byte) error {
	if err := validatePath(path); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkInit(); err != nil {
		return err
	}

	full := s.fullPath(path)

	// Create parent directories.
	dir := filepath.Dir(full)
	if err := os.MkdirAll(dir, dirPermissions); err != nil {
		return fmt.Errorf("storage: failed to create directories: %w", err)
	}

	// Backup existing file before overwriting.
	s.backupIfExists(full)

	if err := os.WriteFile(full, data, filePermissions); err != nil {
		return fmt.Errorf("storage: failed to write file: %w", err)
	}
	// Enforce permissions on existing files.
	if err := os.Chmod(full, filePermissions); err != nil {
		return fmt.Errorf("storage: failed to set permissions: %w", err)
	}
	return nil
}

// List returns relative paths of all files under the given prefix directory.
func (s *FileStorage) List(_ context.Context, prefix string) ([]string, error) {
	if prefix != "" {
		if err := validatePath(prefix); err != nil {
			return nil, err
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := s.checkInit(); err != nil {
		return nil, err
	}

	dir := filepath.Join(s.baseDir, filepath.Clean(prefix))
	var results []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		// Skip .bak files — internal implementation detail.
		if strings.HasSuffix(path, ".bak") {
			return nil
		}
		rel, err := filepath.Rel(s.baseDir, path)
		if err != nil {
			return err
		}
		results = append(results, rel)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("storage: failed to list: %w", err)
	}

	if results == nil {
		results = []string{}
	}
	return results, nil
}

// Delete removes the file at the given logical path.
// Returns nil if the file is already absent (idempotent).
func (s *FileStorage) Delete(_ context.Context, path string) error {
	if err := validatePath(path); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkInit(); err != nil {
		return err
	}

	full := s.fullPath(path)
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("storage: failed to delete: %w", err)
	}
	// Also remove backup file if present.
	_ = os.Remove(full + ".bak")
	return nil
}

// PutJSON marshals v as indented JSON and stores it at the given path.
// Performs a round-trip validation before writing to detect marshaling issues.
func (s *FileStorage) PutJSON(ctx context.Context, path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("storage: failed to marshal JSON: %w", err)
	}
	// Round-trip validation — ensure we can unmarshal what we just marshaled.
	var check json.RawMessage
	if err := json.Unmarshal(data, &check); err != nil {
		return fmt.Errorf("storage: JSON round-trip validation failed: %w", err)
	}
	return s.Put(ctx, path, data)
}

// GetJSON reads the file at path and unmarshals it as JSON into v.
func (s *FileStorage) GetJSON(ctx context.Context, path string, v any) error {
	data, err := s.Get(ctx, path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("storage: failed to unmarshal JSON: %w", err)
	}
	return nil
}

// PluginStatePath returns the sanitized storage path for a plugin's state key.
// Plugin names must be alphanumeric with hyphens only.
func PluginStatePath(pluginName, key string) (string, error) {
	if err := validatePluginName(pluginName); err != nil {
		return "", err
	}
	if err := validatePath(key); err != nil {
		return "", err
	}
	return filepath.Join("plugins", pluginName, "state", key), nil
}

// validatePluginName checks that plugin name contains only alphanumeric characters and hyphens.
func validatePluginName(name string) error {
	if name == "" {
		return fmt.Errorf("storage: empty plugin name")
	}
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-') {
			return fmt.Errorf("storage: invalid plugin name %q: only alphanumeric and hyphens allowed", name)
		}
	}
	return nil
}

// backupIfExists copies the file at full to full.bak if it exists.
func (s *FileStorage) backupIfExists(full string) {
	data, err := os.ReadFile(full)
	if err != nil {
		return // File doesn't exist or unreadable — no backup needed.
	}
	bakPath := full + ".bak"
	if err := os.WriteFile(bakPath, data, filePermissions); err != nil {
		slog.Warn("storage: failed to create backup", "path", bakPath, "error", err)
	}
}

// recoverFromBackup attempts to read from the .bak file.
// Does NOT restore the primary file — Get holds RLock, writing here would race.
// The backup data is returned directly; the caller or a separate repair process
// can restore the primary file under a write lock if needed.
func (s *FileStorage) recoverFromBackup(full string, originalErr error) ([]byte, error) {
	bakPath := full + ".bak"
	data, err := os.ReadFile(bakPath)
	if err != nil {
		// No backup available — return original error.
		return nil, originalErr
	}

	slog.Warn("storage: primary file corrupt, serving from backup",
		"path", full, "error", originalErr)
	return data, nil
}
