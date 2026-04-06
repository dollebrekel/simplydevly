// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package licensing

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartCallbackServer(t *testing.T) {
	port, tokenCh, shutdown, err := StartCallbackServer(0, "")
	require.NoError(t, err)
	defer shutdown()

	assert.Greater(t, port, 0)
	assert.NotNil(t, tokenCh)
}

func TestOAuthCallbackServer(t *testing.T) {
	port, tokenCh, shutdown, err := StartCallbackServer(0, "")
	require.NoError(t, err)
	defer shutdown()

	// Send mock callback with token.
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/callback?token=test-jwt-token", port))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Token should be received.
	select {
	case result := <-tokenCh:
		require.NoError(t, result.Err)
		assert.Equal(t, "test-jwt-token", result.Token)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for token")
	}
}

func TestOAuthCallbackWithState(t *testing.T) {
	state := "test-state-abc123"
	port, tokenCh, shutdown, err := StartCallbackServer(0, state)
	require.NoError(t, err)
	defer shutdown()

	// Callback with correct state should succeed.
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/callback?token=test-jwt&state=%s", port, state))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	select {
	case result := <-tokenCh:
		require.NoError(t, result.Err)
		assert.Equal(t, "test-jwt", result.Token)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for token")
	}
}

func TestOAuthCallbackInvalidState(t *testing.T) {
	port, _, shutdown, err := StartCallbackServer(0, "expected-state")
	require.NoError(t, err)
	defer shutdown()

	// Callback with wrong state should be rejected.
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/callback?token=test-jwt&state=wrong-state", port))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestOAuthCallbackMissingToken(t *testing.T) {
	port, _, shutdown, err := StartCallbackServer(0, "")
	require.NoError(t, err)
	defer shutdown()

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/callback", port))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOAuthTimeout(t *testing.T) {
	_, tokenCh, shutdown, err := StartCallbackServer(0, "")
	require.NoError(t, err)
	defer shutdown()

	// Use a very short timeout context so WaitForToken returns quickly.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = WaitForToken(ctx, tokenCh)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Login timed out")
}

func TestOAuthAutoPort(t *testing.T) {
	// Start two servers — second should auto-find a different port.
	port1, _, shutdown1, err := StartCallbackServer(0, "")
	require.NoError(t, err)
	defer shutdown1()

	port2, _, shutdown2, err := StartCallbackServer(port1, "")
	require.NoError(t, err)
	defer shutdown2()

	assert.NotEqual(t, port1, port2, "second server should use a different port")
}

func TestOAuthGracefulShutdown(t *testing.T) {
	port, _, shutdown, err := StartCallbackServer(0, "")
	require.NoError(t, err)

	shutdown()

	// After shutdown, server should not accept connections.
	time.Sleep(50 * time.Millisecond)
	_, err = http.Get(fmt.Sprintf("http://127.0.0.1:%d/callback?token=test", port))
	assert.Error(t, err, "server should be shut down")
}
