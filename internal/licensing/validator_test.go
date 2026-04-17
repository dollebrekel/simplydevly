// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package licensing

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/events"
)

// mockBrowserOpener captures the URL instead of opening a browser.
func mockBrowserOpener(url string) error {
	return nil
}

func setupValidator(t *testing.T) (*licenseValidator, string) {
	t.Helper()
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".siply")

	bus := events.NewBus()
	ctx := context.Background()
	require.NoError(t, bus.Init(ctx))
	require.NoError(t, bus.Start(ctx))
	t.Cleanup(func() { _ = bus.Stop(ctx) })

	v := NewLicenseValidator(bus, configDir).(*licenseValidator)
	require.NoError(t, v.Init(ctx))

	// Override browser opener for tests.
	origOpener := BrowserOpener
	BrowserOpener = mockBrowserOpener
	t.Cleanup(func() { BrowserOpener = origOpener })

	return v, configDir
}

// loginViaDeviceFlow simulates the GitHub Device Flow using mock HTTP servers
// for the device code, token, and user endpoints.
func loginViaDeviceFlow(t *testing.T, v *licenseValidator) core.LicenseStatus {
	t.Helper()

	// Mock device code endpoint.
	deviceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(DeviceCodeResponse{
			DeviceCode:      "mock-device-code",
			UserCode:        "MOCK-1234",
			VerificationURI: "https://github.com/login/device",
			ExpiresIn:       900,
			Interval:        1,
		})
	}))
	t.Cleanup(deviceServer.Close)

	// Mock token endpoint — returns token immediately.
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "gho_mock_token",
			TokenType:   "bearer",
			Scope:       "user:email,read:user,repo",
		})
	}))
	t.Cleanup(tokenServer.Close)

	// Mock user endpoint.
	userServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(gitHubUser{
			Login: "mockuser",
			ID:    42,
			Name:  "Mock User",
			Email: "mock@example.com",
		})
	}))
	t.Cleanup(userServer.Close)

	// Override endpoints.
	origDevice := GitHubDeviceCodeURL
	origToken := GitHubTokenURL
	origUser := GitHubUserURL
	GitHubDeviceCodeURL = deviceServer.URL
	GitHubTokenURL = tokenServer.URL
	GitHubUserURL = userServer.URL
	t.Cleanup(func() {
		GitHubDeviceCodeURL = origDevice
		GitHubTokenURL = origToken
		GitHubUserURL = origUser
	})

	status, err := v.Login(context.Background(), core.AuthGitHub)
	require.NoError(t, err)
	return status
}

func TestLoginStoresAccountJson(t *testing.T) {
	v, configDir := setupValidator(t)

	status := loginViaDeviceFlow(t, v)

	assert.True(t, status.LoggedIn)
	assert.Equal(t, "github", status.AuthProvider)
	assert.Equal(t, core.TierFree, status.Tier)
	assert.True(t, status.Valid)
	assert.NotEmpty(t, status.InstanceID)

	// Verify file exists with correct permissions.
	path := filepath.Join(configDir, accountFileName)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(filePermissions), info.Mode().Perm())

	// Verify JSON content — token should be encrypted at rest.
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var account accountData
	require.NoError(t, json.Unmarshal(data, &account))
	assert.Equal(t, "github", account.AuthProvider)
	assert.NotEmpty(t, account.InstanceID)
	assert.True(t, account.Encrypted, "token should be encrypted at rest")
	assert.NotEqual(t, "gho_mock_token", account.Token, "raw token should not appear on disk")

	// Verify round-trip: decrypt should yield original token.
	key, err := DeriveKey()
	require.NoError(t, err)
	ciphertext, err := base64.StdEncoding.DecodeString(account.Token)
	require.NoError(t, err)
	plaintext, err := Decrypt(ciphertext, key)
	require.NoError(t, err)
	assert.Equal(t, "gho_mock_token", string(plaintext))
}

func TestLogoutRemovesFile(t *testing.T) {
	v, configDir := setupValidator(t)

	loginViaDeviceFlow(t, v)

	// File should exist.
	path := filepath.Join(configDir, accountFileName)
	_, err := os.Stat(path)
	require.NoError(t, err)

	// Logout.
	require.NoError(t, v.Logout())

	// File should be gone.
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err))

	// Status should show not logged in.
	status := v.Validate()
	assert.False(t, status.LoggedIn)
}

