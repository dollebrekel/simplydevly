// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package licensing

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/fileutil"
)

const (
	accountFileName    = "account.json"
	filePermissions    = 0600
	validationInterval = 5 * 24 * time.Hour // 5 days between checks
)

// ErrNotImplemented is returned by Pro features that are not yet available.
var ErrNotImplemented = errors.New("Pro activation coming soon. Follow siply.dev for updates.")

// ErrExpiredToken is returned when a stored token has expired.
var ErrExpiredToken = errors.New("licensing: token expired")

// defaultTokenExpiry is the assumed token lifetime for GitHub tokens that
// don't report an explicit expiry (classic personal access tokens).
const defaultTokenExpiry = 90 * 24 * time.Hour

// accountData is the on-disk format for account.json.
type accountData struct {
	AuthProvider  string     `json:"auth_provider"`
	AccountEmail  string     `json:"account_email"`
	DisplayName   string     `json:"display_name"`
	GitHubUser    string     `json:"github_user,omitempty"`
	GitHubID      int64      `json:"github_id,omitempty"`
	InstanceID    string     `json:"instance_id"`
	Token         string     `json:"token"`
	TokenExpiresAt *time.Time `json:"token_expires_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// MarketBaseURL is the OAuth endpoint for simply-market.
// Overridable for testing.
var MarketBaseURL = "https://market.siply.dev"

// BrowserOpener is a function that opens a URL in the browser.
// Overridable for testing.
var BrowserOpener = openBrowser

// licenseValidator implements core.LicenseValidator.
type licenseValidator struct {
	bus       core.EventBus
	configDir string
	mu        sync.RWMutex
	cached    *accountData
}

// NewLicenseValidator creates a new LicenseValidator.
// configDir is typically ~/.siply.
func NewLicenseValidator(bus core.EventBus, configDir string) core.LicenseValidator {
	return &licenseValidator{
		bus:       bus,
		configDir: configDir,
	}
}

func (v *licenseValidator) Init(_ context.Context) error {
	if err := os.MkdirAll(v.configDir, 0700); err != nil {
		return fmt.Errorf("licensing: cannot create config dir: %w", err)
	}
	// Load cached account if exists.
	account := v.loadAccount()
	v.mu.Lock()
	v.cached = account
	v.mu.Unlock()
	return nil
}

func (v *licenseValidator) Start(_ context.Context) error { return nil }
func (v *licenseValidator) Stop(_ context.Context) error  { return nil }

func (v *licenseValidator) Health() error {
	if _, err := os.Stat(v.configDir); err != nil {
		return fmt.Errorf("licensing: config dir inaccessible: %w", err)
	}
	return nil
}

// Login authenticates via OAuth provider.
// GitHub uses Device Flow (no server required). Google is not yet supported.
func (v *licenseValidator) Login(ctx context.Context, provider core.AuthProvider) (core.LicenseStatus, error) {
	switch provider {
	case core.AuthGitHub:
		return v.loginGitHubDeviceFlow(ctx)
	case core.AuthGoogle:
		return core.LicenseStatus{}, fmt.Errorf("Google login is not yet available. Use GitHub login instead: `siply login`")
	default:
		return core.LicenseStatus{}, fmt.Errorf("licensing: unknown provider")
	}
}

// loginGitHubDeviceFlow authenticates via GitHub Device Flow (like gh auth login).
func (v *licenseValidator) loginGitHubDeviceFlow(ctx context.Context) (core.LicenseStatus, error) {
	clientID := GitHubClientID
	if envID := os.Getenv("SIPLY_GITHUB_CLIENT_ID"); envID != "" {
		clientID = envID
	}

	scopes := providerScopes(core.AuthGitHub)
	dcr, err := RequestDeviceCode(clientID, scopes)
	if err != nil {
		return core.LicenseStatus{}, err
	}

	// Show the user code and verification URL.
	fmt.Fprintf(os.Stderr, "\n! First, copy your one-time code: %s\n", dcr.UserCode)
	fmt.Fprintf(os.Stderr, "Then press Enter to open %s in your browser...", dcr.VerificationURI)

	// Try to open browser — if it fails, user can visit URL manually.
	if err := BrowserOpener(dcr.VerificationURI); err != nil {
		fmt.Fprintf(os.Stderr, "\nCould not open browser. Please visit: %s\n", dcr.VerificationURI)
	}

	fmt.Fprintf(os.Stderr, "\nWaiting for authorization...\n")

	// Create a timeout context based on the device code expiry.
	expiry := time.Duration(dcr.ExpiresIn) * time.Second
	if expiry == 0 {
		expiry = oauthTimeout
	}
	pollCtx, cancel := context.WithTimeout(ctx, expiry)
	defer cancel()

	accessToken, err := PollForToken(pollCtx, clientID, dcr.DeviceCode, dcr.Interval)
	if err != nil {
		return core.LicenseStatus{}, err
	}

	// Fetch user profile from GitHub API.
	user, err := FetchGitHubUser(accessToken)
	if err != nil {
		return core.LicenseStatus{}, fmt.Errorf("licensing: failed to fetch user profile: %w", err)
	}

	// Preserve existing InstanceID if present.
	instanceID := uuid.New().String()
	v.mu.RLock()
	if v.cached != nil && v.cached.InstanceID != "" {
		instanceID = v.cached.InstanceID
	}
	v.mu.RUnlock()

	// Default token expiry for GitHub tokens that don't report an explicit one.
	tokenExpiry := time.Now().UTC().Add(defaultTokenExpiry)
	account := &accountData{
		AuthProvider:   "github",
		AccountEmail:   user.Email,
		DisplayName:    user.Name,
		GitHubUser:     user.Login,
		GitHubID:       user.ID,
		InstanceID:     instanceID,
		Token:          accessToken,
		TokenExpiresAt: &tokenExpiry,
		CreatedAt:      time.Now().UTC(),
	}

	// Use login as display name if name is empty.
	if account.DisplayName == "" {
		account.DisplayName = user.Login
	}

	if err := v.storeAccount(account); err != nil {
		return core.LicenseStatus{}, err
	}

	v.mu.Lock()
	v.cached = account
	v.mu.Unlock()
	status := v.buildStatus()

	// Publish login event so StatusCollector and other subscribers can update.
	if v.bus != nil {
		if err := v.bus.Publish(ctx, NewLicenseChangedEvent(status)); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to publish license event: %v\n", err)
		}
	}

	return status, nil
}

// Logout removes account.json and clears session.
func (v *licenseValidator) Logout() error {
	path := v.accountPath()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("licensing: failed to remove account: %w", err)
	}
	v.mu.Lock()
	v.cached = nil
	v.mu.Unlock()

	// Publish logout event (LoggedIn: false, Tier: TierFree).
	if v.bus != nil {
		logoutStatus := core.LicenseStatus{Valid: true, Tier: core.TierFree, LoggedIn: false}
		if err := v.bus.Publish(context.Background(), NewLicenseChangedEvent(logoutStatus)); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to publish logout event: %v\n", err)
		}
	}

	return nil
}

// Validate reads cached account.json and returns LicenseStatus (always Free for now).
func (v *licenseValidator) Validate() core.LicenseStatus {
	v.mu.RLock()
	if v.cached != nil {
		v.mu.RUnlock()
		return v.buildStatus()
	}
	v.mu.RUnlock()

	// Double-checked locking: re-check under write lock to avoid redundant I/O.
	v.mu.Lock()
	if v.cached == nil {
		v.cached = v.loadAccount()
	}
	v.mu.Unlock()
	return v.buildStatus()
}

// Refresh forces online check — stub for now.
func (v *licenseValidator) Refresh(_ context.Context) (core.LicenseStatus, error) {
	return v.Validate(), nil
}

// ActivatePro starts Stripe checkout — stub for now.
func (v *licenseValidator) ActivatePro(_ context.Context) (core.LicenseStatus, error) {
	return core.LicenseStatus{}, ErrNotImplemented
}

// DeactivatePro removes Pro license — stub for now.
func (v *licenseValidator) DeactivatePro() error {
	return ErrNotImplemented
}

// DiscoverRepos is implemented in discovery.go.

// --- JWT validation ---

// jwtClaims holds the minimal claim set we need to validate.
type jwtClaims struct {
	Exp int64 `json:"exp"`
}

// validateTokenClaims checks if token is a JWT and, if so, validates
// its expiry claim. Returns nil for opaque (non-JWT) tokens.
// NOTE: Signature verification is not performed here — deferred until
// simply-market provides a JWKS endpoint.
// TODO(pc-3.3): verify JWT signature once simply-market JWKS endpoint is known
func validateTokenClaims(token string) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil // not a JWT — opaque token, skip JWT validation
	}
	// Decode the claims part (index 1).
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("invalid JWT payload: %w", err)
	}
	var claims jwtClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return fmt.Errorf("invalid JWT claims: %w", err)
	}
	// exp == 0 means no expiry claim — fall through to TokenExpiresAt check.
	if claims.Exp != 0 && time.Now().Unix() >= claims.Exp {
		return ErrExpiredToken
	}
	return nil
}

// --- internal helpers ---

func (v *licenseValidator) accountPath() string {
	return filepath.Join(v.configDir, accountFileName)
}

func (v *licenseValidator) loadAccount() *accountData {
	data, err := os.ReadFile(v.accountPath())
	if err != nil {
		return nil
	}
	var account accountData
	if err := json.Unmarshal(data, &account); err != nil {
		fmt.Fprintf(os.Stderr, "warning: %s is corrupted (%v) — run `siply login` again\n", accountFileName, err)
		return nil
	}
	// Validate JWT claims (expiry) before falling through to TokenExpiresAt check.
	if err := validateTokenClaims(account.Token); err != nil {
		slog.Warn("licensing: JWT token validation failed — run `siply login` to re-authenticate",
			"error", err)
		fmt.Fprintf(os.Stderr, "warning: JWT token validation failed (%v) — run `siply login` again\n", err)
		return nil
	}
	// Reject expired tokens.
	if account.TokenExpiresAt != nil && time.Now().After(*account.TokenExpiresAt) {
		slog.Warn("licensing: stored token has expired — run `siply login` to re-authenticate",
			"expired_at", account.TokenExpiresAt.Format(time.RFC3339))
		return nil
	}
	return &account
}

func (v *licenseValidator) storeAccount(account *accountData) error {
	data, err := json.MarshalIndent(account, "", "  ")
	if err != nil {
		return fmt.Errorf("licensing: failed to marshal account: %w", err)
	}
	path := v.accountPath()
	if err := fileutil.AtomicWriteFile(path, data, filePermissions); err != nil {
		return fmt.Errorf("licensing: failed to write account.json: %w", err)
	}
	return nil
}

func (v *licenseValidator) buildStatus() core.LicenseStatus {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if v.cached == nil {
		return core.LicenseStatus{
			Valid:    true,
			Tier:     core.TierFree,
			LoggedIn: false,
		}
	}

	now := time.Now()
	status := core.LicenseStatus{
		Valid:        true,
		Tier:         core.TierFree, // Always Free for now
		LoggedIn:     true,
		AuthProvider: v.cached.AuthProvider,
		AccountEmail: v.cached.AccountEmail,
		DisplayName:  v.cached.DisplayName,
		InstanceID:   v.cached.InstanceID,
		LastChecked:  now,
		NextCheck:    now.Add(validationInterval),
		GracePeriod:  7 * 24 * time.Hour,
	}
	if v.cached.AuthProvider == "github" {
		status.GitHubUser = v.cached.GitHubUser
		status.GitHubID = v.cached.GitHubID
		status.RepoAccess = true // GitHub login requests repo scope
	}
	return status
}

// ProviderName returns the internal string key for an AuthProvider.
func ProviderName(p core.AuthProvider) string {
	switch p {
	case core.AuthGitHub:
		return "github"
	case core.AuthGoogle:
		return "google"
	default:
		return "unknown"
	}
}

// ProviderDisplayName returns a human-friendly provider name for UI display.
func ProviderDisplayName(provider string) string {
	switch provider {
	case "github":
		return "GitHub"
	case "google":
		return "Google"
	default:
		return provider
	}
}

func providerScopes(p core.AuthProvider) string {
	switch p {
	case core.AuthGitHub:
		return "user:email,read:user,repo"
	case core.AuthGoogle:
		return "email,profile"
	default:
		return ""
	}
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "linux":
		return execCommand("xdg-open", url)
	case "darwin":
		return execCommand("open", url)
	case "windows":
		return execCommand("rundll32", "url.dll,FileProtocolHandler", url)
	}
	return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
}

// execCommand wraps os/exec for testability.
var execCommand = execCommandDefault

func execCommandDefault(name string, args ...string) error {
	return exec.Command(name, args...).Start()
}
