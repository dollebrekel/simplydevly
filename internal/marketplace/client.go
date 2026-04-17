// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package marketplace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"time"

	"siply.dev/siply/internal/plugins"
)

// PublishRequest contains the data needed to publish a package.
type PublishRequest struct {
	Token       string
	Manifest    plugins.Metadata
	ArchivePath string
	SHA256      string
	ReadmeText  string
}

// PublishResponse is returned by the marketplace API on successful publish.
type PublishResponse struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	URL     string `json:"url"`
}

// Client is an HTTP client for the marketplace API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new marketplace API client.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

// Publish uploads a plugin archive to the marketplace.
func (c *Client) Publish(ctx context.Context, req PublishRequest) (*PublishResponse, error) {
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)

	// Write multipart form in a goroutine to stream data.
	errCh := make(chan error, 1)
	go func() {
		defer pw.Close()

		manifestJSON, err := json.Marshal(req.Manifest)
		if err != nil {
			errCh <- fmt.Errorf("marketplace: marshal manifest: %w", err)
			return
		}

		if err := mw.WriteField("manifest", string(manifestJSON)); err != nil {
			errCh <- fmt.Errorf("marketplace: write manifest field: %w", err)
			return
		}
		if err := mw.WriteField("sha256", req.SHA256); err != nil {
			errCh <- fmt.Errorf("marketplace: write sha256 field: %w", err)
			return
		}
		if err := mw.WriteField("readme", req.ReadmeText); err != nil {
			errCh <- fmt.Errorf("marketplace: write readme field: %w", err)
			return
		}

		archiveFile, err := os.Open(req.ArchivePath)
		if err != nil {
			errCh <- fmt.Errorf("marketplace: open archive: %w", err)
			return
		}
		defer archiveFile.Close()

		part, err := mw.CreateFormFile("archive", "plugin.tar.gz")
		if err != nil {
			errCh <- fmt.Errorf("marketplace: create archive field: %w", err)
			return
		}
		if _, err := io.Copy(part, archiveFile); err != nil {
			errCh <- fmt.Errorf("marketplace: copy archive: %w", err)
			return
		}

		errCh <- mw.Close()
	}()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/publish", pr)
	if err != nil {
		pr.CloseWithError(err)
		<-errCh
		return nil, fmt.Errorf("marketplace: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", mw.FormDataContentType())
	httpReq.Header.Set("Authorization", "Bearer "+req.Token)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		pr.CloseWithError(err)
		<-errCh
		if isNetworkError(err) {
			return nil, fmt.Errorf("Failed to reach marketplace. Check your connection and try again.")
		}
		return nil, fmt.Errorf("marketplace: publish request failed: %w", err)
	}
	defer resp.Body.Close()

	if writeErr := <-errCh; writeErr != nil {
		return nil, writeErr
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("marketplace: read response: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusCreated:
		var pubResp PublishResponse
		if err := json.Unmarshal(body, &pubResp); err != nil {
			return nil, fmt.Errorf("marketplace: parse response: %w", err)
		}
		return &pubResp, nil
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("marketplace: authentication failed — run 'siply login' first")
	case http.StatusConflict:
		return nil, fmt.Errorf("marketplace: version already exists — bump the version in manifest.yaml")
	case http.StatusUnprocessableEntity:
		return nil, fmt.Errorf("marketplace: validation failed: %s", string(body))
	default:
		return nil, fmt.Errorf("marketplace: unexpected status %d: %s", resp.StatusCode, string(body))
	}
}

func isNetworkError(err error) bool {
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return true
	}
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr)
}
