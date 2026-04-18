// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/marketplace"
)

// fixtureLoader returns a loader backed by the standard marketplace test fixture.
func fixtureLoader(t *testing.T) func() (*marketplace.Index, error) {
	t.Helper()
	fixturePath := filepath.Join("..", "marketplace", "testdata", "marketplace-index.json")
	return func() (*marketplace.Index, error) {
		return marketplace.LoadIndex(fixturePath)
	}
}

// emptyLoader returns a loader that always returns ErrIndexNotFound.
func emptyLoader() func() (*marketplace.Index, error) {
	return func() (*marketplace.Index, error) {
		return nil, marketplace.ErrIndexNotFound
	}
}

// --- info subcommand tests ---

func TestMarketplaceInfo_Found(t *testing.T) {
	cmd := NewMarketplaceCmdWithLoader(fixtureLoader(t))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"info", "memory-default"})

	err := cmd.Execute()

	require.NoError(t, err)
	output := out.String()
	assert.Contains(t, output, "memory-default")
	assert.Contains(t, output, "plugins")
	assert.Contains(t, output, "simplydevly")
	assert.Contains(t, output, "Apache-2.0")
	assert.Contains(t, output, "README")
}

func TestMarketplaceInfo_NotFound(t *testing.T) {
	cmd := NewMarketplaceCmdWithLoader(fixtureLoader(t))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"info", "nonexistent-item-xyz"})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestMarketplaceInfo_NoIndex(t *testing.T) {
	cmd := NewMarketplaceCmdWithLoader(emptyLoader())
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"info", "memory-default"})

	err := cmd.Execute()

	require.NoError(t, err, "must exit 0 when index unavailable (NFR27)")
	assert.Contains(t, out.String(), "Marketplace index unavailable")
}

func TestMarketplaceInfo_JSON(t *testing.T) {
	cmd := NewMarketplaceCmdWithLoader(fixtureLoader(t))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"info", "memory-default", "--json"})

	err := cmd.Execute()

	require.NoError(t, err)
	var item marketplace.Item
	require.NoError(t, json.Unmarshal(out.Bytes(), &item), "output must be valid JSON")
	assert.Equal(t, "memory-default", item.Name)
	assert.NotNil(t, item.Capabilities, "capabilities must be [] not null in JSON")
}

func TestMarketplaceInfo_JSON_CapabilitiesArray(t *testing.T) {
	// Items with capabilities must output them as a JSON array (not null).
	cmd := NewMarketplaceCmdWithLoader(fixtureLoader(t))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"info", "memory-default", "--json"})

	require.NoError(t, cmd.Execute())

	var item marketplace.Item
	require.NoError(t, json.Unmarshal(out.Bytes(), &item))
	// memory-default has capabilities in the fixture — must be a non-nil slice.
	assert.NotNil(t, item.Capabilities, "capabilities must not be nil when the item has them")
	assert.Contains(t, item.Capabilities, "memory")
}

func TestMarketplaceInfo_ReadmeRendered(t *testing.T) {
	cmd := NewMarketplaceCmdWithLoader(fixtureLoader(t))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"info", "memory-default"})

	require.NoError(t, cmd.Execute())
	output := out.String()
	// memory-default has a readme in the fixture.
	assert.Contains(t, output, "--- README ---")
}

func TestMarketplaceInfo_FallsBackToDescription(t *testing.T) {
	// Item without readme should use description as README body.
	loader := func() (*marketplace.Index, error) {
		return &marketplace.Index{
			Items: []marketplace.Item{
				{Name: "no-readme", Category: "plugins", Description: "Just a description", Readme: ""},
			},
		}, nil
	}
	cmd := NewMarketplaceCmdWithLoader(loader)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"info", "no-readme"})

	require.NoError(t, cmd.Execute())
	assert.Contains(t, out.String(), "Just a description")
}

// --- install subcommand tests ---

// mockInstallerFunc is a test double for marketplace.InstallerFunc.
type mockInstallerFunc struct {
	calledWith string
	returnErr  error
}

func (m *mockInstallerFunc) install(_ context.Context, sourceDir string) error {
	m.calledWith = sourceDir
	return m.returnErr
}

func TestMarketplaceInstall_NoIndex(t *testing.T) {
	mock := &mockInstallerFunc{}
	cmd := NewMarketplaceCmdWithLoaderAndInstaller(emptyLoader(), mock.install)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "memory-default"})

	err := cmd.Execute()

	require.NoError(t, err, "must exit 0 when index unavailable (NFR27)")
	assert.Contains(t, out.String(), "Marketplace index unavailable")
	assert.Empty(t, mock.calledWith, "installer must not be called when index is unavailable")
}

func TestMarketplaceInstall_NotFound(t *testing.T) {
	mock := &mockInstallerFunc{}
	cmd := NewMarketplaceCmdWithLoaderAndInstaller(fixtureLoader(t), mock.install)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "nonexistent-xyz"})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.Empty(t, mock.calledWith)
}

