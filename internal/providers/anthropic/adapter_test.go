// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package anthropic

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/providers"
)

// mockCredentialStore implements core.CredentialStore for testing.
type mockCredentialStore struct {
	apiKey string
	err    error
}

func (m *mockCredentialStore) Init(_ context.Context) error  { return nil }
func (m *mockCredentialStore) Start(_ context.Context) error { return nil }
func (m *mockCredentialStore) Stop(_ context.Context) error  { return nil }
func (m *mockCredentialStore) Health() error                 { return nil }

func (m *mockCredentialStore) GetProvider(_ context.Context, provider string) (core.Credential, error) {
	if m.err != nil {
		return core.Credential{}, m.err
	}
	return core.Credential{Value: m.apiKey}, nil
}

func (m *mockCredentialStore) SetProvider(_ context.Context, _ string, _ core.Credential) error {
	return nil
}

func (m *mockCredentialStore) GetPluginCredential(_ context.Context, _ string, _ string) (core.Credential, error) {
	return core.Credential{}, nil
}

func (m *mockCredentialStore) SetPluginCredential(_ context.Context, _ string, _ string, _ core.Credential) error {
	return nil
}

func TestInitWithValidCredentials(t *testing.T) {
	store := &mockCredentialStore{apiKey: "sk-ant-test-key"}
	adapter := New(store)

	err := adapter.Init(context.Background())
	if err != nil {
		t.Fatalf("Init should succeed with valid credentials: %v", err)
	}

	if adapter.apiKey != "sk-ant-test-key" {
		t.Fatalf("expected API key to be set")
	}
}

func TestInitWithNilCredStore(t *testing.T) {
	adapter := New(nil)
	err := adapter.Init(context.Background())
	if err == nil {
		t.Fatal("Init should fail with nil credential store")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Fatalf("error should mention 'nil': %v", err)
	}
}

func TestInitWithMissingCredentials(t *testing.T) {
	store := &mockCredentialStore{err: fmt.Errorf("not found")}
	adapter := New(store)

	err := adapter.Init(context.Background())
	if err == nil {
		t.Fatal("Init should fail with missing credentials")
	}
	if !strings.Contains(err.Error(), "anthropic") {
		t.Fatalf("error should mention 'anthropic': %v", err)
	}
}

func TestInitWithEmptyAPIKey(t *testing.T) {
	store := &mockCredentialStore{apiKey: ""}
	adapter := New(store)

	err := adapter.Init(context.Background())
	if err == nil {
		t.Fatal("Init should fail with empty API key")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Fatalf("error should mention 'empty': %v", err)
	}
}

func TestCapabilities(t *testing.T) {
	adapter := New(&mockCredentialStore{apiKey: "test"})

	caps := adapter.Capabilities()

	if !caps.SupportsToolCalls {
		t.Error("should support tool calls")
	}
	if !caps.SupportsThinking {
		t.Error("should support thinking")
	}
	if !caps.SupportsStreaming {
		t.Error("should support streaming")
	}
	if !caps.SupportsSystemPrompt {
		t.Error("should support system prompt")
	}
	if !caps.SupportsVision {
		t.Error("should support vision")
	}
	if caps.MaxContextTokens != 200000 {
		t.Errorf("expected 200000 max context tokens, got %d", caps.MaxContextTokens)
	}
}

func TestHealthWithKey(t *testing.T) {
	adapter := &Adapter{apiKey: "sk-ant-test", client: &http.Client{}}
	if err := adapter.Health(); err != nil {
		t.Fatalf("Health should pass with key: %v", err)
	}
}

func TestHealthWithoutClient(t *testing.T) {
	adapter := &Adapter{apiKey: "sk-ant-test"}
	if err := adapter.Health(); err == nil {
		t.Fatal("Health should fail without client")
	}
}

func TestHealthWithoutKey(t *testing.T) {
	adapter := &Adapter{}
	if err := adapter.Health(); err == nil {
		t.Fatal("Health should fail without key")
	}
}

