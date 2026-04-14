// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package kimi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/providers"
)

// stubCredentialStore is a minimal credential store for tests.
type stubCredentialStore struct{ key string }

func (s *stubCredentialStore) Init(_ context.Context) error  { return nil }
func (s *stubCredentialStore) Start(_ context.Context) error { return nil }
func (s *stubCredentialStore) Stop(_ context.Context) error  { return nil }
func (s *stubCredentialStore) Health() error                 { return nil }
func (s *stubCredentialStore) GetProvider(_ context.Context, _ string) (core.Credential, error) {
	return core.Credential{Value: s.key}, nil
}
func (s *stubCredentialStore) SetProvider(_ context.Context, _ string, _ core.Credential) error {
	return nil
}
func (s *stubCredentialStore) GetPluginCredential(_ context.Context, _ string, _ string) (core.Credential, error) {
	return core.Credential{}, nil
}
func (s *stubCredentialStore) SetPluginCredential(_ context.Context, _ string, _ string, _ core.Credential) error {
	return nil
}

// TestKimiAdapter_BelowThreshold_NoCacheCreated verifies that when the system
// prompt is below the caching threshold (4000 tokens), no call is made to the
// cache endpoint (POST /v1/caching).
func TestKimiAdapter_BelowThreshold_NoCacheCreated(t *testing.T) {
	var cacheCallCount int

	// Server responds to /v1/caching and /v1/chat/completions.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == cachingPath {
			cacheCallCount++
			w.WriteHeader(http.StatusBadRequest) // should never be called
			return
		}
		// Minimal streaming response for chat completions.
		w.Header().Set("Content-Type", "text/event-stream")
		textChunk := `{"id":"x","model":"moonshot-v1-128k","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}]}`
		doneChunk := `[DONE]`
		w.Write([]byte("data: " + textChunk + "\n\ndata: " + doneChunk + "\n\n"))
	}))
	defer srv.Close()

	adapter := New(&stubCredentialStore{key: "test-key"})
	adapter.baseURL = srv.URL
	if err := adapter.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	adapter.client = srv.Client()

	req := core.QueryRequest{
		SystemPrompt: "short prompt", // well below 4000 tokens
		Model:        "moonshot-v1-128k",
		Messages:     []core.Message{{Role: "user", Content: "hello"}},
	}

	ch, err := adapter.Query(context.Background(), req)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	// Drain the channel.
	for ev := range ch {
		if errEv, ok := ev.(*providers.ErrorEvent); ok {
			t.Logf("stream error (may be expected on done): %v", errEv.Err)
		}
	}

	if cacheCallCount > 0 {
		t.Errorf("expected no cache API calls for short system prompt, got %d", cacheCallCount)
	}
}

// TestKimiAdapter_AboveThreshold_CacheCreated verifies that when the system
// prompt exceeds the caching threshold, the cache endpoint is called.
func TestKimiAdapter_AboveThreshold_CacheCreated(t *testing.T) {
	var cacheCallCount int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == cachingPath {
			cacheCallCount++
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"cache_id":"cache_xyz","expire_at":9999999999,"tokens":5000,"status":"ok"}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	adapter := New(&stubCredentialStore{key: "test-key"})
	adapter.baseURL = srv.URL
	if err := adapter.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	adapter.client = srv.Client()

	// Build a prompt that exceeds the 4000-token threshold (4 chars/token → 16001 chars).
	longPrompt := strings.Repeat("a", 16001*4)

	req := core.QueryRequest{
		SystemPrompt: longPrompt,
		Model:        "moonshot-v1-128k",
		Messages:     []core.Message{{Role: "user", Content: "hello"}},
	}

	ch, err := adapter.Query(context.Background(), req)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	for range ch {
	}

	if cacheCallCount != 1 {
		t.Errorf("expected 1 cache API call, got %d", cacheCallCount)
	}
}