func TestValidateReadsCache(t *testing.T) {
	v, configDir := setupValidator(t)

	// Not logged in initially.
	status := v.Validate()
	assert.False(t, status.LoggedIn)
	assert.True(t, status.Valid)
	assert.Equal(t, core.TierFree, status.Tier)

	// Write account.json manually.
	account := accountData{
		AuthProvider: "google",
		AccountEmail: "test@example.com",
		DisplayName:  "Test User",
		InstanceID:   "test-uuid",
		Token:        "some-token",
		CreatedAt:    time.Now().UTC(),
	}
	data, err := json.Marshal(account)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(configDir, accountFileName), data, filePermissions))

	// Clear cache so Validate re-reads.
	v.cached = nil
	status = v.Validate()
	assert.True(t, status.LoggedIn)
	assert.Equal(t, "google", status.AuthProvider)
	assert.Equal(t, "test@example.com", status.AccountEmail)
	assert.Equal(t, "Test User", status.DisplayName)
	assert.Equal(t, "test-uuid", status.InstanceID)
}

func TestInstanceIDGeneratedOnce(t *testing.T) {
	v, _ := setupValidator(t)

	status1 := loginViaDeviceFlow(t, v)
	firstID := status1.InstanceID
	assert.NotEmpty(t, firstID)

	// Login again — should keep the same InstanceID.
	status2 := loginViaDeviceFlow(t, v)
	assert.Equal(t, firstID, status2.InstanceID, "InstanceID should persist across logins")
}

func TestLogoutWhenNotLoggedIn(t *testing.T) {
	v, _ := setupValidator(t)

	// Logout without login should not error.
	err := v.Logout()
	assert.NoError(t, err)
}

func TestValidateWithoutAccount(t *testing.T) {
	v, _ := setupValidator(t)

	status := v.Validate()
	assert.True(t, status.Valid)
	assert.False(t, status.LoggedIn)
	assert.Equal(t, core.TierFree, status.Tier)
	assert.Empty(t, status.InstanceID)
}

// --- PB-6 tests ---

func TestValidateNoAccount(t *testing.T) {
	// AC#2: Validate() returns LoggedIn: false if no account.json exists.
	v, _ := setupValidator(t)

	status := v.Validate()
	assert.False(t, status.LoggedIn, "should not be logged in without account.json")
	assert.Equal(t, core.TierFree, status.Tier, "should default to TierFree")
	assert.True(t, status.Valid)
}

func TestValidateWithAccount(t *testing.T) {
	// AC#1: Validate() reads account.json and returns LicenseStatus with Tier: TierFree.
	v, configDir := setupValidator(t)

	account := accountData{
		AuthProvider: "github",
		AccountEmail: "dev@siply.dev",
		DisplayName:  "Dev User",
		GitHubUser:   "devuser",
		InstanceID:   "instance-123",
		Token:        "tok",
		CreatedAt:    time.Now().UTC(),
	}
	data, err := json.Marshal(account)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(configDir, accountFileName), data, filePermissions))

	// Clear cache so Validate re-reads from disk.
	v.cached = nil
	status := v.Validate()

	assert.True(t, status.LoggedIn, "should be logged in with account.json")
	assert.Equal(t, core.TierFree, status.Tier, "should always be TierFree for now")
	assert.Equal(t, "github", status.AuthProvider)
	assert.Equal(t, "dev@siply.dev", status.AccountEmail)
	assert.Equal(t, "Dev User", status.DisplayName)
	assert.Equal(t, "devuser", status.GitHubUser)
	assert.Equal(t, "instance-123", status.InstanceID)
}

func TestActivateProNotImplemented(t *testing.T) {
	// AC#4: ActivatePro() returns ErrNotImplemented with helpful message.
	v, _ := setupValidator(t)

	_, err := v.ActivatePro(context.Background())
	assert.ErrorIs(t, err, ErrNotImplemented)
	assert.Contains(t, err.Error(), "Pro activation coming soon")
	assert.Contains(t, err.Error(), "siply.dev")

	// AC#5: DeactivatePro() returns ErrNotImplemented.
	err = v.DeactivatePro()
	assert.ErrorIs(t, err, ErrNotImplemented)
}

func TestNextCheckCalculation(t *testing.T) {
	// AC#6: NextCheck is set to LastChecked + 5 days.
	v, configDir := setupValidator(t)

	account := accountData{
		AuthProvider: "google",
		AccountEmail: "test@example.com",
		DisplayName:  "Tester",
		InstanceID:   "id-1",
		Token:        "tok",
		CreatedAt:    time.Now().UTC(),
	}
	data, err := json.Marshal(account)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(configDir, accountFileName), data, filePermissions))

	v.cached = nil
	status := v.Validate()

	expectedInterval := 5 * 24 * time.Hour
	actualInterval := status.NextCheck.Sub(status.LastChecked)

	assert.InDelta(t, expectedInterval.Seconds(), actualInterval.Seconds(), 1.0,
		"NextCheck should be LastChecked + 5 days")
}

