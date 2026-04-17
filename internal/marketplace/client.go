// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package marketplace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"time"
	"unicode/utf8"

	"siply.dev/siply/internal/plugins"
)

// Sentinel errors for trust system operations.
var (
	ErrInvalidRating = errors.New("rating must be between 1 and 5")
	ErrReviewTooLong = errors.New("review text exceeds 2000 character limit")
	ErrReportTooLong = errors.New("report detail exceeds 500 character limit")
	ErrInvalidReason = errors.New("invalid report reason: must be one of: malware, spam, broken, copyright, other")
)

// ValidReportReasons lists the allowed report reason types.
var ValidReportReasons = []string{"malware", "spam", "broken", "copyright", "other"}

// RateRequest contains the data needed to rate an item.
type RateRequest struct {
	Token string
	Name  string
	Score int // 1–5
}

// RateResponse is returned by the marketplace API after rating.
type RateResponse struct {
	AverageRating float64 `json:"average_rating"`
	TotalRatings  int     `json:"total_ratings"`
}

// ReviewRequest contains the data needed to submit a review.
type ReviewRequest struct {
	Token  string
	Name   string
	Text   string // max 2000 chars
	Rating int    // optional, 0 = no rating
}

// ReviewResponse is returned by the marketplace API after submitting a review.
type ReviewResponse struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
}

// Review represents a single user review.
type Review struct {
	ID        string `json:"id"`
	Author    string `json:"author"`
	Rating    int    `json:"rating"` // 0 if no rating
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
}

// ReviewsResponse is returned by the marketplace API for review listings.
type ReviewsResponse struct {
	Reviews    []Review `json:"reviews"`
	TotalCount int      `json:"total_count"`
	Page       int      `json:"page"`
	PageSize   int      `json:"page_size"`
}

// ReportRequest contains the data needed to report an item.
type ReportRequest struct {
	Token  string
	Name   string
	Reason string // one of: malware, spam, broken, copyright, other
	Detail string // max 500 chars
}

// ReportResponse is returned by the marketplace API after reporting.
type ReportResponse struct {
	ID string `json:"id"`
}

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

