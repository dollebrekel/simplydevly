// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package marketplace

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testIndexJSON returns a minimal valid marketplace index as JSON bytes.
func testIndexJSON(t *testing.T) []byte {
	t.Helper()
	idx := Index{
		Version:   1,
		UpdatedAt: "2026-04-18T00:00:00Z",
		Items: []Item{
			{
				Name:        "memory-default",
				Category:    "plugins",
				Description: "Default memory plugin",
				Author:      "simplydevly",
				Version:     "1.0.0",
				License:     "Apache-2.0",
				UpdatedAt:   "2026-04-01T00:00:00Z",
			},
			{
				Name:        "tree-view",
				Category:    "extensions",
				Description: "File tree sidebar",
				Author:      "community",
				Version:     "0.5.0",
				License:     "Apache-2.0",
				UpdatedAt:   "2026-03-01T00:00:00Z",
			},
		},
	}
	data, err := json.Marshal(idx)
	require.NoError(t, err)
	return data
}

// newIndexServer creates an httptest.Server that serves indexJSON at /index.json.
// The handler captures the received request for inspection.
func newIndexServer(t *testing.T, status int, body []byte) (*httptest.Server, *http.Request) {
	t.Helper()
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r
		w.WriteHeader(status)
		if body != nil {
			_, _ = w.Write(body)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, captured
}

// TestSyncIndex_Success verifies a successful full download writes the cache file
// and returns synced=true with the correct item count (AC #1).
func TestSyncIndex_Success(t *testing.T) {
	indexData := testIndexJSON(t)

	var capturedReq *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(indexData)
	}))
	t.Cleanup(srv.Close)

	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "marketplace-index.json")

	synced, count, err := SyncIndex(context.Background(), SyncConfig{
		PagesBaseURL: srv.URL,
		CachePath:    cachePath,
	})

	require.NoError(t, err)
	assert.True(t, synced, "expected synced=true on fresh download")
	assert.Equal(t, 2, count, "expected 2 items in index")

	// Verify cache file was written with correct content.
	written, readErr := os.ReadFile(cachePath)
	require.NoError(t, readErr)
	var idx Index
	require.NoError(t, json.Unmarshal(written, &idx))
	assert.Equal(t, 2, len(idx.Items))
	assert.Equal(t, 1, idx.Version)

	// Verify no If-Modified-Since on first fetch (no pre-existing cache).
	assert.Empty(t, capturedReq.Header.Get("If-Modified-Since"))
}

// TestSyncIndex_ConditionalRequest verifies that If-Modified-Since is sent when a
// cache file already exists, and that 304 returns synced=false (AC #2).
func TestSyncIndex_ConditionalRequest(t *testing.T) {
	var capturedReq *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		w.WriteHeader(http.StatusNotModified)
	}))
	t.Cleanup(srv.Close)

	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "marketplace-index.json")

	// Write a pre-existing cache file so os.Stat() finds a mtime.
	require.NoError(t, os.WriteFile(cachePath, testIndexJSON(t), 0644))

	synced, count, err := SyncIndex(context.Background(), SyncConfig{
		PagesBaseURL: srv.URL,
		CachePath:    cachePath,
	})

	require.NoError(t, err)
	assert.False(t, synced, "expected synced=false on 304")
	assert.Equal(t, 0, count)

	// Verify If-Modified-Since header was sent.
	assert.NotEmpty(t, capturedReq.Header.Get("If-Modified-Since"), "expected If-Modified-Since header")

	// Verify existing cache was NOT overwritten.
	still, _ := os.ReadFile(cachePath)
	assert.Equal(t, testIndexJSON(t), still, "cache file should be unchanged after 304")
}

// TestSyncIndex_Force verifies that --force skips the If-Modified-Since header
// and always downloads the full index (AC #3).
func TestSyncIndex_Force(t *testing.T) {
	indexData := testIndexJSON(t)
	var capturedReq *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(indexData)
	}))
	t.Cleanup(srv.Close)

	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "marketplace-index.json")

	// Write a pre-existing cache file.
	require.NoError(t, os.WriteFile(cachePath, testIndexJSON(t), 0644))

	synced, count, err := SyncIndex(context.Background(), SyncConfig{
		PagesBaseURL: srv.URL,
		CachePath:    cachePath,
		Force:        true,
	})

	require.NoError(t, err)
	assert.True(t, synced)
	assert.Equal(t, 2, count)

	// No If-Modified-Since when Force=true.
	assert.Empty(t, capturedReq.Header.Get("If-Modified-Since"),
		"If-Modified-Since must NOT be sent when Force=true")
}

