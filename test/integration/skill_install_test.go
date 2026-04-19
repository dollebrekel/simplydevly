// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/marketplace"
	"siply.dev/siply/internal/plugins"
)

// skillFixtureDir returns the absolute path to the prompt-basic plugin (used as skill fixture).
func skillFixtureDir(t *testing.T) string {
	t.Helper()
	rel := filepath.Join("..", "..", "plugins", "prompt-basic")
	abs, err := filepath.Abs(rel)
	if err != nil {
		t.Skipf("cannot resolve prompt-basic path: %v", err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Skipf("prompt-basic fixture not found at %s: %v", abs, err)
	}
	return abs
}

// TestSkillInstall_RoutesToSkillsDir verifies that a skill-category item is
// installed to the skills dir, not the plugins dir (AC#1).
func TestSkillInstall_RoutesToSkillsDir(t *testing.T) {
	skillsDir := t.TempDir()
	pluginsDir := t.TempDir()

	fixtureDir := skillFixtureDir(t)

	var skillsCalled bool
	skillsInstaller := func(_ context.Context, sourceDir string) error {
		skillsCalled = true
		// Copy into a named subdir within skillsDir.
		m, err := plugins.LoadManifestFromDir(sourceDir)
		if err != nil {
			return err
		}
		destDir := filepath.Join(skillsDir, m.Metadata.Name)
		return os.MkdirAll(destDir, 0755)
	}

	var pluginsCalled bool
	pluginsInstaller := func(_ context.Context, _ string) error {
		pluginsCalled = true
		return nil
	}

	item := marketplace.Item{
		Name:        "prompt-basic",
		Category:    "skills",
		DownloadURL: "file://" + fixtureDir,
	}

	err := marketplace.Install(context.Background(), item, pluginsInstaller, skillsInstaller)
	require.NoError(t, err)

	assert.True(t, skillsCalled, "skills installer must be called for skill-category item")
	assert.False(t, pluginsCalled, "plugins installer must NOT be called for skill-category item")
	_ = pluginsDir
}

// TestPluginInstall_RoutesToPluginsDir verifies that a non-skill item continues
// to use the plugins installer (AC#1 — routing only for skills).
func TestPluginInstall_RoutesToPluginsDir(t *testing.T) {
	rel2 := filepath.Join("..", "..", "plugins", "memory-default")
	fixtureDir, absErr := filepath.Abs(rel2)
	if absErr != nil {
		t.Skipf("cannot resolve memory-default path: %v", absErr)
	}
	if _, statErr := os.Stat(fixtureDir); statErr != nil {
		t.Skipf("memory-default fixture not found: %v", statErr)
	}

	var pluginsCalled bool
	pluginsInstaller := func(_ context.Context, _ string) error {
		pluginsCalled = true
		return nil
	}

	var skillsCalled bool
	skillsInstaller := func(_ context.Context, _ string) error {
		skillsCalled = true
		return nil
	}

	item := marketplace.Item{
		Name:        "memory-default",
		Category:    "plugins",
		DownloadURL: "file://" + fixtureDir,
	}

	err := marketplace.Install(context.Background(), item, pluginsInstaller, skillsInstaller)
	require.NoError(t, err)

	assert.True(t, pluginsCalled, "plugins installer must be called for plugin-category item")
	assert.False(t, skillsCalled, "skills installer must NOT be called for plugin-category item")
}

// TestSkillInstall_NoSkillsInstaller falls back to registryInstall when no skills installer provided.
func TestSkillInstall_NoSkillsInstaller(t *testing.T) {
	fixtureDir := skillFixtureDir(t)

	var registryCalled bool
	registryInstaller := func(_ context.Context, _ string) error {
		registryCalled = true
		return nil
	}

	item := marketplace.Item{
		Name:        "prompt-basic",
		Category:    "skills",
		DownloadURL: "file://" + fixtureDir,
	}

	// No skillsInstall passed — should fall back to registryInstall.
	err := marketplace.Install(context.Background(), item, registryInstaller)
	require.NoError(t, err)
	assert.True(t, registryCalled, "registryInstall must be used as fallback when no skills installer given")
}
