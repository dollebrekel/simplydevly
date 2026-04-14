// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package kimi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// newTestCacheServer returns a test server that responds with a cacheCreateResponse.
func newTestCacheServer(t *testing.T, respond func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, r)
	}))
}

func successCacheHandler(cacheID string, expireAt int64) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cacheCreateResponse{
			CacheID:  cacheID,
			ExpireAt: expireAt,
			Tokens:   5000,
			Status:   "ok",
		})
	}
}

// TestKimiCacheManager_EnsureCache verifies that EnsureCache calls POST /v1/caching
// with the correct payload and stores the returned cache_id.
func TestKimiCacheManager_EnsureCache(t *testing.T) {
	expireAt := time.Now().Add(time.Hour).Unix()
	var capturedPath string
	var capturedPayload cacheCreateRequest

	srv := newTestCacheServer(t, func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&capturedPayload)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cacheCreateResponse{
			CacheID:  "cache_test123",
			ExpireAt: expireAt,
			Tokens:   5000,
			Status:   "ok",
		})
	})
	defer srv.Close()

	cm := NewCacheManager()
	client := srv.Client()

	id, err := cm.EnsureCache(context.Background(), client, srv.URL, "test-key", "my system prompt", nil, "moonshot-v1-128k")
	if err != nil {
		t.Fatalf("EnsureCache: unexpected error: %v", err)
	}
	if id != "cache_test123" {
		t.Errorf("got cache_id %q, want %q", id, "cache_test123")
	}
	if capturedPath != cachingPath {
		t.Errorf("got path %q, want %q", capturedPath, cachingPath)
	}
	if capturedPayload.Model != "moonshot-v1-128k" {
		t.Errorf("got model %q, want %q", capturedPayload.Model, "moonshot-v1-128k")
	}
}

// TestKimiCacheManager_ExpiryCheck verifies that an expired cache triggers re-creation.
func TestKimiCacheManager_ExpiryCheck(t *testing.T) {
	var callCount int
	srv := newTestCacheServer(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		expireAt := time.Now().Add(time.Hour).Unix()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cacheCreateResponse{
			CacheID:  "cache_new",
			ExpireAt: expireAt,
			Tokens:   5000,
			Status:   "ok",
		})
	})
	defer srv.Close()

	cm := NewCacheManager()
	// Manually inject an expired cache.
	cm.mu.Lock()
	cm.cacheID = "cache_old"
	cm.expireAt = time.Now().Add(-time.Minute) // already expired
	cm.mu.Unlock()

	id, err := cm.EnsureCache(context.Background(), srv.Client(), srv.URL, "key", "prompt", nil, "moonshot-v1-128k")
	if err != nil {
		t.Fatalf("EnsureCache: unexpected error: %v", err)
	}
	if id != "cache_new" {
		t.Errorf("expected new cache id, got %q", id)
	}
	if callCount != 1 {
		t.Errorf("expected 1 cache creation call, got %d", callCount)
	}
}

// TestKimiCacheManager_FallbackOnError verifies that a 500 from the cache API
// causes EnsureCache to return an error (callers are expected to fall back).
func TestKimiCacheManager_FallbackOnError(t *testing.T) {
	srv := newTestCacheServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer srv.Close()

	cm := NewCacheManager()
	_, err := cm.EnsureCache(context.Background(), srv.Client(), srv.URL, "key", "prompt", nil, "moonshot-v1-128k")
	if err == nil {
		t.Fatal("expected error from 500 response, got nil")
	}

	// After failure, cache_id must be empty so the next call retries.
	if cm.cacheID != "" {
		t.Errorf("expected empty cacheID after error, got %q", cm.cacheID)
	}
}

// TestKimiCacheManager_ConcurrentAccess verifies that EnsureCache is goroutine-safe.
func TestKimiCacheManager_ConcurrentAccess(t *testing.T) {
	var mu sync.Mutex
	callCount := 0

	expireAt := time.Now().Add(time.Hour).Unix()
	srv := newTestCacheServer(t, successCacheHandler("cache_concurrent", expireAt))
	defer srv.Close()

	cm := NewCacheManager()
	client := srv.Client()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := cm.EnsureCache(context.Background(), client, srv.URL, "key", "prompt", nil, "moonshot-v1-128k")
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if id != "cache_concurrent" {
				t.Errorf("unexpected cache id: %q", id)
			}
			mu.Lock()
			callCount++
			mu.Unlock()
		}()
	}
	wg.Wait()

	if callCount != 10 {
		t.Errorf("expected 10 calls, got %d", callCount)
	}
}
