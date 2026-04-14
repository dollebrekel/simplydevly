// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package kimi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
)

// kimiErrorBody is the JSON structure of a Kimi API error response.
type kimiErrorBody struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// handleHTTPStatus converts a non-200 HTTP response to an error.
func handleHTTPStatus(resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	var apiErr kimiErrorBody
	if json.Unmarshal(raw, &apiErr) == nil && apiErr.Error.Message != "" {
		return fmt.Errorf("kimi: API error %d: %s", resp.StatusCode, apiErr.Error.Message)
	}

	if len(raw) > 0 {
		return fmt.Errorf("kimi: HTTP %d: %s", resp.StatusCode, string(raw))
	}
	return fmt.Errorf("kimi: HTTP %d", resp.StatusCode)
}

// isCacheInvalidatingStatus reports whether the HTTP status code indicates that
// the cache_id itself is invalid or not found. Transient errors (429, 5xx) do
// not invalidate the cache — the cache_id remains valid on the Kimi server.
func isCacheInvalidatingStatus(code int) bool {
	return code == http.StatusBadRequest ||
		code == http.StatusNotFound ||
		code == http.StatusUnprocessableEntity
}

// wrapHTTPError wraps network-level errors with provider context.
func wrapHTTPError(err error) error {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("kimi: request timed out: %w", err)
	}
	return fmt.Errorf("kimi: request failed: %w", err)
}
