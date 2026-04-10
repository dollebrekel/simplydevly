// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package plugins

import (
	"fmt"
	"runtime/debug"
	"strings"

	"golang.org/x/mod/semver"
)

// CompareVersions compares two semver strings (with or without "v" prefix).
// Returns -1 if a < b, 0 if a == b, +1 if a > b.
// Invalid versions compare as equal (0).
func CompareVersions(a, b string) int {
	return semver.Compare(normalizeVersion(a), normalizeVersion(b))
}

// IsCompatible checks whether currentVersion satisfies the plugin's minimum
// requirement (siplyMin). Returns true if currentVersion >= siplyMin.
// A "dev" currentVersion is always considered compatible.
func IsCompatible(siplyMin, currentVersion string) bool {
	if currentVersion == "dev" || currentVersion == "" {
		return true
	}
	if siplyMin == "" {
		return true
	}
	return semver.Compare(normalizeVersion(currentVersion), normalizeVersion(siplyMin)) >= 0
}

// FormatIncompatibleMessage generates a user-facing incompatibility message.
// Format matches AC7: "Plugin X v1.2 incompatible with siply v2.1. Requires siply >=1.8"
func FormatIncompatibleMessage(pluginName, pluginVersion, siplyVersion, siplyMin string) string {
	return fmt.Sprintf(
		"Plugin %s v%s incompatible with siply v%s. Requires siply >=%s",
		pluginName,
		strings.TrimPrefix(pluginVersion, "v"),
		strings.TrimPrefix(siplyVersion, "v"),
		strings.TrimPrefix(siplyMin, "v"),
	)
}

// normalizeVersion ensures a version string has a "v" prefix as required by
// golang.org/x/mod/semver.
func normalizeVersion(v string) string {
	if v == "" {
		return ""
	}
	if !strings.HasPrefix(v, "v") {
		return "v" + v
	}
	return v
}

// GetSiplyVersion returns the current siply binary version.
// It reads from build info (set by goreleaser ldflags) or falls back to "dev".
func GetSiplyVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	// The main module version is set by go build -ldflags or go install.
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		return strings.TrimPrefix(info.Main.Version, "v")
	}
	return "dev"
}