// DefaultBaseURL returns the marketplace API base URL, allowing override via SIPLY_MARKET_URL env.
func DefaultBaseURL() string {
	if u := os.Getenv("SIPLY_MARKET_URL"); u != "" {
		return u
	}
	return "https://market.siply.dev"
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
		var writeErr error
		defer func() {
			if writeErr != nil {
				pw.CloseWithError(writeErr)
			} else {
				pw.Close()
			}
			errCh <- writeErr
		}()

		manifestJSON, err := json.Marshal(req.Manifest)
		if err != nil {
			writeErr = fmt.Errorf("marketplace: marshal manifest: %w", err)
			return
		}

		if err := mw.WriteField("manifest", string(manifestJSON)); err != nil {
			writeErr = fmt.Errorf("marketplace: write manifest field: %w", err)
			return
		}
		if err := mw.WriteField("sha256", req.SHA256); err != nil {
			writeErr = fmt.Errorf("marketplace: write sha256 field: %w", err)
			return
		}
		if err := mw.WriteField("readme", req.ReadmeText); err != nil {
			writeErr = fmt.Errorf("marketplace: write readme field: %w", err)
			return
		}

		archiveFile, err := os.Open(req.ArchivePath)
		if err != nil {
			writeErr = fmt.Errorf("marketplace: open archive: %w", err)
			return
		}
		defer archiveFile.Close()

		part, err := mw.CreateFormFile("archive", "plugin.tar.gz")
		if err != nil {
			writeErr = fmt.Errorf("marketplace: create archive field: %w", err)
			return
		}
		if _, err := io.Copy(part, archiveFile); err != nil {
			writeErr = fmt.Errorf("marketplace: copy archive: %w", err)
			return
		}

		if closeErr := mw.Close(); closeErr != nil {
			writeErr = fmt.Errorf("marketplace: close multipart: %w", closeErr)
		}
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
			return nil, fmt.Errorf("marketplace: cannot reach marketplace — check your connection and try again")
		}
		return nil, fmt.Errorf("marketplace: publish request failed: %w", err)
	}
	defer resp.Body.Close()

	writeErr := <-errCh
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("marketplace: read response: %w", err)
	}
	if writeErr != nil {
		return nil, writeErr
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

// Rate submits a rating (1–5) for a marketplace item.
func (c *Client) Rate(ctx context.Context, req RateRequest) (*RateResponse, error) {
	if req.Score < 1 || req.Score > 5 {
		return nil, ErrInvalidRating
	}

	body, err := json.Marshal(map[string]int{"score": req.Score})
	if err != nil {
		return nil, fmt.Errorf("marketplace: marshal rate request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/v1/items/%s/rate", c.baseURL, url.PathEscape(req.Name)),
		bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("marketplace: create rate request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.Token)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if isNetworkError(err) {
			return nil, fmt.Errorf("Failed to reach marketplace. Check your connection and try again.")
		}
		return nil, fmt.Errorf("marketplace: rate request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("marketplace: read rate response: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		var rateResp RateResponse
		if err := json.Unmarshal(respBody, &rateResp); err != nil {
			return nil, fmt.Errorf("marketplace: parse rate response: %w", err)
		}
		return &rateResp, nil
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("marketplace: authentication failed — run 'siply login' first")
	case http.StatusNotFound:
		return nil, fmt.Errorf("marketplace: item not found: %s", req.Name)
	case http.StatusUnprocessableEntity:
		return nil, fmt.Errorf("marketplace: invalid rating: %s", string(respBody))
	default:
		return nil, fmt.Errorf("marketplace: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}
}

// SubmitReview submits a text review for a marketplace item.
func (c *Client) SubmitReview(ctx context.Context, req ReviewRequest) (*ReviewResponse, error) {
	if utf8.RuneCountInString(req.Text) > 2000 {
		return nil, ErrReviewTooLong
	}
	if req.Rating < 0 || req.Rating > 5 {
		return nil, ErrInvalidRating
	}

	body, err := json.Marshal(map[string]any{"text": req.Text, "rating": req.Rating})
	if err != nil {
		return nil, fmt.Errorf("marketplace: marshal review request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/v1/items/%s/review", c.baseURL, url.PathEscape(req.Name)),
		bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("marketplace: create review request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.Token)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if isNetworkError(err) {
			return nil, fmt.Errorf("Failed to reach marketplace. Check your connection and try again.")
		}
		return nil, fmt.Errorf("marketplace: review request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("marketplace: read review response: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusCreated:
		var revResp ReviewResponse
		if err := json.Unmarshal(respBody, &revResp); err != nil {
			return nil, fmt.Errorf("marketplace: parse review response: %w", err)
		}
		return &revResp, nil
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("marketplace: authentication failed — run 'siply login' first")
	case http.StatusNotFound:
		return nil, fmt.Errorf("marketplace: item not found: %s", req.Name)
	case http.StatusUnprocessableEntity:
		return nil, fmt.Errorf("marketplace: validation failed: %s", string(respBody))
	default:
		return nil, fmt.Errorf("marketplace: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}
}

// GetReviews retrieves paginated reviews for a marketplace item. No auth required.
func (c *Client) GetReviews(ctx context.Context, name string, page, pageSize int) (*ReviewsResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/api/v1/items/%s/reviews?page=%d&page_size=%d", c.baseURL, url.PathEscape(name), page, pageSize),
		nil)
	if err != nil {
		return nil, fmt.Errorf("marketplace: create reviews request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if isNetworkError(err) {
			return nil, fmt.Errorf("Reviews unavailable offline. Check your connection and try again.")
		}
		return nil, fmt.Errorf("marketplace: reviews request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("marketplace: read reviews response: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		var revResp ReviewsResponse
		if err := json.Unmarshal(respBody, &revResp); err != nil {
			return nil, fmt.Errorf("marketplace: parse reviews response: %w", err)
		}
		return &revResp, nil
	case http.StatusNotFound:
		return nil, fmt.Errorf("marketplace: item not found: %s", name)
	default:
		return nil, fmt.Errorf("marketplace: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}
}

// ReportItem submits a report for a marketplace item.
func (c *Client) ReportItem(ctx context.Context, req ReportRequest) (*ReportResponse, error) {
	if !isValidReason(req.Reason) {
		return nil, ErrInvalidReason
	}
	if utf8.RuneCountInString(req.Detail) > 500 {
		return nil, ErrReportTooLong
	}

	body, err := json.Marshal(map[string]string{"reason": req.Reason, "detail": req.Detail})
	if err != nil {
		return nil, fmt.Errorf("marketplace: marshal report request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/v1/items/%s/report", c.baseURL, url.PathEscape(req.Name)),
		bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("marketplace: create report request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.Token)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if isNetworkError(err) {
			return nil, fmt.Errorf("Failed to reach marketplace. Check your connection and try again.")
		}
		return nil, fmt.Errorf("marketplace: report request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("marketplace: read report response: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusCreated:
		var repResp ReportResponse
		if err := json.Unmarshal(respBody, &repResp); err != nil {
			return nil, fmt.Errorf("marketplace: parse report response: %w", err)
		}
		return &repResp, nil
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("marketplace: authentication failed — run 'siply login' first")
	case http.StatusNotFound:
		return nil, fmt.Errorf("marketplace: item not found: %s", req.Name)
	case http.StatusUnprocessableEntity:
		return nil, fmt.Errorf("marketplace: invalid reason: %s", string(respBody))
	default:
		return nil, fmt.Errorf("marketplace: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}
}

func isValidReason(reason string) bool {
	return slices.Contains(ValidReportReasons, reason)
}

func isNetworkError(err error) bool {
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return true
	}
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr)
}
