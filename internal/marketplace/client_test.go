// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package marketplace

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"strings"
	"testing"
	"time"

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
		Manifest: plugins.Metadata{
			Name:        "test-plugin",
			Version:     "1.0.0",
			SiplyMin:    "0.1.0",
			Description: "A test plugin",
			Author:      "test-author",
			License:     "MIT",
			Updated:     "2026-04-18",
		},
		ArchivePath: archivePath,
		SHA256:      "abc123def456",
		ReadmeText:  "# Test Plugin\nA test.",
	}
}

// githubAPIMux creates a test server that routes GitHub API endpoints for publish flow.
func githubAPIMux(t *testing.T, opts mockOpts) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	// Create Release
	mux.HandleFunc("POST /repos/test-owner/test-repo/releases", func(w http.ResponseWriter, r *http.Request) {
		if opts.releaseStatus != 0 {
			w.WriteHeader(opts.releaseStatus)
			if opts.releaseStatus == http.StatusUnprocessableEntity {
				_, _ = w.Write([]byte(`{"message":"Validation Failed","errors":[{"code":"already_exists"}]}`))
			} else {
				_, _ = w.Write([]byte(`{"message":"error"}`))
			}
			return
		}

		if got := r.Header.Get("Authorization"); got != "Bearer test-token-123" {
			t.Errorf("expected Bearer test-token-123, got %s", got)
		}
		if got := r.Header.Get("X-GitHub-Api-Version"); got != "2022-11-28" {
			t.Errorf("expected API version 2022-11-28, got %s", got)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["tag_name"] != "test-plugin-v1.0.0" {
			t.Errorf("unexpected tag: %v", body["tag_name"])
		}

		uploadURL := "http://" + r.Host + "/upload{?name,label}"
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":         12345,
			"upload_url": uploadURL,
			"html_url":   "https://github.com/test-owner/test-repo/releases/tag/test-plugin-v1.0.0",
		})
	})

	// Upload Asset
	mux.HandleFunc("POST /upload", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") != "test-plugin-1.0.0.tar.gz" {
			t.Errorf("unexpected asset name: %s", r.URL.Query().Get("name"))
		}
		if r.Header.Get("Content-Type") != "application/gzip" {
			t.Errorf("unexpected content type: %s", r.Header.Get("Content-Type"))
		}

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"browser_download_url": "https://github.com/test-owner/test-repo/releases/download/test-plugin-v1.0.0/test-plugin-1.0.0.tar.gz",
		})
	})

	// Get ref (main branch SHA)
	mux.HandleFunc("GET /repos/test-owner/test-repo/git/ref/heads/main", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": map[string]string{"sha": "abc123main"},
		})
	})

	// Create ref (branch)
	mux.HandleFunc("POST /repos/test-owner/test-repo/git/refs", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(body["ref"], "refs/heads/publish/") {
			t.Errorf("unexpected ref: %s", body["ref"])
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"ref": body["ref"]})
	})

	// Get file contents (index.json)
	mux.HandleFunc("GET /repos/test-owner/test-repo/contents/index.json", func(w http.ResponseWriter, _ *http.Request) {
		idx := Index{
			Version:   1,
			UpdatedAt: "2026-04-18T00:00:00Z",
			Items:     []Item{},
		}
		jsonBytes, _ := json.Marshal(idx)
		encoded := base64.StdEncoding.EncodeToString(jsonBytes)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"content": encoded,
			"sha":     "fileSHA123",
		})
	})

	// Update file contents (commit on branch)
	mux.HandleFunc("PUT /repos/test-owner/test-repo/contents/index.json", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["sha"] != "fileSHA123" {
			t.Errorf("unexpected file SHA: %s", body["sha"])
		}
		if !strings.HasPrefix(body["branch"], "publish/") {
			t.Errorf("unexpected branch: %s", body["branch"])
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"content": map[string]string{"sha": "newSHA"}})
	})

	// Create PR
	mux.HandleFunc("POST /repos/test-owner/test-repo/pulls", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["base"] != "main" {
			t.Errorf("unexpected base: %s", body["base"])
		}
		if !strings.HasPrefix(body["head"], "publish/") {
			t.Errorf("unexpected head: %s", body["head"])
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"number": 1})
	})

	return httptest.NewServer(mux)
}

type mockOpts struct {
	releaseStatus int // override release creation status code
}

func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()

	return NewClient(NewClientConfig{
		RepoOwner:  "test-owner",
		RepoName:   "test-repo",
		Token:      "test-token-123",
		HTTPClient: srv.Client(),
	})
}

