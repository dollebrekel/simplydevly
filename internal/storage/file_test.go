// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/testing/storagetest"
	"siply.dev/siply/internal/storage"
)

// newTestStorage creates an initialized FileStorage in a temp directory.
func newTestStorage(t *testing.T) *storage.FileStorage {
	t.Helper()
	dir := t.TempDir()
	s := storage.NewFileStorage(dir)
	require.NoError(t, s.Init(context.Background()))
	return s
}

// AC#6: Contract tests run against FileStorage
func TestFileStorage_ContractTests(t *testing.T) {
	storagetest.RunContractTests(t, func() core.Storage {
		return newTestStorage(t)
	})
}

// AC#7: path traversal rejected
func TestFileStorage_PathTraversal(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	cases := []struct {
		name string
		path string
	}{
		{"dot-dot slash", "../../../etc/passwd"},
		{"mid traversal", "foo/../../etc/passwd"},
		{"absolute path", "/etc/passwd"},
		{"empty path", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.Get(ctx, tc.path)
			assert.Error(t, err, "Get should reject path %q", tc.path)

			err = s.Put(ctx, tc.path, []byte("bad"))
			assert.Error(t, err, "Put should reject path %q", tc.path)

			err = s.Delete(ctx, tc.path)
			assert.Error(t, err, "Delete should reject path %q", tc.path)
		})
	}
}

// AC#7: Init creates baseDir with 0700 permissions
func TestFileStorage_InitPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub", "dir")
	s := storage.NewFileStorage(dir)
	require.NoError(t, s.Init(context.Background()))

	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0700), info.Mode().Perm())
}

// AC#7: Put creates intermediate directories
func TestFileStorage_PutCreatesDirectories(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	err := s.Put(ctx, "deep/nested/dir/file.txt", []byte("data"))
	require.NoError(t, err)

	got, err := s.Get(ctx, "deep/nested/dir/file.txt")
	require.NoError(t, err)
	assert.Equal(t, []byte("data"), got)
}

// AC#7: Operations before Init return error
func TestFileStorage_NotInitialized(t *testing.T) {
	dir := t.TempDir()
	s := storage.NewFileStorage(dir)
	ctx := context.Background()

	_, err := s.Get(ctx, "key")
	assert.Error(t, err)

	err = s.Put(ctx, "key", []byte("val"))
	assert.Error(t, err)

	_, err = s.List(ctx, "prefix")
	assert.Error(t, err)

	err = s.Delete(ctx, "key")
	assert.Error(t, err)

	err = s.Health()
	assert.Error(t, err)
}

// AC#7: Concurrent Get/Put from multiple goroutines
func TestFileStorage_ConcurrentAccess(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		key := "concurrent/key"
		go func() {
			defer wg.Done()
			_ = s.Put(ctx, key, []byte("data"))
		}()
		go func() {
			defer wg.Done()
			_, _ = s.Get(ctx, key)
		}()
	}
	wg.Wait()
}

// AC#5: Corruption recovery from .bak file
func TestFileStorage_CorruptionRecovery(t *testing.T) {
	dir := t.TempDir()
	s := storage.NewFileStorage(dir)
	require.NoError(t, s.Init(context.Background()))
	ctx := context.Background()

	// Write valid data (creates file).
	require.NoError(t, s.Put(ctx, "config.json", []byte(`{"valid": true}`)))

	// Write again to create .bak of the first write.
	require.NoError(t, s.Put(ctx, "config.json", []byte(`{"valid": true, "v2": true}`)))

	// Corrupt the primary file directly.
	fullPath := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(fullPath, []byte{0xFF, 0xFE}, 0644))

	// Make primary file unreadable to trigger recovery.
	require.NoError(t, os.Chmod(fullPath, 0000))

	// Get should recover from .bak.
	got, err := s.Get(ctx, "config.json")
	require.NoError(t, err)
	assert.Equal(t, []byte(`{"valid": true}`), got)

	// Restore permissions for cleanup.
	_ = os.Chmod(fullPath, 0644)
}

// AC#7: PutJSON/GetJSON roundtrip
func TestFileStorage_JSONHelpers(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	type TestData struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	input := TestData{Name: "test", Count: 42}
	err := s.PutJSON(ctx, "cache/marketplace/test.json", input)
	require.NoError(t, err)

	var output TestData
	err = s.GetJSON(ctx, "cache/marketplace/test.json", &output)
	require.NoError(t, err)
	assert.Equal(t, input, output)
}

// AC#4: PluginStatePath validation
func TestPluginStatePath(t *testing.T) {
	cases := []struct {
		name       string
		pluginName string
		key        string
		wantErr    bool
		wantPath   string
	}{
		{
			name:       "valid",
			pluginName: "my-plugin",
			key:        "config",
			wantPath:   filepath.Join("plugins", "my-plugin", "state", "config"),
		},
		{
			name:       "empty plugin name",
			pluginName: "",
			key:        "config",
			wantErr:    true,
		},
		{
			name:       "invalid chars in name",
			pluginName: "my.plugin",
			key:        "config",
			wantErr:    true,
		},
		{
			name:       "slashes in name",
			pluginName: "my/plugin",
			key:        "config",
			wantErr:    true,
		},
		{
			name:       "traversal in key",
			pluginName: "valid",
			key:        "../../../etc/passwd",
			wantErr:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := storage.PluginStatePath(tc.pluginName, tc.key)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantPath, got)
			}
		})
	}
}

// AC#1: Health returns nil after Init
func TestFileStorage_HealthAfterInit(t *testing.T) {
	s := newTestStorage(t)
	assert.NoError(t, s.Health())
}

// AC#1: List returns relative paths, not absolute
func TestFileStorage_ListRelativePaths(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	require.NoError(t, s.Put(ctx, "prefix/a.txt", []byte("a")))
	require.NoError(t, s.Put(ctx, "prefix/b.txt", []byte("b")))

	got, err := s.List(ctx, "prefix")
	require.NoError(t, err)
	for _, p := range got {
		assert.False(t, filepath.IsAbs(p), "path should be relative: %s", p)
	}
}

// AC#1: Delete also removes .bak file
func TestFileStorage_DeleteRemovesBackup(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	require.NoError(t, s.Put(ctx, "bak-test", []byte("v1")))
	require.NoError(t, s.Put(ctx, "bak-test", []byte("v2")))

	require.NoError(t, s.Delete(ctx, "bak-test"))

	_, err := s.Get(ctx, "bak-test")
	assert.Error(t, err)
}
