// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package core

import (
	"context"
	"time"
)

// LicenseValidator handles account-based licensing: OAuth activation, periodic validation, offline grace.
type LicenseValidator interface {
	Lifecycle
	// Login authenticates via OAuth provider (GitHub or Google).
	Login(ctx context.Context, provider AuthProvider) (LicenseStatus, error)
	// Logout removes account from this machine.
	Logout() error
	// Validate checks cached status (no network call, O(1)).
	Validate() LicenseStatus
	// Refresh forces online check against simply-market API.
	Refresh(ctx context.Context) (LicenseStatus, error)
	// ActivatePro starts Stripe checkout flow (account must be logged in).
	ActivatePro(ctx context.Context) (LicenseStatus, error)
	// DeactivatePro removes Pro license from this machine.
	DeactivatePro() error
	// DiscoverRepos matches GitHub repos against local git remotes (requires GitHub + repo scope).
	DiscoverRepos(ctx context.Context) ([]DiscoveredRepo, error)
}

// AuthProvider identifies the OAuth authentication provider.
type AuthProvider int

const (
	AuthGitHub AuthProvider = iota // Recommended — enables repo auto-setup
	AuthGoogle                     // Universal fallback
)

// LicenseStatus holds the current license state.
type LicenseStatus struct {
	Valid          bool          `json:"valid"`
	Tier           FeatureTier   `json:"tier"`
	LoggedIn       bool          `json:"logged_in"`
	AuthProvider   string        `json:"auth_provider,omitempty"`
	AccountEmail   string        `json:"account_email,omitempty"`
	DisplayName    string        `json:"display_name,omitempty"`
	GitHubUser     string        `json:"github_user,omitempty"`
	GitHubID       int64         `json:"github_id,omitempty"`
	RepoAccess     bool          `json:"repo_access"`
	TokenExpiresAt time.Time     `json:"token_expires_at,omitzero"`
	ExpiresAt      time.Time     `json:"expires_at,omitzero"`
	LastChecked    time.Time     `json:"last_checked,omitzero"`
	NextCheck      time.Time     `json:"next_check,omitzero"`
	OfflineSince   time.Time     `json:"offline_since,omitzero"`
	GracePeriod    time.Duration `json:"-"`
	InGrace        bool          `json:"in_grace"`
	InstanceID     string        `json:"instance_id,omitempty"`
}

// DiscoveredRepo represents a GitHub repo matched against a local git remote.
type DiscoveredRepo struct {
	GitHubFullName string // "user/myapp"
	LocalPath      string // "~/projects/myapp"
	Language       string // "Go", "Python", "Rust"
	LinesOfCode    int    // Approximate, from GitHub API
	HasSiplyConfig bool   // true if .siply/ already exists
}
