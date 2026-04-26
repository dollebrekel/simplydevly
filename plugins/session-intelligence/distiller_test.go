// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newMockOllama(response string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/generate":
			json.NewEncoder(w).Encode(ollamaResponse{Response: response})
		case "/api/tags":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"models":[]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestSessionDistiller_MinTurns(t *testing.T) {
	srv := newMockOllama(`{}`)
	defer srv.Close()

	client := NewOllamaClient(srv.URL, "test-model")
	distiller := NewSessionDistiller(client, 5)

	msgs := []Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
	}

	_, err := distiller.DistillSession(t.Context(), msgs)
	if err == nil {
		t.Error("expected error for too few turns")
	}
}

func TestSessionDistiller_Success(t *testing.T) {
	response := `{
		"key_decisions": ["use JWT for auth"],
		"active_files": ["internal/auth/jwt.go"],
		"current_task": "implementing auth middleware",
		"constraints": ["must be backward compatible"],
		"patterns": [{"pattern": "guard clauses", "confidence": "medium"}]
	}`
	srv := newMockOllama(response)
	defer srv.Close()

	client := NewOllamaClient(srv.URL, "test-model")
	distiller := NewSessionDistiller(client, 2)

	msgs := []Message{
		{Role: "user", Content: "let's implement auth"},
		{Role: "assistant", Content: "sure, I'll use JWT"},
		{Role: "user", Content: "add middleware"},
		{Role: "assistant", Content: "done"},
	}

	d, err := distiller.DistillSession(t.Context(), msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(d.Content.KeyDecisions) != 1 {
		t.Errorf("expected 1 key decision, got %d", len(d.Content.KeyDecisions))
	}
	if d.Content.CurrentTask != "implementing auth middleware" {
		t.Errorf("unexpected current task: %s", d.Content.CurrentTask)
	}
	if len(d.Content.Patterns) != 1 {
		t.Errorf("expected 1 pattern, got %d", len(d.Content.Patterns))
	}
}

func TestSessionDistiller_FiltersSystemAndToolMessages(t *testing.T) {
	response := `{"key_decisions": [], "active_files": [], "current_task": "test", "constraints": [], "patterns": []}`
	srv := newMockOllama(response)
	defer srv.Close()

	client := NewOllamaClient(srv.URL, "test-model")
	distiller := NewSessionDistiller(client, 2)

	msgs := []Message{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
		{Role: "system", Content: "[Code Context — tree-sitter] func main()"},
		{Role: "user", Content: "edit file"},
		{Role: "assistant", Content: "done", ToolID: "tool-123"},
		{Role: "user", Content: "thanks"},
		{Role: "assistant", Content: "you're welcome"},
	}

	d, err := distiller.DistillSession(t.Context(), msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d == nil {
		t.Fatal("expected distillate, got nil")
	}
}

func TestParseDistillateContent_ValidJSON(t *testing.T) {
	input := `{"key_decisions": ["a"], "active_files": ["b.go"], "current_task": "c", "constraints": [], "patterns": []}`
	content, err := parseDistillateContent(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(content.KeyDecisions) != 1 || content.KeyDecisions[0] != "a" {
		t.Error("key decisions not parsed correctly")
	}
}

func TestParseDistillateContent_WrappedJSON(t *testing.T) {
	input := "Here is the JSON:\n```json\n{\"key_decisions\": [\"a\"], \"active_files\": [], \"current_task\": \"c\", \"constraints\": [], \"patterns\": []}\n```"
	content, err := parseDistillateContent(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content.CurrentTask != "c" {
		t.Errorf("expected current_task 'c', got %s", content.CurrentTask)
	}
}

func TestParseDistillateContent_InvalidJSON(t *testing.T) {
	_, err := parseDistillateContent("not json at all")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestCountConversationTurns(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: "prompt"},
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
		{Role: "user", Content: "bye"},
	}
	if turns := countConversationTurns(msgs); turns != 3 {
		t.Errorf("expected 3 turns, got %d", turns)
	}
}

func TestEstimateTokens(t *testing.T) {
	s := "hello world, this is a test string"
	tokens := estimateTokens(s)
	if tokens < 1 {
		t.Errorf("expected at least 1 token, got %d", tokens)
	}
}

func TestOllamaClient_HealthCheck(t *testing.T) {
	srv := newMockOllama("")
	defer srv.Close()

	client := NewOllamaClient(srv.URL, "test-model")
	if err := client.HealthCheck(t.Context()); err != nil {
		t.Errorf("health check should pass: %v", err)
	}
}

func TestOllamaClient_HealthCheck_Unreachable(t *testing.T) {
	client := NewOllamaClient("http://127.0.0.1:1", "test-model")
	if err := client.HealthCheck(t.Context()); err == nil {
		t.Error("health check should fail when Ollama is unreachable")
	}
}
