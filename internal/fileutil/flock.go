// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package fileutil

import (
	"fmt"
	"os"
	"syscall"
)

// FileLock provides advisory file-based locking using flock(2).
type FileLock struct {
	path string
	file *os.File
}

// NewFileLock creates a lock associated with the given path.
// The lock file is created if it does not exist.
func NewFileLock(path string) *FileLock {
	return &FileLock{path: path + ".lock"}
}

// SharedLock acquires a shared (read) lock. Multiple readers can hold shared
// locks simultaneously, but a shared lock blocks exclusive locks.
func (fl *FileLock) SharedLock() error {
	return fl.lock(syscall.LOCK_SH)
}

// ExclusiveLock acquires an exclusive (write) lock. Only one process can hold
// an exclusive lock; it blocks all other locks.
func (fl *FileLock) ExclusiveLock() error {
	return fl.lock(syscall.LOCK_EX)
}

// Unlock releases the lock and closes the lock file.
func (fl *FileLock) Unlock() error {
	if fl.file == nil {
		return nil
	}
	if err := syscall.Flock(int(fl.file.Fd()), syscall.LOCK_UN); err != nil {
		fl.file.Close()
		fl.file = nil
		return fmt.Errorf("fileutil: failed to unlock %s: %w", fl.path, err)
	}
	err := fl.file.Close()
	fl.file = nil
	return err
}

func (fl *FileLock) lock(how int) error {
	if fl.file != nil {
		return fmt.Errorf("fileutil: lock already held on %s", fl.path)
	}
	f, err := os.OpenFile(fl.path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("fileutil: failed to open lock file %s: %w", fl.path, err)
	}
	if err := syscall.Flock(int(f.Fd()), how); err != nil {
		f.Close()
		return fmt.Errorf("fileutil: failed to acquire lock on %s: %w", fl.path, err)
	}
	fl.file = f
	return nil
}
