// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

/*
Package marketplace provides types and functions for browsing and searching the
Simply Devly marketplace index.

The index is a JSON file cached locally at ~/.siply/cache/marketplace-index.json
and populated by `siply marketplace sync` (a future story). This package makes
no network calls — all operations are read-only against the local cache.

Index format (version 1):

	{
	  "version": 1,
	  "updated_at": "<RFC3339>",
	  "items": [
	    {
	      "name": "memory-default",
	      "category": "plugins",
	      "description": "...",
	      "author": "simplydevly",
	      "version": "1.0.0",
	      "rating": 4.8,
	      "install_count": 12500,
	      "verified": true,
	      "tags": ["memory", "context"],
	      "siply_min": "0.1.0",
	      "license": "Apache-2.0",
	      "updated_at": "2026-03-15T00:00:00Z"
	    }
	  ]
	}

Valid categories: plugins, extensions, skills, configs, bundles.

Seeding for local development:

	make marketplace-seed   (runs scripts/seed-marketplace-index.sh)
*/
package marketplace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// BundleComponent represents a single component within a bundle.
type BundleComponent struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Item represents a single marketplace entry.
type Item struct {
	Name         string   `json:"name"`
	Category     string   `json:"category"` // one of: plugins, extensions, skills, configs, bundles
	Description  string   `json:"description"`
	Author       string   `json:"author"`
	Version      string   `json:"version"`
	Rating       float64  `json:"rating"` // 0.0–5.0
	InstallCount int64    `json:"install_count"`
	Verified     bool     `json:"verified"`
	Tags         []string `json:"tags"`
	SiplyMin     string   `json:"siply_min"`
	License      string   `json:"license"`
	UpdatedAt    string   `json:"updated_at"`
	// Fields added in Story 9.2:
	Readme       string            `json:"readme,omitempty"`       // Full README markdown text
	Homepage     string            `json:"homepage,omitempty"`     // Web URL
	DownloadURL  string            `json:"download_url,omitempty"` // Tarball/zip URL; file:// for local
	SHA256       string            `json:"sha256,omitempty"`       // Hex SHA256 of download archive
	Capabilities []string          `json:"capabilities,omitempty"` // e.g. ["memory", "filesystem"]
	RatingCount  int               `json:"rating_count"`
	ReviewCount  int               `json:"review_count"`
	Components   []BundleComponent `json:"components,omitempty"`
}

// Index is the top-level marketplace index.
type Index struct {
	Version   int    `json:"version"`
	UpdatedAt string `json:"updated_at"`
	Items     []Item `json:"items"`
}

// Sentinel errors for marketplace operations.
var (
	ErrIndexNotFound               = errors.New("marketplace: index not found")
	ErrInvalidCategory             = errors.New("marketplace: invalid category")
	ErrItemNotFound                = errors.New("marketplace: item not found")
	ErrNoDownloadURL               = errors.New("marketplace: item has no download URL")
	ErrChecksumMismatch            = errors.New("marketplace: checksum mismatch")
	ErrBundleComponentNotFound     = errors.New("bundle component not found in marketplace index")
	ErrBundleComponentIncompatible = errors.New("bundle component incompatible with current siply version")
	ErrBundleEmptyComponents       = errors.New("bundle has no components")
)

// LoadIndex reads and parses the marketplace index JSON from the given path.
// Returns ErrIndexNotFound if the file does not exist or cannot be opened.
func LoadIndex(path string) (*Index, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrIndexNotFound, path)
		}
		return nil, fmt.Errorf("marketplace: open index: %w", err)
	}
	defer f.Close()

	var idx Index
	dec := json.NewDecoder(f)
	if err := dec.Decode(&idx); err != nil {
		return nil, fmt.Errorf("marketplace: parse index: %w", err)
	}
	return &idx, nil
}
