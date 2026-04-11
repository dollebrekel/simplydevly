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
// Returns an error if either version string is not valid semver.
func CompareVersions(a, b string) (int, error) {
	na := normalizeVersion(a)
	nb := normalizeVersion(b)
	if a != "" && !semver.IsValid(na) {
		return 0, fmt.Errorf("plugins: invalid semver %q", a)
	}
	if b != "" && !semver.IsValid(nb) {
		return 0, fmt.Errorf("plugins: invalid semver %q", b)
	}
	return semver.Compare(na, nb), nil
}

// IsCompatible checks whether currentVersion satisfies the plugin's minimum
// requirement (siplyMin). Returns true if currentVersion >= siplyMin.
// A "dev" currentVersion is always considered compatible.
// Invalid semver strings are treated as incompatible (returns false).
func IsCompatible(siplyMin, currentVersion string) bool {
	if currentVersion == "dev" || currentVersion == "" {
		return true
	}
	if siplyMin == "" {
		return true
	}
	nCurrent := normalizeVersion(currentVersion)
	nMin := normalizeVersion(siplyMin)
	if !semver.IsValid(nCurrent) || !semver.IsValid(nMin) {
		return false
	}
	return semver.Compare(nCurrent, nMin) >= 0
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
