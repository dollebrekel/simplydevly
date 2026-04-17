// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package marketplace

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"siply.dev/siply/internal/plugins"
)

const maxReadmeSize = 1 << 20 // 1MB

// PrePublishResult contains validated data for publishing.
type PrePublishResult struct {
	Manifest *plugins.Manifest
	Readme   string
	Warnings []string
}

// ValidateForPublish runs all pre-publish checks on a plugin directory.
func ValidateForPublish(dir string) (*PrePublishResult, error) {
	manifest, err := plugins.LoadManifestFromDir(dir)
	if err != nil {
		return nil, err
	}

	// Bundle-specific: auto-set category to "bundles".
	if manifest.Kind == "Bundle" {
		manifest.Spec.Category = "bundles"
	}

	readmePath := filepath.Join(dir, "README.md")
	readmeFile, err := os.Open(readmePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("marketplace: README.md is required for publishing")
		}
		return nil, fmt.Errorf("marketplace: open README.md: %w", err)
	}
	defer readmeFile.Close()

	readmeData, err := io.ReadAll(io.LimitReader(readmeFile, maxReadmeSize+1))
	if err != nil {
		return nil, fmt.Errorf("marketplace: read README.md: %w", err)
	}
	if int64(len(readmeData)) > maxReadmeSize {
		return nil, fmt.Errorf("marketplace: README.md exceeds 1MB limit")
	}

	var warnings []string

	if _, err := os.Stat(filepath.Join(dir, "CHANGELOG.md")); os.IsNotExist(err) {
		warnings = append(warnings, "No CHANGELOG.md found — recommended for version history")
	}
	if _, err := os.Stat(filepath.Join(dir, "LICENSE")); os.IsNotExist(err) {
		warnings = append(warnings, "No LICENSE file found — license only declared in manifest")
	}

	today := time.Now().Format("2006-01-02")
	if manifest.Metadata.Updated == "" || manifest.Metadata.Updated < today {
		manifest.Metadata.Updated = today
	}

	return &PrePublishResult{
		Manifest: manifest,
		Readme:   string(readmeData),
		Warnings: warnings,
	}, nil
}
