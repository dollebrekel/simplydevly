// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/providers"
)

const (
	defaultBaseURL        = "http://localhost:11434"
	chatPath              = "/api/chat"
	tagsPath              = "/api/tags"
	dialTimeout           = 10 * time.Second
	responseHeaderTimeout = 30 * time.Second
	healthTimeout         = 2 * time.Second
)

// Adapter implements core.Provider for the Ollama Chat API.
type Adapter struct {
	credStore core.CredentialStore
	baseURL   string
	client    *http.Client
}

// New creates a new Ollama adapter.
func New(credStore core.CredentialStore) *Adapter {
	return &Adapter{
		credStore: credStore,
		baseURL:   defaultBaseURL,
	}
}

// Init loads the base URL from the credential store (or uses default).
// A nil credential store is allowed — the adapter uses defaultBaseURL.
func (a *Adapter) Init(ctx context.Context) error {
	if a.credStore != nil {
		cred, err := a.credStore.GetProvider(ctx, "ollama")
		if err != nil {
			return fmt.Errorf("ollama: failed to get credentials: %w", err)
		}
		if cred.Value != "" {
			a.baseURL = cred.Value
		}
	}
	a.client = &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: dialTimeout,
			}).DialContext,
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

// Health checks that the Ollama instance is reachable.
func (a *Adapter) Health() error {
	if a.client == nil {
		return fmt.Errorf("ollama: HTTP client not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), healthTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, a.baseURL+tagsPath, nil)
	if err != nil {
		return fmt.Errorf("ollama: failed to create health request: %w", err)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("ollama: instance not reachable at %s: %w", a.baseURL, err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama: health check returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// Capabilities returns the conservative capability set for local Ollama models.
func (a *Adapter) Capabilities() core.ProviderCapabilities {
	return core.ProviderCapabilities{
		SupportsToolCalls:    false,
		SupportsThinking:     false,
		SupportsStreaming:    true,
		SupportsSystemPrompt: true,
		SupportsVision:       false,
		MaxContextTokens:     8192,
	}
}

// Query sends a streaming request to the Ollama Chat API.
func (a *Adapter) Query(ctx context.Context, req core.QueryRequest) (<-chan core.StreamEvent, error) {
	if a.client == nil {
		return nil, fmt.Errorf("ollama: adapter not initialized, call Init() first")
	}
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("ollama: at least one message is required")
	}

	apiReq := toAPIRequest(req)

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+chatPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama: failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

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
					finalErr = fmt.Errorf("ollama: stream ended unexpectedly (no done signal received)")
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
			trySend(ch, &providers.ErrorEvent{Err: fmt.Errorf("ollama: stream parse error: %w", err)})
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
