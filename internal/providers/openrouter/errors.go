// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package openrouter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"

	"siply.dev/siply/internal/core"
)

// handleHTTPStatus returns an error for non-200 HTTP responses.
func handleHTTPStatus(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("openrouter: invalid API key (HTTP 401)")
	case http.StatusPaymentRequired:
		return fmt.Errorf("openrouter: insufficient credits (HTTP 402)")
	case http.StatusTooManyRequests:
		retryAfter := resp.Header.Get("Retry-After")
		if retryAfter != "" {
			return fmt.Errorf("openrouter: rate limited, retry after %s (HTTP 429)", retryAfter)
		}
		return fmt.Errorf("openrouter: rate limited (HTTP 429)")
	case http.StatusBadRequest:
		return fmt.Errorf("openrouter: bad request (HTTP 400): %s", body)
	case http.StatusServiceUnavailable:
		return fmt.Errorf("openrouter: model unavailable (HTTP 503): %s", body)
	default:
		if resp.StatusCode >= 500 {
			return fmt.Errorf("openrouter: server error (HTTP %d): %s", resp.StatusCode, body)
		}
		return fmt.Errorf("openrouter: unexpected status (HTTP %d): %s", resp.StatusCode, body)
	}
}

// wrapHTTPError translates Go HTTP client errors into meaningful messages.
func wrapHTTPError(err error) error {
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("openrouter: request canceled: %w", err)
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("openrouter: %w: %w", core.ErrProviderTimeout, err)
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return fmt.Errorf("openrouter: connection failed (%s): %w", opErr.Addr, err)
	}

	return fmt.Errorf("openrouter: request failed: %w", err)
}
