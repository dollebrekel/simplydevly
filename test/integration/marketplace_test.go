// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package integration

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/commands"
)

// fixtureIndexPath returns the path to the marketplace fixture index used by unit tests.
func fixtureIndexPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "internal", "marketplace", "testdata", "marketplace-index.json")
}

// seedIndex copies the fixture index to a new temp cache dir and returns the dir path.
func seedIndex(t *testing.T) string {
	t.Helper()
	cacheDir := t.TempDir()
	data, err := os.ReadFile(fixtureIndexPath(t))
	require.NoError(t, err, "fixture index must be readable")
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, "marketplace-index.json"), data, 0600))
	return cacheDir
}

// TestMarketplaceList_NoIndex_ExitsZero verifies NFR27: when the marketplace
// index is absent, `siply marketplace list` exits with code 0 and prints the
// advisory message. Local plugin functionality must be unaffected.
func TestMarketplaceList_NoIndex_ExitsZero(t *testing.T) {
	// Empty temp dir — no index present.
	cacheDir := t.TempDir()
	cmd := commands.NewMarketplaceCmdWithLoader(commands.NewLocalIndexLoader(cacheDir))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	cmd.SetArgs([]string{"list"})
	err := cmd.Execute()

	require.NoError(t, err, "marketplace list must exit 0 when index is absent (NFR27)")
	assert.Contains(t, out.String(), "Marketplace index unavailable",
		"advisory message must be printed when index is missing")
	assert.Contains(t, out.String(), "siply marketplace sync",
		"advisory must mention sync command")
}

// TestMarketplaceSearch_NoIndex_ExitsZero verifies NFR27 for the search subcommand.
func TestMarketplaceSearch_NoIndex_ExitsZero(t *testing.T) {
	cacheDir := t.TempDir()
	cmd := commands.NewMarketplaceCmdWithLoader(commands.NewLocalIndexLoader(cacheDir))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	cmd.SetArgs([]string{"search", "memory"})
	err := cmd.Execute()

	require.NoError(t, err, "marketplace search must exit 0 when index is absent (NFR27)")
	assert.Contains(t, out.String(), "Marketplace index unavailable")
}

// TestMarketplaceInfo_NoIndex verifies NFR27 for the info subcommand:
// `siply marketplace info <name>` exits 0 when no index is present.
func TestMarketplaceInfo_NoIndex(t *testing.T) {
	cacheDir := t.TempDir()
	cmd := commands.NewMarketplaceCmdWithLoader(commands.NewLocalIndexLoader(cacheDir))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	cmd.SetArgs([]string{"info", "memory-default"})
	err := cmd.Execute()

	require.NoError(t, err, "marketplace info must exit 0 when index is absent (NFR27)")
	assert.Contains(t, out.String(), "Marketplace index unavailable")
}

// TestMarketplaceInstall_NoIndex verifies NFR27 for the install subcommand:
// `siply marketplace install <name>` exits 0 when no index is present.
func TestMarketplaceInstall_NoIndex(t *testing.T) {
	cacheDir := t.TempDir()
	cmd := commands.NewMarketplaceCmdWithLoader(commands.NewLocalIndexLoader(cacheDir))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	cmd.SetArgs([]string{"install", "memory-default"})
	err := cmd.Execute()

	require.NoError(t, err, "marketplace install must exit 0 when index is absent (NFR27)")
	assert.Contains(t, out.String(), "Marketplace index unavailable")
}

// TestMarketplaceList_InvalidCategory_ReturnsError verifies AC3: an invalid
// category produces an actionable error message listing all valid options.
func TestMarketplaceList_InvalidCategory_ReturnsError(t *testing.T) {
	cacheDir := seedIndex(t)
	cmd := commands.NewMarketplaceCmdWithLoader(commands.NewLocalIndexLoader(cacheDir))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	cmd.SetArgs([]string{"list", "--category", "themes"})
	err := cmd.Execute()

	require.Error(t, err, "invalid category must return an error (AC3)")
	errMsg := err.Error()
	for _, c := range []string{"plugins", "extensions", "skills", "configs", "bundles"} {
		assert.Contains(t, errMsg, c, "error message must list valid category %q", c)
	}
}
