// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

// Package profiles provides profile loading, saving, and sharing.
// Profiles are Tier 1 YAML plugins that capture a complete workspace snapshot
// (installed items + config settings) as a reproducible, shareable artifact.
package profiles

import "siply.dev/siply/internal/core"

// Profile represents a loaded profile with its metadata and workspace snapshot.
type Profile struct {
	Name        string
	Version     string
	Description string
	Items       []ProfileItem
	Config      *core.Config
	Source      string // "global" or "project"
	Dir         string
}

// ProfileItem represents a single installed item captured in the profile snapshot.
type ProfileItem struct {
	Name     string
	Version  string
	Category string // one of: "plugins", "skills", "agents"
	Pinned   bool
}