// TestMarketplaceInstall_Incompatible verifies that the compatibility check blocks
// installation and returns the correct error message. Uses an injected versionGetter
// to bypass the "dev always compatible" special-case in GetSiplyVersion.
func TestMarketplaceInstall_Incompatible(t *testing.T) {
	loader := func() (*marketplace.Index, error) {
		return &marketplace.Index{
			Items: []marketplace.Item{
				{
					Name:        "future-plugin",
					Category:    "plugins",
					Description: "Requires a future siply version",
					Version:     "1.0.0",
					SiplyMin:    "99.0.0",
				},
			},
		}, nil
	}
	mock := &mockInstallerFunc{}

	// Build install subcommand directly with an injected version getter so we
	// can exercise the incompatibility path without relying on build ldflags.
	installCmd := newMarketplaceInstallCmd(loader, mock.install, func() string { return "0.1.0" })
	root := &cobra.Command{Use: "marketplace"}
	root.AddCommand(installCmd)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"install", "future-plugin"})

	err := root.Execute()

	require.Error(t, err)
	// F7: assert on stable values (version numbers) rather than wording that
	// may change in FormatIncompatibleMessage.
	assert.Contains(t, err.Error(), "99.0.0", "must mention required siply version")
	assert.Contains(t, err.Error(), "0.1.0", "must mention current siply version")
	assert.Empty(t, mock.calledWith, "installer must not be called for incompatible plugin")
}

// TestMarketplaceInstall_CompatibilityCheckWired verifies the compatibility check
// is wired into the install command. In test environments GetSiplyVersion() returns
// "dev" which is always compatible — incompatibility is therefore tested at the
// plugins.IsCompatible unit level (see internal/plugins/version_test.go).
// This test verifies the happy path: a compatible plugin (SiplyMin <= dev) installs.
func TestMarketplaceInstall_CompatibilityCheckWired(t *testing.T) {
	loader := func() (*marketplace.Index, error) {
		return &marketplace.Index{
			Items: []marketplace.Item{
				{
					Name:        "compat-plugin",
					Category:    "plugins",
					Description: "Compatible plugin",
					Version:     "1.0.0",
					SiplyMin:    "0.1.0",               // satisfied by any real version and "dev"
					DownloadURL: "file:///nonexistent", // will fail at install step
				},
			},
		}, nil
	}
	mock := &mockInstallerFunc{returnErr: errors.New("install step error")}
	cmd := NewMarketplaceCmdWithLoaderAndInstaller(loader, mock.install)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "compat-plugin"})

	err := cmd.Execute()

	// The install step runs (compatibility passed) — error comes from the mock installer.
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "incompatible", "compatible plugin must not trigger incompatibility error")
	assert.Equal(t, "/nonexistent", mock.calledWith, "installer must be called for compatible plugin")
}

func TestMarketplaceInstall_NoDownloadURL(t *testing.T) {
	// Item with no download_url in fixture (git-hooks has empty download_url).
	loader := func() (*marketplace.Index, error) {
		return &marketplace.Index{
			Items: []marketplace.Item{
				{Name: "no-url-item", Category: "plugins", Description: "no url", Version: "1.0.0", SiplyMin: "0.1.0"},
			},
		}, nil
	}
	mock := &mockInstallerFunc{}
	cmd := NewMarketplaceCmdWithLoaderAndInstaller(loader, mock.install)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "no-url-item"})

	err := cmd.Execute()

	require.Error(t, err)
	// P5: advisory is now in the error message (no double Cobra print).
	assert.True(t, errors.Is(err, marketplace.ErrNoDownloadURL), "expected ErrNoDownloadURL, got: %v", err)
	assert.Contains(t, err.Error(), "cannot be installed", "advisory must be in error message")
	assert.Empty(t, mock.calledWith)
}

func TestMarketplaceInstall_NilInstaller(t *testing.T) {
	cmd := NewMarketplaceCmdWithLoaderAndInstaller(fixtureLoader(t), nil)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "memory-default"})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unavailable")
}

func TestMarketplaceInstall_Success(t *testing.T) {
	// Point download_url at the real memory-default plugin directory.
	repoRoot := findRepoRoot(t)
	pluginDir := filepath.Join(repoRoot, "plugins", "memory-default")
	if _, err := os.Stat(pluginDir); err != nil {
		t.Skipf("memory-default plugin not found at %s", pluginDir)
	}

	loader := func() (*marketplace.Index, error) {
		return &marketplace.Index{
			Items: []marketplace.Item{
				{
					Name:        "memory-default",
					Category:    "plugins",
					Description: "test",
					Version:     "1.2.0",
					SiplyMin:    "0.1.0",
					DownloadURL: "file://" + pluginDir,
				},
			},
		}, nil
	}
	mock := &mockInstallerFunc{}
	cmd := NewMarketplaceCmdWithLoaderAndInstaller(loader, mock.install)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "memory-default"})

	err := cmd.Execute()

	require.NoError(t, err)
	output := out.String()
	assert.Contains(t, output, "✅ Installed")
	assert.Contains(t, output, "memory-default")
	assert.True(t, strings.HasSuffix(mock.calledWith, "memory-default") ||
		mock.calledWith == pluginDir,
		"installer must be called with the plugin dir path, got: %s", mock.calledWith)
}

