// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package kimi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/providers"
)

const (
	defaultBaseURL        = "https://api.moonshot.ai"
	chatCompletionsPath   = "/v1/chat/completions"
	cachingPath           = "/v1/caching"
	dialTimeout           = 10 * time.Second
	tlsHandshakeTimeout   = 10 * time.Second
	responseHeaderTimeout = 30 * time.Second

	// cacheTokenThreshold is the minimum estimated tokens required to enable
	// context caching. Kimi requires at least 4000 tokens.
	cacheTokenThreshold = 4000
)

// Adapter implements core.Provider for the Kimi (Moonshot AI) API with
// explicit context caching support.
type Adapter struct {
	credStore    core.CredentialStore
	apiKey       string
	client       *http.Client
	baseURL      string
	cacheManager *CacheManager
}

// New creates a new Kimi adapter.
// Respects KIMI_BASE_URL env var for proxy/benchmark compatibility.
func New(credStore core.CredentialStore) *Adapter {
	base := defaultBaseURL
	if env := os.Getenv("KIMI_BASE_URL"); env != "" {
		base = strings.TrimRight(env, "/")
	}
	return &Adapter{
		credStore: credStore,
		baseURL:   base,
	}
}

// Init loads the API key from the credential store and initialises the cache manager.
func (a *Adapter) Init(ctx context.Context) error {
	if a.credStore == nil {
		return fmt.Errorf("kimi: credential store is nil")
	}
	cred, err := a.credStore.GetProvider(ctx, "kimi")
	if err != nil {
		return fmt.Errorf("kimi: failed to get credentials: %w", err)
	}
	if cred.Value == "" {
		return fmt.Errorf("kimi: API key is empty")
	}
	a.apiKey = cred.Value
	a.client = &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: dialTimeout,
			}).DialContext,
			TLSHandshakeTimeout:   tlsHandshakeTimeout,
			ResponseHeaderTimeout: responseHeaderTimeout,
		},
	}
	a.cacheManager = NewCacheManager()
	return nil
}

// Start is a no-op for HTTP-based providers.
func (a *Adapter) Start(_ context.Context) error {
	return nil
}

// Stop clears the cache and closes idle HTTP connections.
func (a *Adapter) Stop(_ context.Context) error {
	if a.cacheManager != nil {
		a.cacheManager.Invalidate()
	}
	if a.client != nil {
		a.client.CloseIdleConnections()
	}
	return nil
}

// Health checks that the API key is configured and the adapter is initialised.
func (a *Adapter) Health() error {
	if a.apiKey == "" {
		return fmt.Errorf("kimi: API key not configured")
	}
	if a.client == nil {
		return fmt.Errorf("kimi: HTTP client not initialized")
	}
	return nil
}

// Capabilities returns the capability set for Kimi models.
func (a *Adapter) Capabilities() core.ProviderCapabilities {
	return core.ProviderCapabilities{
		SupportsToolCalls:    true,
		SupportsThinking:     false,
		SupportsStreaming:    true,
		SupportsSystemPrompt: true,
		SupportsVision:       false,
		MaxContextTokens:     1000000,
	}
}

// Query sends a streaming request to the Kimi Chat Completions API.
// If the system prompt exceeds the caching threshold, it will attempt to
// create a context cache on the first request and reuse it on subsequent ones.
func (a *Adapter) Query(ctx context.Context, req core.QueryRequest) (<-chan core.StreamEvent, error) {
	if a.client == nil {
		return nil, fmt.Errorf("kimi: adapter not initialized, call Init() first")
	}
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("kimi: at least one message is required")
	}

	// Build API tools list for reuse.
	apiTools := buildAPITools(req.Tools)

	// Attempt cache management when system prompt is large enough.
	var cacheID string
	if req.SystemPrompt != "" && estimateTokens(req.SystemPrompt, apiTools) >= cacheTokenThreshold {
		id, err := a.cacheManager.EnsureCache(ctx, a.client, a.baseURL, a.apiKey, req.SystemPrompt, apiTools, req.Model)
		if err != nil {
			// Non-fatal: fall back to sending full request without cache.
			slog.WarnContext(ctx, "kimi: context cache creation failed, falling back to full request", "error", err)
		} else {
			cacheID = id
		}
	}

	apiReq := toAPIRequest(req, apiTools, cacheID)

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("kimi: failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+chatCompletionsPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("kimi: failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, wrapHTTPError(err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		if cacheID != "" {
			a.cacheManager.Invalidate()
		}
		return nil, handleHTTPStatus(resp)
	}

	ch := make(chan core.StreamEvent, 16)
	go a.readStream(ctx, resp.Body, ch)

	return ch, nil
}

func (a *Adapter) readStream(ctx context.Context, body io.ReadCloser, ch chan<- core.StreamEvent) {
	defer close(ch)
	defer body.Close()

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			body.Close()
		case <-done:
		}
	}()

	parser := newStreamParser(body)
	sentDone := false
	for {
		event, err := parser.next()
		if err == io.EOF {
			if !sentDone {
				var finalErr error
				if ctx.Err() != nil {
					finalErr = ctx.Err()
				} else {
					finalErr = fmt.Errorf("kimi: stream ended unexpectedly (no [DONE] received)")
				}
				trySend(ch, &providers.ErrorEvent{Err: finalErr})
			}
			return
		}
		if err != nil {
			if ctx.Err() != nil {
				trySend(ch, &providers.ErrorEvent{Err: ctx.Err()})
				return
			}
			trySend(ch, &providers.ErrorEvent{Err: fmt.Errorf("kimi: stream parse error: %w", err)})
			return
		}
		if event != nil {
			if _, ok := event.(*providers.DoneEvent); ok {
				sentDone = true
			}
			select {
			case ch <- event:
			case <-ctx.Done():
				return
			}
		}
	}
}

// trySend attempts to send an event on the channel without blocking.
func trySend(ch chan<- core.StreamEvent, event core.StreamEvent) {
	select {
	case ch <- event:
	default:
	}
}