func TestPublish_Success(t *testing.T) {
	t.Parallel()

	srv := githubAPIMux(t, mockOpts{})
	defer srv.Close()

	// Override githubAPIBase by using a custom HTTP client that redirects to the test server.
	client := &Client{
		repoOwner:    "test-owner",
		repoName:     "test-repo",
		token:        "test-token-123",
		httpClient:   srv.Client(),
		pagesBaseURL: srv.URL,
	}
	// Patch the API base to point to test server.
	origBase := githubAPIBase
	_ = origBase

	// For testing, we need a client that talks to the test server.
	// We'll use a transport that rewrites URLs.
	transport := &rewriteTransport{
		base:    srv.URL,
		wrapped: http.DefaultTransport,
	}
	client.httpClient = &http.Client{Transport: transport}

	req := newTestPublishRequest(t)
	resp, err := client.Publish(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Name != "test-plugin" {
		t.Errorf("name: got %q, want %q", resp.Name, "test-plugin")
	}
	if resp.Version != "1.0.0" {
		t.Errorf("version: got %q, want %q", resp.Version, "1.0.0")
	}
	if resp.URL != "https://github.com/test-owner/test-repo/releases/tag/test-plugin-v1.0.0" {
		t.Errorf("url: got %q", resp.URL)
	}
}

// rewriteTransport rewrites api.github.com and uploads.github.com URLs to a test server.
type rewriteTransport struct {
	base    string
	wrapped http.RoundTripper
}

func (rt *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host == "api.github.com" || strings.HasPrefix(req.URL.Host, "uploads.github.com") {
		req = req.Clone(req.Context())
		req.URL.Scheme = "http"
		// Parse base URL to get host
		req.URL.Host = strings.TrimPrefix(rt.base, "http://")
	}
	return rt.wrapped.RoundTrip(req)
}

func TestPublish_Unauthorized(t *testing.T) {
	t.Parallel()

	srv := githubAPIMux(t, mockOpts{releaseStatus: http.StatusUnauthorized})
	defer srv.Close()

	transport := &rewriteTransport{base: srv.URL, wrapped: http.DefaultTransport}
	client := &Client{
		repoOwner:    "test-owner",
		repoName:     "test-repo",
		token:        "bad-token",
		httpClient:   &http.Client{Transport: transport},
		pagesBaseURL: srv.URL,
	}

	req := newTestPublishRequest(t)
	_, err := client.Publish(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got: %v", err)
	}
}

func TestPublish_VersionConflict(t *testing.T) {
	t.Parallel()

	srv := githubAPIMux(t, mockOpts{releaseStatus: http.StatusUnprocessableEntity})
	defer srv.Close()

	transport := &rewriteTransport{base: srv.URL, wrapped: http.DefaultTransport}
	client := &Client{
		repoOwner:    "test-owner",
		repoName:     "test-repo",
		token:        "test-token",
		httpClient:   &http.Client{Transport: transport},
		pagesBaseURL: srv.URL,
	}

	req := newTestPublishRequest(t)
	_, err := client.Publish(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for 422")
	}
	if !errors.Is(err, ErrVersionConflict) {
		t.Errorf("expected ErrVersionConflict, got: %v", err)
	}
}

func TestPublish_Forbidden(t *testing.T) {
	t.Parallel()

	srv := githubAPIMux(t, mockOpts{releaseStatus: http.StatusForbidden})
	defer srv.Close()

	transport := &rewriteTransport{base: srv.URL, wrapped: http.DefaultTransport}
	client := &Client{
		repoOwner:    "test-owner",
		repoName:     "test-repo",
		token:        "test-token",
		httpClient:   &http.Client{Transport: transport},
		pagesBaseURL: srv.URL,
	}

	req := newTestPublishRequest(t)
	_, err := client.Publish(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("expected ErrForbidden, got: %v", err)
	}
}

func TestPublish_ContextCanceled(t *testing.T) {
	t.Parallel()

	srv := githubAPIMux(t, mockOpts{})
	defer srv.Close()

	transport := &rewriteTransport{base: srv.URL, wrapped: http.DefaultTransport}
	client := &Client{
		repoOwner:    "test-owner",
		repoName:     "test-repo",
		token:        "test-token",
		httpClient:   &http.Client{Transport: transport},
		pagesBaseURL: srv.URL,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := newTestPublishRequest(t)
	_, err := client.Publish(ctx, req)
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

func TestFetchIndex_Success(t *testing.T) {
	t.Parallel()

	idx := Index{
		Version:   1,
		UpdatedAt: "2026-04-18T00:00:00Z",
		Items: []Item{
			{Name: "test-plugin", Version: "1.0.0", Category: "plugins"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/index.json" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(idx)
	}))
	defer srv.Close()

	client := &Client{
		pagesBaseURL: srv.URL,
		httpClient:   srv.Client(),
	}

	result, err := client.FetchIndex(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Version != 1 {
		t.Errorf("version: got %d, want 1", result.Version)
	}
	if len(result.Items) != 1 {
		t.Fatalf("items: got %d, want 1", len(result.Items))
	}
	if result.Items[0].Name != "test-plugin" {
		t.Errorf("item name: got %q, want %q", result.Items[0].Name, "test-plugin")
	}
}

func TestFetchIndex_NotModified(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-Modified-Since") == "" {
			t.Error("expected If-Modified-Since header")
		}
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	client := &Client{
		pagesBaseURL: srv.URL,
		httpClient:   srv.Client(),
	}

	since := time.Now().Add(-1 * time.Hour)
	_, err := client.FetchIndex(context.Background(), &since)
	if err == nil {
		t.Fatal("expected error for 304")
	}
	if !errors.Is(err, ErrIndexNotModified) {
		t.Errorf("expected ErrIndexNotModified, got: %v", err)
	}
}

func TestFetchIndex_InvalidVersion(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version": 0,
			"items":   []any{},
		})
	}))
	defer srv.Close()

	client := &Client{
		pagesBaseURL: srv.URL,
		httpClient:   srv.Client(),
	}

	_, err := client.FetchIndex(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for invalid version")
	}
	if !strings.Contains(err.Error(), "invalid index version") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewClient_DefaultRepoConfig(t *testing.T) {
	t.Parallel()

	client := NewClient(NewClientConfig{Token: "tok"})
	if client.repoOwner != "dollebrekel" {
		t.Errorf("owner: got %q, want %q", client.repoOwner, "dollebrekel")
	}
	if client.repoName != "simply-market" {
		t.Errorf("repo: got %q, want %q", client.repoName, "simply-market")
	}
	if client.pagesBaseURL != "https://dollebrekel.github.io/simply-market" {
		t.Errorf("pages URL: got %q", client.pagesBaseURL)
	}
}

