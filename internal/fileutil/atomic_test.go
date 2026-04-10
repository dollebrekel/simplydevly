// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package fileutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/fileutil"
)

func TestAtomicWriteFile_CreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	data := []byte("hello world")

	err := fileutil.AtomicWriteFile(path, data, 0600)
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, data, got)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestAtomicWriteFile_OverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	require.NoError(t, os.WriteFile(path, []byte("old"), 0600))

	err := fileutil.AtomicWriteFile(path, []byte("new"), 0600)
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, []byte("new"), got)
}

func TestAtomicWriteFile_ErrorOnMissingDir(t *testing.T) {
	// If the target directory doesn't exist, the write fails cleanly.
	path := filepath.Join(t.TempDir(), "nonexistent", "file.txt")

	err := fileutil.AtomicWriteFile(path, []byte("data"), 0600)
	assert.Error(t, err)
}

func TestAtomicWriteFile_PreservesExistingOnFailure(t *testing.T) {
	// If a write fails, the original file must remain intact.
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")
	original := []byte("original content")
	require.NoError(t, os.WriteFile(path, original, 0600))

	// Make the dir read-only to force CreateTemp failure.
	require.NoError(t, os.Chmod(dir, 0500))
	t.Cleanup(func() { os.Chmod(dir, 0700) })

	err := fileutil.AtomicWriteFile(path, []byte("new content"), 0600)
	assert.Error(t, err)

	// Restore read permission and verify original is intact.
	require.NoError(t, os.Chmod(dir, 0700))
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, got, "original file should be intact after failed write")
}

func TestAtomicWriteFile_NoTempFileLeftOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	// Verify no stale temp files are left after a successful write.
	err := fileutil.AtomicWriteFile(path, []byte("data"), 0644)
	require.NoError(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "only the target file should remain, no temp files")
}

func TestAtomicWriteFile_EmptyData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")

	err := fileutil.AtomicWriteFile(path, []byte{}, 0644)
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Empty(t, got)
}
