// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/providers"
)

const (
	defaultBaseURL        = "https://openrouter.ai/api/v1"
	chatCompletionsPath   = "/chat/completions"
	dialTimeout           = 10 * time.Second
	tlsHandshakeTimeout   = 10 * time.Second
	responseHeaderTimeout = 30 * time.Second
)

// Adapter implements core.Provider for the OpenRouter API (OpenAI-compatible).
type Adapter struct {
	credStore core.CredentialStore
	apiKey    string
	client    *http.Client
	baseURL   string
}

// New creates a new OpenRouter adapter.
// Respects OPENROUTER_BASE_URL env var for proxy/benchmark compatibility.
func New(credStore core.CredentialStore) *Adapter {
	base := defaultBaseURL
	if env := os.Getenv("OPENROUTER_BASE_URL"); env != "" {
		base = strings.TrimSuffix(env, "/")
	}
	return &Adapter{
		credStore: credStore,
		baseURL:   base,
	}
}

// Init loads the API key from the credential store.
func (a *Adapter) Init(ctx context.Context) error {
	if a.credStore == nil {
		return fmt.Errorf("openrouter: credential store is nil")
	}
	cred, err := a.credStore.GetProvider(ctx, "openrouter")
	if err != nil {
		return fmt.Errorf("openrouter: failed to get credentials: %w", err)
	}
	if cred.Value == "" {
		return fmt.Errorf("openrouter: API key is empty")
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
	return nil
}

// Start is a no-op for HTTP-based providers.
func (a *Adapter) Start(_ context.Context) error {
	return nil
}

// Stop closes idle HTTP connections.
func (a *Adapter) Stop(_ context.Context) error {
	if a.client != nil {
		a.client.CloseIdleConnections()
	}
	return nil
}

// Health checks that the API key is configured and the adapter is initialized.
func (a *Adapter) Health() error {
	if a.apiKey == "" {
		return fmt.Errorf("openrouter: API key not configured")
	}
	if a.client == nil {
		return fmt.Errorf("openrouter: HTTP client not initialized")
	}
	return nil
}

// Capabilities returns the capability set for OpenRouter (matching OpenAI passthrough).
func (a *Adapter) Capabilities() core.ProviderCapabilities {
	return core.ProviderCapabilities{
		SupportsToolCalls:    true,
		SupportsThinking:     false,
		SupportsStreaming:    true,
		SupportsSystemPrompt: true,
		SupportsVision:       true,
		MaxContextTokens:     128000,
	}
}

// Query sends a streaming request to the OpenRouter API.
func (a *Adapter) Query(ctx context.Context, req core.QueryRequest) (<-chan core.StreamEvent, error) {
	if a.client == nil {
		return nil, fmt.Errorf("openrouter: adapter not initialized, call Init() first")
	}
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("openrouter: at least one message is required")
	}

	apiReq := toAPIRequest(req)

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("openrouter: failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+chatCompletionsPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openrouter: failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	httpReq.Header.Set("HTTP-Referer", "https://siply.dev")
	httpReq.Header.Set("X-Title", "siply")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, wrapHTTPError(err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
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
					finalErr = fmt.Errorf("openrouter: stream ended unexpectedly (no [DONE] received)")
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
			trySend(ch, &providers.ErrorEvent{Err: fmt.Errorf("openrouter: stream parse error: %w", err)})
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
