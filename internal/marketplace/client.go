// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package marketplace

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"siply.dev/siply/internal/plugins"
)

// Sentinel errors for marketplace operations.
var (
	ErrUnauthorized     = errors.New("marketplace: authentication failed — run 'siply auth login' first")
	ErrForbidden        = errors.New("marketplace: insufficient permissions — GitHub token may lack 'public_repo' scope")
	ErrVersionConflict  = errors.New("marketplace: version already exists — bump the version in manifest.yaml")
	ErrIndexNotModified = errors.New("index not modified") // also referenced from index.go
	ErrNotFound         = errors.New("marketplace: resource not found")
	ErrInvalidRating    = errors.New("rating must be between 1 and 5")
	ErrReviewTooLong    = errors.New("review text exceeds 2000 character limit")
	ErrReportTooLong    = errors.New("report detail exceeds 500 character limit")
	ErrInvalidReason    = errors.New("invalid report reason: must be one of: malware, spam, broken, copyright, other")
)

// ValidReportReasons lists the accepted reason values for marketplace item reports.
var ValidReportReasons = []string{"malware", "spam", "broken", "copyright", "other"}

// ReviewEntry represents a single review or rating for a marketplace item.
type ReviewEntry struct {
	Author    string `json:"author"`
	Rating    int    `json:"rating"`     // 1-5
	Text      string `json:"text"`       // empty for rating-only
	CreatedAt string `json:"created_at"` // RFC3339
}

// ReviewFile is the container for all reviews of a single marketplace item.
type ReviewFile struct {
	Reviews []ReviewEntry `json:"reviews"`
}

// SubmitReviewRequest contains the data needed to submit a review.
type SubmitReviewRequest struct {
	Name   string // item name
	Rating int    // 1-5
	Text   string // max 2000 chars, empty for rating-only
}

// SubmitReviewResponse is returned on successful review submission.
type SubmitReviewResponse struct {
	PRURL string // GitHub PR URL
}

// ReportRequest contains the data needed to report a marketplace item.
type ReportRequest struct {
	Name   string
	Reason string // malware, spam, broken, copyright, other
	Detail string // max 500 chars
}

// ReportResponse is returned on successful report submission.
type ReportResponse struct {
	IssueURL string
}

const (
	defaultRepoOwner = "dollebrekel"
	defaultRepoName  = "simply-market"

	githubAPIBase    = "https://api.github.com"
	githubAPIVersion = "2022-11-28"
)

// NewClientConfig holds configuration for creating a new Client.
type NewClientConfig struct {
	RepoOwner  string
	RepoName   string
	Token      string
	HTTPClient *http.Client // optional, for testing
}

// Client is a GitHub API client for the simply-market registry.
type Client struct {
	repoOwner    string
	repoName     string
	token        string
	httpClient   *http.Client
	pagesBaseURL string
	usernameOnce sync.Once
	username     string
	usernameErr  error
}

// NewClient creates a new marketplace client configured for GitHub API access.
func NewClient(cfg NewClientConfig) *Client {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 5 * time.Minute,
		}
	}

	owner := cfg.RepoOwner
	repo := cfg.RepoName
	if owner == "" || repo == "" {
		owner, repo = DefaultRepoConfig()
	}

	return &Client{
		repoOwner:    owner,
		repoName:     repo,
		token:        cfg.Token,
		httpClient:   httpClient,
		pagesBaseURL: fmt.Sprintf("https://%s.github.io/%s", owner, repo),
	}
}

// DefaultRepoConfig returns the owner and repo name for the marketplace registry.
// The SIPLY_MARKET_REPO env var overrides the default (format: "owner/repo").
func DefaultRepoConfig() (owner, repo string) {
	if envRepo := os.Getenv("SIPLY_MARKET_REPO"); envRepo != "" {
		parts := strings.SplitN(envRepo, "/", 2)
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" && !strings.Contains(parts[1], "/") {
			return parts[0], parts[1]
		}
	}
	return defaultRepoOwner, defaultRepoName
}

// PagesBaseURL returns the GitHub Pages base URL for the registry.
func (c *Client) PagesBaseURL() string {
	return c.pagesBaseURL
}

// PublishRequest contains the data needed to publish a package.
type PublishRequest struct {
	Manifest    plugins.Metadata
	ArchivePath string
	SHA256      string
	ReadmeText  string
}

// PublishResponse is returned on successful publish.
type PublishResponse struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	URL     string `json:"url"`
}

