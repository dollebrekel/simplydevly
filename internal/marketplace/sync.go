// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package marketplace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"siply.dev/siply/internal/fileutil"
)

// SyncConfig holds configuration for SyncIndex.
type SyncConfig struct {
	// PagesBaseURL overrides the default GitHub Pages URL (useful for testing).
	// When empty, the URL is derived from DefaultRepoConfig() / SIPLY_MARKET_REPO.
	PagesBaseURL string
	// CachePath is the local path where the marketplace index JSON is cached.
	CachePath string
	// Force skips the If-Modified-Since header and always downloads the index.
	Force bool
	// HTTPClient overrides the default HTTP client (useful for testing).
	HTTPClient *http.Client
}

// SyncIndex fetches the marketplace index from GitHub Pages and caches it locally.
//
//   - Returns (true, N, nil) when a fresh index was downloaded and written (N items).
//   - Returns (false, 0, nil) when the server returns 304 Not Modified (cache is current).
//   - Returns (false, 0, err) on any error; the existing cache file is NOT modified.
func SyncIndex(ctx context.Context, cfg SyncConfig) (synced bool, itemCount int, err error) {
	// Ensure the cache directory exists (AC #7).
	dir := filepath.Dir(cfg.CachePath)
	if mkErr := os.MkdirAll(dir, 0755); mkErr != nil {
		return false, 0, fmt.Errorf("marketplace sync: create cache directory: %w", mkErr)
	}

	// Determine If-Modified-Since from cache file mtime (AC #2, #3).
	var ifModifiedSince *time.Time
	if !cfg.Force {
		if info, statErr := os.Stat(cfg.CachePath); statErr == nil {
			mtime := info.ModTime()
			ifModifiedSince = &mtime
		}
	}

	// Create client; override pagesBaseURL if specified (AC #6, testability).
	client := NewClient(NewClientConfig{HTTPClient: cfg.HTTPClient})
	if cfg.PagesBaseURL != "" {
		client.pagesBaseURL = cfg.PagesBaseURL
	}

	// Fetch the index from GitHub Pages (AC #1, #2, #3, #4, #6).
	idx, fetchErr := client.FetchIndex(ctx, ifModifiedSince)
	if fetchErr != nil {
		if errors.Is(fetchErr, ErrIndexNotModified) {
			// 304 Not Modified — cache is current (AC #2).
			return false, 0, nil
		}
		// Network or HTTP error — return as-is; existing cache is untouched (AC #4).
		return false, 0, fetchErr
	}

	// Marshal index back to JSON for caching (AC #1).
	data, marshalErr := json.Marshal(idx)
	if marshalErr != nil {
		return false, 0, fmt.Errorf("marketplace sync: marshal index: %w", marshalErr)
	}

	// Atomic write — no partial writes on crash (AC #4).
	if writeErr := fileutil.AtomicWriteFile(cfg.CachePath, data, 0644); writeErr != nil {
		return false, 0, fmt.Errorf("marketplace sync: write cache: %w", writeErr)
	}

	return true, len(idx.Items), nil
}