func TestGracePeriodDefault(t *testing.T) {
	// AC#7: GracePeriod defaults to 7 * 24 * time.Hour.
	v, configDir := setupValidator(t)

	account := accountData{
		AuthProvider: "google",
		AccountEmail: "test@example.com",
		DisplayName:  "Tester",
		InstanceID:   "id-2",
		Token:        "tok",
		CreatedAt:    time.Now().UTC(),
	}
	data, err := json.Marshal(account)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(configDir, accountFileName), data, filePermissions))

	v.cached = nil
	status := v.Validate()

	assert.Equal(t, 7*24*time.Hour, status.GracePeriod, "GracePeriod should default to 7 days")
}

func TestLicenseChangedEventOnLogin(t *testing.T) {
	// AC#8: Publish LicenseChangedEvent to EventBus on Login.
	v, _ := setupValidator(t)

	var mu sync.Mutex
	var receivedEvent LicenseChangedEvent
	var eventReceived bool

	// Subscribe to license change events via the bus (access bus through the validator).
	bus := v.bus
	bus.Subscribe(LicenseChangedEventType, func(_ context.Context, event core.Event) {
		if lce, ok := event.(LicenseChangedEvent); ok {
			mu.Lock()
			receivedEvent = lce
			eventReceived = true
			mu.Unlock()
		}
	})

	loginViaDeviceFlow(t, v)

	// EventBus delivers asynchronously — wait for handler to execute.
	assert.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return eventReceived
	}, 2*time.Second, 10*time.Millisecond, "LicenseChangedEvent should be published on Login")

	mu.Lock()
	assert.True(t, receivedEvent.Status.LoggedIn, "event status should show LoggedIn")
	assert.Equal(t, core.TierFree, receivedEvent.Status.Tier, "event status should show TierFree")
	mu.Unlock()
}

func TestExpiredTokenRejectedOnLoad(t *testing.T) {
	// B1: expired tokens are rejected when loaded from account.json.
	v, configDir := setupValidator(t)

	expired := time.Now().Add(-1 * time.Hour)
	account := accountData{
		AuthProvider:   "github",
		AccountEmail:   "expired@example.com",
		DisplayName:    "Expired User",
		InstanceID:     "inst-expired",
		Token:          "gho_expired",
		TokenExpiresAt: &expired,
		CreatedAt:      time.Now().UTC(),
	}
	data, err := json.Marshal(account)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(configDir, accountFileName), data, filePermissions))

	v.cached = nil
	status := v.Validate()
	assert.False(t, status.LoggedIn, "expired token should not be treated as logged in")
}

func TestNilExpiresAtAccepted(t *testing.T) {
	// B1: tokens without ExpiresAt are accepted (backwards compat with old account.json).
	v, configDir := setupValidator(t)

	account := accountData{
		AuthProvider: "github",
		AccountEmail: "noexpiry@example.com",
		DisplayName:  "No Expiry",
		InstanceID:   "inst-no-exp",
		Token:        "gho_noexp",
		CreatedAt:    time.Now().UTC(),
	}
	data, err := json.Marshal(account)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(configDir, accountFileName), data, filePermissions))

	v.cached = nil
	status := v.Validate()
	assert.True(t, status.LoggedIn, "nil ExpiresAt should be accepted")
}

func TestLoginSetsTokenExpiresAt(t *testing.T) {
	// B1: loginGitHubDeviceFlow sets TokenExpiresAt on the stored account.
	v, configDir := setupValidator(t)
	loginViaDeviceFlow(t, v)

	data, err := os.ReadFile(filepath.Join(configDir, accountFileName))
	require.NoError(t, err)
	var account accountData
	require.NoError(t, json.Unmarshal(data, &account))
	assert.NotNil(t, account.TokenExpiresAt, "login should set TokenExpiresAt")
	assert.True(t, account.TokenExpiresAt.After(time.Now()), "token expiry should be in the future")
}

