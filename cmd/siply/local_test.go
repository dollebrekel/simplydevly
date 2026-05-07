// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTUIFlags_LocalFlag(t *testing.T) {
	root := testRootCmd()
	var gotLocal bool
	tuiCmd := &cobra.Command{
		Use: "tui",
		RunE: func(cmd *cobra.Command, _ []string) error {
			flags, err := parseTUIFlags(cmd)
			if err != nil {
				return err
			}
			gotLocal = flags.Local
			return nil
		},
	}
	root.AddCommand(tuiCmd)
	root.SetArgs([]string{"tui", "--local"})

	err := root.Execute()
	require.NoError(t, err)
	assert.True(t, gotLocal)
}

func TestParseTUIFlags_LocalEnvVar(t *testing.T) {
	t.Setenv("SIPLY_LOCAL", "1")

	root := testRootCmd()
	var gotLocal bool
	tuiCmd := &cobra.Command{
		Use: "tui",
		RunE: func(cmd *cobra.Command, _ []string) error {
			flags, err := parseTUIFlags(cmd)
			if err != nil {
				return err
			}
			gotLocal = flags.Local
			return nil
		},
	}
	root.AddCommand(tuiCmd)
	root.SetArgs([]string{"tui"})

	err := root.Execute()
	require.NoError(t, err)
	assert.True(t, gotLocal)
}

func TestParseTUIFlags_LocalEnvVarTrue(t *testing.T) {
	t.Setenv("SIPLY_LOCAL", "true")

	root := testRootCmd()
	var gotLocal bool
	tuiCmd := &cobra.Command{
		Use: "tui",
		RunE: func(cmd *cobra.Command, _ []string) error {
			flags, err := parseTUIFlags(cmd)
			if err != nil {
				return err
			}
			gotLocal = flags.Local
			return nil
		},
	}
	root.AddCommand(tuiCmd)
	root.SetArgs([]string{"tui"})

	err := root.Execute()
	require.NoError(t, err)
	assert.True(t, gotLocal)
}

func TestParseTUIFlags_NoLocalByDefault(t *testing.T) {
	t.Setenv("SIPLY_LOCAL", "")

	root := testRootCmd()
	var gotLocal bool
	tuiCmd := &cobra.Command{
		Use: "tui",
		RunE: func(cmd *cobra.Command, _ []string) error {
			flags, err := parseTUIFlags(cmd)
			if err != nil {
				return err
			}
			gotLocal = flags.Local
			return nil
		},
	}
	root.AddCommand(tuiCmd)
	root.SetArgs([]string{"tui"})

	err := root.Execute()
	require.NoError(t, err)
	assert.False(t, gotLocal)
}

func TestParseTUIFlags_ModelOverride(t *testing.T) {
	root := testRootCmd()
	var gotModel string
	tuiCmd := &cobra.Command{
		Use: "tui",
		RunE: func(cmd *cobra.Command, _ []string) error {
			flags, err := parseTUIFlags(cmd)
			if err != nil {
				return err
			}
			gotModel = flags.ModelOverride
			return nil
		},
	}
	root.AddCommand(tuiCmd)
	root.SetArgs([]string{"tui", "--local", "--model", "codellama:13b"})

	err := root.Execute()
	require.NoError(t, err)
	assert.Equal(t, "codellama:13b", gotModel)
}

func TestIsLocalMode_Flag(t *testing.T) {
	root := testRootCmd()
	root.SetArgs([]string{"--local"})
	err := root.Execute()
	require.NoError(t, err)

	assert.True(t, isLocalMode(root))
}

func TestIsLocalMode_EnvVar(t *testing.T) {
	t.Setenv("SIPLY_LOCAL", "1")

	root := testRootCmd()
	root.SetArgs([]string{})
	err := root.Execute()
	require.NoError(t, err)

	assert.True(t, isLocalMode(root))
}

func TestIsLocalMode_EnvVarTrue(t *testing.T) {
	t.Setenv("SIPLY_LOCAL", "true")

	root := testRootCmd()
	root.SetArgs([]string{})
	err := root.Execute()
	require.NoError(t, err)

	assert.True(t, isLocalMode(root))
}

func TestIsLocalMode_NotSet(t *testing.T) {
	t.Setenv("SIPLY_LOCAL", "")

	root := testRootCmd()
	root.SetArgs([]string{})
	err := root.Execute()
	require.NoError(t, err)

	assert.False(t, isLocalMode(root))
}

func TestWithLocalGuard_BlocksInLocalMode(t *testing.T) {
	root := testRootCmd()
	var ran bool
	guardedCmd := withLocalGuard(&cobra.Command{
		Use: "cloud-cmd",
		RunE: func(_ *cobra.Command, _ []string) error {
			ran = true
			return nil
		},
	})
	root.AddCommand(guardedCmd)
	root.SetArgs([]string{"--local", "cloud-cmd"})

	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cloud connectivity")
	assert.False(t, ran)
}

func TestWithLocalGuard_AllowsWithoutLocal(t *testing.T) {
	t.Setenv("SIPLY_LOCAL", "")

	root := testRootCmd()
	var ran bool
	guardedCmd := withLocalGuard(&cobra.Command{
		Use: "cloud-cmd",
		RunE: func(_ *cobra.Command, _ []string) error {
			ran = true
			return nil
		},
	})
	root.AddCommand(guardedCmd)
	root.SetArgs([]string{"cloud-cmd"})

	err := root.Execute()
	require.NoError(t, err)
	assert.True(t, ran)
}

func TestBuildProvider_Ollama(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	require.NoError(t, os.MkdirAll(filepath.Join(home, ".siply"), 0o700))

	provider, err := buildProvider("ollama", nil)
	assert.NotNil(t, provider)
	assert.NoError(t, err)
}