func TestQueryWithMockServer(t *testing.T) {
	sseResponse := `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","model":"claude-sonnet-4-20250514","usage":{"input_tokens":25}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello!"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}

event: message_stop
data: {"type":"message_stop"}

`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		if r.Header.Get("x-api-key") != "sk-ant-test" {
			t.Error("expected x-api-key header")
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Error("expected anthropic-version header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("expected Content-Type header")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseResponse))
	}))
	defer server.Close()

	adapter := &Adapter{
		apiKey:  "sk-ant-test",
		client:  server.Client(),
		baseURL: server.URL,
	}

	ch, err := adapter.Query(context.Background(), core.QueryRequest{
		Messages:  []core.Message{{Role: "user", Content: "Hi"}},
		MaxTokens: 100,
	})
	if err != nil {
		t.Fatalf("Query should succeed: %v", err)
	}

	var events []core.StreamEvent
	for ev := range ch {
		events = append(events, ev)
	}

	// Expected event sequence: UsageEvent(input), TextChunk, UsageEvent(output), DoneEvent
	if len(events) < 4 {
		t.Fatalf("expected at least 4 events, got %d", len(events))
	}

	// First: UsageEvent with input tokens from message_start
	ue, ok := events[0].(*providers.UsageEvent)
	if !ok {
		t.Fatalf("expected UsageEvent first (input tokens), got %T", events[0])
	}
	if ue.Usage.InputTokens != 25 {
		t.Fatalf("expected 25 input tokens, got %d", ue.Usage.InputTokens)
	}

	// TextChunkEvent
	tc, ok := events[1].(*providers.TextChunkEvent)
	if !ok {
		t.Fatalf("expected TextChunkEvent second, got %T", events[1])
	}
	if tc.Text != "Hello!" {
		t.Fatalf("expected 'Hello!', got %q", tc.Text)
	}

	// UsageEvent with output tokens
	foundOutput := false
	for _, ev := range events {
		if ue, ok := ev.(*providers.UsageEvent); ok {
			if ue.Usage.OutputTokens == 5 {
				foundOutput = true
			}
		}
	}
	if !foundOutput {
		t.Error("expected UsageEvent with 5 output tokens")
	}

	// DoneEvent
	_, ok = events[len(events)-1].(*providers.DoneEvent)
	if !ok {
		t.Fatalf("expected DoneEvent last, got %T", events[len(events)-1])
	}
}

func TestQueryHTTPError401(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"type":"authentication_error","message":"invalid x-api-key"}}`))
	}))
	defer server.Close()

	adapter := &Adapter{
		apiKey:  "bad-key",
		client:  server.Client(),
		baseURL: server.URL,
	}

	_, err := adapter.Query(context.Background(), core.QueryRequest{
		Messages: []core.Message{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "invalid API key") {
		t.Fatalf("expected 'invalid API key' in error, got: %v", err)
	}
}

func TestQueryHTTPError429(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"type":"rate_limit_error","message":"rate limited"}}`))
	}))
	defer server.Close()

	adapter := &Adapter{
		apiKey:  "test-key",
		client:  server.Client(),
		baseURL: server.URL,
	}

	_, err := adapter.Query(context.Background(), core.QueryRequest{
		Messages: []core.Message{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error for 429")
	}
	if !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("expected 'rate limited' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "30") {
		t.Fatalf("expected retry-after in error, got: %v", err)
	}
}

func TestQueryHTTPError500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`internal server error`))
	}))
	defer server.Close()

	adapter := &Adapter{
		apiKey:  "test-key",
		client:  server.Client(),
		baseURL: server.URL,
	}

	_, err := adapter.Query(context.Background(), core.QueryRequest{
		Messages: []core.Message{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if !strings.Contains(err.Error(), "server error") {
		t.Fatalf("expected 'server error' in error, got: %v", err)
	}
}

func TestQueryContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Write start then hang — the context cancel should trigger
		w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"model\":\"test\"}}\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Hang to simulate slow response
		time.Sleep(2 * time.Second)
	}))
	defer server.Close()

	adapter := &Adapter{
		apiKey:  "test-key",
		client:  server.Client(),
		baseURL: server.URL,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	ch, err := adapter.Query(ctx, core.QueryRequest{
		Messages: []core.Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("Query should not fail immediately: %v", err)
	}

	// Drain events — expect an error event due to cancellation
	var gotError bool
	for ev := range ch {
		if _, ok := ev.(*providers.ErrorEvent); ok {
			gotError = true
		}
	}
	if !gotError {
		t.Fatal("expected ErrorEvent from context cancellation")
	}
}

func TestQueryTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adapter := &Adapter{
		apiKey:  "test-key",
		client:  &http.Client{Timeout: 100 * time.Millisecond},
		baseURL: server.URL,
	}

	_, err := adapter.Query(context.Background(), core.QueryRequest{
		Messages: []core.Message{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "anthropic") {
		t.Fatalf("expected 'anthropic' in error, got: %v", err)
	}
}
