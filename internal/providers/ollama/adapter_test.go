package ollama

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
	value string
	err   error
}

func (m *mockCredentialStore) Init(_ context.Context) error  { return nil }
func (m *mockCredentialStore) Start(_ context.Context) error { return nil }
func (m *mockCredentialStore) Stop(_ context.Context) error  { return nil }
func (m *mockCredentialStore) Health() error                 { return nil }

func (m *mockCredentialStore) GetProvider(_ context.Context, _ string) (core.Credential, error) {
	if m.err != nil {
		return core.Credential{}, m.err
	}
	return core.Credential{Value: m.value}, nil
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

func TestInitWithDefaultURL(t *testing.T) {
	store := &mockCredentialStore{value: ""}
	adapter := New(store)

	err := adapter.Init(context.Background())
	if err != nil {
		t.Fatalf("Init should succeed with empty credential (default URL): %v", err)
	}
	if adapter.baseURL != "http://localhost:11434" {
		t.Fatalf("expected default base URL, got %q", adapter.baseURL)
	}
}

func TestInitWithCustomURL(t *testing.T) {
	store := &mockCredentialStore{value: "http://myhost:11434"}
	adapter := New(store)

	err := adapter.Init(context.Background())
	if err != nil {
		t.Fatalf("Init should succeed: %v", err)
	}
	if adapter.baseURL != "http://myhost:11434" {
		t.Fatalf("expected custom base URL, got %q", adapter.baseURL)
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

func TestInitWithCredStoreError(t *testing.T) {
	store := &mockCredentialStore{err: fmt.Errorf("store unavailable")}
	adapter := New(store)

	err := adapter.Init(context.Background())
	if err == nil {
		t.Fatal("Init should fail when cred store errors")
	}
	if !strings.Contains(err.Error(), "ollama") {
		t.Fatalf("error should mention 'ollama': %v", err)
	}
}

func TestCapabilities(t *testing.T) {
	adapter := New(&mockCredentialStore{})

	caps := adapter.Capabilities()

	if caps.SupportsToolCalls {
		t.Error("should NOT support tool calls")
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
	if caps.SupportsVision {
		t.Error("should NOT support vision")
	}
	if caps.MaxContextTokens != 8192 {
		t.Errorf("expected 8192 max context tokens, got %d", caps.MaxContextTokens)
	}
}

func TestHealthWithMockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("expected /api/tags, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adapter := &Adapter{
		baseURL: server.URL,
		client:  server.Client(),
	}

	if err := adapter.Health(); err != nil {
		t.Fatalf("Health should pass: %v", err)
	}
}

func TestHealthWithoutClient(t *testing.T) {
	adapter := &Adapter{baseURL: "http://localhost:11434"}
	if err := adapter.Health(); err == nil {
		t.Fatal("Health should fail without client")
	}
}

func TestHealthUnreachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	adapter := &Adapter{
		baseURL: server.URL,
		client:  server.Client(),
	}

	if err := adapter.Health(); err == nil {
		t.Fatal("Health should fail with non-200 status")
	}
}

func TestQueryWithoutInit(t *testing.T) {
	adapter := &Adapter{}
	_, err := adapter.Query(context.Background(), core.QueryRequest{
		Messages: []core.Message{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("Query should fail without Init")
	}
}

func TestQueryEmptyMessages(t *testing.T) {
	adapter := &Adapter{baseURL: "http://localhost:11434", client: &http.Client{}}
	_, err := adapter.Query(context.Background(), core.QueryRequest{})
	if err == nil {
		t.Fatal("Query should fail with empty messages")
	}
}

func TestQueryWithMockServer(t *testing.T) {
	ndjsonResponse := `{"model":"llama3.2","created_at":"2024-01-01T00:00:00Z","message":{"role":"assistant","content":"Hello"},"done":false}
{"model":"llama3.2","created_at":"2024-01-01T00:00:01Z","message":{"role":"assistant","content":"!"},"done":false}
{"model":"llama3.2","created_at":"2024-01-01T00:00:02Z","message":{"role":"assistant","content":""},"done":true,"prompt_eval_count":26,"eval_count":42}
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("expected Content-Type header")
		}

		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(ndjsonResponse))
	}))
	defer server.Close()

	adapter := &Adapter{
		baseURL: server.URL,
		client:  server.Client(),
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

	// Expected: TextChunk("Hello"), TextChunk("!"), UsageEvent, DoneEvent
	if len(events) < 4 {
		t.Fatalf("expected at least 4 events, got %d", len(events))
	}

	// First two: text chunks
	tc1, ok := events[0].(*providers.TextChunkEvent)
	if !ok {
		t.Fatalf("expected TextChunkEvent, got %T", events[0])
	}
	if tc1.Text != "Hello" {
		t.Fatalf("expected 'Hello', got %q", tc1.Text)
	}

	tc2, ok := events[1].(*providers.TextChunkEvent)
	if !ok {
		t.Fatalf("expected TextChunkEvent, got %T", events[1])
	}
	if tc2.Text != "!" {
		t.Fatalf("expected '!', got %q", tc2.Text)
	}

	// UsageEvent
	ue, ok := events[2].(*providers.UsageEvent)
	if !ok {
		t.Fatalf("expected UsageEvent, got %T", events[2])
	}
	if ue.Usage.InputTokens != 26 {
		t.Errorf("expected 26 input tokens, got %d", ue.Usage.InputTokens)
	}
	if ue.Usage.OutputTokens != 42 {
		t.Errorf("expected 42 output tokens, got %d", ue.Usage.OutputTokens)
	}

	// DoneEvent
	_, ok = events[3].(*providers.DoneEvent)
	if !ok {
		t.Fatalf("expected DoneEvent, got %T", events[3])
	}
}

func TestQueryHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"model not found"}`))
	}))
	defer server.Close()

	adapter := &Adapter{
		baseURL: server.URL,
		client:  server.Client(),
	}

	_, err := adapter.Query(context.Background(), core.QueryRequest{
		Messages: []core.Message{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error for 400")
	}
	if !strings.Contains(err.Error(), "bad request") {
		t.Fatalf("expected 'bad request' in error, got: %v", err)
	}
}

func TestQueryContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{\"model\":\"llama3.2\",\"message\":{\"role\":\"assistant\",\"content\":\"Hi\"},\"done\":false}\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		time.Sleep(2 * time.Second)
	}))
	defer server.Close()

	adapter := &Adapter{
		baseURL: server.URL,
		client:  server.Client(),
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
