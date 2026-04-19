// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeProfileFixture creates a minimal valid profile directory for cmd tests.
func writeProfileFixture(t *testing.T, dir, name string) {
	t.Helper()
	profileDir := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(profileDir, 0o755))

	manifest := "apiVersion: siply/v1\nkind: Profile\nmetadata:\n  name: " + name +
		"\n  version: 1.0.0\n  siply_min: 0.1.0\n  description: Test profile\n  author: test\n  license: MIT\n  updated: \"2026-01-01\"\nspec:\n  tier: 1\n  category: profiles\n"
	require.NoError(t, os.WriteFile(filepath.Join(profileDir, "manifest.yaml"), []byte(manifest), 0o600))

	profileYAML := "items:\n  - name: plugin-a\n    version: 0.1.0\n    category: plugins\n    pinned: true\n"
	require.NoError(t, os.WriteFile(filepath.Join(profileDir, "profile.yaml"), []byte(profileYAML), 0o600))
}

func TestProfileList_Empty(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("SIPLY_HOME", tmpHome)

	var buf bytes.Buffer
	cmd := newProfileCmd()
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"list"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No profiles saved.")
}

func TestProfileList_WithProfiles(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("SIPLY_HOME", tmpHome)

	profilesDir := filepath.Join(tmpHome, "profiles")
	require.NoError(t, os.MkdirAll(profilesDir, 0o755))
	writeProfileFixture(t, profilesDir, "ml-workflow")

	var buf bytes.Buffer
	cmd := newProfileCmd()
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"list"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "ml-workflow")
	assert.Contains(t, buf.String(), "global")
}

func TestProfileSave_Success(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("SIPLY_HOME", tmpHome)

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)

	err := executeProfileSave(cmd, "my-profile", "My profile", false)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "my-profile")
	assert.DirExists(t, filepath.Join(tmpHome, "profiles", "my-profile"))
	assert.FileExists(t, filepath.Join(tmpHome, "profiles", "my-profile", "manifest.yaml"))
}

func TestProfileSave_Force(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("SIPLY_HOME", tmpHome)

	// First save
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, executeProfileSave(cmd, "my-profile", "", false))

	// Second save without force should fail
	err := executeProfileSave(cmd, "my-profile", "", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	// With force should succeed
	err = executeProfileSave(cmd, "my-profile", "", true)
	require.NoError(t, err)
}

func TestProfileSave_NameCollision(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("SIPLY_HOME", tmpHome)

	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})

	require.NoError(t, executeProfileSave(cmd, "dup-profile", "", false))

	err := executeProfileSave(cmd, "dup-profile", "", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestProfileSave_InvalidName(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("SIPLY_HOME", tmpHome)

	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})

	invalidNames := []string{"MyProfile", "1profile", "", "-profile", "profile!", strings.Repeat("a", 64)}
	for _, name := range invalidNames {
		err := executeProfileSave(cmd, name, "", false)
		require.Error(t, err, "name=%q", name)
	}
}

func TestProfileInstall_BuiltinMinimal(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("SIPLY_HOME", tmpHome)
	t.Setenv("HOME", tmpHome)

	var buf bytes.Buffer
	cmd := newProfileCmd()
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"install", "minimal"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "minimal")
}

func TestProfileInstall_BuiltinStandard(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("SIPLY_HOME", tmpHome)
	t.Setenv("HOME", tmpHome)

	var buf bytes.Buffer
	cmd := newProfileCmd()
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"install", "standard"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "standard")
}

func TestProfileInstall_MarketplaceNotFound(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("SIPLY_HOME", tmpHome)
	t.Setenv("HOME", tmpHome)

	var buf bytes.Buffer
	cmd := newProfileCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"install", "team-setup"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "team-setup")
}

func TestProfileInstall_YesFlagAccepted(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("SIPLY_HOME", tmpHome)
	t.Setenv("HOME", tmpHome)

	var buf bytes.Buffer
	cmd := newProfileCmd()
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"install", "--yes", "minimal"})

	err := cmd.Execute()
	require.NoError(t, err)
}

func TestProfileInstall_GlobalFlagAccepted(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("SIPLY_HOME", tmpHome)
	t.Setenv("HOME", tmpHome)

	var buf bytes.Buffer
	cmd := newProfileCmd()
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"install", "--global", "standard"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "standard")
}
