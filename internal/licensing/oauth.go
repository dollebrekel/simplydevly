// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package licensing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"time"
)

const (
	defaultCallbackPort = 19284
	oauthTimeout        = 5 * time.Minute
)

// CallbackResult holds the result from the OAuth callback.
type CallbackResult struct {
	Token string
	Err   error
}

// GenerateState creates a cryptographically random state parameter for CSRF protection.
func GenerateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("licensing: failed to generate state: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// StartCallbackServer starts a temporary HTTP server to receive the OAuth callback token.
// It returns the port the server is listening on, a channel that receives the token,
// and a shutdown function. The expectedState parameter is validated against the state
// query parameter on the callback to prevent CSRF attacks.
func StartCallbackServer(port int, expectedState string) (actualPort int, tokenCh <-chan CallbackResult, shutdown func(), err error) {
	if port == 0 {
		port = defaultCallbackPort
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		// Auto-find available port if default is taken.
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return 0, nil, nil, fmt.Errorf("licensing: failed to start callback server: %w", err)
		}
	}

	actualPort = listener.Addr().(*net.TCPAddr).Port
	ch := make(chan CallbackResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if expectedState != "" {
			state := r.URL.Query().Get("state")
			if state != expectedState {
				http.Error(w, "invalid state parameter", http.StatusForbidden)
				return
			}
		}

		token := r.URL.Query().Get("token")
		if token == "" {
			http.Error(w, "missing token parameter", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body><h2>Login successful!</h2><p>You can close this tab and return to the terminal.</p></body></html>`)

		// Non-blocking send — only the first callback is accepted.
		select {
		case ch <- CallbackResult{Token: token}:
		default:
		}
	})

	server := &http.Server{Handler: mux}

	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			select {
			case ch <- CallbackResult{Err: fmt.Errorf("licensing: callback server error: %w", serveErr)}:
			default:
			}
		}
	}()

	shutdownFn := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}

	return actualPort, ch, shutdownFn, nil
}

// WaitForToken waits for the OAuth callback token with a timeout.
// Returns the token string or an error if the timeout is exceeded.
func WaitForToken(ctx context.Context, tokenCh <-chan CallbackResult) (string, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, oauthTimeout)
	defer cancel()

	select {
	case result := <-tokenCh:
		if result.Err != nil {
			return "", result.Err
		}
		return result.Token, nil
	case <-timeoutCtx.Done():
		return "", fmt.Errorf("Login timed out. Try again with `siply login`")
	}
}