// Publish uploads a plugin archive via GitHub Releases and creates a PR to update index.json.
func (c *Client) Publish(ctx context.Context, req PublishRequest) (*PublishResponse, error) {
	name := req.Manifest.Name
	version := req.Manifest.Version
	tag := fmt.Sprintf("%s-v%s", name, version)
	assetName := fmt.Sprintf("%s-%s.tar.gz", name, version)

	// Step 1: Create GitHub Release.
	_, uploadURL, htmlURL, err := c.createRelease(ctx, tag, name, version, req.Manifest.Description)
	if err != nil {
		return nil, fmt.Errorf("publish: create release: %w", err)
	}

	// Step 2: Upload asset.
	assetDownloadURL, err := c.uploadAsset(ctx, uploadURL, assetName, req.ArchivePath)
	if err != nil {
		return nil, fmt.Errorf("publish: upload asset: %w", err)
	}

	// Step 3: Update index.json on a new branch.
	branchName := fmt.Sprintf("publish/%s", tag)
	releaseURL := fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s", c.repoOwner, c.repoName, tag)
	if err := c.updateIndexJSON(ctx, branchName, name, version, req, assetDownloadURL); err != nil {
		return nil, fmt.Errorf("publish: update index failed — release was created at %s. "+
			"Delete it manually on GitHub, then retry: %w", releaseURL, err)
	}

	// Step 4: Create PR.
	if err := c.createPR(ctx, fmt.Sprintf("Publish %s v%s", name, version), branchName); err != nil {
		return nil, fmt.Errorf("publish: create PR failed — release was created at %s. "+
			"Delete the release and branch 'publish/%s' manually on GitHub, then retry: %w", releaseURL, tag, err)
	}

	return &PublishResponse{
		Name:    name,
		Version: version,
		URL:     htmlURL,
	}, nil
}

// createRelease creates a GitHub Release and returns the release ID, upload URL, and HTML URL.
func (c *Client) createRelease(ctx context.Context, tag, name, version, description string) (int64, string, string, error) {
	body := map[string]any{
		"tag_name": tag,
		"name":     fmt.Sprintf("%s v%s", name, version),
		"body":     fmt.Sprintf("Published via siply marketplace\n\n%s", description),
	}

	respBody, err := c.githubAPI(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/%s/releases", c.repoOwner, c.repoName), body)
	if err != nil {
		return 0, "", "", err
	}

	var result struct {
		ID        int64  `json:"id"`
		UploadURL string `json:"upload_url"`
		HTMLURL   string `json:"html_url"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return 0, "", "", fmt.Errorf("parse release response: %w", err)
	}

	// Strip the upload_url template part {?name,label}
	if idx := strings.Index(result.UploadURL, "{"); idx != -1 {
		result.UploadURL = result.UploadURL[:idx]
	}

	return result.ID, result.UploadURL, result.HTMLURL, nil
}

// uploadAsset uploads a tar.gz file as a release asset and returns the download URL.
func (c *Client) uploadAsset(ctx context.Context, uploadURL, assetName, archivePath string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()

	reqURL := fmt.Sprintf("%s?name=%s", uploadURL, url.QueryEscape(assetName))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, f)
	if err != nil {
		return "", fmt.Errorf("create upload request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/gzip")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)
	httpReq.Header.Set("Accept", "application/vnd.github+json")
	httpReq.Header.Set("X-GitHub-Api-Version", githubAPIVersion)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if isNetworkError(err) {
			return "", fmt.Errorf("cannot reach GitHub — check your connection and try again")
		}
		return "", fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read upload response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return "", c.apiError(resp.StatusCode, respBody)
	}

	var result struct {
		BrowserDownloadURL string `json:"browser_download_url"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse upload response: %w", err)
	}

	return result.BrowserDownloadURL, nil
}

// updateIndexJSON fetches the current index.json, adds/updates the item entry,
// and commits it on a new branch.
func (c *Client) updateIndexJSON(ctx context.Context, branchName, name, version string, req PublishRequest, downloadURL string) error {
	// Get main branch SHA for creating the new branch.
	mainRef, err := c.getRef(ctx, "heads/main")
	if err != nil {
		return fmt.Errorf("get main ref: %w", err)
	}

	// Create the publish branch from main.
	if err := c.createRef(ctx, "refs/heads/"+branchName, mainRef); err != nil {
		return fmt.Errorf("create branch: %w", err)
	}

	// Get current index.json content and SHA.
	content, fileSHA, err := c.getFileContent(ctx, "index.json", "main")
	if err != nil {
		return fmt.Errorf("get index.json: %w", err)
	}

	// Parse and update the index.
	var idx Index
	if err := json.Unmarshal(content, &idx); err != nil {
		return fmt.Errorf("parse index.json: %w", err)
	}

	newItem := Item{
		Name:        name,
		Category:    "plugins", // default; overridden by CI validation on simply-market
		Description: req.Manifest.Description,
		Author:      req.Manifest.Author,
		Version:     version,
		SiplyMin:    req.Manifest.SiplyMin,
		License:     req.Manifest.License,
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
		DownloadURL: downloadURL,
		SHA256:      req.SHA256,
		Readme:      req.ReadmeText,
	}

	// Update existing or append.
	found := false
	for i, item := range idx.Items {
		if item.Name == name {
			idx.Items[i] = newItem
			found = true
			break
		}
	}
	if !found {
		idx.Items = append(idx.Items, newItem)
	}

	idx.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	updatedJSON, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal updated index: %w", err)
	}

	// Commit the updated index.json to the publish branch.
	if err := c.updateFileContent(ctx, "index.json", updatedJSON, fileSHA, branchName,
		fmt.Sprintf("Publish %s v%s", name, version)); err != nil {
		return fmt.Errorf("commit index.json: %w", err)
	}

	return nil
}

