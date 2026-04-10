// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package fileutil_test

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/fileutil"
)

func TestFileLock_ExclusivePreventsOverlap(t *testing.T) {
	// B8: verify exclusive lock prevents concurrent writes.
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test")

	var overlap atomic.Int32
	var maxOverlap atomic.Int32
	var wg sync.WaitGroup
	errs := make(chan error, 20) // buffered for all goroutines

	for i := range 10 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			fl := fileutil.NewFileLock(lockPath)
			if err := fl.ExclusiveLock(); err != nil {
				errs <- err
				return
			}

			cur := overlap.Add(1)
			for {
				old := maxOverlap.Load()
				if cur <= old || maxOverlap.CompareAndSwap(old, cur) {
					break
				}
			}
			// Simulate work.
			for range 1000 {
			}
			overlap.Add(-1)

			if err := fl.Unlock(); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	assert.Equal(t, int32(1), maxOverlap.Load(), "exclusive lock should prevent overlap")
}

func TestFileLock_SharedAllowsConcurrent(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test")

	const n = 5
	var current atomic.Int32
	ready := make(chan struct{}, n)
	release := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, 2*n) // buffered for lock+unlock per goroutine

	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()

			fl := fileutil.NewFileLock(lockPath)
			if err := fl.SharedLock(); err != nil {
				errs <- err
				ready <- struct{}{} // still signal to prevent deadlock
				return
			}
			current.Add(1)
			ready <- struct{}{}
			<-release
			if err := fl.Unlock(); err != nil {
				errs <- err
			}
		}()
	}
	// Wait until all goroutines have acquired the shared lock.
	for range n {
		<-ready
	}
	assert.Equal(t, int32(n), current.Load(), "all shared locks should be held concurrently")
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}

func TestFileLock_UnlockWithoutLock(t *testing.T) {
	fl := fileutil.NewFileLock(filepath.Join(t.TempDir(), "noop"))
	assert.NoError(t, fl.Unlock(), "unlock without lock should not error")
}
