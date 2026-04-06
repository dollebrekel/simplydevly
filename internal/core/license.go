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
	Valid        bool
	Tier         FeatureTier
	LoggedIn     bool          // true if user has authenticated (Free or Pro)
	AuthProvider string        // "github", "google"
	AccountEmail string        // From OAuth provider
	DisplayName  string        // From OAuth provider
	GitHubUser   string        // GitHub username (only when AuthProvider=github)
	GitHubID     int64         // GitHub user ID (stable identifier, only github)
	RepoAccess   bool          // true if user granted GitHub repo scope
	ExpiresAt    time.Time     // Pro subscription period end
	LastChecked  time.Time     // Last successful validation
	NextCheck    time.Time     // Next mandatory check (LastChecked + 5 days)
	OfflineSince time.Time     // When offline started (zero = online)
	GracePeriod  time.Duration // 7 days default (FR93)
	InGrace      bool          // true = offline but within grace
	InstanceID   string        // Random UUID per installation (for concurrent machine limit)
}

// DiscoveredRepo represents a GitHub repo matched against a local git remote.
type DiscoveredRepo struct {
	GitHubFullName string // "user/myapp"
	LocalPath      string // "~/projects/myapp"
	Language       string // "Go", "Python", "Rust"
	LinesOfCode    int    // Approximate, from GitHub API
	HasSiplyConfig bool   // true if .siply/ already exists
}
