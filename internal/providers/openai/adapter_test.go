package openai

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
	store := &mockCredentialStore{apiKey: "sk-test-key"}
	adapter := New(store)

	err := adapter.Init(context.Background())
	if err != nil {
		t.Fatalf("Init should succeed with valid credentials: %v", err)
	}
	if adapter.apiKey != "sk-test-key" {
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

func TestInitWithMissingCredentials(t *testing.T) {
	store := &mockCredentialStore{err: fmt.Errorf("not found")}
	adapter := New(store)

	err := adapter.Init(context.Background())
	if err == nil {
		t.Fatal("Init should fail with missing credentials")
	}
	if !strings.Contains(err.Error(), "openai") {
		t.Fatalf("error should mention 'openai': %v", err)
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
	adapter := &Adapter{apiKey: "sk-test", client: &http.Client{}}
	if err := adapter.Health(); err != nil {
		t.Fatalf("Health should pass with key: %v", err)
	}
}

func TestHealthWithoutClient(t *testing.T) {
	adapter := &Adapter{apiKey: "sk-test"}
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

func TestQueryWithoutInit(t *testing.T) {
	adapter := &Adapter{}
	_, err := adapter.Query(context.Background(), core.QueryRequest{
		Messages: []core.Message{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("Query should fail without Init")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("error should mention 'not initialized': %v", err)
	}
}

func TestQueryEmptyMessages(t *testing.T) {
	adapter := &Adapter{apiKey: "test", client: &http.Client{}}
	_, err := adapter.Query(context.Background(), core.QueryRequest{})
	if err == nil {
		t.Fatal("Query should fail with empty messages")
	}
	if !strings.Contains(err.Error(), "at least one message") {
		t.Fatalf("error should mention 'at least one message': %v", err)
	}
}

func TestQueryWithMockServer(t *testing.T) {
	sseResponse := `data: {"id":"chatcmpl-1","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"Hello!"},"finish_reason":null}]}

data: {"id":"chatcmpl-1","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":25,"completion_tokens":5,"total_tokens":30}}

data: [DONE]

`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Error("expected Authorization header")
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
		apiKey:  "sk-test",
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

	// Expected: TextChunk, UsageEvent, DoneEvent
	if len(events) < 3 {
		t.Fatalf("expected at least 3 events, got %d", len(events))
	}

	// TextChunkEvent
	tc, ok := events[0].(*providers.TextChunkEvent)
	if !ok {
		t.Fatalf("expected TextChunkEvent first, got %T", events[0])
	}
	if tc.Text != "Hello!" {
		t.Fatalf("expected 'Hello!', got %q", tc.Text)
	}

	// UsageEvent
	foundUsage := false
	for _, ev := range events {
		if ue, ok := ev.(*providers.UsageEvent); ok {
			if ue.Usage.InputTokens == 25 && ue.Usage.OutputTokens == 5 {
				foundUsage = true
			}
		}
	}
	if !foundUsage {
		t.Error("expected UsageEvent with 25 input and 5 output tokens")
	}

	// DoneEvent last
	_, ok = events[len(events)-1].(*providers.DoneEvent)
	if !ok {
		t.Fatalf("expected DoneEvent last, got %T", events[len(events)-1])
	}
}

func TestQueryWithToolCalls(t *testing.T) {
	sseResponse := `data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"file_read","arguments":""}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":\"/tmp/test\"}"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[],"usage":{"prompt_tokens":50,"completion_tokens":20,"total_tokens":70}}

data: [DONE]

`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseResponse))
	}))
	defer server.Close()

	adapter := &Adapter{
		apiKey:  "sk-test",
		client:  server.Client(),
		baseURL: server.URL,
	}

	ch, err := adapter.Query(context.Background(), core.QueryRequest{
		Messages: []core.Message{{Role: "user", Content: "Read /tmp/test"}},
	})
	if err != nil {
		t.Fatalf("Query should succeed: %v", err)
	}

	var events []core.StreamEvent
	for ev := range ch {
		events = append(events, ev)
	}

	// Find ToolCallEvent
	foundTool := false
	for _, ev := range events {
		if tc, ok := ev.(*providers.ToolCallEvent); ok {
			foundTool = true
			if tc.ToolName != "file_read" {
				t.Errorf("expected tool name 'file_read', got %q", tc.ToolName)
			}
			if tc.ToolID != "call_1" {
				t.Errorf("expected tool ID 'call_1', got %q", tc.ToolID)
			}
			if string(tc.Input) != `{"path":"/tmp/test"}` {
				t.Errorf("unexpected input: %s", string(tc.Input))
			}
		}
	}
	if !foundTool {
		t.Fatal("expected ToolCallEvent")
	}
}

func TestQueryHTTPError401(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
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
		w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hi\"},\"finish_reason\":null}]}\n\n"))
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
	if !strings.Contains(err.Error(), "openai") {
		t.Fatalf("expected 'openai' in error, got: %v", err)
	}
}
