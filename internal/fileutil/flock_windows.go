// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

//go:build windows

package fileutil

import (
	"fmt"
	"os"
	"sync"
)

// FileLock provides file-based locking. On Windows, this uses a simple
// mutex-based implementation since flock(2) is not available.
type FileLock struct {
	path string
	file *os.File
	mu   sync.Mutex
}

// NewFileLock creates a lock associated with the given path.
func NewFileLock(path string) *FileLock {
	return &FileLock{path: path + ".lock"}
}

// SharedLock acquires a shared (read) lock.
func (fl *FileLock) SharedLock() error {
	return fl.lock()
}

// ExclusiveLock acquires an exclusive (write) lock.
func (fl *FileLock) ExclusiveLock() error {
	return fl.lock()
}

// Unlock releases the lock and closes the lock file.
func (fl *FileLock) Unlock() error {
	fl.mu.Lock()
	defer fl.mu.Unlock()
	if fl.file == nil {
		return nil
	}
	err := fl.file.Close()
	fl.file = nil
	return err
}

func (fl *FileLock) lock() error {
	fl.mu.Lock()
	defer fl.mu.Unlock()
	if fl.file != nil {
		return fmt.Errorf("fileutil: lock already held on %s", fl.path)
	}
	f, err := os.OpenFile(fl.path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("fileutil: failed to open lock file %s: %w", fl.path, err)
	}
	fl.file = f
	return nil
}
