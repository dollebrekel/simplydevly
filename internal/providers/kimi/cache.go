// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package kimi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// CacheManager manages a single Kimi context cache for the session lifetime.
// It is goroutine-safe: multiple goroutines may concurrently call EnsureCache.
type CacheManager struct {
	mu       sync.Mutex
	cacheID  string
	expireAt time.Time // zero means no cache set
}

// NewCacheManager creates an empty CacheManager.
func NewCacheManager() *CacheManager {
	return &CacheManager{}
}

// EnsureCache returns the active cache_id, creating a new cache when none
// exists or the existing one has expired. On error it returns ("", err);
// callers should fall back to sending the full request.
func (cm *CacheManager) EnsureCache(
	ctx context.Context,
	client *http.Client,
	baseURL, apiKey string,
	systemPrompt string,
	tools []apiTool,
	model string,
) (string, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.cacheID != "" && !cm.isExpiredLocked() {
		return cm.cacheID, nil
	}

	// Create a fresh cache.
	id, expireAt, err := createCache(ctx, client, baseURL, apiKey, model, systemPrompt, tools)
	if err != nil {
		// Invalidate stale id so next call retries.
		cm.cacheID = ""
		cm.expireAt = time.Time{}
		return "", err
	}

	cm.cacheID = id
	cm.expireAt = expireAt
	return id, nil
}

// IsExpired reports whether the current cache entry is expired or missing.
// Safe to call from any goroutine.
func (cm *CacheManager) IsExpired() bool {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.isExpiredLocked()
}

// isExpiredLocked must be called with cm.mu held.
func (cm *CacheManager) isExpiredLocked() bool {
	if cm.cacheID == "" {
		return true
	}
	// Apply a 60-second safety margin to avoid using a cache that's about to expire.
	return time.Now().Add(60 * time.Second).After(cm.expireAt)
}

// Invalidate clears the current cache entry.
func (cm *CacheManager) Invalidate() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.cacheID = ""
	cm.expireAt = time.Time{}
}

// createCache calls POST /v1/caching and returns the cache_id and expiry time.
func createCache(
	ctx context.Context,
	client *http.Client,
	baseURL, apiKey string,
	model, systemPrompt string,
	tools []apiTool,
) (string, time.Time, error) {
	payload, err := buildCacheRequest(model, systemPrompt, tools)
	if err != nil {
		return "", time.Time{}, err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("kimi cache: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+cachingPath, bytes.NewReader(body))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("kimi cache: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("kimi cache: HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if len(raw) > 0 {
			return "", time.Time{}, fmt.Errorf("kimi cache: unexpected status %d: %s", resp.StatusCode, string(raw))
		}
		return "", time.Time{}, fmt.Errorf("kimi cache: unexpected status %d", resp.StatusCode)
	}

	var result cacheCreateResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&result); err != nil {
		return "", time.Time{}, fmt.Errorf("kimi cache: decode response: %w", err)
	}

	if result.CacheID == "" {
		return "", time.Time{}, fmt.Errorf("kimi cache: empty cache_id in response")
	}

	var expireAt time.Time
	if result.ExpireAt > 0 {
		expireAt = time.Unix(result.ExpireAt, 0)
	} else {
		// Default TTL: 1 hour from now.
		expireAt = time.Now().Add(time.Hour)
	}

	return result.CacheID, expireAt, nil
}
