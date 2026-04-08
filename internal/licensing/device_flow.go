// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package licensing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GitHubClientID is the OAuth App client ID for Device Flow.
// Override via SIPLY_GITHUB_CLIENT_ID env var for development.
var GitHubClientID = "Ov23liXXXXXXXXXXXXXX" // TODO: replace with real OAuth App client ID

// GitHubDeviceCodeURL and GitHubTokenURL are the GitHub Device Flow endpoints.
// Overridable for testing.
var (
	GitHubDeviceCodeURL = "https://github.com/login/device/code"
	GitHubTokenURL      = "https://github.com/login/oauth/access_token"
	GitHubUserURL       = "https://api.github.com/user"
)

// DeviceCodeResponse is returned by GitHub's device/code endpoint.
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// tokenResponse is the response from the token polling endpoint.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
}

// gitHubUser is the response from the /user endpoint.
type gitHubUser struct {
	Login string `json:"login"`
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// HTTPClient is the HTTP client used for Device Flow requests. Overridable for testing.
var HTTPClient = &http.Client{Timeout: 10 * time.Second}

// RequestDeviceCode initiates the Device Flow by requesting a device code from GitHub.
func RequestDeviceCode(clientID, scopes string) (*DeviceCodeResponse, error) {
	data := url.Values{
		"client_id": {clientID},
		"scope":     {scopes},
	}

	req, err := http.NewRequest("POST", GitHubDeviceCodeURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("licensing: device code request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("licensing: device code request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return nil, fmt.Errorf("licensing: reading device code response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("licensing: device code request returned %d: %s", resp.StatusCode, body)
	}

	var dcr DeviceCodeResponse
	if err := json.Unmarshal(body, &dcr); err != nil {
		return nil, fmt.Errorf("licensing: parsing device code response: %w", err)
	}

	if dcr.DeviceCode == "" || dcr.UserCode == "" {
		return nil, fmt.Errorf("licensing: device code response missing required fields")
	}

	return &dcr, nil
}

// PollForToken polls GitHub's token endpoint until the user authorizes or the code expires.
func PollForToken(ctx context.Context, clientID string, deviceCode string, interval int) (string, error) {
	if interval < 1 {
		interval = 5
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("Login timed out. Try again with `siply login`")
		case <-ticker.C:
			token, done, err := pollOnce(clientID, deviceCode)
			if err != nil {
				return "", err
			}
			if done {
				return token, nil
			}
		}
	}
}

func pollOnce(clientID, deviceCode string) (token string, done bool, err error) {
	data := url.Values{
		"client_id":   {clientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}

	req, err := http.NewRequest("POST", GitHubTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", false, fmt.Errorf("licensing: token poll request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("licensing: token poll failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return "", false, fmt.Errorf("licensing: reading token response: %w", err)
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", false, fmt.Errorf("licensing: parsing token response: %w", err)
	}

	switch tr.Error {
	case "":
		if tr.AccessToken == "" {
			return "", false, fmt.Errorf("licensing: empty access token in response")
		}
		return tr.AccessToken, true, nil
	case "authorization_pending":
		return "", false, nil
	case "slow_down":
		// GitHub asks us to increase interval — just wait for next tick.
		return "", false, nil
	case "expired_token":
		return "", false, fmt.Errorf("Device code expired. Run `siply login` to try again")
	case "access_denied":
		return "", false, fmt.Errorf("Authorization denied by user")
	default:
		return "", false, fmt.Errorf("licensing: unexpected error from GitHub: %s", tr.Error)
	}
}

// FetchGitHubUser retrieves the authenticated user's profile using an access token.
func FetchGitHubUser(accessToken string) (*gitHubUser, error) {
	req, err := http.NewRequest("GET", GitHubUserURL, nil)
	if err != nil {
		return nil, fmt.Errorf("licensing: user request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("licensing: fetching user profile: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return nil, fmt.Errorf("licensing: reading user response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("licensing: user request returned %d: %s", resp.StatusCode, body)
	}

	var user gitHubUser
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("licensing: parsing user response: %w", err)
	}

	return &user, nil
}
