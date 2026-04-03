package anthropic

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
	defaultBaseURL        = "https://api.anthropic.com"
	messagesPath          = "/v1/messages"
	anthropicVersion      = "2023-06-01"
	dialTimeout           = 10 * time.Second
	tlsHandshakeTimeout   = 10 * time.Second
	responseHeaderTimeout = 30 * time.Second
)

// Adapter implements core.Provider for the Anthropic Messages API.
type Adapter struct {
	credStore core.CredentialStore
	apiKey    string
	client    *http.Client
	baseURL   string
}

// New creates a new Anthropic adapter.
func New(credStore core.CredentialStore) *Adapter {
	return &Adapter{
		credStore: credStore,
		baseURL:   defaultBaseURL,
	}
}

// Init loads the API key from the credential store.
func (a *Adapter) Init(ctx context.Context) error {
	cred, err := a.credStore.GetProvider(ctx, "anthropic")
	if err != nil {
		return fmt.Errorf("anthropic: failed to get credentials: %w", err)
	}
	if cred.Value == "" {
		return fmt.Errorf("anthropic: API key is empty")
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
		return fmt.Errorf("anthropic: API key not configured")
	}
	if a.client == nil {
		return fmt.Errorf("anthropic: HTTP client not initialized")
	}
	return nil
}

// Capabilities returns the capability set for Claude models.
func (a *Adapter) Capabilities() core.ProviderCapabilities {
	return core.ProviderCapabilities{
		SupportsToolCalls:    true,
		SupportsThinking:     true,
		SupportsStreaming:    true,
		SupportsSystemPrompt: true,
		SupportsVision:       true,
		MaxContextTokens:     200000,
	}
}

// Query sends a streaming request to the Anthropic Messages API.
func (a *Adapter) Query(ctx context.Context, req core.QueryRequest) (<-chan core.StreamEvent, error) {
	if a.client == nil {
		return nil, fmt.Errorf("anthropic: adapter not initialized, call Init() first")
	}
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("anthropic: at least one message is required")
	}

	apiReq := toAPIRequest(req)

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+messagesPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("anthropic: failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)

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

	// Close body when context is canceled to unblock the scanner.
	go func() {
		<-ctx.Done()
		body.Close()
	}()

	parser := newStreamParser(body)
	sentDone := false
	for {
		event, err := parser.next()
		if err == io.EOF {
			if !sentDone {
				select {
				case ch <- &providers.DoneEvent{}:
				case <-ctx.Done():
				}
			}
			return
		}
		if err != nil {
			if ctx.Err() != nil {
				select {
				case ch <- &providers.ErrorEvent{Err: ctx.Err()}:
				case <-ctx.Done():
				}
				return
			}
			select {
			case ch <- &providers.ErrorEvent{Err: fmt.Errorf("anthropic: stream parse error: %w", err)}:
			case <-ctx.Done():
			}
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