func TestNewClient_EnvOverride(t *testing.T) {
	t.Setenv("SIPLY_MARKET_REPO", "custom-owner/custom-repo")

	client := NewClient(NewClientConfig{Token: "tok"})
	if client.repoOwner != "custom-owner" {
		t.Errorf("owner: got %q, want %q", client.repoOwner, "custom-owner")
	}
	if client.repoName != "custom-repo" {
		t.Errorf("repo: got %q, want %q", client.repoName, "custom-repo")
	}
	if client.pagesBaseURL != "https://custom-owner.github.io/custom-repo" {
		t.Errorf("pages URL: got %q", client.pagesBaseURL)
	}
}

func TestNewClient_InvalidEnvFallsBack(t *testing.T) {
	t.Setenv("SIPLY_MARKET_REPO", "invalid-no-slash")

	client := NewClient(NewClientConfig{Token: "tok"})
	if client.repoOwner != "dollebrekel" {
		t.Errorf("owner: got %q, want %q", client.repoOwner, "dollebrekel")
	}
}

func TestNewClient_MultiSlashEnvFallsBack(t *testing.T) {
	t.Setenv("SIPLY_MARKET_REPO", "owner/repo/extra")

	client := NewClient(NewClientConfig{Token: "tok"})
	if client.repoOwner != "dollebrekel" {
		t.Errorf("owner: got %q, want %q", client.repoOwner, "dollebrekel")
	}
	if client.repoName != "simply-market" {
		t.Errorf("repo: got %q, want %q", client.repoName, "simply-market")
	}
}

func TestDefaultRepoConfig(t *testing.T) {
	t.Parallel()

	owner, repo := DefaultRepoConfig()
	if owner != "dollebrekel" {
		t.Errorf("owner: got %q, want %q", owner, "dollebrekel")
	}
	if repo != "simply-market" {
		t.Errorf("repo: got %q, want %q", repo, "simply-market")
	}
}