func TestConcurrentValidateAndLogin(t *testing.T) {
	// B2: concurrent Validate + Login/Logout must not race on cached field.
	// This test should be run with -race flag.
	v, _ := setupValidator(t)

	var wg sync.WaitGroup
	// Goroutine 1: concurrent Validate calls.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 50 {
			_ = v.Validate()
		}
	}()
	// Goroutine 2: concurrent Logout calls (Login requires network, Logout exercises the same mutex paths).
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 50 {
			_ = v.Logout()
		}
	}()
	// Main goroutine: concurrent Validate calls.
	for range 50 {
		_ = v.Validate()
	}
	wg.Wait()
}

func TestLicenseChangedEventOnLogout(t *testing.T) {
	// AC#8: Publish LicenseChangedEvent to EventBus on Logout.
	v, _ := setupValidator(t)

	loginViaDeviceFlow(t, v)

	var mu sync.Mutex
	var receivedEvent LicenseChangedEvent
	var eventReceived bool

	v.bus.Subscribe(LicenseChangedEventType, func(_ context.Context, event core.Event) {
		if lce, ok := event.(LicenseChangedEvent); ok {
			mu.Lock()
			receivedEvent = lce
			eventReceived = true
			mu.Unlock()
		}
	})

	require.NoError(t, v.Logout())

	// EventBus delivers asynchronously — wait for handler to execute.
	assert.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return eventReceived
	}, 2*time.Second, 10*time.Millisecond, "LicenseChangedEvent should be published on Logout")

	mu.Lock()
	assert.False(t, receivedEvent.Status.LoggedIn, "event status should show not LoggedIn after logout")
	mu.Unlock()
}

// --- JWT validation tests (PC-3.3) ---

// buildJWT creates a minimal unsigned JWT with the given exp claim.
// This is for testing only — not for production use.
func buildJWT(exp int64) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	var claimsJSON string
	if exp == 0 {
		claimsJSON = `{"sub":"test"}`
	} else {
		claimsJSON = fmt.Sprintf(`{"sub":"test","exp":%d}`, exp)
	}
	claims := base64.RawURLEncoding.EncodeToString([]byte(claimsJSON))
	return header + "." + claims + ".fakesig"
}

func TestValidateTokenClaims_OpaqueToken(t *testing.T) {
	// AC #5: opaque (non-JWT) token → JWT validation skipped, returns nil.
	err := validateTokenClaims("gho_mock_token")
	assert.NoError(t, err, "opaque token should pass JWT validation unchanged")
}

func TestValidateTokenClaims_ValidJWT(t *testing.T) {
	// AC #3: JWT with future exp → returns nil.
	futureExp := time.Now().Add(1 * time.Hour).Unix()
	err := validateTokenClaims(buildJWT(futureExp))
	assert.NoError(t, err, "JWT with future exp should be accepted")
}

func TestValidateTokenClaims_ExpiredJWT(t *testing.T) {
	// AC #2: JWT with past exp → returns ErrExpiredToken.
	pastExp := time.Now().Add(-1 * time.Hour).Unix()
	err := validateTokenClaims(buildJWT(pastExp))
	assert.ErrorIs(t, err, ErrExpiredToken, "JWT with past exp should return ErrExpiredToken")
}

func TestValidateTokenClaims_JWTNoExp(t *testing.T) {
	// AC #4: JWT without exp claim (exp=0 sentinel) → returns nil (falls through to TokenExpiresAt).
	err := validateTokenClaims(buildJWT(0))
	assert.NoError(t, err, "JWT without exp claim should return nil")
}

func TestValidateTokenClaims_MalformedPayload(t *testing.T) {
	// AC #6: JWT with invalid base64 payload → returns error.
	malformed := "header.!!!invalidbase64!!!.sig"
	err := validateTokenClaims(malformed)
	assert.Error(t, err, "malformed JWT payload should return an error")
}

func TestValidateWithExpiredJWTToken(t *testing.T) {
	// AC #1, #2: integration test — account.json with expired JWT token → Validate() returns LoggedIn: false.
	v, configDir := setupValidator(t)

	pastExp := time.Now().Add(-1 * time.Hour).Unix()
	account := accountData{
		AuthProvider: "github",
		AccountEmail: "jwt@example.com",
		DisplayName:  "JWT User",
		InstanceID:   "inst-jwt",
		Token:        buildJWT(pastExp),
		CreatedAt:    time.Now().UTC(),
	}
	data, err := json.Marshal(account)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(configDir, accountFileName), data, filePermissions))

	v.cached = nil
	status := v.Validate()
	assert.False(t, status.LoggedIn, "expired JWT token should not be treated as logged in")
}
