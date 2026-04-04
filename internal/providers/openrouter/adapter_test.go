package openrouter

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

func (m *mockCredentialStore) GetProvider(_ context.Context, _ string) (core.Credential, error) {
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
	store := &mockCredentialStore{apiKey: "sk-or-test-key"}
	adapter := New(store)

	err := adapter.Init(context.Background())
	if err != nil {
		t.Fatalf("Init should succeed: %v", err)
	}
	if adapter.apiKey != "sk-or-test-key" {
		t.Fatal("expected API key to be set")
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

func TestInitWithCredStoreError(t *testing.T) {
	store := &mockCredentialStore{err: fmt.Errorf("unavailable")}
	adapter := New(store)

	err := adapter.Init(context.Background())
	if err == nil {
		t.Fatal("Init should fail")
	}
	if !strings.Contains(err.Error(), "openrouter") {
		t.Fatalf("error should mention 'openrouter': %v", err)
	}
}

func TestCapabilities(t *testing.T) {
	adapter := New(&mockCredentialStore{apiKey: "test"})

	caps := adapter.Capabilities()

	if !caps.SupportsToolCalls {
		t.Error("should support tool calls")
	}
	if caps.SupportsThinking {
		t.Error("should NOT support thinking")
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
	if caps.MaxContextTokens != 128000 {
		t.Errorf("expected 128000 max context tokens, got %d", caps.MaxContextTokens)
	}
}

func TestHealthWithKey(t *testing.T) {
	adapter := &Adapter{apiKey: "test", client: &http.Client{}}
	if err := adapter.Health(); err != nil {
		t.Fatalf("Health should pass: %v", err)
	}
}

func TestHealthWithoutClient(t *testing.T) {
	adapter := &Adapter{apiKey: "test"}
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

func TestQueryExtraHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify OpenRouter-specific headers
		if r.Header.Get("HTTP-Referer") != "https://siply.dev" {
			t.Errorf("expected HTTP-Referer header 'https://siply.dev', got %q", r.Header.Get("HTTP-Referer"))
		}
		if r.Header.Get("X-Title") != "siply" {
			t.Errorf("expected X-Title header 'siply', got %q", r.Header.Get("X-Title"))
		}
		if r.Header.Get("Authorization") != "Bearer sk-or-test" {
			t.Errorf("expected Authorization header, got %q", r.Header.Get("Authorization"))
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"model\":\"anthropic/claude-sonnet-4-20250514\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n\ndata: [DONE]\n\n"))
	}))
	defer server.Close()

	adapter := &Adapter{
		apiKey:  "sk-or-test",
		client:  server.Client(),
		baseURL: server.URL,
	}

	ch, err := adapter.Query(context.Background(), core.QueryRequest{
		Messages: []core.Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("Query should succeed: %v", err)
	}

	var events []core.StreamEvent
	for ev := range ch {
		events = append(events, ev)
	}

	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(events))
	}

	// TextChunkEvent
	tc, ok := events[0].(*providers.TextChunkEvent)
	if !ok {
		t.Fatalf("expected TextChunkEvent, got %T", events[0])
	}
	if tc.Text != "Hello" {
		t.Fatalf("expected 'Hello', got %q", tc.Text)
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

func TestQueryHTTPError402(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
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
		t.Fatal("expected error for 402")
	}
	if !strings.Contains(err.Error(), "insufficient credits") {
		t.Fatalf("expected 'insufficient credits' in error, got: %v", err)
	}
}

func TestQueryHTTPError503(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`model temporarily unavailable`))
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
		t.Fatal("expected error for 503")
	}
	if !strings.Contains(err.Error(), "model unavailable") {
		t.Fatalf("expected 'model unavailable' in error, got: %v", err)
	}
}

func TestQueryContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"model\":\"test\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hi\"},\"finish_reason\":null}]}\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
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