// reviewAPIMux creates a test server for review/rate/report flows.
func reviewAPIMux(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	// GET /user
	mux.HandleFunc("GET /user", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"login": "testuser"})
	})

	// GET /repos/{owner}/{repo}/contents/reviews/{name}.json (404 = new file)
	mux.HandleFunc("GET /repos/test-owner/test-repo/contents/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/repos/test-owner/test-repo/contents/")
		if strings.HasPrefix(path, "reviews/existing-item.json") {
			rf := ReviewFile{
				Reviews: []ReviewEntry{
					{Author: "alice", Rating: 3, Text: "OK", CreatedAt: "2026-04-01T00:00:00Z"},
				},
			}
			jsonBytes, _ := json.Marshal(rf)
			encoded := base64.StdEncoding.EncodeToString(jsonBytes)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"content": encoded,
				"sha":     "existingSHA",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	})

	// GET ref
	mux.HandleFunc("GET /repos/test-owner/test-repo/git/ref/heads/main", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": map[string]string{"sha": "mainSHA"},
		})
	})

	// Create ref
	mux.HandleFunc("POST /repos/test-owner/test-repo/git/refs", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"ref": body["ref"]})
	})

	// PUT file (create or update)
	mux.HandleFunc("PUT /repos/test-owner/test-repo/contents/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"content": map[string]string{"sha": "newSHA"}})
	})

	// Create PR
	mux.HandleFunc("POST /repos/test-owner/test-repo/pulls", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"number":   42,
			"html_url": "https://github.com/test-owner/test-repo/pull/42",
		})
	})

	// Create issue
	mux.HandleFunc("POST /repos/test-owner/test-repo/issues", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"number":   5,
			"html_url": "https://github.com/test-owner/test-repo/issues/5",
		})
	})

	return httptest.NewServer(mux)
}

func newReviewTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	transport := &rewriteTransport{base: srv.URL, wrapped: http.DefaultTransport}
	return &Client{
		repoOwner:    "test-owner",
		repoName:     "test-repo",
		token:        "test-token",
		httpClient:   &http.Client{Transport: transport},
		pagesBaseURL: srv.URL,
	}
}

func TestSubmitReview_Success_NewFile(t *testing.T) {
	t.Parallel()
	srv := reviewAPIMux(t)
	defer srv.Close()

	client := newReviewTestClient(t, srv)
	resp, err := client.SubmitReview(context.Background(), SubmitReviewRequest{
		Name:   "new-item",
		Rating: 4,
		Text:   "Great plugin",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.PRURL != "https://github.com/test-owner/test-repo/pull/42" {
		t.Errorf("PRURL: got %q", resp.PRURL)
	}
}

func TestSubmitReview_Success_ExistingFile(t *testing.T) {
	t.Parallel()
	srv := reviewAPIMux(t)
	defer srv.Close()

	client := newReviewTestClient(t, srv)
	resp, err := client.SubmitReview(context.Background(), SubmitReviewRequest{
		Name:   "existing-item",
		Rating: 5,
		Text:   "Even better now",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.PRURL == "" {
		t.Error("expected non-empty PR URL")
	}
}

func TestSubmitReview_InvalidRating(t *testing.T) {
	t.Parallel()

	client := &Client{}
	for _, rating := range []int{0, -1, 6, 100} {
		_, err := client.SubmitReview(context.Background(), SubmitReviewRequest{
			Name:   "test",
			Rating: rating,
			Text:   "text",
		})
		if !errors.Is(err, ErrInvalidRating) {
			t.Errorf("rating %d: expected ErrInvalidRating, got: %v", rating, err)
		}
	}
}

func TestSubmitReview_TextTooLong(t *testing.T) {
	t.Parallel()

	client := &Client{}
	longText := strings.Repeat("a", 2001)
	_, err := client.SubmitReview(context.Background(), SubmitReviewRequest{
		Name:   "test",
		Rating: 3,
		Text:   longText,
	})
	if !errors.Is(err, ErrReviewTooLong) {
		t.Errorf("expected ErrReviewTooLong, got: %v", err)
	}
}

func TestSubmitReview_Unauthorized(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /user", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	transport := &rewriteTransport{base: srv.URL, wrapped: http.DefaultTransport}
	client := &Client{
		repoOwner:  "test-owner",
		repoName:   "test-repo",
		token:      "bad-token",
		httpClient: &http.Client{Transport: transport},
	}

	_, err := client.SubmitReview(context.Background(), SubmitReviewRequest{
		Name:   "test",
		Rating: 4,
		Text:   "hello",
	})
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got: %v", err)
	}
}

func TestGetReviews_Success(t *testing.T) {
	t.Parallel()

	rf := ReviewFile{
		Reviews: []ReviewEntry{
			{Author: "alice", Rating: 4, Text: "Good", CreatedAt: "2026-04-01T00:00:00Z"},
			{Author: "bob", Rating: 5, Text: "", CreatedAt: "2026-04-02T00:00:00Z"},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(rf)
	}))
	defer srv.Close()

	client := &Client{
		httpClient:   srv.Client(),
		pagesBaseURL: srv.URL,
	}
	got, err := client.GetReviews(context.Background(), "test-item")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Reviews) != 2 {
		t.Errorf("reviews count: got %d, want 2", len(got.Reviews))
	}
	if got.Reviews[0].Author != "alice" {
		t.Errorf("first author: got %q, want %q", got.Reviews[0].Author, "alice")
	}
}

func TestGetReviews_NotFound(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := &Client{
		httpClient:   srv.Client(),
		pagesBaseURL: srv.URL,
	}
	got, err := client.GetReviews(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Reviews) != 0 {
		t.Errorf("expected empty ReviewFile, got %d reviews", len(got.Reviews))
	}
}

func TestGetReviews_NetworkError(t *testing.T) {
	t.Parallel()

	client := &Client{
		httpClient:   &http.Client{Timeout: time.Millisecond},
		pagesBaseURL: "http://192.0.2.1:1", // unreachable
	}
	_, err := client.GetReviews(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for network failure")
	}
}

func TestReportItem_Success(t *testing.T) {
	t.Parallel()
	srv := reviewAPIMux(t)
	defer srv.Close()

	client := newReviewTestClient(t, srv)
	resp, err := client.ReportItem(context.Background(), ReportRequest{
		Name:   "bad-item",
		Reason: "malware",
		Detail: "Contains keylogger",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.IssueURL != "https://github.com/test-owner/test-repo/issues/5" {
		t.Errorf("IssueURL: got %q", resp.IssueURL)
	}
}

func TestReportItem_InvalidReason(t *testing.T) {
	t.Parallel()

	client := &Client{}
	_, err := client.ReportItem(context.Background(), ReportRequest{
		Name:   "test",
		Reason: "invalid-reason",
	})
	if !errors.Is(err, ErrInvalidReason) {
		t.Errorf("expected ErrInvalidReason, got: %v", err)
	}
}

func TestReportItem_DetailTooLong(t *testing.T) {
	t.Parallel()

	client := &Client{}
	_, err := client.ReportItem(context.Background(), ReportRequest{
		Name:   "test",
		Reason: "spam",
		Detail: strings.Repeat("x", 501),
	})
	if !errors.Is(err, ErrReportTooLong) {
		t.Errorf("expected ErrReportTooLong, got: %v", err)
	}
}

// TD-6: FetchIndex returns clear error when index exceeds 10 MB size limit.
func TestFetchIndex_ExceedsSizeLimit(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Write >10 MB of data.
		chunk := strings.Repeat("x", 1024)
		for i := 0; i < 11*1024; i++ {
			_, _ = w.Write([]byte(chunk))
		}
	}))
	defer srv.Close()

	client := &Client{
		pagesBaseURL: srv.URL,
		httpClient:   srv.Client(),
	}

	_, err := client.FetchIndex(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for oversized index")
	}
	if !strings.Contains(err.Error(), "10 MB size limit") {
		t.Errorf("expected '10 MB size limit' in error, got: %v", err)
	}
}

