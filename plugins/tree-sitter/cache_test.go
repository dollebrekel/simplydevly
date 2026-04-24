// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileCache_GetOrParse(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "main.go")

	err := os.WriteFile(goFile, []byte(`package main

func hello() {}
`), 0o644)
	require.NoError(t, err)

	parser := NewParser()
	cache := NewFileCache(parser, 100)

	symbols, err := cache.GetOrParse(goFile)
	require.NoError(t, err)
	assert.NotEmpty(t, symbols)

	stats := cache.Stats()
	assert.Equal(t, int64(1), stats.Misses)
	assert.Equal(t, int64(0), stats.Hits)

	symbols2, err := cache.GetOrParse(goFile)
	require.NoError(t, err)
	assert.Equal(t, len(symbols), len(symbols2))

	stats = cache.Stats()
	assert.Equal(t, int64(1), stats.Misses)
	assert.Equal(t, int64(1), stats.Hits)
}

func TestFileCache_MtimeInvalidation(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "main.go")

	err := os.WriteFile(goFile, []byte(`package main

func original() {}
`), 0o644)
	require.NoError(t, err)

	parser := NewParser()
	cache := NewFileCache(parser, 100)

	symbols1, err := cache.GetOrParse(goFile)
	require.NoError(t, err)
	assert.Len(t, symbols1, 1)

	origInfo, err := os.Stat(goFile)
	require.NoError(t, err)
	for {
		err = os.WriteFile(goFile, []byte(`package main

func modified() {}
func added() {}
`), 0o644)
		require.NoError(t, err)
		newInfo, statErr := os.Stat(goFile)
		require.NoError(t, statErr)
		if !newInfo.ModTime().Equal(origInfo.ModTime()) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	symbols2, err := cache.GetOrParse(goFile)
	require.NoError(t, err)
	assert.Len(t, symbols2, 2)

	stats := cache.Stats()
	assert.Equal(t, int64(2), stats.Misses)
}

func TestFileCache_Invalidate(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "main.go")

	err := os.WriteFile(goFile, []byte(`package main

func hello() {}
`), 0o644)
	require.NoError(t, err)

	parser := NewParser()
	cache := NewFileCache(parser, 100)

	_, err = cache.GetOrParse(goFile)
	require.NoError(t, err)

	cache.Invalidate(goFile)

	_, err = cache.GetOrParse(goFile)
	require.NoError(t, err)

	stats := cache.Stats()
	assert.Equal(t, int64(2), stats.Misses)
}

func TestFileCache_CacheHitRate(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "main.go")

	err := os.WriteFile(goFile, []byte(`package main

func hello() {}
`), 0o644)
	require.NoError(t, err)

	parser := NewParser()
	cache := NewFileCache(parser, 100)

	assert.Equal(t, 0.0, cache.CacheHitRate())

	_, _ = cache.GetOrParse(goFile)
	_, _ = cache.GetOrParse(goFile)
	_, _ = cache.GetOrParse(goFile)

	assert.InDelta(t, 0.666, cache.CacheHitRate(), 0.01)
}

func TestFileCache_LRUEviction(t *testing.T) {
	dir := t.TempDir()
	parser := NewParser()
	cache := NewFileCache(parser, 2)

	fileA := filepath.Join(dir, "filea.go")
	fileB := filepath.Join(dir, "fileb.go")
	fileC := filepath.Join(dir, "filec.go")

	for _, f := range []struct{ path, body string }{
		{fileA, "package main\nfunc fa() {}"},
		{fileB, "package main\nfunc fb() {}"},
	} {
		require.NoError(t, os.WriteFile(f.path, []byte(f.body), 0o644))
		_, err := cache.GetOrParse(f.path)
		require.NoError(t, err)
	}

	// Touch fileA so it becomes most-recently-used; fileB is now LRU.
	_, err := cache.GetOrParse(fileA)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(fileC, []byte("package main\nfunc fc() {}"), 0o644))
	_, err = cache.GetOrParse(fileC)
	require.NoError(t, err)

	cache.mu.RLock()
	_, hasA := cache.entries[fileA]
	_, hasB := cache.entries[fileB]
	_, hasC := cache.entries[fileC]
	cache.mu.RUnlock()

	assert.True(t, hasA, "fileA should survive (most recently used)")
	assert.False(t, hasB, "fileB should be evicted (least recently used)")
	assert.True(t, hasC, "fileC should be present (just added)")
}

func TestFileCache_NonexistentFile(t *testing.T) {
	parser := NewParser()
	cache := NewFileCache(parser, 100)

	_, err := cache.GetOrParse("/nonexistent/file.go")
	assert.Error(t, err)
}

func TestFileCache_UnsupportedFile(t *testing.T) {
	dir := t.TempDir()
	jsFile := filepath.Join(dir, "app.js")
	err := os.WriteFile(jsFile, []byte("console.log('hello')"), 0o644)
	require.NoError(t, err)

	parser := NewParser()
	cache := NewFileCache(parser, 100)

	_, err = cache.GetOrParse(jsFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported")
}
