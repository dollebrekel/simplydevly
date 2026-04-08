// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package licensing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestDeviceCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

		require.NoError(t, r.ParseForm())
		assert.Equal(t, "test-client-id", r.FormValue("client_id"))
		assert.Equal(t, "user:email,read:user,repo", r.FormValue("scope"))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(DeviceCodeResponse{
			DeviceCode:      "dev-code-123",
			UserCode:        "ABCD-1234",
			VerificationURI: "https://github.com/login/device",
			ExpiresIn:       900,
			Interval:        5,
		})
	}))
	defer server.Close()

	origURL := GitHubDeviceCodeURL
	GitHubDeviceCodeURL = server.URL
	defer func() { GitHubDeviceCodeURL = origURL }()

	dcr, err := RequestDeviceCode("test-client-id", "user:email,read:user,repo")
	require.NoError(t, err)

	assert.Equal(t, "dev-code-123", dcr.DeviceCode)
	assert.Equal(t, "ABCD-1234", dcr.UserCode)
	assert.Equal(t, "https://github.com/login/device", dcr.VerificationURI)
	assert.Equal(t, 900, dcr.ExpiresIn)
	assert.Equal(t, 5, dcr.Interval)
}

func TestPollForToken_Success(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount < 3 {
			json.NewEncoder(w).Encode(tokenResponse{Error: "authorization_pending"})
			return
		}
		json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "gho_test_token",
			TokenType:   "bearer",
			Scope:       "user:email,read:user,repo",
		})
	}))
	defer server.Close()

	origURL := GitHubTokenURL
	GitHubTokenURL = server.URL
	defer func() { GitHubTokenURL = origURL }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	token, err := PollForToken(ctx, "test-client-id", "dev-code-123", 1)
	require.NoError(t, err)
	assert.Equal(t, "gho_test_token", token)
	assert.GreaterOrEqual(t, callCount, 3)
}

func TestPollForToken_Expired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse{Error: "expired_token"})
	}))
	defer server.Close()

	origURL := GitHubTokenURL
	GitHubTokenURL = server.URL
	defer func() { GitHubTokenURL = origURL }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := PollForToken(ctx, "test-client-id", "dev-code-123", 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestPollForToken_AccessDenied(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse{Error: "access_denied"})
	}))
	defer server.Close()

	origURL := GitHubTokenURL
	GitHubTokenURL = server.URL
	defer func() { GitHubTokenURL = origURL }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := PollForToken(ctx, "test-client-id", "dev-code-123", 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "denied")
}

func TestFetchGitHubUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer gho_test_token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(gitHubUser{
			Login: "testuser",
			ID:    12345,
			Name:  "Test User",
			Email: "test@example.com",
		})
	}))
	defer server.Close()

	origURL := GitHubUserURL
	GitHubUserURL = server.URL
	defer func() { GitHubUserURL = origURL }()

	user, err := FetchGitHubUser("gho_test_token")
	require.NoError(t, err)

	assert.Equal(t, "testuser", user.Login)
	assert.Equal(t, int64(12345), user.ID)
	assert.Equal(t, "Test User", user.Name)
	assert.Equal(t, "test@example.com", user.Email)
}

func TestPollForToken_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse{Error: "authorization_pending"})
	}))
	defer server.Close()

	origURL := GitHubTokenURL
	GitHubTokenURL = server.URL
	defer func() { GitHubTokenURL = origURL }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := PollForToken(ctx, "test-client-id", "dev-code-123", 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
}
