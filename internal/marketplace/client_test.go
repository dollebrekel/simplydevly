// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package marketplace

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"siply.dev/siply/internal/plugins"
)

func newTestPublishRequest(t *testing.T) PublishRequest {
	t.Helper()

	dir := t.TempDir()
	archivePath := filepath.Join(dir, "test.tar.gz")
	if err := os.WriteFile(archivePath, []byte("fake-archive-data"), 0644); err != nil {
		t.Fatal(err)
	}

	return PublishRequest{
		Token: "test-token-123",
		Manifest: plugins.Metadata{
			Name:        "test-plugin",
			Version:     "1.0.0",
			SiplyMin:    "0.1.0",
			Description: "A test plugin",
			Author:      "test-author",
			License:     "MIT",
			Updated:     "2026-04-17",
		},
		ArchivePath: archivePath,
		SHA256:      "abc123def456",
		ReadmeText:  "# Test Plugin\nA test.",
	}
}

func TestPublish_Success(t *testing.T) {
	t.Parallel()

	expected := PublishResponse{
		Name:    "test-plugin",
		Version: "1.0.0",
		URL:     "siply.dev/marketplace/test-plugin",
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/publish" {
			t.Errorf("expected /api/v1/publish, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token-123" {
			t.Errorf("expected Bearer test-token-123, got %s", got)
		}
		if ct := r.Header.Get("Content-Type"); ct == "" {
			t.Error("expected Content-Type header")
		}

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(expected)
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	req := newTestPublishRequest(t)
	resp, err := client.Publish(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Name != expected.Name {
		t.Errorf("name: got %q, want %q", resp.Name, expected.Name)
	}
	if resp.Version != expected.Version {
		t.Errorf("version: got %q, want %q", resp.Version, expected.Version)
	}
	if resp.URL != expected.URL {
		t.Errorf("url: got %q, want %q", resp.URL, expected.URL)
	}
}

func TestPublish_Unauthorized(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid token"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	req := newTestPublishRequest(t)
	_, err := client.Publish(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if got := err.Error(); got != "marketplace: authentication failed — run 'siply login' first" {
		t.Errorf("unexpected error: %s", got)
	}
}

func TestPublish_VersionConflict(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"version exists"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	req := newTestPublishRequest(t)
	_, err := client.Publish(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for 409")
	}
	if got := err.Error(); got != "marketplace: version already exists — bump the version in manifest.yaml" {
		t.Errorf("unexpected error: %s", got)
	}
}

func TestPublish_ServerError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	req := newTestPublishRequest(t)
	_, err := client.Publish(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

func TestPublish_ContextCanceled(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(PublishResponse{})
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := NewClient(srv.URL)
	req := newTestPublishRequest(t)
	_, err := client.Publish(ctx, req)
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

func TestPublish_InvalidURL(t *testing.T) {
	t.Parallel()

	client := NewClient("://invalid-url")
	req := newTestPublishRequest(t)
	_, err := client.Publish(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}