// TestSyncIndex_NetworkError verifies that a network error leaves the existing
// cache file intact (AC #4).
func TestSyncIndex_NetworkError(t *testing.T) {
	// Use a server that immediately closes the connection.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		conn, _, _ := hj.Hijack()
		conn.Close() // abrupt close → network error on client
	}))
	t.Cleanup(srv.Close)

	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "marketplace-index.json")
	original := testIndexJSON(t)
	require.NoError(t, os.WriteFile(cachePath, original, 0644))

	synced, count, err := SyncIndex(context.Background(), SyncConfig{
		PagesBaseURL: srv.URL,
		CachePath:    cachePath,
	})

	require.Error(t, err, "expected error on network failure")
	assert.False(t, synced)
	assert.Equal(t, 0, count)
	// AC #4: error must be actionable — contains enough context to diagnose.
	assert.Contains(t, err.Error(), "marketplace", "error should identify the marketplace subsystem")

	// Existing cache must be untouched.
	still, readErr := os.ReadFile(cachePath)
	require.NoError(t, readErr)
	assert.Equal(t, original, still, "cache file must remain intact on error")
}

// TestSyncIndex_CreatesCacheDir verifies that the cache directory is created if
// it does not exist (AC #7).
func TestSyncIndex_CreatesCacheDir(t *testing.T) {
	indexData := testIndexJSON(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(indexData)
	}))
	t.Cleanup(srv.Close)

	tmpDir := t.TempDir()
	// Use a nested directory that does not yet exist.
	cacheDir := filepath.Join(tmpDir, "new", "nested", "dir")
	cachePath := filepath.Join(cacheDir, "marketplace-index.json")

	synced, count, err := SyncIndex(context.Background(), SyncConfig{
		PagesBaseURL: srv.URL,
		CachePath:    cachePath,
	})

	require.NoError(t, err)
	assert.True(t, synced)
	assert.Equal(t, 2, count)

	// Directory must now exist with correct permissions.
	info, statErr := os.Stat(cacheDir)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())

	// Cache file must exist.
	_, statErr = os.Stat(cachePath)
	assert.NoError(t, statErr)
}

// TestSyncIndex_NonOKStatus verifies that a non-200/304 HTTP response is treated
// as an error and the existing cache is not overwritten (AC #4).
func TestSyncIndex_NonOKStatus(t *testing.T) {
	srv, _ := newIndexServer(t, http.StatusServiceUnavailable, []byte("service unavailable"))

	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "marketplace-index.json")
	original := testIndexJSON(t)
	require.NoError(t, os.WriteFile(cachePath, original, 0644))

	synced, count, err := SyncIndex(context.Background(), SyncConfig{
		PagesBaseURL: srv.URL,
		CachePath:    cachePath,
	})

	require.Error(t, err)
	assert.False(t, synced)
	assert.Equal(t, 0, count)
	// AC #4: error must be actionable — contains HTTP status context.
	assert.Contains(t, err.Error(), "marketplace", "error should identify the marketplace subsystem")

	// Cache must be untouched.
	still, _ := os.ReadFile(cachePath)
	assert.Equal(t, original, still)
}

// TestSyncIndex_ContextCancellation verifies that a cancelled context returns an
// error and does not write the cache.
func TestSyncIndex_ContextCancellation(t *testing.T) {
	// Server that blocks until the context is cancelled.
	waiting := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-waiting // block until test signals
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(testIndexJSON(t))
	}))
	t.Cleanup(func() {
		close(waiting)
		srv.Close()
	})

	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "marketplace-index.json")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	synced, count, err := SyncIndex(ctx, SyncConfig{
		PagesBaseURL: srv.URL,
		CachePath:    cachePath,
	})

	require.Error(t, err)
	assert.False(t, synced)
	assert.Equal(t, 0, count)

	// No cache file should have been written.
	_, statErr := os.Stat(cachePath)
	assert.True(t, errors.Is(statErr, os.ErrNotExist), "cache must not be written on ctx cancel")
}

func TestDefaultCacheDir_SiplyHome(t *testing.T) {
	t.Setenv("SIPLY_HOME", "/custom/siply")
	t.Setenv("XDG_CACHE_HOME", "/xdg/cache")

	dir, err := DefaultCacheDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/custom/siply", "cache"), dir)
}

func TestDefaultCacheDir_XDGCache(t *testing.T) {
	t.Setenv("SIPLY_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "/xdg/cache")

	dir, err := DefaultCacheDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/xdg/cache", "siply", "marketplace"), dir)
}

func TestDefaultCacheDir_Default(t *testing.T) {
	t.Setenv("SIPLY_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")

	dir, err := DefaultCacheDir()
	require.NoError(t, err)

	home, _ := os.UserHomeDir()
	assert.Equal(t, filepath.Join(home, ".siply", "cache"), dir)
}
