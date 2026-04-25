// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOllamaClient_Distill(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/generate" {
			t.Errorf("expected /api/generate, got %s", r.URL.Path)
		}

		var req ollamaRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Stream {
			t.Error("expected stream=false")
		}
		if req.Model != "test-model" {
			t.Errorf("expected model test-model, got %s", req.Model)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ollamaResponse{
			Response: "KEY_DECISIONS: use Ollama\nACTIVE_FILES: main.go",
		})
	}))
	defer srv.Close()

	client := NewOllamaClient(srv.URL, "test-model")
	result, err := client.Distill(context.Background(), "test prompt")
	if err != nil {
		t.Fatalf("Distill() error: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestOllamaClient_Distill_ConnectionRefused(t *testing.T) {
	client := NewOllamaClient("http://127.0.0.1:1", "test-model")
	_, err := client.Distill(context.Background(), "test prompt")
	if err == nil {
		t.Error("expected error for connection refused")
	}
}

func TestOllamaClient_Distill_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	client := NewOllamaClient(srv.URL, "test-model")
	_, err := client.Distill(ctx, "test prompt")
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestOllamaClient_Distill_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("model not found"))
	}))
	defer srv.Close()

	client := NewOllamaClient(srv.URL, "nonexistent")
	_, err := client.Distill(context.Background(), "test prompt")
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestOllamaClient_HealthCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("expected /api/tags, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"models":[]}`))
	}))
	defer srv.Close()

	client := NewOllamaClient(srv.URL, "test-model")
	if err := client.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck() error: %v", err)
	}
}

func TestOllamaClient_HealthCheck_Down(t *testing.T) {
	client := NewOllamaClient("http://127.0.0.1:1", "test-model")
	if err := client.HealthCheck(context.Background()); err == nil {
		t.Error("expected error for health check when Ollama is down")
	}
}
