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

// testRootCmd creates a root command with persistent flags matching main.go.
func testRootCmd() *cobra.Command {
	root := &cobra.Command{Use: "siply"}
	root.PersistentFlags().Bool("no-color", false, "")
	root.PersistentFlags().Bool("no-emoji", false, "")
	root.PersistentFlags().Bool("no-borders", false, "")
	root.PersistentFlags().Bool("no-motion", false, "")
	root.PersistentFlags().Bool("accessible", false, "")
	root.PersistentFlags().Bool("low-bandwidth", false, "")
	root.PersistentFlags().Bool("minimal", false, "")
	root.PersistentFlags().Bool("standard", false, "")
	root.PersistentFlags().Bool("offline", false, "")
	root.PersistentFlags().String("model", "", "")
	root.SilenceUsage = true
	root.SilenceErrors = true
	return root
}

func TestParseTUIFlags_MinimalAndStandardMutuallyExclusive(t *testing.T) {
	root := testRootCmd()
	tuiCmd := &cobra.Command{
		Use: "tui",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := parseTUIFlags(cmd)
			return err
		},
	}
	root.AddCommand(tuiCmd)
	root.SetArgs([]string{"tui", "--minimal", "--standard"})

	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot use --minimal and --standard together")
}

func TestParseTUIFlags_MinimalOnly(t *testing.T) {
	root := testRootCmd()
	var gotMinimal, gotStandard bool
	tuiCmd := &cobra.Command{
		Use: "tui",
		RunE: func(cmd *cobra.Command, _ []string) error {
			flags, err := parseTUIFlags(cmd)
			if err != nil {
				return err
			}
			gotMinimal = flags.Minimal
			gotStandard = flags.Standard
			return nil
		},
	}
	root.AddCommand(tuiCmd)
	root.SetArgs([]string{"tui", "--minimal"})

	err := root.Execute()
	require.NoError(t, err)
	assert.True(t, gotMinimal)
	assert.False(t, gotStandard)
}

func TestParseTUIFlags_StandardOnly(t *testing.T) {
	root := testRootCmd()
	var gotMinimal, gotStandard bool
	tuiCmd := &cobra.Command{
		Use: "tui",
		RunE: func(cmd *cobra.Command, _ []string) error {
			flags, err := parseTUIFlags(cmd)
			if err != nil {
				return err
			}
			gotMinimal = flags.Minimal
			gotStandard = flags.Standard
			return nil
		},
	}
	root.AddCommand(tuiCmd)
	root.SetArgs([]string{"tui", "--standard"})

	err := root.Execute()
	require.NoError(t, err)
	assert.False(t, gotMinimal)
	assert.True(t, gotStandard)
}

func TestPromptProfile_ChooseMinimal(t *testing.T) {
	r := strings.NewReader("1\n")
	w := &bytes.Buffer{}
	profile, err := promptProfile(r, w)
	require.NoError(t, err)
	assert.Equal(t, "minimal", profile)
	assert.Contains(t, w.String(), "Choose default layout:")
}

func TestPromptProfile_ChooseStandard(t *testing.T) {
	r := strings.NewReader("2\n")
	w := &bytes.Buffer{}
	profile, err := promptProfile(r, w)
	require.NoError(t, err)
	assert.Equal(t, "standard", profile)
}

func TestPromptProfile_InvalidInputDefaultsToStandard(t *testing.T) {
	r := strings.NewReader("x\n")
	w := &bytes.Buffer{}
	profile, err := promptProfile(r, w)
	require.NoError(t, err)
	assert.Equal(t, "standard", profile) // safe default
}

func TestPromptProfile_EmptyInputDefaultsToStandard(t *testing.T) {
	r := strings.NewReader("\n")
	w := &bytes.Buffer{}
	profile, err := promptProfile(r, w)
	require.NoError(t, err)
	assert.Equal(t, "standard", profile) // safe default
}

func TestSaveAndLoadProfileConfig(t *testing.T) {
	// Use temp dir as home to avoid touching real config.
	dir := t.TempDir()
	siplyDir := filepath.Join(dir, ".siply")
	require.NoError(t, os.MkdirAll(siplyDir, 0o700))

	configPath := filepath.Join(siplyDir, "config.yaml")

	// Write a profile.
	t.Setenv("HOME", dir)
	require.NoError(t, saveProfileToConfig("minimal"))

	// Verify file exists.
	_, err := os.Stat(configPath)
	require.NoError(t, err)

	// Read it back.
	profile, err := loadProfileFromConfig()
	require.NoError(t, err)
	assert.Equal(t, "minimal", profile)
}

func TestLoadProfileFromConfig_NoFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	profile, err := loadProfileFromConfig()
	assert.Error(t, err) // file not found
	assert.Equal(t, "", profile)
}

func TestSaveProfileToConfig_PreservesExistingFields(t *testing.T) {
	dir := t.TempDir()
	siplyDir := filepath.Join(dir, ".siply")
	require.NoError(t, os.MkdirAll(siplyDir, 0o700))

	configPath := filepath.Join(siplyDir, "config.yaml")
	existing := []byte("provider:\n  default: anthropic\n  model: claude-opus\n")
	require.NoError(t, os.WriteFile(configPath, existing, 0o600))

	t.Setenv("HOME", dir)
	require.NoError(t, saveProfileToConfig("standard"))

	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "profile: standard")
	// The existing provider field should be preserved in Extra.
	assert.Contains(t, content, "provider")
}

func TestParseTUIFlags_NeitherMinimalNorStandard(t *testing.T) {
	root := testRootCmd()
	var gotMinimal, gotStandard bool
	tuiCmd := &cobra.Command{
		Use: "tui",
		RunE: func(cmd *cobra.Command, _ []string) error {
			flags, err := parseTUIFlags(cmd)
			if err != nil {
				return err
			}
			gotMinimal = flags.Minimal
			gotStandard = flags.Standard
			return nil
		},
	}
	root.AddCommand(tuiCmd)
	root.SetArgs([]string{"tui"})

	err := root.Execute()
	require.NoError(t, err)
	assert.False(t, gotMinimal)
	assert.False(t, gotStandard)
}
