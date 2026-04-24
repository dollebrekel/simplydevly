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

	// Wait to ensure mtime changes.
	time.Sleep(10 * time.Millisecond)

	err = os.WriteFile(goFile, []byte(`package main

func modified() {}
func added() {}
`), 0o644)
	require.NoError(t, err)

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

	for i := 0; i < 3; i++ {
		f := filepath.Join(dir, "file"+string(rune('a'+i))+".go")
		err := os.WriteFile(f, []byte(`package main
func f`+string(rune('a'+i))+`() {}`), 0o644)
		require.NoError(t, err)
		_, err = cache.GetOrParse(f)
		require.NoError(t, err)
	}

	cache.mu.RLock()
	assert.LessOrEqual(t, len(cache.entries), 2)
	cache.mu.RUnlock()
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