// TD-7: getUsername uses sync.Once — concurrent calls result in single API request.
func TestGetUsername_ConcurrentSingleRequest(t *testing.T) {
	t.Parallel()

	var requestCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user" {
			atomic.AddInt32(&requestCount, 1)
			_ = json.NewEncoder(w).Encode(map[string]string{"login": "testuser"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	transport := &rewriteTransport{base: srv.URL, wrapped: http.DefaultTransport}
	client := &Client{
		token:      "test-token",
		httpClient: &http.Client{Transport: transport},
	}

	const goroutines = 10
	results := make(chan string, goroutines)
	errs := make(chan error, goroutines)
	start := make(chan struct{})

	for i := 0; i < goroutines; i++ {
		go func() {
			<-start
			username, err := client.getUsername(context.Background())
			if err != nil {
				errs <- err
				return
			}
			results <- username
		}()
	}

	close(start)

	for i := 0; i < goroutines; i++ {
		select {
		case username := <-results:
			if username != "testuser" {
				t.Errorf("expected 'testuser', got %q", username)
			}
		case err := <-errs:
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if atomic.LoadInt32(&requestCount) != 1 {
		t.Errorf("expected exactly 1 API request, got %d", atomic.LoadInt32(&requestCount))
	}
}

// TD-8: PathEscape produces correct paths for items with special characters.
func TestPathEscape_SpecialCharacters(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{"at sign", "my@plugin", "my@plugin"},
		{"plus sign", "my+plugin", "my+plugin"},
		{"space", "my plugin", "my%20plugin"},
		{"slash", "my/plugin", "my%2Fplugin"},
		{"plain", "my-plugin", "my-plugin"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := url.PathEscape(tc.input)
			if got != tc.expected {
				t.Errorf("PathEscape(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}