// getRef returns the SHA of a git reference (e.g., "heads/main").
func (c *Client) getRef(ctx context.Context, ref string) (string, error) {
	respBody, err := c.githubAPI(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/git/ref/%s", c.repoOwner, c.repoName, ref), nil)
	if err != nil {
		return "", err
	}

	var result struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse ref response: %w", err)
	}
	return result.Object.SHA, nil
}

// createRef creates a new git reference.
func (c *Client) createRef(ctx context.Context, ref, sha string) error {
	_, err := c.githubAPI(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/%s/git/refs", c.repoOwner, c.repoName), map[string]string{
		"ref": ref,
		"sha": sha,
	})
	return err
}

// getFileContent returns the decoded content and SHA of a file from the repo.
func (c *Client) getFileContent(ctx context.Context, path, ref string) ([]byte, string, error) {
	respBody, err := c.githubAPI(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/contents/%s?ref=%s", c.repoOwner, c.repoName, path, ref), nil)
	if err != nil {
		return nil, "", err
	}

	var result struct {
		Content string `json:"content"`
		SHA     string `json:"sha"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, "", fmt.Errorf("parse file content response: %w", err)
	}

	// GitHub API base64 content may contain \n, \r\n, or other whitespace.
	cleaned := strings.NewReplacer("\n", "", "\r", "", " ", "").Replace(result.Content)
	decoded, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		return nil, "", fmt.Errorf("decode base64 content: %w", err)
	}

	return decoded, result.SHA, nil
}

// updateFileContent updates a file on a specific branch.
func (c *Client) updateFileContent(ctx context.Context, path string, content []byte, sha, branch, message string) error {
	_, err := c.githubAPI(ctx, http.MethodPut, fmt.Sprintf("/repos/%s/%s/contents/%s", c.repoOwner, c.repoName, path), map[string]string{
		"message": message,
		"content": base64.StdEncoding.EncodeToString(content),
		"sha":     sha,
		"branch":  branch,
	})
	return err
}

// createPR creates a pull request from the given branch to main.
func (c *Client) createPR(ctx context.Context, title, head string) error {
	_, err := c.githubAPI(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/%s/pulls", c.repoOwner, c.repoName), map[string]string{
		"title": title,
		"head":  head,
		"base":  "main",
	})
	return err
}

// FetchIndex fetches the marketplace index from GitHub Pages.
// If ifModifiedSince is non-nil, sends an If-Modified-Since header
// and returns ErrIndexNotModified on 304 responses.
func (c *Client) FetchIndex(ctx context.Context, ifModifiedSince *time.Time) (*Index, error) {
	url := c.pagesBaseURL + "/index.json"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("marketplace: create fetch request: %w", err)
	}

	if ifModifiedSince != nil {
		httpReq.Header.Set("If-Modified-Since", ifModifiedSince.UTC().Format(http.TimeFormat))
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if isNetworkError(err) {
			return nil, fmt.Errorf("marketplace: cannot reach GitHub Pages — check your connection and try again")
		}
		return nil, fmt.Errorf("marketplace: fetch index failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return nil, ErrIndexNotModified
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("marketplace: fetch index: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	const maxIndexSize = 10 << 20 // 10 MB
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxIndexSize)+1))
	if err != nil {
		return nil, fmt.Errorf("marketplace: read index response: %w", err)
	}
	if len(body) > maxIndexSize {
		return nil, fmt.Errorf("marketplace: index exceeds 10 MB size limit")
	}

	var idx Index
	if err := json.Unmarshal(body, &idx); err != nil {
		return nil, fmt.Errorf("marketplace: parse index: %w", err)
	}

	if idx.Version < 1 {
		return nil, fmt.Errorf("marketplace: invalid index version: %d", idx.Version)
	}

	return &idx, nil
}

// getUsername returns the authenticated user's GitHub login, caching it on the Client.
// Safe for concurrent use via sync.Once.
func (c *Client) getUsername(ctx context.Context) (string, error) {
	c.usernameOnce.Do(func() {
		respBody, err := c.githubAPI(ctx, http.MethodGet, "/user", nil)
		if err != nil {
			c.usernameErr = fmt.Errorf("get GitHub user: %w", err)
			return
		}
		var result struct {
			Login string `json:"login"`
		}
		if err := json.Unmarshal(respBody, &result); err != nil {
			c.usernameErr = fmt.Errorf("parse user response: %w", err)
			return
		}
		c.username = result.Login
	})
	return c.username, c.usernameErr
}

// createFileContent creates a new file on a specific branch (no SHA needed).
func (c *Client) createFileContent(ctx context.Context, path string, content []byte, branch, message string) error {
	_, err := c.githubAPI(ctx, http.MethodPut, fmt.Sprintf("/repos/%s/%s/contents/%s", c.repoOwner, c.repoName, path), map[string]string{
		"message": message,
		"content": base64.StdEncoding.EncodeToString(content),
		"branch":  branch,
	})
	return err
}

// createPRWithURL creates a pull request and returns the PR HTML URL.
func (c *Client) createPRWithURL(ctx context.Context, title, head, base string) (string, error) {
	respBody, err := c.githubAPI(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/%s/pulls", c.repoOwner, c.repoName), map[string]string{
		"title": title,
		"head":  head,
		"base":  base,
	})
	if err != nil {
		return "", err
	}
	var result struct {
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse PR response: %w", err)
	}
	return result.HTMLURL, nil
}

// SubmitReview submits a review for a marketplace item by creating a PR on the registry repo.
func (c *Client) SubmitReview(ctx context.Context, req SubmitReviewRequest) (*SubmitReviewResponse, error) {
	if req.Rating < 1 || req.Rating > 5 {
		return nil, ErrInvalidRating
	}
	if utf8.RuneCountInString(req.Text) > 2000 {
		return nil, ErrReviewTooLong
	}

	author, err := c.getUsername(ctx)
	if err != nil {
		return nil, fmt.Errorf("marketplace review: %w", err)
	}

	// Fetch existing reviews file (or create empty one).
	escapedName := url.PathEscape(req.Name)
	reviewPath := fmt.Sprintf("reviews/%s.json", escapedName)
	var reviewFile ReviewFile
	var fileSHA string
	isNew := false

	content, sha, err := c.getFileContent(ctx, reviewPath, "main")
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			isNew = true
		} else {
			return nil, fmt.Errorf("marketplace review: get reviews file: %w", err)
		}
	} else {
		fileSHA = sha
		if err := json.Unmarshal(content, &reviewFile); err != nil {
			return nil, fmt.Errorf("marketplace review: parse reviews file: %w", err)
		}
	}

	// Append new review entry.
	reviewFile.Reviews = append(reviewFile.Reviews, ReviewEntry{
		Author:    author,
		Rating:    req.Rating,
		Text:      req.Text,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})

	updatedJSON, err := json.MarshalIndent(reviewFile, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marketplace review: marshal reviews: %w", err)
	}

	// Create branch.
	timestamp := time.Now().Unix()
	branchName := fmt.Sprintf("review/%s-%d", escapedName, timestamp)

	mainRef, err := c.getRef(ctx, "heads/main")
	if err != nil {
		return nil, fmt.Errorf("marketplace review: get main ref: %w", err)
	}
	if err := c.createRef(ctx, "refs/heads/"+branchName, mainRef); err != nil {
		return nil, fmt.Errorf("marketplace review: create branch: %w", err)
	}

	// Commit review file.
	commitMsg := fmt.Sprintf("Review: %s by %s", req.Name, author)
	if isNew {
		if err := c.createFileContent(ctx, reviewPath, updatedJSON, branchName, commitMsg); err != nil {
			return nil, fmt.Errorf("marketplace review: create reviews file: %w", err)
		}
	} else {
		if err := c.updateFileContent(ctx, reviewPath, updatedJSON, fileSHA, branchName, commitMsg); err != nil {
			return nil, fmt.Errorf("marketplace review: update reviews file: %w", err)
		}
	}

	// Create PR.
	prURL, err := c.createPRWithURL(ctx, commitMsg, branchName, "main")
	if err != nil {
		return nil, fmt.Errorf("marketplace review: create PR: %w", err)
	}

	return &SubmitReviewResponse{PRURL: prURL}, nil
}

// GetReviews fetches reviews for a marketplace item from GitHub Pages.
func (c *Client) GetReviews(ctx context.Context, name string) (*ReviewFile, error) {
	escapedName := url.PathEscape(name)
	reviewURL := fmt.Sprintf("%s/reviews/%s.json", c.pagesBaseURL, escapedName)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, reviewURL, nil)
	if err != nil {
		return nil, fmt.Errorf("marketplace reviews: create request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if isNetworkError(err) {
			return nil, fmt.Errorf("marketplace reviews: cannot reach GitHub Pages — check your connection and try again")
		}
		return nil, fmt.Errorf("marketplace reviews: fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &ReviewFile{}, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("marketplace reviews: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20)) // 5 MB cap
	if err != nil {
		return nil, fmt.Errorf("marketplace reviews: read response: %w", err)
	}

	var rf ReviewFile
	if err := json.Unmarshal(body, &rf); err != nil {
		return nil, fmt.Errorf("marketplace reviews: parse response: %w", err)
	}

	return &rf, nil
}

// ReportItem creates a GitHub Issue to report a marketplace item.
func (c *Client) ReportItem(ctx context.Context, req ReportRequest) (*ReportResponse, error) {
	// Validate reason.
	validReason := false
	for _, r := range ValidReportReasons {
		if req.Reason == r {
			validReason = true
			break
		}
	}
	if !validReason {
		return nil, ErrInvalidReason
	}

	if utf8.RuneCountInString(req.Detail) > 500 {
		return nil, ErrReportTooLong
	}

	issueBody := req.Detail
	if strings.TrimSpace(issueBody) == "" {
		issueBody = "No additional details"
	}

	respBody, err := c.githubAPI(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/%s/issues", c.repoOwner, c.repoName), map[string]any{
		"title":  fmt.Sprintf("Report: %s — %s", req.Name, req.Reason),
		"body":   issueBody,
		"labels": []string{"report", fmt.Sprintf("reason:%s", req.Reason)},
	})
	if err != nil {
		return nil, fmt.Errorf("marketplace report: create issue: %w", err)
	}

	var result struct {
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("marketplace report: parse issue response: %w", err)
	}

	return &ReportResponse{IssueURL: result.HTMLURL}, nil
}

// githubAPI makes an authenticated request to the GitHub API.
func (c *Client) githubAPI(ctx context.Context, method, path string, body any) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		jsonBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonBytes)
	}

	url := githubAPIBase + path
	httpReq, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}
	httpReq.Header.Set("Accept", "application/vnd.github+json")
	httpReq.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if isNetworkError(err) {
			return nil, fmt.Errorf("cannot reach GitHub — check your connection and try again")
		}
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, c.apiError(resp.StatusCode, respBody)
	}

	return respBody, nil
}

// apiError converts HTTP error status codes to appropriate sentinel errors.
func (c *Client) apiError(statusCode int, body []byte) error {
	switch statusCode {
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusForbidden:
		return ErrForbidden
	case http.StatusUnprocessableEntity:
		bodyStr := string(body)
		if strings.Contains(bodyStr, "already_exists") {
			return fmt.Errorf("%w: %s", ErrVersionConflict, bodyStr)
		}
		return fmt.Errorf("marketplace: validation failed (422): %s", bodyStr)
	default:
		return fmt.Errorf("marketplace: unexpected status %d: %s", statusCode, string(body))
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