// --- update subcommand tests ---

func TestMarketplaceUpdate_BundleAllComponentsFound(t *testing.T) {
	// fullstack-starter has 3 components: memory-default, code-review-skill, golang-defaults —
	// all present in the fixture → expects "Updated 3/3 items in bundle".
	cmd := NewMarketplaceCmdWithLoader(fixtureLoader(t))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"update", "fullstack-starter"})

	err := cmd.Execute()

	require.NoError(t, err)
	output := out.String()
	assert.Contains(t, output, "Updated 3/3 items in bundle")
	assert.Contains(t, output, "fullstack-starter")
}

func TestMarketplaceUpdate_BundleComponentNotFound(t *testing.T) {
	// Bundle where one component is missing from the index → partial count.
	loader := func() (*marketplace.Index, error) {
		return &marketplace.Index{
			Items: []marketplace.Item{
				{
					Name:       "partial-bundle",
					Category:   "bundles",
					Components: []marketplace.BundleComponent{{Name: "memory-default", Version: "1.2.0"}, {Name: "nonexistent-comp", Version: "0.1.0"}},
				},
				{Name: "memory-default", Category: "plugins", Version: "1.2.0"},
			},
		}, nil
	}
	cmd := NewMarketplaceCmdWithLoader(loader)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"update", "partial-bundle"})

	err := cmd.Execute()

	require.NoError(t, err)
	output := out.String()
	// Only 1 of 2 components found → Updated 1/2.
	assert.Contains(t, output, "Updated 1/2 items in bundle")
	assert.Contains(t, output, "nonexistent-comp")
}

func TestMarketplaceUpdate_NonBundle(t *testing.T) {
	// Regular plugin (not a bundle) → "Update command coming in a future release".
	cmd := NewMarketplaceCmdWithLoader(fixtureLoader(t))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"update", "memory-default"})

	err := cmd.Execute()

	require.NoError(t, err)
	output := out.String()
	assert.Contains(t, output, "Update command coming in a future release")
	assert.Contains(t, output, "memory-default")
}

func TestMarketplaceUpdate_NotFound(t *testing.T) {
	// Unknown item name → ErrItemNotFound.
	cmd := NewMarketplaceCmdWithLoader(fixtureLoader(t))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"update", "does-not-exist-xyz"})

	err := cmd.Execute()

	require.Error(t, err)
	assert.True(t, errors.Is(err, marketplace.ErrItemNotFound), "expected ErrItemNotFound, got: %v", err)
}

func TestMarketplaceUpdate_NoIndex(t *testing.T) {
	// No index available → advisory message, exit 0.
	cmd := NewMarketplaceCmdWithLoader(emptyLoader())
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"update", "memory-default"})

	err := cmd.Execute()

	require.NoError(t, err, "must exit 0 when index unavailable (NFR27)")
	assert.Contains(t, out.String(), "Marketplace index unavailable")
}

// --- bundle info subcommand tests ---

func TestMarketplaceInfo_BundleShowsContents(t *testing.T) {
	// Bundle item → output must include "Bundle Contents:" and component lines.
	cmd := NewMarketplaceCmdWithLoader(fixtureLoader(t))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"info", "fullstack-starter"})

	err := cmd.Execute()

	require.NoError(t, err)
	output := out.String()
	assert.Contains(t, output, "Bundle Contents:")
	assert.Contains(t, output, "memory-default")
}

func TestMarketplaceInfo_BundleJSON_IncludesComponents(t *testing.T) {
	// Bundle with --json flag → "components" array must be non-empty.
	cmd := NewMarketplaceCmdWithLoader(fixtureLoader(t))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"info", "fullstack-starter", "--json"})

	err := cmd.Execute()

	require.NoError(t, err)
	var item marketplace.Item
	require.NoError(t, json.Unmarshal(out.Bytes(), &item), "output must be valid JSON")
	assert.NotEmpty(t, item.Components, "components array must be non-empty for bundle items")
}

func TestMarketplaceInfo_NonBundleNoContentsSection(t *testing.T) {
	// Regular plugin → output must NOT contain "Bundle Contents:".
	cmd := NewMarketplaceCmdWithLoader(fixtureLoader(t))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"info", "memory-default"})

	err := cmd.Execute()

	require.NoError(t, err)
	assert.NotContains(t, out.String(), "Bundle Contents:")
}

// findRepoRoot walks up from the current directory to find the go.mod for siply.dev/siply.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		gomod := filepath.Join(dir, "go.mod")
		if data, err := os.ReadFile(gomod); err == nil {
			if strings.Contains(string(data), "siply.dev/siply") {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root")
		}
		dir = parent
	}
}
