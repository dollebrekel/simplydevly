package licensing

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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

// loginViaCallback simulates the OAuth callback flow using a mock market server
// that intercepts the OAuth URL and redirects back to the CLI's callback server
// with the correct state parameter.
func loginViaCallback(t *testing.T, v *licenseValidator, provider core.AuthProvider) core.LicenseStatus {
	t.Helper()

	// Start a mock market server that captures the callback URL and state from the OAuth request,
	// then redirects back with the token and state.
	mockMarket := http.NewServeMux()
	mockMarket.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		callbackURL := r.URL.Query().Get("callback")
		state := r.URL.Query().Get("state")
		if callbackURL == "" {
			http.Error(w, "missing callback", http.StatusBadRequest)
			return
		}
		// Redirect to the CLI callback with token and state.
		redirectURL := fmt.Sprintf("%s?token=mock-jwt-token&state=%s", callbackURL, url.QueryEscape(state))
		go func() {
			time.Sleep(50 * time.Millisecond)
			resp, err := http.Get(redirectURL)
			if err == nil {
				resp.Body.Close()
			}
		}()
		w.WriteHeader(http.StatusOK)
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	mockServer := &http.Server{Handler: mockMarket}
	go func() { _ = mockServer.Serve(listener) }()
	t.Cleanup(func() { _ = mockServer.Close() })

	origURL := MarketBaseURL
	MarketBaseURL = fmt.Sprintf("http://127.0.0.1:%d", listener.Addr().(*net.TCPAddr).Port)
	t.Cleanup(func() { MarketBaseURL = origURL })

	// Override BrowserOpener to actually hit the mock market URL instead of opening a browser.
	origOpener := BrowserOpener
	BrowserOpener = func(url string) error {
		go func() {
			resp, err := http.Get(url)
			if err == nil {
				resp.Body.Close()
			}
		}()
		return nil
	}
	t.Cleanup(func() { BrowserOpener = origOpener })

	// Start login in a goroutine since it blocks waiting for callback.
	type loginResult struct {
		status core.LicenseStatus
		err    error
	}
	resultCh := make(chan loginResult, 1)

	go func() {
		status, err := v.Login(context.Background(), provider)
		resultCh <- loginResult{status, err}
	}()

	select {
	case result := <-resultCh:
		require.NoError(t, result.err)
		return result.status
	case <-time.After(10 * time.Second):
		t.Fatal("loginViaCallback timed out")
		return core.LicenseStatus{}
	}
}

func TestLoginStoresAccountJson(t *testing.T) {
	v, configDir := setupValidator(t)

	status := loginViaCallback(t, v, core.AuthGitHub)

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

	// Verify JSON content.
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var account accountData
	require.NoError(t, json.Unmarshal(data, &account))
	assert.Equal(t, "github", account.AuthProvider)
	assert.NotEmpty(t, account.InstanceID)
	assert.Equal(t, "mock-jwt-token", account.Token)
}

func TestLogoutRemovesFile(t *testing.T) {
	v, configDir := setupValidator(t)

	loginViaCallback(t, v, core.AuthGitHub)

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

	status1 := loginViaCallback(t, v, core.AuthGitHub)
	firstID := status1.InstanceID
	assert.NotEmpty(t, firstID)

	// Login again — should keep the same InstanceID.
	status2 := loginViaCallback(t, v, core.AuthGoogle)
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
