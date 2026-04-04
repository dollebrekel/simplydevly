package ollama

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
	case http.StatusBadRequest:
		return fmt.Errorf("ollama: bad request (HTTP 400): %s", body)
	case http.StatusNotFound:
		return fmt.Errorf("ollama: model not found (HTTP 404): %s", body)
	default:
		if resp.StatusCode >= 500 {
			return fmt.Errorf("ollama: server error (HTTP %d): %s", resp.StatusCode, body)
		}
		return fmt.Errorf("ollama: unexpected status (HTTP %d): %s", resp.StatusCode, body)
	}
}

// wrapHTTPError translates Go HTTP client errors into meaningful messages.
func wrapHTTPError(err error) error {
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("ollama: request canceled: %w", err)
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("ollama: %w: %w", core.ErrProviderTimeout, err)
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return fmt.Errorf("ollama: connection refused at %s: %w", opErr.Addr, err)
	}

	return fmt.Errorf("ollama: request failed: %w", err)
}
